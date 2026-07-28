package claw

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"shared/consts"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/storages/claws"
	"simpleClaw/internal/service/claw/commands"
)

type connectTestClawStorage struct {
	claw entities.Claw
}

func (s *connectTestClawStorage) Create(context.Context, entities.Claw, []uuid.UUID) error {
	return nil
}

func (s *connectTestClawStorage) GetBySystemID(context.Context, uuid.UUID) (entities.Claw, error) {
	return entities.Claw{}, sql.ErrNotFound
}

func (s *connectTestClawStorage) GetNextReconcilePending(context.Context) (entities.Claw, error) {
	return entities.Claw{}, sql.ErrNotFound
}

func (s *connectTestClawStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.Claw, error) {
	return s.claw, nil
}

func (s *connectTestClawStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return nil, nil
}

func (s *connectTestClawStorage) ListRuntimeSyncCandidates(context.Context, int) ([]entities.Claw, error) {
	return nil, nil
}

func (s *connectTestClawStorage) CountOccupiedByServer(context.Context) (map[uuid.UUID]int, error) {
	return map[uuid.UUID]int{}, nil
}

func (s *connectTestClawStorage) UpdateLifecycle(context.Context, uuid.UUID, claws.LifecycleUpdate) error {
	return nil
}

func (s *connectTestClawStorage) Update(context.Context, entities.Claw, []uuid.UUID, bool) error {
	return nil
}

func (s *connectTestClawStorage) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (s *connectTestClawStorage) UpdateRuntime(context.Context, uuid.UUID, entities.ClawRuntimeUpdate) error {
	return nil
}

type connectTestServerStorage struct {
	server entities.Server
}

func (s *connectTestServerStorage) GetAll(context.Context) ([]entities.Server, error) {
	return []entities.Server{s.server}, nil
}

func (s *connectTestServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return s.server, nil
}

func (s *connectTestServerStorage) GetByID(context.Context, uuid.UUID) (entities.Server, error) {
	return s.server, nil
}

type connectTestHosting struct {
	provider string
	payload  []byte
	calls    int
}

func (h *connectTestHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
	return hosting.Container{}, nil
}

func (h *connectTestHosting) Start(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *connectTestHosting) Stop(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *connectTestHosting) Delete(context.Context, entities.Claw, entities.Server, bool) error {
	return nil
}

func (h *connectTestHosting) State(context.Context, entities.Claw, entities.Server) (hosting.RuntimeState, error) {
	return hosting.RuntimeState{}, nil
}

func (h *connectTestHosting) ApprovePairing(context.Context, entities.Claw, entities.Server, string, string) error {
	return nil
}

func (h *connectTestHosting) Connect(
	_ context.Context,
	_ entities.Claw,
	_ entities.Server,
	provider string,
	token []byte,
) error {
	h.calls++
	h.provider = provider
	h.payload = append([]byte(nil), token...)

	return nil
}

func (h *connectTestHosting) ConfigArchive(context.Context, entities.Claw, entities.Server, bool) (io.ReadCloser, error) {
	return nil, nil
}

func (h *connectTestHosting) RestoreConfigArchive(context.Context, entities.Claw, entities.Server, io.Reader) error {
	return nil
}

type connectTestAttachmentStorage struct {
	items []entities.ClawCapabilityAttachment
}

func (s *connectTestAttachmentStorage) Upsert(context.Context, entities.ClawCapabilityAttachment) error {
	return nil
}

func (s *connectTestAttachmentStorage) ListByClawID(context.Context, uuid.UUID, uuid.UUID) ([]entities.ClawCapabilityAttachment, error) {
	return append([]entities.ClawCapabilityAttachment(nil), s.items...), nil
}

type connectTestIntegrationStorage struct {
	integration entities.AccountIntegration
	err         error
}

func (s *connectTestIntegrationStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.AccountIntegration, error) {
	if s.err != nil {
		return entities.AccountIntegration{}, s.err
	}

	return s.integration, nil
}

type connectTestOperationStorage struct{}

func (s *connectTestOperationStorage) Create(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

func (s *connectTestOperationStorage) GetActiveByClawID(context.Context, uuid.UUID) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, sql.ErrNotFound
}

func (s *connectTestOperationStorage) LockNextRunnable(context.Context, time.Time) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, sql.ErrNotFound
}

func (s *connectTestOperationStorage) Update(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

func TestServiceConnect_RequiresAttachedGmailCapability(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()

	svc := NewClaw(
		&connectTestClawStorage{claw: entities.Claw{
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "runtime-1",
		}},
		&connectTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{},
		&connectTestServerStorage{server: entities.Server{ID: serverID, URL: "http://runtime.local", SecretKey: "secret"}},
		&connectTestHosting{},
		&createTestKeys{},
		"",
		GmailWatchConfig{Topic: "projects/demo/topics/gmail-watch"},
	).WithCapabilityDependencies(&connectTestAttachmentStorage{}, &connectTestIntegrationStorage{})

	err := svc.Connect(context.Background(), commands.ConnectClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if !errors.Is(err, ErrGmailCapabilityRequired) {
		t.Fatalf("Connect() error = %v, want ErrGmailCapabilityRequired", err)
	}
}

func TestServiceConnect_UsesAttachedIntegrationPayload(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()
	integrationID := uuid.New()
	hostingStub := &connectTestHosting{}

	svc := NewClaw(
		&connectTestClawStorage{claw: entities.Claw{
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "runtime-1",
		}},
		&connectTestOperationStorage{},
		&createTestChannelStorage{},
		&createTestUserStorage{},
		&connectTestServerStorage{server: entities.Server{ID: serverID, URL: "http://runtime.local", SecretKey: "secret"}},
		hostingStub,
		&createTestKeys{},
		"",
		GmailWatchConfig{
			Topic:  "projects/demo/topics/gmail-watch",
			Labels: []string{"INBOX", "UNREAD"},
		},
	).WithCapabilityDependencies(
		&connectTestAttachmentStorage{items: []entities.ClawCapabilityAttachment{{
			ID:                   uuid.New(),
			ClawID:               clawID,
			UserID:               userID,
			CapabilityID:         entities.CapabilityGmail,
			AccountIntegrationID: &integrationID,
			Enabled:              true,
		}}},
		&connectTestIntegrationStorage{integration: entities.AccountIntegration{
			ID:                integrationID,
			UserID:            userID,
			CapabilityID:      entities.CapabilityGmail,
			Provider:          consts.ProviderGmail,
			ExternalAccountID: "user@example.com",
			Metadata: map[string]any{
				"client": "snapclaw",
			},
			SecretPayload: map[string]any{
				"refresh_token": "refresh-token-1",
			},
		}},
	)

	err := svc.Connect(context.Background(), commands.ConnectClaw{
		UserID:   userID,
		ClawID:   clawID,
		Provider: consts.ProviderGmail,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if hostingStub.calls != 1 {
		t.Fatalf("hosting connect calls = %d, want 1", hostingStub.calls)
	}

	if hostingStub.provider != consts.ProviderGmail {
		t.Fatalf("provider = %q, want %q", hostingStub.provider, consts.ProviderGmail)
	}

	var payload struct {
		Email        string   `json:"email"`
		Client       string   `json:"client"`
		RefreshToken string   `json:"refresh_token"`
		Topic        string   `json:"topic"`
		Labels       []string `json:"labels"`
	}
	if err := json.Unmarshal(hostingStub.payload, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if payload.Email != "user@example.com" {
		t.Fatalf("payload email = %q, want %q", payload.Email, "user@example.com")
	}

	if payload.Client != "snapclaw" {
		t.Fatalf("payload client = %q, want %q", payload.Client, "snapclaw")
	}

	if payload.RefreshToken != "refresh-token-1" {
		t.Fatalf("payload refresh token = %q, want %q", payload.RefreshToken, "refresh-token-1")
	}

	if payload.Topic != "projects/demo/topics/gmail-watch" {
		t.Fatalf("payload topic = %q, want %q", payload.Topic, "projects/demo/topics/gmail-watch")
	}

	if len(payload.Labels) != 2 || payload.Labels[0] != "INBOX" || payload.Labels[1] != "UNREAD" {
		t.Fatalf("payload labels = %#v", payload.Labels)
	}
}
