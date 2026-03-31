package claw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"shared/consts"
	"shared/pkg/observability"
	"strings"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/claw/commands"

	"github.com/google/uuid"
)

const (
	openClawConfigVersion   = "2026.2.16"
	openRouterAPIKeyVar     = "OPENROUTER_API_KEY"
	openClawGatewayTokenVar = "OPENCLAW_GATEWAY_TOKEN"
)

const (
	updateStageValidate      = "validate"
	updateStageHostingUpdate = "hosting_update"
	updateStageDBUpdate      = "db_update"
	updateStageArchiveSync   = "archive_sync"
)

var (
	ErrUserIDRequired            = errors.New("user id is required")
	ErrClawIDRequired            = errors.New("claw id is required")
	ErrNameRequired              = errors.New("name is required")
	ErrModelRequired             = errors.New("model is required")
	ErrChannelNotFound           = errors.New("channel not found")
	ErrHostingMissing            = errors.New("hosting manager is not configured")
	ErrOpenRouterClient          = errors.New("openrouter manager is not configured")
	ErrServerIDRequired          = errors.New("server id is required")
	ErrContainerIDRequired       = errors.New("claw container id is required")
	ErrNoServerCapacity          = errors.New("no server capacity available")
	ErrConfigArchivePathRequired = errors.New("config archive path is required")
	ErrPairingCodeRequired       = errors.New("pairing code is required")
	ErrPairingCodeInvalid        = errors.New("invalid code")
	ErrProviderUnsupported       = errors.New("provider is not supported")
	ErrGmailTokenRequired        = errors.New("gmail token is required")
	ErrGmailWatchTopicRequired   = errors.New("gmail watch topic is required")
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
	claws       clawStorage
	channels    channelStorage
	users       userStorage
	servers     serverStorage
	hosting     hostingManager
	keys        apiKeyManager
	archivePath string
	watch       GmailWatchConfig
	metrics     *observability.OperationMetrics
}

type GmailWatchConfig struct {
	Topic  string
	Labels []string
}

func NewClaw(
	claws clawStorage,
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
		channels:    channels,
		users:       users,
		servers:     servers,
		hosting:     hosting,
		keys:        keys,
		archivePath: archivePath,
		watch: GmailWatchConfig{
			Topic:  strings.TrimSpace(watch.Topic),
			Labels: normalizeWatchLabels(watch.Labels),
		},
		metrics: opMetrics,
	}
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

	if s.hosting == nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrHostingMissing)
	}

	user, err := s.users.GetByID(ctx, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	server, err := s.selectAvailableServer(ctx)
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

	cfg := entities.NewCreateClawConfig(entities.CreateClawConfigInput{
		PrimaryModel: primaryModel,
	})
	ensureConfigVars(&cfg, keyValue, "")

	for _, ch := range chs {
		if ch.Config.Telegram != nil {
			cfg.AddTelegramChannel(ch.Config.Telegram)
		}
	}

	now := time.Now()
	cl := entities.Claw{
		ID:        uuid.New(),
		Name:      cm.Name,
		UserID:    user.ID,
		ServerID:  server.ID,
		Status:    entities.StatusStop,
		Config:    cfg,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err = s.claws.Create(ctx, cl, channelIDs); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	container, err := s.hosting.Create(ctx, cl, server)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if container.ServerID != uuid.Nil {
		cl.ServerID = container.ServerID
	}

	cl.ContainerID = container.ID
	if container.Status != "" {
		cl.Status = container.Status
	} else {
		cl.Status = entities.StatusStop
	}

	if err = s.claws.UpdateRuntime(ctx, cl.ID, cl.ServerID, cl.ContainerID, cl.Status); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	return cl, nil
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

	if desired.ContainerID != "" && desired.ServerID != uuid.Nil {
		if s.hosting == nil {
			return entities.Claw{}, wrapUpdateStage(op, updateStageHostingUpdate, ErrHostingMissing)
		}

		availableSrv, err := s.servers.GetByID(ctx, desired.ServerID)
		if err != nil {
			return entities.Claw{}, wrapUpdateStage(op, updateStageHostingUpdate, err)
		}

		if err := s.hosting.Update(ctx, desired, availableSrv); err != nil {
			return entities.Claw{}, wrapUpdateStage(op, updateStageHostingUpdate, err)
		}
	}

	if err := s.claws.Update(ctx, desired, channelUpdate.ChannelIDs, channelUpdate.Replace); err != nil {
		return entities.Claw{}, wrapUpdateStage(op, updateStageDBUpdate, err)
	}

	if err := s.writeArchiveFromConfig(desired); err != nil {
		return entities.Claw{}, wrapUpdateStage(op, updateStageArchiveSync, err)
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

	return s.Delete(ctx, commands.DeleteClaw{
		UserID: userID,
		ClawID: clawID,
	})
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

	if s.hosting == nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrHostingMissing)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID != "" && cl.ServerID != uuid.Nil {
		srv, err := s.servers.GetByID(ctx, cl.ServerID)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		if err := s.hosting.Delete(ctx, cl, srv, false); err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	srv, err := s.selectAvailableServer(ctx)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.restoreConfigArchive(ctx, cl, srv); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	container, err := s.hosting.Create(ctx, cl, srv)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	cl.ContainerID = container.ID
	if container.ServerID != uuid.Nil {
		cl.ServerID = container.ServerID
	} else {
		cl.ServerID = srv.ID
	}

	if err := s.hosting.Start(ctx, cl, srv); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	cl.Status = entities.StatusRunning

	if err := s.claws.UpdateRuntime(
		ctx,
		cl.ID,
		cl.ServerID,
		cl.ContainerID,
		cl.Status,
	); err != nil {
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

	if s.hosting == nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrHostingMissing)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrContainerIDRequired)
	}

	if cl.ServerID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, ErrServerIDRequired)
	}

	srv, err := s.servers.GetByID(ctx, cl.ServerID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.backupConfigArchive(ctx, cl, srv, true); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.hosting.Delete(ctx, cl, srv, false); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	cl.Status = entities.StatusStop
	cl.ServerID = uuid.Nil
	cl.ContainerID = ""

	if err := s.claws.UpdateRuntime(
		ctx,
		cl.ID,
		cl.ServerID,
		cl.ContainerID,
		cl.Status,
	); err != nil {
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

	if err := s.hosting.ApprovePairing(ctx, cl, srv, code); err != nil {
		if errors.Is(err, hosting.ErrInvalidCode) {
			return fmt.Errorf("%s: %w", op, ErrPairingCodeInvalid)
		}

		return fmt.Errorf("%s: %w", op, err)
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

	token, err := s.users.GetGmailToken(ctx, cm.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNotFound) {
			return fmt.Errorf("%s: %w", op, ErrGmailTokenRequired)
		}

		return fmt.Errorf("%s: %w", op, err)
	}

	if token.Client == "" {
		token.Client = "default"
	}

	if token.Token.TokenType == "" {
		token.Token.TokenType = "Bearer"
	}

	if token.Token.RefreshToken == "" {
		return fmt.Errorf("%s: %w", op, ErrGmailTokenRequired)
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
		Email:        token.Email,
		Client:       token.Client,
		RefreshToken: token.Token.RefreshToken,
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
) (err error) {
	const op = "service.Claw.Delete"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.delete",
		"claw_lifecycle",
	)

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID != "" {
		if s.hosting == nil {
			return fmt.Errorf("%s: %w", op, ErrHostingMissing)
		}

		if cl.ServerID == uuid.Nil {
			return fmt.Errorf("%s: %w", op, ErrServerIDRequired)
		}

		srv, err := s.servers.GetByID(ctx, cl.ServerID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		if err := s.hosting.Delete(ctx, cl, srv, true); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	if err := s.claws.Delete(ctx, cm.ClawID, cm.UserID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) backupConfigArchive(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
	deleteAfter bool,
) error {
	const op = "service.Claw.backupConfigArchive"

	if s.archivePath == "" {
		return fmt.Errorf("%s: %w", op, ErrConfigArchivePathRequired)
	}

	if s.hosting == nil {
		return fmt.Errorf("%s: %w", op, ErrHostingMissing)
	}

	body, err := s.hosting.ConfigArchive(ctx, cl, server, deleteAfter)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer body.Close()

	archiveFile, err := s.archiveFilePath(cl)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.MkdirAll(filepath.Dir(archiveFile), 0o755); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tmp := archiveFile + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	_, err = io.Copy(f, body)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)

		return fmt.Errorf("%s: %w", op, err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)

		return fmt.Errorf("%s: %w", op, err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.Rename(tmp, archiveFile); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) syncConfigArchive(
	ctx context.Context,
	cl entities.Claw,
	server *entities.Server,
) error {
	const op = "service.Claw.syncConfigArchive"

	if server != nil && cl.ContainerID != "" && cl.ServerID != uuid.Nil {
		err := s.backupConfigArchive(ctx, cl, *server, false)
		if err == nil {
			return nil
		}
	}

	err := s.writeArchiveFromConfig(cl)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) writeArchiveFromConfig(cl entities.Claw) error {
	const op = "service.Claw.writeArchiveFromConfig"

	if s.archivePath == "" {
		return fmt.Errorf("%s: %w", op, ErrConfigArchivePathRequired)
	}

	archiveFile, err := s.archiveFilePath(cl)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.MkdirAll(filepath.Dir(archiveFile), 0o755); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tmp := archiveFile + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := hosting.WriteConfigArchive(cl.Config, cl.ID.String(), f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)

		return fmt.Errorf("%s: %w", op, err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)

		return fmt.Errorf("%s: %w", op, err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.Rename(tmp, archiveFile); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) restoreConfigArchive(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) error {
	const op = "service.Claw.restoreConfigArchive"

	if s.archivePath == "" {
		return fmt.Errorf("%s: %w", op, ErrConfigArchivePathRequired)
	}

	if s.hosting == nil {
		return fmt.Errorf("%s: %w", op, ErrHostingMissing)
	}

	archiveFile, err := s.archiveFilePath(cl)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	f, err := os.Open(archiveFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("%s: %w", op, err)
	}
	defer f.Close()

	if err := s.hosting.RestoreConfigArchive(ctx, cl, server, f); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) archiveFilePath(cl entities.Claw) (string, error) {
	if s.archivePath == "" {
		return "", ErrConfigArchivePathRequired
	}

	return filepath.Join(s.archivePath, cl.UserID.String(), cl.ID.String()+".tar"), nil
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
