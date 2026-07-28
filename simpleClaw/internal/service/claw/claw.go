package claw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"shared/consts"
	"shared/pkg/hostingapi"
	"shared/pkg/observability"
	"strings"
	"sync/atomic"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/entities/channels"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/storages/claws"
	"simpleClaw/internal/service/claw/capabilities"
	"simpleClaw/internal/service/claw/commands"

	"github.com/google/uuid"
)

const (
	openClawConfigVersion   = "2026.2.16"
	openRouterAPIKeyVar     = "OPENROUTER_API_KEY"
	openClawGatewayTokenVar = "OPENCLAW_GATEWAY_TOKEN"
)

const (
	updateStageValidate = "validate"
	updateStageDBUpdate = "db_update"
)

var (
	ErrUserIDRequired               = errors.New("user id is required")
	ErrClawIDRequired               = errors.New("claw id is required")
	ErrNameRequired                 = errors.New("name is required")
	ErrModelRequired                = errors.New("model is required")
	ErrChannelNotFound              = errors.New("channel not found")
	ErrHostingMissing               = errors.New("hosting manager is not configured")
	ErrOpenRouterClient             = errors.New("openrouter manager is not configured")
	ErrServerIDRequired             = errors.New("server id is required")
	ErrContainerIDRequired          = errors.New("claw container id is required")
	ErrNoServerCapacity             = errors.New("no server capacity available")
	ErrPairingCodeRequired          = errors.New("pairing code is required")
	ErrPairingCodeInvalid           = errors.New("invalid code")
	ErrApproveChannelRequired       = errors.New("channel type is required")
	ErrApproveChannelUnsupported    = errors.New("approve channel is not supported")
	ErrBraveAPIKeyMissing           = errors.New("brave api key is not configured")
	ErrProviderUnsupported          = errors.New("provider is not supported")
	ErrGmailCapabilityRequired      = errors.New("gmail capability is not attached")
	ErrGmailIntegrationRequired     = errors.New("gmail integration is required")
	ErrGmailWatchTopicRequired      = errors.New("gmail watch topic is required")
	ErrOperationStorageRequired     = errors.New("lifecycle operation storage is not configured")
	ErrLifecycleOperationInProgress = errors.New("lifecycle operation already in progress")
	ErrWebSearchRequired            = errors.New("web search capability is required")
	ErrWebSearchProviderUnsupported = errors.New("web search provider is not supported")
)

type UpdateStageError struct {
	Stage string
	Err   error
}

func (e *UpdateStageError) Error() string {
	if e == nil {
		return ""
	}

	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

func (e *UpdateStageError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

type Service struct {
	claws              clawStorage
	operations         lifecycleOperationStorage
	channels           channelStorage
	users              userStorage
	integrations       integrationStorage
	attachments        capabilityAttachmentStorage
	servers            serverStorage
	hosting            hostingManager
	keys               apiKeyManager
	archives           archiveStore
	archivePath        string
	watch              GmailWatchConfig
	braveAPIKey        string
	metrics            *observability.OperationMetrics
	runtimeSyncRunning atomic.Bool
}

type GmailWatchConfig struct {
	Topic  string
	Labels []string
}

func NewClaw(
	claws clawStorage,
	operations lifecycleOperationStorage,
	channels channelStorage,
	users userStorage,
	servers serverStorage,
	hosting hostingManager,
	keys apiKeyManager,
	archivePath string,
	watch GmailWatchConfig,
	metrics ...*observability.OperationMetrics,
) *Service {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Service{
		claws:       claws,
		operations:  operations,
		channels:    channels,
		users:       users,
		servers:     servers,
		hosting:     hosting,
		keys:        keys,
		archives:    newFileArchiveStore(archivePath),
		archivePath: archivePath,
		watch: GmailWatchConfig{
			Topic:  strings.TrimSpace(watch.Topic),
			Labels: normalizeWatchLabels(watch.Labels),
		},
		metrics: opMetrics,
	}
}

func (s *Service) WithCapabilityDependencies(
	attachments capabilityAttachmentStorage,
	integrations integrationStorage,
) *Service {
	s.attachments = attachments
	s.integrations = integrations

	return s
}

func (s *Service) WithBraveAPIKey(apiKey string) *Service {
	s.braveAPIKey = strings.TrimSpace(apiKey)

	return s
}

func (s *Service) Create(
	ctx context.Context,
	cm commands.CreateClaw,
) (entities.Claw, error) {
	const op = "service.Claw.Create"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.create",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if cm.Name == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrNameRequired)
	}

	if cm.Model == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrModelRequired)
	}

	user, err := s.users.GetByID(ctx, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	var (
		channelIDs []uuid.UUID
		chs        []entities.Channel
	)

	if cm.ChannelIDs != nil {
		if len(cm.ChannelIDs) == 0 {
			channelIDs = []uuid.UUID{}
		} else {
			channelIDs = deduplicateUUIDs(cm.ChannelIDs)

			chs, err = s.channels.GetByIDs(ctx, channelIDs, cm.UserID)
			if err != nil {
				return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
			}

			if len(chs) != len(channelIDs) {
				return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrChannelNotFound)
			}
		}
	}

	keyValue := user.OpenRouterApiKey
	if keyValue == "" || user.OpenRouterKeyID == "" {
		if s.keys == nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrOpenRouterClient)
		}

		monthly := normalizeLimits(cm.ApiKeyLimit)

		apiKey, err := s.keys.Create(ctx, user.ID, monthly)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		err = s.users.UpdateOpenRouterKey(ctx, user.ID, apiKey)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		keyValue = apiKey.Secret
	}

	primaryModel, err := s.keys.ResolveModel(ctx, cm.Model)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	cfg := capabilities.BuildBaseClawConfig(primaryModel)
	ensureConfigVars(&cfg, keyValue, "")
	cfg.Channels = mergeChannelConfigs(chs)

	if err := capabilities.ApplyCreateCapabilities(ctx, &cfg, cm.Capabilities); err != nil {
		switch {
		case errors.Is(err, capabilities.ErrWebSearchRequired):
			return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrWebSearchRequired)
		case errors.Is(err, capabilities.ErrWebSearchProviderUnsupported):
			return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrWebSearchProviderUnsupported)
		default:
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	if _, ok := cfg.Env.Vars["BRAVE_API_KEY"]; ok && s.braveAPIKey == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrBraveAPIKeyMissing)
	}

	now := time.Now()
	cl := entities.Claw{
		ID:                 uuid.New(),
		Name:               cm.Name,
		UserID:             user.ID,
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             cfg,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	attachments, err := s.buildCreateAttachments(ctx, cl, cm.Capabilities, now)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if err = s.claws.Create(ctx, cl, channelIDs); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if err = s.persistCreateAttachments(ctx, cl, attachments); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	return cl, nil
}

func (s *Service) buildCreateAttachments(
	ctx context.Context,
	cl entities.Claw,
	input commands.CreateCapabilitySet,
	now time.Time,
) ([]entities.ClawCapabilityAttachment, error) {
	requests := []struct {
		capabilityID entities.CapabilityID
		input        *commands.IntegrationBoundCapabilityInput
	}{
		{capabilityID: entities.CapabilityGmail, input: input.Gmail},
		{capabilityID: entities.CapabilityGoogleCalendar, input: input.GoogleCalendar},
		{capabilityID: entities.CapabilitySheets, input: input.Sheets},
	}

	enabledCount := 0
	for _, request := range requests {
		if request.input != nil && request.input.Enabled {
			enabledCount++
		}
	}

	if enabledCount == 0 {
		return nil, nil
	}

	if s.attachments == nil || s.integrations == nil {
		return nil, sql.ErrUnavailable
	}

	attachments := make([]entities.ClawCapabilityAttachment, 0, enabledCount)
	for _, request := range requests {
		attachment, ok, err := s.buildCreateAttachment(ctx, cl, request.capabilityID, request.input, now)
		if err != nil {
			return nil, err
		}

		if ok {
			attachments = append(attachments, attachment)
		}
	}

	return attachments, nil
}

func (s *Service) buildCreateAttachment(
	ctx context.Context,
	cl entities.Claw,
	capabilityID entities.CapabilityID,
	input *commands.IntegrationBoundCapabilityInput,
	now time.Time,
) (entities.ClawCapabilityAttachment, bool, error) {
	if input == nil || !input.Enabled {
		return entities.ClawCapabilityAttachment{}, false, nil
	}

	if input.AccountIntegrationID == nil || *input.AccountIntegrationID == uuid.Nil {
		return entities.ClawCapabilityAttachment{}, false, sql.ErrInvalid
	}

	integration, err := s.integrations.GetByID(ctx, *input.AccountIntegrationID, cl.UserID)
	if err != nil {
		return entities.ClawCapabilityAttachment{}, false, err
	}

	if integration.CapabilityID != capabilityID {
		return entities.ClawCapabilityAttachment{}, false, sql.ErrInvalid
	}

	provider := strings.TrimSpace(input.Provider)
	if provider != "" && !strings.EqualFold(provider, integration.Provider) {
		return entities.ClawCapabilityAttachment{}, false, sql.ErrInvalid
	}

	return entities.ClawCapabilityAttachment{
		ID:                   uuid.New(),
		ClawID:               cl.ID,
		UserID:               cl.UserID,
		CapabilityID:         capabilityID,
		Provider:             integration.Provider,
		AccountIntegrationID: input.AccountIntegrationID,
		Enabled:              true,
		CreatedAt:            now,
		UpdatedAt:            now,
	}, true, nil
}

func (s *Service) persistCreateAttachments(
	ctx context.Context,
	cl entities.Claw,
	attachments []entities.ClawCapabilityAttachment,
) error {
	for _, attachment := range attachments {
		if err := s.attachments.Upsert(ctx, attachment); err != nil {
			_ = s.claws.Delete(ctx, cl.ID, cl.UserID)
			return err
		}
	}

	return nil
}

func (s *Service) GetByID(
	ctx context.Context,
	clawID uuid.UUID,
	userID uuid.UUID,
) (entities.Claw, error) {
	const op = "service.Claw.GetByID"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.get",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if clawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	cl, err := s.claws.GetByID(ctx, clawID, userID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	return cl, nil
}

func (s *Service) GetByUserID(
	ctx context.Context,
	userID uuid.UUID,
) ([]entities.Claw, error) {
	const op = "service.Claw.GetByUserID"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.list",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	cls, err := s.claws.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return cls, nil
}

func (s *Service) Update(
	ctx context.Context,
	cm commands.UpdateClaw,
) (entities.Claw, error) {
	const op = "service.Claw.Update"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.update",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, ErrUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, ErrClawIDRequired)
	}

	existing, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	desired := existing

	name := desired.Name
	if cm.Name != nil {
		name = strings.TrimSpace(*cm.Name)
		if name == "" {
			return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, ErrNameRequired)
		}
	}

	channelUpdate := normalizeChannelUpdate(cm.ChannelIDs)

	var chs []entities.Channel

	if channelUpdate.Replace {
		if len(channelUpdate.ChannelIDs) > 0 {
			chs, err = s.channels.GetByIDs(ctx, channelUpdate.ChannelIDs, cm.UserID)
			if err != nil {
				return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, err)
			}

			if len(chs) != len(channelUpdate.ChannelIDs) {
				return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, ErrChannelNotFound)
			}
		}
	}

	keyValue := ""

	if desired.Config.Env.Vars != nil {
		keyValue = desired.Config.Env.Vars[openRouterAPIKeyVar]
	}

	if keyValue == "" {
		keyValue = strings.TrimSpace(desired.Config.Env.OpenRouterAPIKey)
		if isVarRef(keyValue, openRouterAPIKeyVar) {
			keyValue = ""
		}
	}

	if keyValue == "" {
		user, err := s.users.GetByID(ctx, cm.UserID)
		if err != nil {
			return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, err)
		}

		keyValue = user.OpenRouterApiKey
		if keyValue == "" || user.OpenRouterKeyID == "" {
			if s.keys == nil {
				return entities.Claw{}, wrapUpdateStage(
					op,
					updateStageValidate,
					ErrOpenRouterClient,
				)
			}

			monthly := normalizeLimits(cm.ApiKeyLimit)

			apiKey, err := s.keys.Create(ctx, user.ID, monthly)
			if err != nil {
				return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, err)
			}

			err = s.users.UpdateOpenRouterKey(ctx, user.ID, apiKey)
			if err != nil {
				return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, err)
			}

			keyValue = apiKey.Secret
		}
	}

	primaryModel := currentPrimaryModel(desired.Config)

	if cm.Model != nil {
		model := strings.TrimSpace(*cm.Model)
		if model == "" {
			return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, ErrModelRequired)
		}

		if s.keys == nil {
			return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, ErrOpenRouterClient)
		}

		primaryModel, err = s.keys.ResolveModel(ctx, model)
		if err != nil {
			return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, err)
		}
	}

	if primaryModel == "" {
		return entities.Claw{}, wrapUpdateStage(op, updateStageValidate, ErrModelRequired)
	}

	updatedCfg := applyBaseUpdates(desired.Config, primaryModel, keyValue)

	if channelUpdate.Replace {
		updatedCfg.Channels = mergeChannelConfigs(chs)
	}

	updatedCfg.Meta = desired.Config.Meta
	if isConfigChanged(desired.Config, updatedCfg) {
		updatedCfg.Meta = newConfigMeta(time.Now())
	}

	desired.Name = name
	desired.Config = updatedCfg
	desired.UpdatedAt = time.Now()

	if err := s.claws.Update(ctx, desired, channelUpdate.ChannelIDs, channelUpdate.Replace); err != nil {
		return entities.Claw{}, wrapUpdateStage(op, updateStageDBUpdate, err)
	}

	return desired, nil
}

func (s *Service) DeleteByID(
	ctx context.Context,
	clawID uuid.UUID,
	userID uuid.UUID,
) error {
	const op = "service.Claw.DeleteByID"

	if userID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if clawID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	_, err := s.Delete(ctx, commands.DeleteClaw{
		UserID: userID,
		ClawID: clawID,
	})
	return err
}

func (s *Service) Start(
	ctx context.Context,
	cm commands.StartClaw,
) (entities.Claw, error) {
	const op = "service.Claw.Start"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.start",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	cl, err = s.enqueueLifecycleIntent(
		ctx,
		cl,
		entities.ClawLifecycleOperationTypeStart,
		entities.ClawDesiredStateRunning,
		entities.ClawLifecycleStatusStartPending,
		queuedOnboardingState(cl),
	)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	return cl, nil
}

func (s *Service) Stop(
	ctx context.Context,
	cm commands.StopClaw,
) (entities.Claw, error) {
	const op = "service.Claw.Stop"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.stop",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	cl, err = s.enqueueLifecycleIntent(
		ctx,
		cl,
		entities.ClawLifecycleOperationTypeStop,
		entities.ClawDesiredStateStopped,
		entities.ClawLifecycleStatusStopPending,
		nil,
	)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	return cl, nil
}

func (s *Service) Restart(
	ctx context.Context,
	cm commands.RestartClaw,
) (entities.Claw, error) {
	const op = "service.Claw.Restart"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.restart",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	cl, err = s.enqueueLifecycleIntent(
		ctx,
		cl,
		entities.ClawLifecycleOperationTypeRestart,
		entities.ClawDesiredStateRunning,
		entities.ClawLifecycleStatusRestartPending,
		queuedOnboardingState(cl),
	)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	return cl, nil
}

func (s *Service) ApprovePairing(
	ctx context.Context,
	cm commands.ApprovePairing,
) (err error) {
	const op = "service.Claw.ApprovePairing"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.approve",
		"claw_pairing",
	)

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	code := strings.TrimSpace(cm.Code)
	if code == "" {
		return fmt.Errorf("%s: %w", op, ErrPairingCodeRequired)
	}

	if s.hosting == nil {
		return fmt.Errorf("%s: %w", op, ErrHostingMissing)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID == "" {
		return fmt.Errorf("%s: %w", op, ErrContainerIDRequired)
	}

	if cl.ServerID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrServerIDRequired)
	}

	srv, err := s.servers.GetByID(ctx, cl.ServerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	channelType, err := resolveApproveChannelType(cl, cm.ChannelType)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.hosting.ApprovePairing(ctx, cl, srv, channelType, code); err != nil {
		if errors.Is(err, hosting.ErrInvalidCode) {
			return fmt.Errorf("%s: %w", op, ErrPairingCodeInvalid)
		}

		if errors.Is(err, hosting.ErrApproveChannelUnsupported) {
			return fmt.Errorf("%s: %w", op, ErrApproveChannelUnsupported)
		}

		return fmt.Errorf("%s: %w", op, err)
	}

	completed := true
	if err := s.claws.UpdateLifecycle(ctx, cl.ID, claws.LifecycleUpdate{
		DesiredState:       cl.DesiredState,
		ObservedState:      cl.ObservedState,
		LifecycleStatus:    cl.LifecycleStatus,
		CurrentOperationID: cl.CurrentOperationID,
		LastLifecycleError: cl.LastError,
		OnboardingComplete: &completed,
	}); err != nil {
		return fmt.Errorf("%s: mark onboarding complete: %w", op, err)
	}

	return nil
}

func (s *Service) Connect(
	ctx context.Context,
	cm commands.ConnectClaw,
) (err error) {
	const op = "service.Claw.Connect"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.connect",
		"claw_pairing",
	)

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	if s.hosting == nil {
		return fmt.Errorf("%s: %w", op, ErrHostingMissing)
	}

	provider := strings.TrimSpace(cm.Provider)
	if provider == "" {
		provider = consts.ProviderGmail
	}

	if !strings.EqualFold(provider, consts.ProviderGmail) {
		return fmt.Errorf("%s: %w", op, ErrProviderUnsupported)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID == "" {
		return fmt.Errorf("%s: %w", op, ErrContainerIDRequired)
	}

	if cl.ServerID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrServerIDRequired)
	}

	srv, err := s.servers.GetByID(ctx, cl.ServerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if s.attachments == nil || s.integrations == nil {
		return fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	attachments, err := s.attachments.ListByClawID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	var gmailAttachment *entities.ClawCapabilityAttachment
	for i := range attachments {
		if attachments[i].CapabilityID == entities.CapabilityGmail && attachments[i].Enabled {
			gmailAttachment = &attachments[i]
			break
		}
	}

	if gmailAttachment == nil || gmailAttachment.AccountIntegrationID == nil {
		return fmt.Errorf("%s: %w", op, ErrGmailCapabilityRequired)
	}

	integration, err := s.integrations.GetByID(
		ctx,
		*gmailAttachment.AccountIntegrationID,
		cm.UserID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNotFound) {
			return fmt.Errorf("%s: %w", op, ErrGmailIntegrationRequired)
		}

		return fmt.Errorf("%s: %w", op, err)
	}

	email := firstNonEmpty(
		integration.ExternalAccountID,
		stringFromMap(integration.Metadata, "email"),
	)
	client := firstNonEmpty(
		stringFromMap(integration.Metadata, "client"),
		"default",
	)
	refreshToken := stringFromMap(integration.SecretPayload, "refresh_token")
	if email == "" || refreshToken == "" {
		return fmt.Errorf("%s: %w", op, ErrGmailIntegrationRequired)
	}

	watchTopic := strings.TrimSpace(s.watch.Topic)
	watchLabels := append([]string(nil), s.watch.Labels...)

	if cl.Config.Hooks != nil {
		if hookTopic := strings.TrimSpace(cl.Config.Hooks.Gmail.Topic); hookTopic != "" {
			watchTopic = hookTopic
		}

		if hookLabel := strings.TrimSpace(cl.Config.Hooks.Gmail.Label); hookLabel != "" {
			watchLabels = []string{hookLabel}
		}
	}

	if watchTopic == "" {
		return fmt.Errorf("%s: %w", op, ErrGmailWatchTopicRequired)
	}

	payload, err := json.Marshal(struct {
		Email        string   `json:"email"`
		Client       string   `json:"client,omitempty"`
		RefreshToken string   `json:"refresh_token"`
		Topic        string   `json:"topic"`
		Labels       []string `json:"labels,omitempty"`
	}{
		Email:        email,
		Client:       client,
		RefreshToken: refreshToken,
		Topic:        watchTopic,
		Labels:       watchLabels,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.hosting.Connect(ctx, cl, srv, consts.ProviderGmail, payload); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) Delete(
	ctx context.Context,
	cm commands.DeleteClaw,
) (entities.Claw, error) {
	const op = "service.Claw.Delete"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.delete",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	cl, err = s.enqueueLifecycleIntent(
		ctx,
		cl,
		entities.ClawLifecycleOperationTypeDelete,
		entities.ClawDesiredStateDeleted,
		entities.ClawLifecycleStatusDeletePending,
		nil,
	)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	return cl, nil
}

func (s *Service) enqueueLifecycleIntent(
	ctx context.Context,
	cl entities.Claw,
	opType entities.ClawLifecycleOperationType,
	desiredState entities.ClawDesiredState,
	lifecycleStatus entities.ClawLifecycleStatus,
	onboardingComplete *bool,
) (entities.Claw, error) {
	if s.operations == nil {
		return entities.Claw{}, ErrOperationStorageRequired
	}

	active, err := s.operations.GetActiveByClawID(ctx, cl.ID)
	switch {
	case err == nil:
		return entities.Claw{}, fmt.Errorf(
			"active lifecycle operation %s exists: %w",
			active.ID,
			ErrLifecycleOperationInProgress,
		)
	case errors.Is(err, sql.ErrNotFound):
	default:
		return entities.Claw{}, err
	}

	now := time.Now().UTC()
	opID := uuid.New()
	op := entities.ClawLifecycleOperation{
		ID:        opID,
		ClawID:    cl.ID,
		Type:      opType,
		Status:    entities.ClawLifecycleOperationStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.operations.Create(ctx, op); err != nil {
		return entities.Claw{}, err
	}

	if err := s.claws.UpdateLifecycle(ctx, cl.ID, claws.LifecycleUpdate{
		DesiredState:       desiredState,
		ObservedState:      cl.ObservedState,
		LifecycleStatus:    lifecycleStatus,
		CurrentOperationID: &opID,
		LastLifecycleError: "",
		OnboardingComplete: onboardingComplete,
	}); err != nil {
		return entities.Claw{}, err
	}

	cl.DesiredState = desiredState
	cl.LifecycleStatus = lifecycleStatus
	cl.CurrentOperationID = &opID
	cl.LastError = ""
	if onboardingComplete != nil {
		cl.OnboardingComplete = *onboardingComplete
	}

	s.logLifecycleEvent("queued_operation", op,
		slog.String("desired_state", string(desiredState)),
		slog.String("lifecycle_status", string(lifecycleStatus)),
	)

	return cl, nil
}

func requiresOnboardingApprove(cfg entities.ClawConfig) bool {
	return len(listApproveChannels(cfg)) > 0
}

func queuedOnboardingState(cl entities.Claw) *bool {
	if cl.OnboardingComplete {
		return boolPtr(true)
	}

	return boolPtr(!requiresOnboardingApprove(cl.Config))
}

func boolPtr(v bool) *bool {
	return &v
}

func (s *Service) selectAvailableServer(ctx context.Context) (entities.Server, error) {
	servers, err := s.servers.GetAll(ctx)
	if err != nil {
		return entities.Server{}, err
	}

	occupiedByServer, err := s.claws.CountOccupiedByServer(ctx)
	if err != nil {
		return entities.Server{}, err
	}

	var (
		selected     entities.Server
		selectedSet  bool
		selectedFree int
	)

	for _, srv := range servers {
		free := srv.MaxClaws - occupiedByServer[srv.ID]
		if free <= 0 {
			continue
		}

		if !selectedSet || free > selectedFree ||
			(free == selectedFree && srv.CreatedAt.Before(selected.CreatedAt)) {
			selected = srv
			selectedSet = true
			selectedFree = free
		}
	}

	if !selectedSet {
		return entities.Server{}, ErrNoServerCapacity
	}

	return selected, nil
}

func applyBaseUpdates(
	existing entities.ClawConfig,
	primaryModel string,
	apiKey string,
) entities.ClawConfig {
	cfg := existing

	ensureConfigVars(&cfg, apiKey, "")

	setPrimaryModel(&cfg, primaryModel)

	if cfg.Gateway.Mode == "" {
		cfg.Gateway.Mode = "local"
	}

	if cfg.Gateway.Auth.Mode == "" {
		cfg.Gateway.Auth.Mode = "token"
	}

	return cfg
}

func ensureConfigVars(cfg *entities.ClawConfig, apiKey string, gatewayToken string) {
	if cfg == nil {
		return
	}

	if cfg.Env.Vars == nil {
		cfg.Env.Vars = map[string]string{}
	}

	if apiKey != "" {
		cfg.Env.Vars[openRouterAPIKeyVar] = apiKey
	}

	if cfg.Env.Vars[openRouterAPIKeyVar] == "" {
		key := strings.TrimSpace(cfg.Env.OpenRouterAPIKey)
		if key != "" && !isVarRef(key, openRouterAPIKeyVar) {
			cfg.Env.Vars[openRouterAPIKeyVar] = key
		}
	}

	if cfg.Env.Vars[openRouterAPIKeyVar] != "" {
		cfg.Env.OpenRouterAPIKey = varRef(openRouterAPIKeyVar)
	}

	if gatewayToken != "" {
		cfg.Env.Vars[openClawGatewayTokenVar] = gatewayToken
	}

	if cfg.Env.Vars[openClawGatewayTokenVar] == "" {
		token := strings.TrimSpace(cfg.Gateway.Auth.Token)
		if token != "" && !isVarRef(token, openClawGatewayTokenVar) {
			cfg.Env.Vars[openClawGatewayTokenVar] = token
		}
	}

	if cfg.Env.Vars[openClawGatewayTokenVar] == "" {
		cfg.Env.Vars[openClawGatewayTokenVar] = generateGatewayToken()
	}

	cfg.Gateway.Auth.Token = varRef(openClawGatewayTokenVar)
}

func varRef(name string) string {
	return "${" + name + "}"
}

func isVarRef(value string, name string) bool {
	return strings.TrimSpace(value) == varRef(name)
}

func stringFromMap(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}

	raw, ok := values[key]
	if !ok {
		return ""
	}

	value, ok := raw.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

func newConfigMeta(now time.Time) *entities.ConfigMeta {
	return &entities.ConfigMeta{
		LastTouchedVersion: openClawConfigVersion,
		LastTouchedAt:      now.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
}

func isConfigChanged(before, after entities.ClawConfig) bool {
	before.Meta = &entities.ConfigMeta{}
	after.Meta = &entities.ConfigMeta{}

	return !reflect.DeepEqual(before, after)
}

func mergeChannelConfigs(chs []entities.Channel) *entities.ClawChannels {
	cfg := &entities.ClawChannels{}

	for _, ch := range chs {
		switch ch.ChannelType {
		case entities.ChannelTelegramType:
			if ch.Config.Telegram != nil {
				tg := *ch.Config.Telegram
				tg.Enabled = true
				cfg.Telegram = &tg
			}
		case entities.ChannelDiscordType:
			if ch.Config.Discord != nil {
				discord := *ch.Config.Discord
				discord.Enabled = true
				cfg.Discord = &discord
			}
		case entities.ChannelWhatsappType:
			if ch.Config.WhatsApp != nil {
				wa := *ch.Config.WhatsApp
				cfg.WhatsApp = &wa
			}
		case entities.ChannelSlackType:
			if ch.Config.Slack != nil {
				slack := *ch.Config.Slack
				cfg.Slack = &slack
			}
		}
	}

	return cfg
}

func resolveApproveChannelType(cl entities.Claw, raw string) (string, error) {
	explicit := strings.TrimSpace(raw)
	if explicit != "" {
		channelType, err := hostingapi.NormalizeApproveChannelType(explicit)
		if err != nil {
			return "", ErrApproveChannelUnsupported
		}

		if !clawSupportsApproveChannel(cl.Config, channelType) {
			return "", ErrApproveChannelUnsupported
		}

		return channelType, nil
	}

	candidates := listApproveChannels(cl.Config)
	if len(candidates) == 0 {
		return "", ErrApproveChannelUnsupported
	}

	if len(candidates) > 1 {
		return "", ErrApproveChannelRequired
	}

	return candidates[0], nil
}

func listApproveChannels(cfg entities.ClawConfig) []string {
	if cfg.Channels == nil {
		return nil
	}

	var out []string

	if cfg.Channels.Telegram != nil && cfg.Channels.Telegram.Enabled &&
		cfg.Channels.Telegram.DmPolicy == channels.DmPairing {
		out = append(out, entities.ChannelTelegramType)
	}

	if cfg.Channels.WhatsApp != nil &&
		strings.EqualFold(cfg.Channels.WhatsApp.DmPolicy, "pairing") {
		out = append(out, entities.ChannelWhatsappType)
	}

	return out
}

func clawSupportsApproveChannel(cfg entities.ClawConfig, channelType string) bool {
	for _, candidate := range listApproveChannels(cfg) {
		if candidate == channelType {
			return true
		}
	}

	return false
}

func currentPrimaryModel(cfg entities.ClawConfig) string {
	for _, agent := range cfg.Agents.List {
		if agent.Model == nil {
			continue
		}

		model := strings.TrimSpace(agent.Model.Primary)
		if model == "" {
			continue
		}

		if agent.Default {
			return model
		}
	}

	for _, agent := range cfg.Agents.List {
		if agent.Model == nil {
			continue
		}

		model := strings.TrimSpace(agent.Model.Primary)
		if model != "" {
			return model
		}
	}

	return ""
}

func setPrimaryModel(cfg *entities.ClawConfig, model string) {
	if cfg == nil {
		return
	}

	for i := range cfg.Agents.List {
		if cfg.Agents.List[i].Model == nil {
			cfg.Agents.List[i].Model = &entities.AgentModelSelection{}
		}

		if cfg.Agents.List[i].Default {
			cfg.Agents.List[i].Model.Primary = model

			return
		}
	}

	if len(cfg.Agents.List) == 0 {
		cfg.Agents.List = append(cfg.Agents.List, entities.NewDefaultMainAgentConfig(model))

		return
	}

	if cfg.Agents.List[0].Model == nil {
		cfg.Agents.List[0].Model = &entities.AgentModelSelection{}
	}

	cfg.Agents.List[0].Model.Primary = model
}

type channelUpdate struct {
	ChannelIDs []uuid.UUID
	Replace    bool
}

func normalizeChannelUpdate(ids []uuid.UUID) channelUpdate {
	if ids == nil {
		return channelUpdate{
			ChannelIDs: nil,
			Replace:    false,
		}
	}

	normalized := deduplicateUUIDs(ids)

	return channelUpdate{
		ChannelIDs: normalized,
		Replace:    true,
	}
}

func wrapUpdateStage(op string, stage string, err error) error {
	return fmt.Errorf("%s: %w", op, &UpdateStageError{
		Stage: stage,
		Err:   err,
	})
}

func normalizeLimits(l commands.ApiKeyLimits) float64 {
	monthly := l.MonthlyBudgetUSD
	if monthly <= 0 {
		monthly = 50
	}

	return monthly
}

func deduplicateUUIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return nil
	}

	if len(ids) == 0 {
		return []uuid.UUID{}
	}

	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]uuid.UUID, 0, len(ids))

	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}

		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		result = append(result, id)
	}

	if len(result) == 0 {
		return []uuid.UUID{}
	}

	return result
}

func generateGatewayToken() string {
	return uuid.NewString()
}

func normalizeWatchLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(labels))

	out := make([]string, 0, len(labels))
	for _, raw := range labels {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}

		if _, ok := seen[label]; ok {
			continue
		}

		seen[label] = struct{}{}
		out = append(out, label)
	}

	return out
}
