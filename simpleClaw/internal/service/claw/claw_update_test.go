package claw

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/storages/claws"
	"simpleClaw/internal/service/claw/commands"
)

type updateRuntimeCall struct {
	ID     uuid.UUID
	Update entities.ClawRuntimeUpdate
}

type updateTestClawStorage struct {
	existing        entities.Claw
	updateCalled    bool
	updatedClaw     entities.Claw
	updatedChannels []uuid.UUID
	replaceChannels bool
	updateErr       error
	lifecycleUpdate claws.LifecycleUpdate
	lifecycleID     uuid.UUID
	deleteCalled    bool
	deletedID       uuid.UUID
	deletedUserID   uuid.UUID
	runtimeUpdated  updateRuntimeCall
}

func (s *updateTestClawStorage) Create(context.Context, entities.Claw, []uuid.UUID) error {
	return nil
}

func (s *updateTestClawStorage) GetByID(_ context.Context, _, _ uuid.UUID) (entities.Claw, error) {
	if s.existing.ID == uuid.Nil {
		return entities.Claw{}, sql.ErrNotFound
	}

	return s.existing, nil
}

func (s *updateTestClawStorage) GetBySystemID(_ context.Context, id uuid.UUID) (entities.Claw, error) {
	if s.existing.ID == uuid.Nil || s.existing.ID != id {
		return entities.Claw{}, sql.ErrNotFound
	}

	return s.existing, nil
}

func (s *updateTestClawStorage) GetNextReconcilePending(context.Context) (entities.Claw, error) {
	if s.existing.ID == uuid.Nil || s.existing.LifecycleStatus != entities.ClawLifecycleStatusReconcilePending {
		return entities.Claw{}, sql.ErrNotFound
	}

	return s.existing, nil
}

func (s *updateTestClawStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return nil, nil
}

func (s *updateTestClawStorage) ListRuntimeSyncCandidates(context.Context, int) ([]entities.Claw, error) {
	if s.existing.ID == uuid.Nil {
		return nil, nil
	}

	return []entities.Claw{s.existing}, nil
}

func (s *updateTestClawStorage) CountOccupiedByServer(context.Context) (map[uuid.UUID]int, error) {
	return map[uuid.UUID]int{}, nil
}

func (s *updateTestClawStorage) Update(
	_ context.Context,
	cl entities.Claw,
	channelIDs []uuid.UUID,
	replaceChannels bool,
) error {
	s.updateCalled = true
	s.updatedClaw = cl
	s.updatedChannels = channelIDs
	s.replaceChannels = replaceChannels

	return s.updateErr
}

func (s *updateTestClawStorage) UpdateLifecycle(_ context.Context, clID uuid.UUID, update claws.LifecycleUpdate) error {
	s.lifecycleID = clID
	s.lifecycleUpdate = update

	return nil
}

func (s *updateTestClawStorage) Delete(_ context.Context, id uuid.UUID, userID uuid.UUID) error {
	s.deleteCalled = true
	s.deletedID = id
	s.deletedUserID = userID

	return nil
}

func (s *updateTestClawStorage) UpdateRuntime(
	_ context.Context,
	clID uuid.UUID,
	update entities.ClawRuntimeUpdate,
) error {
	s.runtimeUpdated.ID = clID
	s.runtimeUpdated.Update = update

	return nil
}

type updateTestChannelStorage struct{}

type updateTestOperationStorage struct {
	active entities.ClawLifecycleOperation
	create entities.ClawLifecycleOperation
}

func (s *updateTestOperationStorage) Create(_ context.Context, op entities.ClawLifecycleOperation) error {
	s.create = op

	return nil
}

func (s *updateTestOperationStorage) GetActiveByClawID(context.Context, uuid.UUID) (entities.ClawLifecycleOperation, error) {
	if s.active.ID == uuid.Nil {
		return entities.ClawLifecycleOperation{}, sql.ErrNotFound
	}

	return s.active, nil
}

func (s *updateTestOperationStorage) LockNextRunnable(context.Context, time.Time) (entities.ClawLifecycleOperation, error) {
	return entities.ClawLifecycleOperation{}, sql.ErrNotFound
}

func (s *updateTestOperationStorage) Update(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

func (s *updateTestChannelStorage) GetByIDs(context.Context, []uuid.UUID, uuid.UUID) ([]entities.Channel, error) {
	return nil, nil
}

type updateTestUserStorage struct{}

func (s *updateTestUserStorage) GetByID(context.Context, uuid.UUID) (entities.User, error) {
	return entities.User{}, nil
}

func (s *updateTestUserStorage) UpdateOpenRouterKey(context.Context, uuid.UUID, entities.OpenRouterKey) error {
	return nil
}

type updateTestServerStorage struct {
	server  entities.Server
	servers []entities.Server
}

func (s *updateTestServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return s.server, nil
}

func (s *updateTestServerStorage) GetAll(context.Context) ([]entities.Server, error) {
	if len(s.servers) > 0 {
		return append([]entities.Server(nil), s.servers...), nil
	}

	if s.server.ID == uuid.Nil {
		return nil, nil
	}

	return []entities.Server{s.server}, nil
}

func (s *updateTestServerStorage) GetByID(_ context.Context, id uuid.UUID) (entities.Server, error) {
	if s.server.ID == id {
		return s.server, nil
	}

	for _, srv := range s.servers {
		if srv.ID == id {
			return srv, nil
		}
	}

	return entities.Server{}, nil
}

type updateTestKeys struct {
	resolveModelResult string
	resolveModelCalls  int
}

func (k *updateTestKeys) Create(context.Context, uuid.UUID, float64) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{}, nil
}

func (k *updateTestKeys) ResolveModel(context.Context, string) (string, error) {
	k.resolveModelCalls++

	return k.resolveModelResult, nil
}

type updateTestHosting struct {
	createContainer hosting.Container
	createErr       error
	updateErr       error
	startErr        error
	deleteErr       error
	restoreErr      error
	updateCalls     int
	createCalls     int
	startCalls      int
	deleteCalls     int
	restoreCalls    int
}

func (h *updateTestHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
	h.createCalls++

	if h.createErr != nil {
		return hosting.Container{}, h.createErr
	}

	return h.createContainer, nil
}

func (h *updateTestHosting) Start(context.Context, entities.Claw, entities.Server) error {
	h.startCalls++

	return h.startErr
}

func (h *updateTestHosting) Stop(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *updateTestHosting) Delete(context.Context, entities.Claw, entities.Server, bool) error {
	h.deleteCalls++

	return h.deleteErr
}

func (h *updateTestHosting) State(context.Context, entities.Claw, entities.Server) (hosting.RuntimeState, error) {
	return hosting.RuntimeState{}, nil
}

func (h *updateTestHosting) ApprovePairing(context.Context, entities.Claw, entities.Server, string, string) error {
	return nil
}

func (h *updateTestHosting) Connect(context.Context, entities.Claw, entities.Server, string, []byte) error {
	return nil
}

func (h *updateTestHosting) ConfigArchive(context.Context, entities.Claw, entities.Server, bool) (io.ReadCloser, error) {
	return nil, nil
}

func (h *updateTestHosting) RestoreConfigArchive(context.Context, entities.Claw, entities.Server, io.Reader) error {
	h.restoreCalls++

	return h.restoreErr
}

func strPtr(v string) *string {
	return &v
}

func TestServiceUpdate_AllowsPartialWithoutNameAndModel(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	keys := &updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		keys,
		t.TempDir(),
		GmailWatchConfig{},
	)

	updated, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	if !storage.updateCalled {
		t.Fatalf("expected storage update to be called")
	}

	if storage.replaceChannels {
		t.Fatalf("did not expect channel replacement flag for omitted channelIds")
	}

	if updated.Name != "existing-name" {
		t.Fatalf("expected name to stay unchanged, got %q", updated.Name)
	}

	if keys.resolveModelCalls != 0 {
		t.Fatalf("expected model resolver to not be called, got %d calls", keys.resolveModelCalls)
	}
}

func TestServiceUpdate_EmptyChannelIDsPreserveClearIntent(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	keys := &updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		keys,
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID:     userID,
		ClawID:     clawID,
		Name:       strPtr("updated-name"),
		Model:      strPtr("openai/gpt-4.1-mini"),
		ChannelIDs: []uuid.UUID{},
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	if storage.updatedChannels == nil {
		t.Fatalf("expected explicit empty channel list to stay non-nil")
	}

	if len(storage.updatedChannels) != 0 {
		t.Fatalf("expected no channel ids, got %d", len(storage.updatedChannels))
	}

	if !storage.replaceChannels {
		t.Fatalf("expected channel replacement flag for explicit empty channelIds")
	}
}

func TestServiceUpdate_RejectsEmptyNameWhenProvided(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Name:   strPtr("   "),
	})
	if err == nil {
		t.Fatalf("expected validation error for empty name")
	}
}

func TestServiceUpdate_RejectsEmptyModelWhenProvided(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Model:  strPtr(" "),
	})
	if err == nil {
		t.Fatalf("expected validation error for empty model")
	}
}

func TestServiceUpdate_SkipsHostingForRuntimeBoundClaw(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:          clawID,
		UserID:      userID,
		Name:        "existing-name",
		ServerID:    uuid.New(),
		ContainerID: "container-id",
		Config:      cfg,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}}

	hostingStub := &updateTestHosting{updateErr: errors.New("hosting failed")}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Name:   strPtr("updated"),
	})
	if err != nil {
		t.Fatalf("expected update to succeed without hosting call, got %v", err)
	}

	if hostingStub.updateCalls != 0 {
		t.Fatalf("expected hosting update to be skipped, got %d calls", hostingStub.updateCalls)
	}

	if !storage.updateCalled {
		t.Fatalf("expected db update to be called")
	}
}

func TestServiceUpdate_DBErrorIsStageAware(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{
		existing: entities.Claw{
			ID:        clawID,
			UserID:    userID,
			Name:      "existing-name",
			Config:    cfg,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		updateErr: sql.ErrConflict,
	}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Name:   strPtr("updated"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	var stageErr *UpdateStageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("expected UpdateStageError, got %v", err)
	}

	if stageErr.Stage != updateStageDBUpdate {
		t.Fatalf("unexpected stage: %s", stageErr.Stage)
	}
}

func TestServiceUpdate_StoppedClawSkipsHostingAndArchiveSync(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	archiveDir := t.TempDir()
	hostingStub := &updateTestHosting{}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		archiveDir,
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Name:   strPtr("updated"),
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	if hostingStub.updateCalls != 0 {
		t.Fatalf("expected hosting update to be skipped, got %d calls", hostingStub.updateCalls)
	}

	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	if _, err := os.Stat(archivePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no archive to be written, stat err=%v", err)
	}
}

func TestServiceStart_WithoutRuntimeQueuesOperation(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:                 clawID,
		UserID:             userID,
		Name:               "existing-name",
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             cfg,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}}

	hostingStub := &updateTestHosting{}
	operations := &updateTestOperationStorage{}

	svc := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	cl, err := svc.Start(context.Background(), commands.StartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("start returned error: %v", err)
	}

	if hostingStub.restoreCalls != 0 || hostingStub.createCalls != 0 || hostingStub.startCalls != 0 {
		t.Fatalf(
			"expected no hosting lifecycle calls, got restore=%d create=%d start=%d",
			hostingStub.restoreCalls,
			hostingStub.createCalls,
			hostingStub.startCalls,
		)
	}

	if operations.create.Type != entities.ClawLifecycleOperationTypeStart {
		t.Fatalf("operation type = %q, want %q", operations.create.Type, entities.ClawLifecycleOperationTypeStart)
	}

	if storage.lifecycleID != clawID {
		t.Fatalf("lifecycle update claw id = %s, want %s", storage.lifecycleID, clawID)
	}

	if storage.lifecycleUpdate.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("desired state = %q, want %q", storage.lifecycleUpdate.DesiredState, entities.ClawDesiredStateRunning)
	}

	if storage.lifecycleUpdate.LifecycleStatus != entities.ClawLifecycleStatusStartPending {
		t.Fatalf("lifecycle status = %q, want %q", storage.lifecycleUpdate.LifecycleStatus, entities.ClawLifecycleStatusStartPending)
	}

	if storage.lifecycleUpdate.CurrentOperationID == nil {
		t.Fatal("expected current operation id to be set")
	}

	if cl.CurrentOperationID == nil {
		t.Fatal("expected returned claw current operation id")
	}

	if cl.ObservedState != entities.ClawObservedStateUnknown {
		t.Fatalf("observed state = %q, want %q", cl.ObservedState, entities.ClawObservedStateUnknown)
	}
}

func TestServiceStart_RejectsWhenActiveOperationExists(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:                 clawID,
		UserID:             userID,
		Name:               "existing-name",
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             cfg,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{
			active: entities.ClawLifecycleOperation{
				ID:     uuid.New(),
				ClawID: clawID,
				Type:   entities.ClawLifecycleOperationTypeStop,
			},
		},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Start(context.Background(), commands.StartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if !errors.Is(err, ErrLifecycleOperationInProgress) {
		t.Fatalf("start error = %v, want ErrLifecycleOperationInProgress", err)
	}
}

func TestServiceRestart_QueuesOperation(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:                 clawID,
		UserID:             userID,
		Name:               "existing-name",
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             cfg,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}}

	operations := &updateTestOperationStorage{}

	svc := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	cl, err := svc.Restart(context.Background(), commands.RestartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("restart returned error: %v", err)
	}

	if operations.create.Type != entities.ClawLifecycleOperationTypeRestart {
		t.Fatalf("operation type = %q, want %q", operations.create.Type, entities.ClawLifecycleOperationTypeRestart)
	}

	if storage.lifecycleUpdate.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("desired state = %q, want %q", storage.lifecycleUpdate.DesiredState, entities.ClawDesiredStateRunning)
	}

	if storage.lifecycleUpdate.LifecycleStatus != entities.ClawLifecycleStatusRestartPending {
		t.Fatalf("lifecycle status = %q, want %q", storage.lifecycleUpdate.LifecycleStatus, entities.ClawLifecycleStatusRestartPending)
	}

	if cl.CurrentOperationID == nil {
		t.Fatal("expected returned claw current operation id")
	}
}

func TestServiceRestart_RejectsWhenActiveOperationExists(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:                 clawID,
		UserID:             userID,
		Name:               "existing-name",
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             cfg,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}}

	svc := NewClaw(
		storage,
		&updateTestOperationStorage{
			active: entities.ClawLifecycleOperation{
				ID:     uuid.New(),
				ClawID: clawID,
				Type:   entities.ClawLifecycleOperationTypeStop,
			},
		},
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Restart(context.Background(), commands.RestartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if !errors.Is(err, ErrLifecycleOperationInProgress) {
		t.Fatalf("restart error = %v, want ErrLifecycleOperationInProgress", err)
	}
}

func TestServiceRestart_StoppedClawTargetsRunning(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ClawLifecycleState: entities.ClawLifecycleState{
			DesiredState:    entities.ClawDesiredStateStopped,
			ObservedState:   entities.ClawObservedStateStopped,
			LifecycleStatus: entities.ClawLifecycleStatusIdle,
		},
	}}

	operations := &updateTestOperationStorage{}

	svc := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	cl, err := svc.Restart(context.Background(), commands.RestartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("restart returned error: %v", err)
	}

	if cl.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("desired state = %q, want %q", cl.DesiredState, entities.ClawDesiredStateRunning)
	}

	if storage.lifecycleUpdate.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("lifecycle desired state = %q, want %q", storage.lifecycleUpdate.DesiredState, entities.ClawDesiredStateRunning)
	}
}

func TestServiceDelete_WithoutRuntimeQueuesOperation(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:                 clawID,
		UserID:             userID,
		Name:               "existing-name",
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             cfg,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}}

	hostingStub := &updateTestHosting{}
	operations := &updateTestOperationStorage{}

	svc := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	cl, err := svc.Delete(context.Background(), commands.DeleteClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("delete returned error: %v", err)
	}

	if hostingStub.deleteCalls != 0 {
		t.Fatalf("expected hosting delete to be skipped, got %d", hostingStub.deleteCalls)
	}

	if storage.deleteCalled {
		t.Fatalf("expected db delete to be skipped")
	}

	if operations.create.Type != entities.ClawLifecycleOperationTypeDelete {
		t.Fatalf("operation type = %q, want %q", operations.create.Type, entities.ClawLifecycleOperationTypeDelete)
	}

	if cl.LifecycleStatus != entities.ClawLifecycleStatusDeletePending {
		t.Fatalf("lifecycle status = %q, want %q", cl.LifecycleStatus, entities.ClawLifecycleStatusDeletePending)
	}
}

func TestServiceDelete_WithRuntimeQueuesOperation(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:          clawID,
		UserID:      userID,
		Name:        "existing-name",
		ServerID:    serverID,
		ContainerID: "container-id",
		Config:      cfg,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}}

	hostingStub := &updateTestHosting{}
	operations := &updateTestOperationStorage{}

	svc := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{server: entities.Server{ID: serverID}},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	cl, err := svc.Delete(context.Background(), commands.DeleteClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("delete returned error: %v", err)
	}

	if hostingStub.deleteCalls != 0 {
		t.Fatalf("expected hosting delete to be skipped, got %d", hostingStub.deleteCalls)
	}

	if storage.deleteCalled {
		t.Fatalf("expected db delete to be skipped")
	}

	if cl.DesiredState != entities.ClawDesiredStateDeleted {
		t.Fatalf("desired state = %q, want %q", cl.DesiredState, entities.ClawDesiredStateDeleted)
	}
}
