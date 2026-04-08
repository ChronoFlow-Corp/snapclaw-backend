package claw

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/storages/claws"

	"github.com/google/uuid"
)

type runtimeSyncClawStorage struct {
	candidates    []entities.Claw
	runtimeUpdate entities.ClawRuntimeUpdate
}

func (s *runtimeSyncClawStorage) Create(context.Context, entities.Claw, []uuid.UUID) error {
	return nil
}

func (s *runtimeSyncClawStorage) GetBySystemID(context.Context, uuid.UUID) (entities.Claw, error) {
	return entities.Claw{}, errors.New("not implemented")
}

func (s *runtimeSyncClawStorage) GetNextReconcilePending(context.Context) (entities.Claw, error) {
	return entities.Claw{}, errors.New("not implemented")
}

func (s *runtimeSyncClawStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.Claw, error) {
	return entities.Claw{}, errors.New("not implemented")
}

func (s *runtimeSyncClawStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return nil, errors.New("not implemented")
}

func (s *runtimeSyncClawStorage) ListRuntimeSyncCandidates(context.Context, int) ([]entities.Claw, error) {
	return append([]entities.Claw(nil), s.candidates...), nil
}

func (s *runtimeSyncClawStorage) CountOccupiedByServer(context.Context) (map[uuid.UUID]int, error) {
	return nil, errors.New("not implemented")
}

func (s *runtimeSyncClawStorage) UpdateLifecycle(context.Context, uuid.UUID, claws.LifecycleUpdate) error {
	return nil
}

func (s *runtimeSyncClawStorage) Update(context.Context, entities.Claw, []uuid.UUID, bool) error {
	return nil
}

func (s *runtimeSyncClawStorage) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (s *runtimeSyncClawStorage) UpdateRuntime(_ context.Context, _ uuid.UUID, update entities.ClawRuntimeUpdate) error {
	s.runtimeUpdate = update
	return nil
}

type runtimeSyncOperationStorage struct{}

func (s *runtimeSyncOperationStorage) Create(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

func (s *runtimeSyncOperationStorage) GetActiveByClawID(context.Context, uuid.UUID) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, errors.New("not implemented")
}

func (s *runtimeSyncOperationStorage) LockNextRunnable(context.Context, time.Time) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, errors.New("not implemented")
}

func (s *runtimeSyncOperationStorage) Update(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

type runtimeSyncServerStorage struct {
	server entities.Server
}

func (s *runtimeSyncServerStorage) GetAll(context.Context) ([]entities.Server, error) {
	return []entities.Server{s.server}, nil
}

func (s *runtimeSyncServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return s.server, nil
}

func (s *runtimeSyncServerStorage) GetByID(context.Context, uuid.UUID) (entities.Server, error) {
	return s.server, nil
}

type runtimeSyncHosting struct {
	state    hosting.RuntimeState
	stateErr error
}

func (h *runtimeSyncHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
	return hosting.Container{}, nil
}

func (h *runtimeSyncHosting) Start(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *runtimeSyncHosting) Stop(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *runtimeSyncHosting) Delete(context.Context, entities.Claw, entities.Server, bool) error {
	return nil
}

func (h *runtimeSyncHosting) State(context.Context, entities.Claw, entities.Server) (hosting.RuntimeState, error) {
	return h.state, h.stateErr
}

func (h *runtimeSyncHosting) ApprovePairing(context.Context, entities.Claw, entities.Server, string, string) error {
	return nil
}

func (h *runtimeSyncHosting) Connect(context.Context, entities.Claw, entities.Server, string, []byte) error {
	return nil
}

func (h *runtimeSyncHosting) ConfigArchive(context.Context, entities.Claw, entities.Server, bool) (io.ReadCloser, error) {
	return nil, nil
}

func (h *runtimeSyncHosting) RestoreConfigArchive(context.Context, entities.Claw, entities.Server, io.Reader) error {
	return nil
}

type runtimeSyncChannelStorage struct{}

func (s *runtimeSyncChannelStorage) GetByIDs(context.Context, []uuid.UUID, uuid.UUID) ([]entities.Channel, error) {
	return nil, nil
}

type runtimeSyncUserStorage struct{}

func (s *runtimeSyncUserStorage) GetByID(context.Context, uuid.UUID) (entities.User, error) {
	return entities.User{}, nil
}

func (s *runtimeSyncUserStorage) UpdateOpenRouterKey(context.Context, uuid.UUID, entities.OpenRouterKey) error {
	return nil
}

type runtimeSyncKeys struct{}

func (k *runtimeSyncKeys) Create(context.Context, uuid.UUID, float64) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{}, nil
}

func (k *runtimeSyncKeys) ResolveModel(context.Context, string) (string, error) {
	return "", nil
}

func TestSyncRuntimeBatchMarksMissingRuntimeStopped(t *testing.T) {
	t.Parallel()

	serverID := uuid.New()
	clawID := uuid.New()
	userID := uuid.New()

	storage := &runtimeSyncClawStorage{
		candidates: []entities.Claw{{
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "runtime-record-1",
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:    entities.ClawDesiredStateRunning,
				ObservedState:   entities.ClawObservedStateRunning,
				LifecycleStatus: entities.ClawLifecycleStatusIdle,
			},
		}},
	}

	svc := NewClaw(
		storage,
		&runtimeSyncOperationStorage{},
		&runtimeSyncChannelStorage{},
		&runtimeSyncUserStorage{},
		&runtimeSyncServerStorage{server: entities.Server{ID: serverID, URL: "http://runtime.local", SecretKey: "secret"}},
		&runtimeSyncHosting{stateErr: hosting.ErrRuntimeNotFound},
		&runtimeSyncKeys{},
		t.TempDir(),
		GmailWatchConfig{},
	)

	if err := svc.syncRuntimeBatch(context.Background(), time.Second, 10, 1); err != nil {
		t.Fatalf("syncRuntimeBatch() error = %v", err)
	}

	if storage.runtimeUpdate.ContainerRecordID != "" {
		t.Fatalf("container record id = %q, want empty", storage.runtimeUpdate.ContainerRecordID)
	}

	if storage.runtimeUpdate.ServerID != uuid.Nil {
		t.Fatalf("server id = %s, want nil", storage.runtimeUpdate.ServerID)
	}

	if storage.runtimeUpdate.ObservedState != entities.ClawObservedStateStopped {
		t.Fatalf("observed state = %q, want %q", storage.runtimeUpdate.ObservedState, entities.ClawObservedStateStopped)
	}
}

func TestSyncRuntimeBatchUpdatesObservedStateFromHosting(t *testing.T) {
	t.Parallel()

	serverID := uuid.New()
	clawID := uuid.New()
	userID := uuid.New()

	storage := &runtimeSyncClawStorage{
		candidates: []entities.Claw{{
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "runtime-record-1",
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:    entities.ClawDesiredStateRunning,
				ObservedState:   entities.ClawObservedStateUnknown,
				LifecycleStatus: entities.ClawLifecycleStatusIdle,
			},
		}},
	}

	svc := NewClaw(
		storage,
		&runtimeSyncOperationStorage{},
		&runtimeSyncChannelStorage{},
		&runtimeSyncUserStorage{},
		&runtimeSyncServerStorage{server: entities.Server{ID: serverID, URL: "http://runtime.local", SecretKey: "secret"}},
		&runtimeSyncHosting{state: hosting.RuntimeState{
			RuntimeRecordID: "runtime-record-2",
			ObservedState:   "running",
			LastError:       "",
		}},
		&runtimeSyncKeys{},
		t.TempDir(),
		GmailWatchConfig{},
	)

	if err := svc.syncRuntimeBatch(context.Background(), time.Second, 10, 1); err != nil {
		t.Fatalf("syncRuntimeBatch() error = %v", err)
	}

	if storage.runtimeUpdate.ContainerRecordID != "runtime-record-2" {
		t.Fatalf("container record id = %q, want %q", storage.runtimeUpdate.ContainerRecordID, "runtime-record-2")
	}

	if storage.runtimeUpdate.ObservedState != entities.ClawObservedStateRunning {
		t.Fatalf("observed state = %q, want %q", storage.runtimeUpdate.ObservedState, entities.ClawObservedStateRunning)
	}
}
