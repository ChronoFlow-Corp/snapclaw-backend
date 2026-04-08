package claw

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/entities/channels"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/storages/claws"
	"simpleClaw/internal/service/claw/commands"
)

type approveTestHosting struct {
	channelType string
	code        string
	err         error
}

func (h *approveTestHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
	return hosting.Container{}, nil
}

func (h *approveTestHosting) Start(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *approveTestHosting) Stop(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *approveTestHosting) Delete(context.Context, entities.Claw, entities.Server, bool) error {
	return nil
}

func (h *approveTestHosting) State(context.Context, entities.Claw, entities.Server) (hosting.RuntimeState, error) {
	return hosting.RuntimeState{}, nil
}

func (h *approveTestHosting) ApprovePairing(
	_ context.Context,
	_ entities.Claw,
	_ entities.Server,
	channelType string,
	code string,
) error {
	h.channelType = channelType
	h.code = code

	return h.err
}

func (h *approveTestHosting) Connect(context.Context, entities.Claw, entities.Server, string, []byte) error {
	return nil
}

type approveTestClawStorage struct {
	claw            entities.Claw
	lifecycleUpdate claws.LifecycleUpdate
}

func (s *approveTestClawStorage) Create(context.Context, entities.Claw, []uuid.UUID) error {
	return nil
}

func (s *approveTestClawStorage) GetBySystemID(context.Context, uuid.UUID) (entities.Claw, error) {
	return entities.Claw{}, nil
}

func (s *approveTestClawStorage) GetNextReconcilePending(context.Context) (entities.Claw, error) {
	return entities.Claw{}, nil
}

func (s *approveTestClawStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.Claw, error) {
	return s.claw, nil
}

func (s *approveTestClawStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return nil, nil
}

func (s *approveTestClawStorage) ListRuntimeSyncCandidates(context.Context, int) ([]entities.Claw, error) {
	return nil, nil
}

func (s *approveTestClawStorage) CountOccupiedByServer(context.Context) (map[uuid.UUID]int, error) {
	return map[uuid.UUID]int{}, nil
}

func (s *approveTestClawStorage) Update(context.Context, entities.Claw, []uuid.UUID, bool) error {
	return nil
}

func (s *approveTestClawStorage) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (s *approveTestClawStorage) UpdateRuntime(context.Context, uuid.UUID, entities.ClawRuntimeUpdate) error {
	return nil
}

func (s *approveTestClawStorage) UpdateLifecycle(_ context.Context, _ uuid.UUID, update claws.LifecycleUpdate) error {
	s.lifecycleUpdate = update

	return nil
}

type approveTestOperationStorage struct{}

func (s *approveTestOperationStorage) Create(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

func (s *approveTestOperationStorage) GetActiveByClawID(context.Context, uuid.UUID) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, nil
}

func (s *approveTestOperationStorage) LockNextRunnable(context.Context, time.Time) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, nil
}

func (s *approveTestOperationStorage) Update(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

type approveTestServerStorage struct {
	server entities.Server
}

func (s *approveTestServerStorage) GetAll(context.Context) ([]entities.Server, error) {
	return []entities.Server{s.server}, nil
}

func (s *approveTestServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return s.server, nil
}

func (s *approveTestServerStorage) GetByID(context.Context, uuid.UUID) (entities.Server, error) {
	return s.server, nil
}

type approveTestChannelStorage struct{}

func (s *approveTestChannelStorage) GetByIDs(context.Context, []uuid.UUID, uuid.UUID) ([]entities.Channel, error) {
	return nil, nil
}

type approveTestUserStorage struct{}

func (s *approveTestUserStorage) GetByID(context.Context, uuid.UUID) (entities.User, error) {
	return entities.User{}, nil
}

func (s *approveTestUserStorage) UpdateOpenRouterKey(context.Context, uuid.UUID, entities.OpenRouterKey) error {
	return nil
}

type approveTestKeys struct{}

func (k *approveTestKeys) Create(context.Context, uuid.UUID, float64) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{}, nil
}

func (k *approveTestKeys) ResolveModel(context.Context, string) (string, error) {
	return "", nil
}

func TestServiceApprovePairing_InfersTelegramChannel(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()
	hostingStub := &approveTestHosting{}

	svc := NewClaw(
		&approveTestClawStorage{claw: entities.Claw{
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "container-1",
			Config: entities.ClawConfig{
				Channels: &entities.ClawChannels{
					Telegram: &channels.TelegramConfig{
						Enabled:  true,
						DmPolicy: channels.DmPairing,
					},
				},
			},
		}},
		&approveTestOperationStorage{},
		&approveTestChannelStorage{},
		&approveTestUserStorage{},
		&approveTestServerStorage{server: entities.Server{ID: serverID, URL: "http://runtime.local", SecretKey: "secret"}},
		hostingStub,
		&approveTestKeys{},
		"",
		GmailWatchConfig{},
	)

	err := svc.ApprovePairing(context.Background(), commands.ApprovePairing{
		UserID: userID,
		ClawID: clawID,
		Code:   "123456",
	})
	if err != nil {
		t.Fatalf("ApprovePairing() error = %v", err)
	}

	if hostingStub.channelType != entities.ChannelTelegramType {
		t.Fatalf("channelType = %q, want %q", hostingStub.channelType, entities.ChannelTelegramType)
	}
}

func TestServiceApprovePairing_RequiresChannelForMultipleCandidates(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()

	svc := NewClaw(
		&approveTestClawStorage{claw: entities.Claw{
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "container-1",
			Config: entities.ClawConfig{
				Channels: &entities.ClawChannels{
					Telegram: &channels.TelegramConfig{
						Enabled:  true,
						DmPolicy: channels.DmPairing,
					},
					WhatsApp: &entities.WhatsAppConfig{
						DmPolicy: "pairing",
					},
				},
			},
		}},
		&approveTestOperationStorage{},
		&approveTestChannelStorage{},
		&approveTestUserStorage{},
		&approveTestServerStorage{server: entities.Server{ID: serverID, URL: "http://runtime.local", SecretKey: "secret"}},
		&approveTestHosting{},
		&approveTestKeys{},
		"",
		GmailWatchConfig{},
	)

	err := svc.ApprovePairing(context.Background(), commands.ApprovePairing{
		UserID: userID,
		ClawID: clawID,
		Code:   "123456",
	})
	if !errors.Is(err, ErrApproveChannelRequired) {
		t.Fatalf("ApprovePairing() error = %v, want ErrApproveChannelRequired", err)
	}
}

func TestServiceApprovePairing_RejectsUnsupportedChannel(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()

	svc := NewClaw(
		&approveTestClawStorage{claw: entities.Claw{
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "container-1",
			Config: entities.ClawConfig{
				Channels: &entities.ClawChannels{
					Telegram: &channels.TelegramConfig{
						Enabled:  true,
						DmPolicy: channels.DmPairing,
					},
				},
			},
		}},
		&approveTestOperationStorage{},
		&approveTestChannelStorage{},
		&approveTestUserStorage{},
		&approveTestServerStorage{server: entities.Server{ID: serverID, URL: "http://runtime.local", SecretKey: "secret"}},
		&approveTestHosting{},
		&approveTestKeys{},
		"",
		GmailWatchConfig{},
	)

	err := svc.ApprovePairing(context.Background(), commands.ApprovePairing{
		UserID:      userID,
		ClawID:      clawID,
		Code:        "123456",
		ChannelType: entities.ChannelSlackType,
	})
	if !errors.Is(err, ErrApproveChannelUnsupported) {
		t.Fatalf("ApprovePairing() error = %v, want ErrApproveChannelUnsupported", err)
	}
}
