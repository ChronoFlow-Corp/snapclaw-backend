package claw

import (
	"context"
	"io"
	"testing"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/entities/channels"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/service/claw/commands"

	"github.com/google/uuid"
)

type createTestClawStorage struct {
	created         entities.Claw
	createdChannels []uuid.UUID
	runtimeUpdated  entities.Claw
}

func (s *createTestClawStorage) Create(_ context.Context, cl entities.Claw, channelIDs []uuid.UUID) error {
	s.created = cl
	s.createdChannels = append([]uuid.UUID(nil), channelIDs...)
	return nil
}

func (s *createTestClawStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.Claw, error) {
	return entities.Claw{}, nil
}

func (s *createTestClawStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return nil, nil
}

func (s *createTestClawStorage) Update(context.Context, entities.Claw, []uuid.UUID, bool) error {
	return nil
}

func (s *createTestClawStorage) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (s *createTestClawStorage) UpdateRuntime(
	_ context.Context,
	clID uuid.UUID,
	serverID uuid.UUID,
	containerID string,
	status string,
) error {
	s.runtimeUpdated.ID = clID
	s.runtimeUpdated.ServerID = serverID
	s.runtimeUpdated.ContainerID = containerID
	s.runtimeUpdated.Status = status
	return nil
}

type createTestChannelStorage struct {
	channels []entities.Channel
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

func (s *createTestUserStorage) GetGmailToken(context.Context, uuid.UUID) (entities.GmailToken, error) {
	return entities.GmailToken{}, nil
}

type createTestServerStorage struct {
	server entities.Server
}

func (s *createTestServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return s.server, nil
}

func (s *createTestServerStorage) GetByID(context.Context, uuid.UUID) (entities.Server, error) {
	return s.server, nil
}

type createTestKeys struct {
	resolveModelResult string
	resolveModelCalls  int
}

func (k *createTestKeys) Create(context.Context, uuid.UUID, string, float64) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{}, nil
}

func (k *createTestKeys) ResolveModel(context.Context, string) (string, error) {
	k.resolveModelCalls++
	return k.resolveModelResult, nil
}

type createTestHosting struct {
	container hosting.Container
}

func (h *createTestHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
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

func (h *createTestHosting) Update(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *createTestHosting) ApprovePairing(context.Context, entities.Claw, entities.Server, string) error {
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
	serverID := uuid.New()

	storage := &createTestClawStorage{}
	keys := &createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}
	svc := NewClaw(
		storage,
		&createTestChannelStorage{},
		&createTestUserStorage{user: entities.User{
			ID:               userID,
			OpenRouterApiKey: "secret",
			OpenRouterKeyID:  "key-id",
			CreatedAt:        time.Now(),
		}},
		&createTestServerStorage{server: entities.Server{ID: serverID}},
		&createTestHosting{container: hosting.Container{ID: "container-1", ServerID: serverID}},
		keys,
		"",
		GmailWatchConfig{},
	)

	cl, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: userID,
		Name:   "demo",
		Model:  "openai/gpt-4.1-mini",
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

	if keys.resolveModelCalls != 1 {
		t.Fatalf("expected one resolve model call, got %d", keys.resolveModelCalls)
	}
}

func TestServiceCreate_AddsNormalizedTelegramChannel(t *testing.T) {
	userID := uuid.New()
	serverID := uuid.New()
	channelID := uuid.New()

	storage := &createTestClawStorage{}
	keys := &createTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}
	svc := NewClaw(
		storage,
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
		&createTestServerStorage{server: entities.Server{ID: serverID}},
		&createTestHosting{container: hosting.Container{ID: "container-1", ServerID: serverID}},
		keys,
		"",
		GmailWatchConfig{},
	)

	cl, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID:     userID,
		Name:       "demo",
		Model:      "openai/gpt-4.1-mini",
		ChannelIDs: []uuid.UUID{channelID},
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
}
