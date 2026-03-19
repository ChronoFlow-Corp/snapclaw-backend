package claw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"shared/consts"

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
	ErrConfigArchivePathRequired = errors.New("config archive path is required")
	ErrPairingCodeRequired       = errors.New("pairing code is required")
	ErrPairingCodeInvalid        = errors.New("invalid code")
	ErrProviderUnsupported       = errors.New("provider is not supported")
	ErrGmailTokenRequired        = errors.New("gmail token is required")
	ErrGmailWatchTopicRequired   = errors.New("gmail watch topic is required")

	errUserIDRequired            = ErrUserIDRequired
	errClawIDRequired            = ErrClawIDRequired
	errNameRequired              = ErrNameRequired
	errModelRequired             = ErrModelRequired
	errChannelNotFound           = ErrChannelNotFound
	errHostingMissing            = ErrHostingMissing
	errOpenRouterClient          = ErrOpenRouterClient
	errServerIDRequired          = ErrServerIDRequired
	errContainerIDRequired       = ErrContainerIDRequired
	errConfigArchivePathRequired = ErrConfigArchivePathRequired
	errPairingCodeRequired       = ErrPairingCodeRequired
	errPairingCodeInvalid        = ErrPairingCodeInvalid
	errProviderUnsupported       = ErrProviderUnsupported
	errGmailTokenRequired        = ErrGmailTokenRequired
	errGmailWatchTopicRequired   = ErrGmailWatchTopicRequired
)

type Service struct {
	claws       clawStorage
	channels    channelStorage
	users       userStorage
	servers     serverStorage
	hosting     hostingManager
	keys        apiKeyManager
	archivePath string
	watch       GmailWatchConfig
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
) *Service {
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
	}
}

func (s *Service) Create(
	ctx context.Context,
	cm commands.CreateClaw,
) (entities.Claw, error) {
	const op = "service.Claw.Create"

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if cm.Name == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errNameRequired)
	}

	if cm.Model == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errModelRequired)
	}

	if s.hosting == nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errHostingMissing)
	}

	user, err := s.users.GetByID(ctx, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	server, err := s.servers.GetAvailable(ctx)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	var channelIDs []uuid.UUID
	var chs []entities.Channel
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
				return entities.Claw{}, fmt.Errorf("%s: %w", op, errChannelNotFound)
			}
		}
	}

	keyValue := user.OpenRouterApiKey
	if keyValue == "" || user.OpenRouterKeyID == "" {
		if s.keys == nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, errOpenRouterClient)
		}

		monthly := normalizeLimits(cm.ApiKeyLimit)
		apiKey, err := s.keys.Create(ctx, user.ID, cm.Name, monthly)
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

	cfg := entities.NewDefaultClawConfig(primaryModel)
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

	if userID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if clawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errClawIDRequired)
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

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, errUserIDRequired)
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

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errClawIDRequired)
	}

	if cm.Name == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errNameRequired)
	}

	if cm.Model == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errModelRequired)
	}

	if s.keys == nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errOpenRouterClient)
	}

	existing, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	channelIDs := deduplicateUUIDs(cm.ChannelIDs)
	var chs []entities.Channel
	if len(channelIDs) > 0 {
		chs, err = s.channels.GetByIDs(ctx, channelIDs, cm.UserID)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		if len(chs) != len(channelIDs) {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, errChannelNotFound)
		}
	}

	keyValue := ""
	if existing.Config.Env.Vars != nil {
		keyValue = existing.Config.Env.Vars[openRouterAPIKeyVar]
	}
	if keyValue == "" {
		keyValue = strings.TrimSpace(existing.Config.Env.OpenRouterAPIKey)
		if isVarRef(keyValue, openRouterAPIKeyVar) {
			keyValue = ""
		}
	}
	if keyValue == "" {
		user, err := s.users.GetByID(ctx, cm.UserID)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		keyValue = user.OpenRouterApiKey
		if keyValue == "" || user.OpenRouterKeyID == "" {
			monthly := normalizeLimits(cm.ApiKeyLimit)
			apiKey, err := s.keys.Create(ctx, user.ID, cm.Name, monthly)
			if err != nil {
				return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
			}

			err = s.users.UpdateOpenRouterKey(ctx, user.ID, apiKey)
			if err != nil {
				return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
			}

			keyValue = apiKey.Secret
		}
	}

	primaryModel, err := s.keys.ResolveModel(ctx, cm.Model)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	updatedCfg := applyBaseUpdates(existing.Config, primaryModel, keyValue)
	if cm.ChannelIDs != nil {
		updatedCfg.Channels = mergeChannelConfigs(chs)
	}

	updatedCfg.Meta = existing.Config.Meta
	if isConfigChanged(existing.Config, updatedCfg) {
		updatedCfg.Meta = newConfigMeta(time.Now())
	}

	existing.Name = cm.Name
	existing.Config = updatedCfg
	existing.UpdatedAt = time.Now()

	if err := s.claws.Update(ctx, existing, channelIDs); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	var srv *entities.Server
	var updateErr error
	if existing.ContainerID != "" && existing.ServerID != uuid.Nil {
		if s.hosting == nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, errHostingMissing)
		}

		availableSrv, err := s.servers.GetByID(ctx, existing.ServerID)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		srv = &availableSrv
		if err := s.hosting.Update(ctx, existing, availableSrv); err != nil {
			updateErr = fmt.Errorf("%s: %w", op, err)
		}
	}

	if err := s.syncConfigArchive(ctx, existing, srv); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if updateErr != nil {
		return entities.Claw{}, updateErr
	}

	return existing, nil
}

func (s *Service) DeleteByID(
	ctx context.Context,
	clawID uuid.UUID,
	userID uuid.UUID,
) error {
	const op = "service.Claw.DeleteByID"

	if userID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if clawID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errClawIDRequired)
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

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errClawIDRequired)
	}

	if s.hosting == nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errHostingMissing)
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

	srv, err := s.servers.GetAvailable(ctx)
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

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errClawIDRequired)
	}

	if s.hosting == nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errHostingMissing)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errContainerIDRequired)
	}

	if cl.ServerID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errServerIDRequired)
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
) error {
	const op = "service.Claw.ApprovePairing"

	if cm.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errClawIDRequired)
	}

	code := strings.TrimSpace(cm.Code)
	if code == "" {
		return fmt.Errorf("%s: %w", op, errPairingCodeRequired)
	}

	if s.hosting == nil {
		return fmt.Errorf("%s: %w", op, errHostingMissing)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID == "" {
		return fmt.Errorf("%s: %w", op, errContainerIDRequired)
	}

	if cl.ServerID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errServerIDRequired)
	}

	srv, err := s.servers.GetByID(ctx, cl.ServerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.hosting.ApprovePairing(ctx, cl, srv, code); err != nil {
		if errors.Is(err, hosting.ErrInvalidCode) {
			return fmt.Errorf("%s: %w", op, errPairingCodeInvalid)
		}
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) Connect(
	ctx context.Context,
	cm commands.ConnectClaw,
) error {
	const op = "service.Claw.Connect"

	if cm.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errClawIDRequired)
	}

	if s.hosting == nil {
		return fmt.Errorf("%s: %w", op, errHostingMissing)
	}

	provider := strings.TrimSpace(cm.Provider)
	if provider == "" {
		provider = "gmail"
	}

	if !strings.EqualFold(provider, "gmail") {
		return fmt.Errorf("%s: %w", op, errProviderUnsupported)
	}

	token, err := s.users.GetGmailToken(ctx, cm.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNotFound) {
			return fmt.Errorf("%s: %w", op, errGmailTokenRequired)
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
		return fmt.Errorf("%s: %w", op, errGmailTokenRequired)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID == "" {
		return fmt.Errorf("%s: %w", op, errContainerIDRequired)
	}

	if cl.ServerID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errServerIDRequired)
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
		return fmt.Errorf("%s: %w", op, errGmailWatchTopicRequired)
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

	if err := s.hosting.Connect(ctx, cl, srv, consts.GmailProvider, payload); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) Delete(
	ctx context.Context,
	cm commands.DeleteClaw,
) error {
	const op = "service.Claw.Delete"

	if cm.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if cm.ClawID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, errClawIDRequired)
	}

	cl, err := s.claws.GetByID(ctx, cm.ClawID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cl.ContainerID != "" {
		if s.hosting == nil {
			return fmt.Errorf("%s: %w", op, errHostingMissing)
		}

		if cl.ServerID == uuid.Nil {
			return fmt.Errorf("%s: %w", op, errServerIDRequired)
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
		return fmt.Errorf("%s: %w", op, errConfigArchivePathRequired)
	}

	if s.hosting == nil {
		return fmt.Errorf("%s: %w", op, errHostingMissing)
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
		if err := s.backupConfigArchive(ctx, cl, *server, false); err == nil {
			return nil
		}
	}

	if err := s.writeArchiveFromConfig(cl); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) writeArchiveFromConfig(cl entities.Claw) error {
	const op = "service.Claw.writeArchiveFromConfig"

	if s.archivePath == "" {
		return fmt.Errorf("%s: %w", op, errConfigArchivePathRequired)
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
		return fmt.Errorf("%s: %w", op, errConfigArchivePathRequired)
	}

	if s.hosting == nil {
		return fmt.Errorf("%s: %w", op, errHostingMissing)
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
		return "", errConfigArchivePathRequired
	}

	return filepath.Join(s.archivePath, cl.UserID.String(), cl.ID.String()+".tar"), nil
}

func applyBaseUpdates(
	existing entities.ClawConfig,
	primaryModel string,
	apiKey string,
) entities.ClawConfig {
	cfg := existing

	ensureConfigVars(&cfg, apiKey, "")

	cfg.Agents.Defaults.Model.Primary = primaryModel

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

func normalizeLimits(l commands.ApiKeyLimits) float64 {
	monthly := l.MonthlyBudgetUSD
	if monthly <= 0 {
		monthly = 50
	}

	return monthly
}

func deduplicateUUIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return nil
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
