package claw

import (
	"context"
	"errors"
	"io"
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/entities/channels"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/storages/claws"
	"simpleClaw/internal/service/claw/commands"
)

type createRuntimeUpdateCall struct {
	ID     uuid.UUID
	Update entities.ClawRuntimeUpdate
}

type createTestClawStorage struct {
	created          entities.Claw
	createdChannels  []uuid.UUID
	runtimeUpdated   createRuntimeUpdateCall
	occupiedByServer map[uuid.UUID]int
	deleteCalled     bool
}

func (s *createTestClawStorage) Create(_ context.Context, cl entities.Claw, channelIDs []uuid.UUID) error {
	s.created = cl

	s.createdChannels = append([]uuid.UUID(nil), channelIDs...)

	return nil
}

func (s *createTestClawStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.Claw, error) {
	return entities.Claw{}, nil
}

func (s *createTestClawStorage) GetBySystemID(context.Context, uuid.UUID) (entities.Claw, error) {
	return entities.Claw{}, nil
}

func (s *createTestClawStorage) GetNextReconcilePending(context.Context) (entities.Claw, error) {
	return entities.Claw{}, sql.ErrNotFound
}

func (s *createTestClawStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return nil, nil
}

func (s *createTestClawStorage) ListRuntimeSyncCandidates(context.Context, int) ([]entities.Claw, error) {
	return nil, nil
}

func (s *createTestClawStorage) CountOccupiedByServer(context.Context) (map[uuid.UUID]int, error) {
	if s.occupiedByServer == nil {
		return map[uuid.UUID]int{}, nil
	}

	res := make(map[uuid.UUID]int, len(s.occupiedByServer))
	maps.Copy(res, s.occupiedByServer)

	return res, nil
}

func (s *createTestClawStorage) Update(context.Context, entities.Claw, []uuid.UUID, bool) error {
	return nil
}

func (s *createTestClawStorage) UpdateLifecycle(context.Context, uuid.UUID, claws.LifecycleUpdate) error {
	return nil
}

func (s *createTestClawStorage) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	s.deleteCalled = true
	return nil
}

func (s *createTestClawStorage) UpdateRuntime(
	_ context.Context,
	clID uuid.UUID,
	update entities.ClawRuntimeUpdate,
) error {
	s.runtimeUpdated.ID = clID
	s.runtimeUpdated.Update = update

	return nil
}

type createTestChannelStorage struct {
	channels []entities.Channel
}

type createTestOperationStorage struct{}

func (s *createTestOperationStorage) Create(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

func (s *createTestOperationStorage) GetActiveByClawID(context.Context, uuid.UUID) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, sql.ErrNotFound
}

func (s *createTestOperationStorage) LockNextRunnable(context.Context, time.Time) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, sql.ErrNotFound
}

func (s *createTestOperationStorage) Update(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

func (s *createTestChannelStorage) GetByIDs(context.Context, []uuid.UUID, uuid.UUID) ([]entities.Channel, error) {
	return append([]entities.Channel(nil), s.channels...), nil
}

type createTestUserStorage struct {
	user entities.User
}

func (s *createTestUserStorage) GetByID(context.Context, uuid.UUID) (entities.User, error) {
	return s.user, nil
}

func (s *createTestUserStorage) UpdateOpenRouterKey(context.Context, uuid.UUID, entities.OpenRouterKey) error {
	return nil
}

type createTestIntegrationStorage struct {
	integrations map[uuid.UUID]entities.AccountIntegration
}

func (s *createTestIntegrationStorage) GetByID(_ context.Context, id, userID uuid.UUID) (entities.AccountIntegration, error) {
	integration, ok := s.integrations[id]
	if !ok || integration.UserID != userID {
		return entities.AccountIntegration{}, sql.ErrNotFound
	}

	return integration, nil
}

type createTestAttachmentStorage struct {
	items []entities.ClawCapabilityAttachment
	err   error
}

func (s *createTestAttachmentStorage) Upsert(_ context.Context, attachment entities.ClawCapabilityAttachment) error {
	if s.err != nil {
		return s.err
	}

	s.items = append(s.items, attachment)
	return nil
}

func (s *createTestAttachmentStorage) ListByClawID(context.Context, uuid.UUID, uuid.UUID) ([]entities.ClawCapabilityAttachment, error) {
	return append([]entities.ClawCapabilityAttachment(nil), s.items...), nil
}

type createTestServerStorage struct {
	server  entities.Server
	servers []entities.Server
}

func (s *createTestServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return s.server, nil
}

func (s *createTestServerStorage) GetAll(context.Context) ([]entities.Server, error) {
	if len(s.servers) == 0 {
		if s.server.ID == uuid.Nil {
			return nil, nil
		}

		server := s.server
		if server.MaxClaws == 0 {
			server.MaxClaws = 1
		}

		return []entities.Server{server}, nil
	}

	servers := append([]entities.Server(nil), s.servers...)
	for i := range servers {
		if servers[i].MaxClaws == 0 {
			servers[i].MaxClaws = 1
		}
	}

	return servers, nil
}

func (s *createTestServerStorage) GetByID(context.Context, uuid.UUID) (entities.Server, error) {
	return s.server, nil
}

type createTestKeys struct {
	resolveModelResult string
	resolveModelCalls  int
}

func (k *createTestKeys) Create(context.Context, uuid.UUID, float64) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{}, nil
}

func (k *createTestKeys) ResolveModel(context.Context, string) (string, error) {
	k.resolveModelCalls++

	return k.resolveModelResult, nil
}

type createTestHosting struct {
	container   hosting.Container
	createCalls int
}

func (h *createTestHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
	h.createCalls++

	return h.container, nil
}

func (h *createTestHosting) Start(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *createTestHosting) Stop(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *createTestHosting) Delete(context.Context, entities.Claw, entities.Server, bool) error {
	return nil
}

func (h *createTestHosting) State(context.Context, entities.Claw, entities.Server) (hosting.RuntimeState, error) {
	return hosting.RuntimeState{}, nil
}

func (h *createTestHosting) ApprovePairing(context.Context, entities.Claw, entities.Server, string, string) error {
	return nil
}

func (h *createTestHosting) Connect(context.Context, entities.Claw, entities.Server, string, []byte) error {
	return nil
}

func (h *createTestHosting) ConfigArchive(context.Context, entities.Claw, entities.Server, bool) (io.ReadCloser, error) {
	return nil, nil
}

func (h *createTestHosting) RestoreConfigArchive(context.Context, entities.Claw, entities.Server, io.Reader) error {
	return nil
}

func TestServiceCreate_BuildsMinimalConfigWithMainAgent(t *testing.T) {
	userID := uuid.New()

	storage := &createTestClawStorage{}
	keys := &createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}
	hostingStub := &createTestHosting{}
	svc := NewClaw(
		storage,
		&createTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		hostingStub,
		keys,
		"",
		GmailWatchConfig{},
	).WithBraveAPIKey("brave-secret")

	cl, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: userID,
		Name:   "demo",
		Model:  "openai/gpt-4.1-mini",
		Capabilities: commands.CreateCapabilitySet{
			WebSearch: &commands.WebSearchCapabilityInput{
				Enabled:  true,
				Provider: "brave",
			},
		},
	})
	if err != nil {
		t.Fatalf("create claw: %v", err)
	}

	if len(cl.Config.Agents.List) != 1 {
		t.Fatalf("expected one default agent, got %d", len(cl.Config.Agents.List))
	}

	agent := cl.Config.Agents.List[0]
	if agent.ID != "main" {
		t.Fatalf("expected main agent, got %q", agent.ID)
	}

	if agent.Model == nil || agent.Model.Primary != "openrouter/openai/gpt-4.1-mini" {
		t.Fatalf("expected resolved model on main agent, got %#v", agent.Model)
	}

	if cl.Config.Env.OpenRouterAPIKey != "${OPENROUTER_API_KEY}" {
		t.Fatalf("expected env reference for openrouter key, got %q", cl.Config.Env.OpenRouterAPIKey)
	}

	if cl.Config.Env.Vars[openRouterAPIKeyVar] != "secret" {
		t.Fatalf("expected openrouter key var to be injected, got %#v", cl.Config.Env.Vars)
	}

	if cl.Config.Gateway == nil || cl.Config.Gateway.Auth.Token != "${OPENCLAW_GATEWAY_TOKEN}" {
		t.Fatalf("expected gateway token reference, got %#v", cl.Config.Gateway)
	}

	if cl.Config.Tools == nil {
		t.Fatalf("expected unified tools config")
	}

	if cl.Config.Plugins == nil {
		t.Fatalf("expected plugins config")
	}

	brave, ok := cl.Config.Plugins.Entries["brave"]
	if !ok {
		t.Fatalf("expected brave plugin entry, got %#v", cl.Config.Plugins.Entries)
	}

	webSearch, ok := brave.Config["webSearch"].(map[string]any)
	if !ok {
		t.Fatalf("expected brave webSearch config, got %#v", brave.Config["webSearch"])
	}

	if webSearch["apiKey"] != "${BRAVE_API_KEY}" {
		t.Fatalf("expected brave api key ref, got %#v", webSearch["apiKey"])
	}

	if cl.Config.Tools.Web == nil || cl.Config.Tools.Web.Search == nil {
		t.Fatalf("expected web search tools config, got %#v", cl.Config.Tools)
	}

	if !cl.Config.Tools.Web.Search.Enabled {
		t.Fatalf("expected web search enabled")
	}

	if cl.Config.Tools.Web.Search.Provider != "brave" {
		t.Fatalf("expected brave provider, got %q", cl.Config.Tools.Web.Search.Provider)
	}

	if keys.resolveModelCalls != 1 {
		t.Fatalf("expected one resolve model call, got %d", keys.resolveModelCalls)
	}

	if cl.ContainerID != "" {
		t.Fatalf("expected empty container id, got %q", cl.ContainerID)
	}

	if cl.ServerID != uuid.Nil {
		t.Fatalf("expected zero server id, got %s", cl.ServerID)
	}

	if hostingStub.createCalls != 0 {
		t.Fatalf("expected hosting create to not be called, got %d", hostingStub.createCalls)
	}

	if storage.runtimeUpdated.ID != uuid.Nil {
		t.Fatalf("expected runtime update to not be called, got %#v", storage.runtimeUpdated)
	}
}

func TestServiceCreate_AddsNormalizedTelegramChannel(t *testing.T) {
	userID := uuid.New()
	channelID := uuid.New()

	storage := &createTestClawStorage{}
	keys := &createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}
	hostingStub := &createTestHosting{}
	svc := NewClaw(
		storage,
		&createTestOperationStorage{},
		&createTestChannelStorage{channels: []entities.Channel{
			{
				ID:          channelID,
				ChannelType: entities.ChannelTelegramType,
				UserID:      userID,
				Config: entities.ClawChannels{
					Telegram: &channels.TelegramConfig{
						BotToken:       "telegram-secret",
						DmPolicy:       channels.DmPairing,
						AllowFrom:      []string{"tg:1"},
						GroupPolicy:    "allowlist",
						GroupAllowFrom: []string{"tg:2"},
					},
				},
				CreatedAt: time.Now(),
			},
		}},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		hostingStub,
		keys,
		"",
		GmailWatchConfig{},
	).WithBraveAPIKey("brave-secret")

	cl, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID:     userID,
		Name:       "demo",
		Model:      "openai/gpt-4.1-mini",
		ChannelIDs: []uuid.UUID{channelID},
		Capabilities: commands.CreateCapabilitySet{
			WebSearch: &commands.WebSearchCapabilityInput{
				Enabled:  true,
				Provider: "brave",
			},
		},
	})
	if err != nil {
		t.Fatalf("create claw: %v", err)
	}

	if cl.Config.Channels == nil || cl.Config.Channels.Telegram == nil {
		t.Fatalf("expected telegram channel config, got %#v", cl.Config.Channels)
	}

	tg := cl.Config.Channels.Telegram
	if !tg.Enabled {
		t.Fatalf("expected telegram channel to be enabled")
	}

	if tg.DmPolicy != channels.DmPairing {
		t.Fatalf("expected telegram dm policy pairing, got %q", tg.DmPolicy)
	}

	if tg.BotToken != "telegram-secret" {
		t.Fatalf("expected telegram bot token to be preserved, got %q", tg.BotToken)
	}

	if hostingStub.createCalls != 0 {
		t.Fatalf("expected hosting create to not be called, got %d", hostingStub.createCalls)
	}
}

func TestServiceCreate_AddsWhatsAppChannel(t *testing.T) {
	userID := uuid.New()
	channelID := uuid.New()

	storage := &createTestClawStorage{}
	keys := &createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}
	hostingStub := &createTestHosting{}
	svc := NewClaw(
		storage,
		&createTestOperationStorage{},
		&createTestChannelStorage{channels: []entities.Channel{
			{
				ID:          channelID,
				ChannelType: entities.ChannelWhatsappType,
				UserID:      userID,
				Config: entities.ClawChannels{
					WhatsApp: &entities.WhatsAppConfig{
						DmPolicy:       "pairing",
						AllowFrom:      []string{"wa:1"},
						GroupPolicy:    "allowlist",
						GroupAllowFrom: []string{"wa:2"},
					},
				},
				CreatedAt: time.Now(),
			},
		}},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		hostingStub,
		keys,
		"",
		GmailWatchConfig{},
	).WithBraveAPIKey("brave-secret")

	cl, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID:     userID,
		Name:       "demo",
		Model:      "openai/gpt-4.1-mini",
		ChannelIDs: []uuid.UUID{channelID},
		Capabilities: commands.CreateCapabilitySet{
			WebSearch: &commands.WebSearchCapabilityInput{
				Enabled:  true,
				Provider: "brave",
			},
		},
	})
	if err != nil {
		t.Fatalf("create claw: %v", err)
	}

	if cl.Config.Channels == nil || cl.Config.Channels.WhatsApp == nil {
		t.Fatalf("expected whatsapp channel config, got %#v", cl.Config.Channels)
	}

	if cl.Config.Channels.WhatsApp.DmPolicy != "pairing" {
		t.Fatalf("expected whatsapp dm policy pairing, got %q", cl.Config.Channels.WhatsApp.DmPolicy)
	}
}

func TestServiceCreate_RejectsMissingRequiredWebSearchCapability(t *testing.T) {
	userID := uuid.New()

	svc := NewClaw(
		&createTestClawStorage{},
		&createTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		&createTestHosting{},
		&createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		"",
		GmailWatchConfig{},
	).WithBraveAPIKey("brave-secret")

	_, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: userID,
		Name:   "demo",
		Model:  "openai/gpt-4.1-mini",
	})
	if err == nil {
		t.Fatal("expected error when web search capability is missing")
	}
}

func TestServiceCreate_AttachesEnabledGoogleCapabilities(t *testing.T) {
	userID := uuid.New()
	gmailIntegrationID := uuid.New()
	attachmentStorage := &createTestAttachmentStorage{}

	svc := NewClaw(
		&createTestClawStorage{},
		&createTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		&createTestHosting{},
		&createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		"",
		GmailWatchConfig{},
	).WithCapabilityDependencies(
		attachmentStorage,
		&createTestIntegrationStorage{integrations: map[uuid.UUID]entities.AccountIntegration{
			gmailIntegrationID: {
				ID:                gmailIntegrationID,
				UserID:            userID,
				CapabilityID:      entities.CapabilityGmail,
				Provider:          "gmail",
				ExternalAccountID: "user@example.com",
			},
		}},
	).WithBraveAPIKey("brave-secret")

	cl, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: userID,
		Name:   "demo",
		Model:  "openai/gpt-4.1-mini",
		Capabilities: commands.CreateCapabilitySet{
			WebSearch: &commands.WebSearchCapabilityInput{
				Enabled:  true,
				Provider: "brave",
			},
			Gmail: &commands.IntegrationBoundCapabilityInput{
				Enabled:              true,
				Provider:             "gmail",
				AccountIntegrationID: &gmailIntegrationID,
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if len(attachmentStorage.items) != 1 {
		t.Fatalf("attachment count = %d, want 1", len(attachmentStorage.items))
	}

	if attachmentStorage.items[0].CapabilityID != entities.CapabilityGmail {
		t.Fatalf("capability = %q", attachmentStorage.items[0].CapabilityID)
	}

	if attachmentStorage.items[0].AccountIntegrationID == nil || *attachmentStorage.items[0].AccountIntegrationID != gmailIntegrationID {
		t.Fatalf("account integration id = %#v", attachmentStorage.items[0].AccountIntegrationID)
	}

	if cl.Config.Hooks == nil || cl.Config.Hooks.Gmail.Serve.Path != "/gmail-pubsub" {
		t.Fatalf("gmail hook path = %#v", cl.Config.Hooks)
	}
}

func TestServiceCreate_RejectsBraveWhenBackendKeyMissing(t *testing.T) {
	userID := uuid.New()

	svc := NewClaw(
		&createTestClawStorage{},
		&createTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		&createTestHosting{},
		&createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		"",
		GmailWatchConfig{},
	)

	_, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: userID,
		Name:   "demo",
		Model:  "openai/gpt-4.1-mini",
		Capabilities: commands.CreateCapabilitySet{
			WebSearch: &commands.WebSearchCapabilityInput{
				Enabled:  true,
				Provider: "brave",
			},
		},
	})
	if !errors.Is(err, ErrBraveAPIKeyMissing) {
		t.Fatalf("Create() error = %v, want ErrBraveAPIKeyMissing", err)
	}
}

func TestServiceCreate_RejectsEnabledGoogleCapabilityWithoutIntegrationID(t *testing.T) {
	userID := uuid.New()

	svc := NewClaw(
		&createTestClawStorage{},
		&createTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		&createTestHosting{},
		&createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		"",
		GmailWatchConfig{},
	).WithCapabilityDependencies(&createTestAttachmentStorage{}, &createTestIntegrationStorage{}).WithBraveAPIKey("brave-secret")

	_, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: userID,
		Name:   "demo",
		Model:  "openai/gpt-4.1-mini",
		Capabilities: commands.CreateCapabilitySet{
			WebSearch: &commands.WebSearchCapabilityInput{
				Enabled:  true,
				Provider: "brave",
			},
			Gmail: &commands.IntegrationBoundCapabilityInput{
				Enabled: true,
			},
		},
	})
	if !errors.Is(err, sql.ErrInvalid) {
		t.Fatalf("Create() error = %v, want sql.ErrInvalid", err)
	}
}

func TestServiceCreate_RejectsMismatchedGoogleIntegration(t *testing.T) {
	userID := uuid.New()
	integrationID := uuid.New()

	svc := NewClaw(
		&createTestClawStorage{},
		&createTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		&createTestHosting{},
		&createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		"",
		GmailWatchConfig{},
	).WithCapabilityDependencies(
		&createTestAttachmentStorage{},
		&createTestIntegrationStorage{integrations: map[uuid.UUID]entities.AccountIntegration{
			integrationID: {
				ID:           integrationID,
				UserID:       userID,
				CapabilityID: entities.CapabilityGoogleCalendar,
				Provider:     "google_calendar",
			},
		}},
	).WithBraveAPIKey("brave-secret")

	_, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: userID,
		Name:   "demo",
		Model:  "openai/gpt-4.1-mini",
		Capabilities: commands.CreateCapabilitySet{
			WebSearch: &commands.WebSearchCapabilityInput{
				Enabled:  true,
				Provider: "brave",
			},
			Gmail: &commands.IntegrationBoundCapabilityInput{
				Enabled:              true,
				Provider:             "gmail",
				AccountIntegrationID: &integrationID,
			},
		},
	})
	if !errors.Is(err, sql.ErrInvalid) {
		t.Fatalf("Create() error = %v, want sql.ErrInvalid", err)
	}
}

func TestServiceCreate_RollsBackClawWhenAttachmentPersistenceFails(t *testing.T) {
	userID := uuid.New()
	gmailIntegrationID := uuid.New()
	clawStorage := &createTestClawStorage{}

	svc := NewClaw(
		clawStorage,
		&createTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{},
		&createTestHosting{},
		&createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		"",
		GmailWatchConfig{},
	).WithCapabilityDependencies(
		&createTestAttachmentStorage{err: errors.New("persist attachment")},
		&createTestIntegrationStorage{integrations: map[uuid.UUID]entities.AccountIntegration{
			gmailIntegrationID: {
				ID:           gmailIntegrationID,
				UserID:       userID,
				CapabilityID: entities.CapabilityGmail,
				Provider:     "gmail",
			},
		}},
	).WithBraveAPIKey("brave-secret")

	_, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: userID,
		Name:   "demo",
		Model:  "openai/gpt-4.1-mini",
		Capabilities: commands.CreateCapabilitySet{
			WebSearch: &commands.WebSearchCapabilityInput{
				Enabled:  true,
				Provider: "brave",
			},
			Gmail: &commands.IntegrationBoundCapabilityInput{
				Enabled:              true,
				Provider:             "gmail",
				AccountIntegrationID: &gmailIntegrationID,
			},
		},
	})
	if err == nil {
		t.Fatal("Create() error = nil, want non-nil")
	}

	if !clawStorage.deleteCalled {
		t.Fatal("expected claw rollback delete to be called")
	}
}
