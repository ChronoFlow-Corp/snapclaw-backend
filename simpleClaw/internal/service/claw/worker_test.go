package claw

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
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
)

type workerTestClawStorage struct {
	claws map[uuid.UUID]entities.Claw
}

func (s *workerTestClawStorage) Create(context.Context, entities.Claw, []uuid.UUID) error {
	return nil
}

func (s *workerTestClawStorage) GetBySystemID(_ context.Context, id uuid.UUID) (entities.Claw, error) {
	cl, ok := s.claws[id]
	if !ok {
		return entities.Claw{}, sql.ErrNotFound
	}

	return cl, nil
}

func (s *workerTestClawStorage) GetByID(_ context.Context, id, _ uuid.UUID) (entities.Claw, error) {
	return s.GetBySystemID(context.Background(), id)
}

func (s *workerTestClawStorage) GetNextReconcilePending(context.Context) (entities.Claw, error) {
	var (
		found entities.Claw
		ok    bool
	)

	for _, cl := range s.claws {
		if cl.LifecycleStatus != entities.ClawLifecycleStatusReconcilePending {
			continue
		}
		if !ok || cl.UpdatedAt.Before(found.UpdatedAt) {
			found = cl
			ok = true
		}
	}

	if !ok {
		return entities.Claw{}, sql.ErrNotFound
	}

	return found, nil
}

func (s *workerTestClawStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return nil, nil
}

func (s *workerTestClawStorage) ListRuntimeSyncCandidates(context.Context, int) ([]entities.Claw, error) {
	res := make([]entities.Claw, 0, len(s.claws))
	for _, cl := range s.claws {
		res = append(res, cl)
	}

	return res, nil
}

func (s *workerTestClawStorage) CountOccupiedByServer(context.Context) (map[uuid.UUID]int, error) {
	return map[uuid.UUID]int{}, nil
}

func (s *workerTestClawStorage) UpdateLifecycle(_ context.Context, clID uuid.UUID, update claws.LifecycleUpdate) error {
	cl, ok := s.claws[clID]
	if !ok {
		return sql.ErrNotFound
	}

	cl.DesiredState = update.DesiredState
	cl.ObservedState = update.ObservedState
	cl.LifecycleStatus = update.LifecycleStatus
	cl.CurrentOperationID = update.CurrentOperationID
	cl.LastError = update.LastLifecycleError
	if update.OnboardingComplete != nil {
		cl.OnboardingComplete = *update.OnboardingComplete
	}
	s.claws[clID] = cl

	return nil
}

func (s *workerTestClawStorage) Update(context.Context, entities.Claw, []uuid.UUID, bool) error {
	return nil
}

func (s *workerTestClawStorage) Delete(_ context.Context, id, _ uuid.UUID) error {
	delete(s.claws, id)

	return nil
}

func (s *workerTestClawStorage) UpdateRuntime(_ context.Context, clID uuid.UUID, update entities.ClawRuntimeUpdate) error {
	cl, ok := s.claws[clID]
	if !ok {
		return sql.ErrNotFound
	}

	if update.ServerID != uuid.Nil {
		cl.ServerID = update.ServerID
	}

	cl.ContainerID = update.ContainerRecordID
	cl.DesiredState = update.DesiredState
	cl.ObservedState = update.ObservedState
	cl.LifecycleStatus = update.LifecycleStatus
	cl.LastError = update.LastLifecycleError
	s.claws[clID] = cl

	return nil
}

type workerTestOperationStorage struct {
	operations map[uuid.UUID]entities.ClawLifecycleOperation
	queue      []uuid.UUID
}

func (s *workerTestOperationStorage) Create(context.Context, entities.ClawLifecycleOperation) error {
	return nil
}

func (s *workerTestOperationStorage) GetActiveByClawID(_ context.Context, clawID uuid.UUID) (entities.ClawLifecycleOperation, error) {
	for _, op := range s.operations {
		if op.ClawID != clawID {
			continue
		}

		switch op.Status {
		case entities.ClawLifecycleOperationStatusPending,
			entities.ClawLifecycleOperationStatusRunning,
			entities.ClawLifecycleOperationStatusRetryScheduled:
			return op, nil
		}
	}

	return entities.ClawLifecycleOperation{}, sql.ErrNotFound
}

func (s *workerTestOperationStorage) LockNextRunnable(_ context.Context, now time.Time) (entities.ClawLifecycleOperation, error) {
	for _, id := range s.queue {
		op := s.operations[id]
		if op.Status == entities.ClawLifecycleOperationStatusPending ||
			(op.Status == entities.ClawLifecycleOperationStatusRetryScheduled && op.NextRetryAt != nil && !op.NextRetryAt.After(now)) {
			op.Status = entities.ClawLifecycleOperationStatusRunning
			op.UpdatedAt = now
			op.NextRetryAt = nil
			s.operations[id] = op

			return op, nil
		}
	}

	return entities.ClawLifecycleOperation{}, sql.ErrNotFound
}

func (s *workerTestOperationStorage) Update(_ context.Context, op entities.ClawLifecycleOperation) error {
	s.operations[op.ID] = op

	return nil
}

type workerTestChannelStorage struct{}

func (s *workerTestChannelStorage) GetByIDs(context.Context, []uuid.UUID, uuid.UUID) ([]entities.Channel, error) {
	return nil, nil
}

type workerTestUserStorage struct{}

func (s *workerTestUserStorage) GetByID(context.Context, uuid.UUID) (entities.User, error) {
	return entities.User{}, nil
}

func (s *workerTestUserStorage) UpdateOpenRouterKey(context.Context, uuid.UUID, entities.OpenRouterKey) error {
	return nil
}

type workerTestServerStorage struct {
	server entities.Server
}

func (s *workerTestServerStorage) GetAll(context.Context) ([]entities.Server, error) {
	return []entities.Server{s.server}, nil
}

func (s *workerTestServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return s.server, nil
}

func (s *workerTestServerStorage) GetByID(context.Context, uuid.UUID) (entities.Server, error) {
	return s.server, nil
}

type workerTestKeys struct{}

func (k *workerTestKeys) Create(context.Context, uuid.UUID, float64) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{}, nil
}

func (k *workerTestKeys) ResolveModel(context.Context, string) (string, error) {
	return "", nil
}

type workerTestHosting struct {
	createContainer hosting.Container
	createErr       error
	startErr        error
	deleteErr       error
	configArchive   []byte
	configArchiveErr error
	restoreErr      error
	state           hosting.RuntimeState
	stateErr        error
	createCalls     int
	startCalls      int
	deleteCalls     int
	configCalls     int
	restoreCalls    int
	stateCalls      int
	callOrder       []string
	restoreBody     []byte
}

func (h *workerTestHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
	h.createCalls++
	h.callOrder = append(h.callOrder, "create")
	if h.createErr != nil {
		return hosting.Container{}, h.createErr
	}

	return h.createContainer, nil
}

func (h *workerTestHosting) Start(context.Context, entities.Claw, entities.Server) error {
	h.startCalls++
	h.callOrder = append(h.callOrder, "start")
	return h.startErr
}

func (h *workerTestHosting) Stop(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *workerTestHosting) Delete(context.Context, entities.Claw, entities.Server, bool) error {
	h.deleteCalls++
	h.callOrder = append(h.callOrder, "delete")
	return h.deleteErr
}

func (h *workerTestHosting) State(context.Context, entities.Claw, entities.Server) (hosting.RuntimeState, error) {
	h.stateCalls++
	return h.state, h.stateErr
}

func (h *workerTestHosting) ApprovePairing(context.Context, entities.Claw, entities.Server, string, string) error {
	return nil
}

func (h *workerTestHosting) Connect(context.Context, entities.Claw, entities.Server, string, []byte) error {
	return nil
}

func (h *workerTestHosting) ConfigArchive(context.Context, entities.Claw, entities.Server, bool) (io.ReadCloser, error) {
	h.configCalls++
	h.callOrder = append(h.callOrder, "archive")
	if h.configArchiveErr != nil {
		return nil, h.configArchiveErr
	}

	return io.NopCloser(bytes.NewReader(h.configArchive)), nil
}

func (h *workerTestHosting) RestoreConfigArchive(_ context.Context, _ entities.Claw, _ entities.Server, body io.Reader) error {
	h.restoreCalls++
	h.callOrder = append(h.callOrder, "restore")
	if h.restoreErr != nil {
		return h.restoreErr
	}

	if body != nil {
		payload, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		h.restoreBody = append([]byte(nil), payload...)
	}

	return nil
}

func TestProcessNextOperationStartTransitionsClawToRunning(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             uuid.New(),
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{createContainer: hosting.Container{ID: "ctr-1", ServerID: serverID}}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		"",
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	cl := clawStorage.claws[clawID]
	if cl.ServerID != serverID {
		t.Fatalf("server id = %s, want %s", cl.ServerID, serverID)
	}
	if cl.ContainerID != "ctr-1" {
		t.Fatalf("container id = %q, want ctr-1", cl.ContainerID)
	}
	if cl.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("desired state = %q, want %q", cl.DesiredState, entities.ClawDesiredStateRunning)
	}
	if cl.ObservedState != entities.ClawObservedStateRunning {
		t.Fatalf("observed state = %q, want %q", cl.ObservedState, entities.ClawObservedStateRunning)
	}
	if cl.LifecycleStatus != entities.ClawLifecycleStatusIdle {
		t.Fatalf("lifecycle status = %q, want %q", cl.LifecycleStatus, entities.ClawLifecycleStatusIdle)
	}
	if cl.CurrentOperationID != nil {
		t.Fatalf("current operation id = %v, want nil", *cl.CurrentOperationID)
	}

	op := operationStorage.operations[opID]
	if op.Status != entities.ClawLifecycleOperationStatusSucceeded {
		t.Fatalf("operation status = %q, want %q", op.Status, entities.ClawLifecycleOperationStatusSucceeded)
	}
	if hostingStub.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", hostingStub.createCalls)
	}
	if hostingStub.startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", hostingStub.startCalls)
	}
}

func TestProcessNextOperationStopTreatsMissingRuntimeAsSuccess(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()
	archiveDir := t.TempDir()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "ctr-1",
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateStopped,
				ObservedState:      entities.ClawObservedStateRunning,
				LifecycleStatus:    entities.ClawLifecycleStatusStopPending,
				CurrentOperationID: &opID,
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStop,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{
		deleteErr:      errors.New("delete failed"),
		configArchive:  []byte("archive-payload"),
		stateErr:       hosting.ErrRuntimeNotFound,
	}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		archiveDir,
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	cl := clawStorage.claws[clawID]
	if cl.ContainerID != "" {
		t.Fatalf("container id = %q, want empty", cl.ContainerID)
	}
	if cl.DesiredState != entities.ClawDesiredStateStopped {
		t.Fatalf("desired state = %q, want %q", cl.DesiredState, entities.ClawDesiredStateStopped)
	}
	if cl.ObservedState != entities.ClawObservedStateStopped {
		t.Fatalf("observed state = %q, want %q", cl.ObservedState, entities.ClawObservedStateStopped)
	}
	if cl.LifecycleStatus != entities.ClawLifecycleStatusIdle {
		t.Fatalf("lifecycle status = %q, want %q", cl.LifecycleStatus, entities.ClawLifecycleStatusIdle)
	}
	if cl.CurrentOperationID != nil {
		t.Fatalf("current operation id = %v, want nil", *cl.CurrentOperationID)
	}

	op := operationStorage.operations[opID]
	if op.Status != entities.ClawLifecycleOperationStatusSucceeded {
		t.Fatalf("operation status = %q, want %q", op.Status, entities.ClawLifecycleOperationStatusSucceeded)
	}
	if hostingStub.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", hostingStub.deleteCalls)
	}
	if hostingStub.stateCalls != 1 {
		t.Fatalf("state calls = %d, want 1", hostingStub.stateCalls)
	}
}

func TestProcessNextOperationStopArchivesRuntimeBeforeDelete(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()
	archiveDir := t.TempDir()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "ctr-1",
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateStopped,
				ObservedState:      entities.ClawObservedStateRunning,
				LifecycleStatus:    entities.ClawLifecycleStatusStopPending,
				CurrentOperationID: &opID,
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStop,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{configArchive: []byte("archive-payload")}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		archiveDir,
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if hostingStub.configCalls != 1 {
		t.Fatalf("archive calls = %d, want 1", hostingStub.configCalls)
	}
	if hostingStub.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", hostingStub.deleteCalls)
	}
	if len(hostingStub.callOrder) < 2 || hostingStub.callOrder[0] != "archive" || hostingStub.callOrder[1] != "delete" {
		t.Fatalf("call order = %v, want archive before delete", hostingStub.callOrder)
	}

	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	got, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", archivePath, err)
	}
	if !bytes.Equal(got, []byte("archive-payload")) {
		t.Fatalf("saved archive = %q, want %q", got, "archive-payload")
	}
}

func TestProcessNextOperationStopDoesNotDeleteRuntimeWhenArchiveFails(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "ctr-1",
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateStopped,
				ObservedState:      entities.ClawObservedStateRunning,
				LifecycleStatus:    entities.ClawLifecycleStatusStopPending,
				CurrentOperationID: &opID,
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStop,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{configArchiveErr: errors.New("archive failed")}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		t.TempDir(),
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if hostingStub.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", hostingStub.deleteCalls)
	}

	cl := clawStorage.claws[clawID]
	if cl.ContainerID != "ctr-1" {
		t.Fatalf("container id = %q, want runtime untouched", cl.ContainerID)
	}
	if cl.LifecycleStatus != entities.ClawLifecycleStatusReconcilePending {
		t.Fatalf("lifecycle status = %q, want %q", cl.LifecycleStatus, entities.ClawLifecycleStatusReconcilePending)
	}
}

func TestProcessNextOperationStopArchiveFailureMarksOnboardingIncomplete(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             userID,
			ServerID:           serverID,
			ContainerID:        "ctr-1",
			OnboardingComplete: true,
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateStopped,
				ObservedState:      entities.ClawObservedStateRunning,
				LifecycleStatus:    entities.ClawLifecycleStatusStopPending,
				CurrentOperationID: &opID,
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStop,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{configArchiveErr: errors.New("archive failed")}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		t.TempDir(),
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if clawStorage.claws[clawID].OnboardingComplete {
		t.Fatal("expected onboarding to be reset after archive failure")
	}
}

func TestProcessNextOperationStartSchedulesRetryWhenOutcomeUnknown(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             uuid.New(),
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{createErr: errors.New("ensure failed")}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		"",
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	cl := clawStorage.claws[clawID]
	if cl.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("desired state = %q, want %q", cl.DesiredState, entities.ClawDesiredStateRunning)
	}
	if cl.LifecycleStatus != entities.ClawLifecycleStatusReconcilePending {
		t.Fatalf("lifecycle status = %q, want %q", cl.LifecycleStatus, entities.ClawLifecycleStatusReconcilePending)
	}
	if cl.CurrentOperationID == nil || *cl.CurrentOperationID != opID {
		t.Fatalf("current operation id = %v, want %s", cl.CurrentOperationID, opID)
	}
	if cl.LastError != "ensure failed" {
		t.Fatalf("last error = %q, want %q", cl.LastError, "ensure failed")
	}

	op := operationStorage.operations[opID]
	if op.Status != entities.ClawLifecycleOperationStatusRetryScheduled {
		t.Fatalf("operation status = %q, want %q", op.Status, entities.ClawLifecycleOperationStatusRetryScheduled)
	}
	if op.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", op.Attempt)
	}
	if op.NextRetryAt == nil {
		t.Fatal("next retry at = nil, want value")
	}
	if op.LastError != "ensure failed" {
		t.Fatalf("operation last error = %q, want %q", op.LastError, "ensure failed")
	}
}

func TestProcessNextOperationRestartRecreatesRuntime(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()
	archiveDir := t.TempDir()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "ctr-old",
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateRunning,
				ObservedState:      entities.ClawObservedStateRunning,
				LifecycleStatus:    entities.ClawLifecycleStatusRestartPending,
				CurrentOperationID: &opID,
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeRestart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{
		createContainer: hosting.Container{ID: "ctr-new", ServerID: serverID},
		configArchive:   []byte("archive-payload"),
	}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		archiveDir,
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	cl := clawStorage.claws[clawID]
	if cl.ContainerID != "ctr-new" {
		t.Fatalf("container id = %q, want ctr-new", cl.ContainerID)
	}
	if cl.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("desired state = %q, want %q", cl.DesiredState, entities.ClawDesiredStateRunning)
	}
	if cl.ObservedState != entities.ClawObservedStateRunning {
		t.Fatalf("observed state = %q, want %q", cl.ObservedState, entities.ClawObservedStateRunning)
	}
	if cl.LifecycleStatus != entities.ClawLifecycleStatusIdle {
		t.Fatalf("lifecycle status = %q, want %q", cl.LifecycleStatus, entities.ClawLifecycleStatusIdle)
	}

	op := operationStorage.operations[opID]
	if op.Status != entities.ClawLifecycleOperationStatusSucceeded {
		t.Fatalf("operation status = %q, want %q", op.Status, entities.ClawLifecycleOperationStatusSucceeded)
	}
	if hostingStub.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", hostingStub.deleteCalls)
	}
	if hostingStub.configCalls != 1 {
		t.Fatalf("archive calls = %d, want 1", hostingStub.configCalls)
	}
	if hostingStub.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", hostingStub.createCalls)
	}
	if hostingStub.startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", hostingStub.startCalls)
	}
	if len(hostingStub.callOrder) < 4 || hostingStub.callOrder[0] != "archive" || hostingStub.callOrder[1] != "delete" {
		t.Fatalf("call order prefix = %v, want archive then delete", hostingStub.callOrder)
	}

	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	got, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", archivePath, err)
	}
	if !bytes.Equal(got, []byte("archive-payload")) {
		t.Fatalf("saved archive = %q, want %q", got, "archive-payload")
	}
}

func TestProcessNextOperationRestartWithoutRuntimeStartsDirectly(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:     clawID,
			UserID: uuid.New(),
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateRunning,
				ObservedState:      entities.ClawObservedStateStopped,
				LifecycleStatus:    entities.ClawLifecycleStatusRestartPending,
				CurrentOperationID: &opID,
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeRestart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{createContainer: hosting.Container{ID: "ctr-new", ServerID: serverID}}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		"",
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	cl := clawStorage.claws[clawID]
	if cl.ContainerID != "ctr-new" {
		t.Fatalf("container id = %q, want ctr-new", cl.ContainerID)
	}
	if cl.LifecycleStatus != entities.ClawLifecycleStatusIdle {
		t.Fatalf("lifecycle status = %q, want %q", cl.LifecycleStatus, entities.ClawLifecycleStatusIdle)
	}
	if hostingStub.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", hostingStub.deleteCalls)
	}
	if hostingStub.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", hostingStub.createCalls)
	}
	if hostingStub.startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", hostingStub.startCalls)
	}
}

func TestProcessNextOperationRestartDoesNotDeleteRuntimeWhenArchiveFails(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "ctr-old",
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateRunning,
				ObservedState:      entities.ClawObservedStateRunning,
				LifecycleStatus:    entities.ClawLifecycleStatusRestartPending,
				CurrentOperationID: &opID,
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeRestart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{configArchiveErr: errors.New("archive failed")}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		t.TempDir(),
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if hostingStub.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", hostingStub.deleteCalls)
	}
	if hostingStub.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", hostingStub.createCalls)
	}
	if hostingStub.startCalls != 0 {
		t.Fatalf("start calls = %d, want 0", hostingStub.startCalls)
	}

	cl := clawStorage.claws[clawID]
	if cl.ContainerID != "ctr-old" {
		t.Fatalf("container id = %q, want runtime untouched", cl.ContainerID)
	}
	if cl.LifecycleStatus != entities.ClawLifecycleStatusReconcilePending {
		t.Fatalf("lifecycle status = %q, want %q", cl.LifecycleStatus, entities.ClawLifecycleStatusReconcilePending)
	}
}

func TestProcessNextOperationRestartArchiveFailureMarksOnboardingIncomplete(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             userID,
			ServerID:           serverID,
			ContainerID:        "ctr-old",
			OnboardingComplete: true,
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateRunning,
				ObservedState:      entities.ClawObservedStateRunning,
				LifecycleStatus:    entities.ClawLifecycleStatusRestartPending,
				CurrentOperationID: &opID,
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeRestart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{configArchiveErr: errors.New("archive failed")}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		t.TempDir(),
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if clawStorage.claws[clawID].OnboardingComplete {
		t.Fatal("expected onboarding to be reset after archive failure")
	}
}

func TestProcessNextOperationStartRestoresSavedArchiveBeforeEnsure(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()
	archiveDir := t.TempDir()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Meta = &entities.ConfigMeta{LastTouchedVersion: "db-truth"}

	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	writeWorkerArchiveFixture(t, archivePath, clawID.String(), map[string]string{
		"openclaw.json":      `{"meta":{"lastTouchedVersion":"stale"}}`,
		"openclaw.json.bak":  "bak",
		"session/auth.json":  `{"token":"keep-me"}`,
		"workspace/memo.txt": "hello",
	})

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             userID,
			Config:             cfg,
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{createContainer: hosting.Container{ID: "ctr-1", ServerID: serverID}}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		archiveDir,
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if hostingStub.restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", hostingStub.restoreCalls)
	}
	if hostingStub.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", hostingStub.createCalls)
	}
	if hostingStub.startCalls != 1 {
		t.Fatalf("start calls = %d, want 1", hostingStub.startCalls)
	}
	if len(hostingStub.callOrder) < 3 || hostingStub.callOrder[0] != "restore" || hostingStub.callOrder[1] != "create" || hostingStub.callOrder[2] != "start" {
		t.Fatalf("call order = %v, want restore before create/start", hostingStub.callOrder)
	}

	restored := readWorkerArchiveFixture(t, bytes.NewReader(hostingStub.restoreBody))
	assertWorkerArchiveConfigEquals(t, restored[clawID.String()+"/openclaw.json"], cfg)
	if _, ok := restored[clawID.String()+"/openclaw.json.bak"]; ok {
		t.Fatal("expected backup file to be removed before restore")
	}
	if got := string(restored[clawID.String()+"/session/auth.json"]); got != `{"token":"keep-me"}` {
		t.Fatalf("session/auth.json = %q, want unchanged payload", got)
	}
	if got := string(restored[clawID.String()+"/workspace/memo.txt"]); got != "hello" {
		t.Fatalf("workspace/memo.txt = %q, want unchanged payload", got)
	}
}

func TestProcessNextOperationStartFallsBackWhenArchiveMissing(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()
	archiveDir := t.TempDir()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             uuid.New(),
			Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{createContainer: hosting.Container{ID: "ctr-1", ServerID: serverID}}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		archiveDir,
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if hostingStub.restoreCalls != 0 {
		t.Fatalf("restore calls = %d, want 0", hostingStub.restoreCalls)
	}
	if hostingStub.createCalls != 1 || hostingStub.startCalls != 1 {
		t.Fatalf("create/start calls = %d/%d, want 1/1", hostingStub.createCalls, hostingStub.startCalls)
	}
}

func TestProcessNextOperationStartMissingArchiveMarksOnboardingIncomplete(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()
	archiveDir := t.TempDir()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             uuid.New(),
			OnboardingComplete: true,
			Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{createContainer: hosting.Container{ID: "ctr-1", ServerID: serverID}}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		archiveDir,
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if clawStorage.claws[clawID].OnboardingComplete {
		t.Fatal("expected onboarding to be reset after missing archive fallback")
	}
}

func TestProcessNextOperationStartFallsBackWhenArchiveIsCorrupted(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()
	archiveDir := t.TempDir()

	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("not-a-tar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             userID,
			Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{createContainer: hosting.Container{ID: "ctr-1", ServerID: serverID}}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		archiveDir,
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if hostingStub.restoreCalls != 0 {
		t.Fatalf("restore calls = %d, want 0", hostingStub.restoreCalls)
	}
	if hostingStub.createCalls != 1 || hostingStub.startCalls != 1 {
		t.Fatalf("create/start calls = %d/%d, want 1/1", hostingStub.createCalls, hostingStub.startCalls)
	}
}

func TestProcessNextOperationStartCorruptedArchiveMarksOnboardingIncomplete(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	userID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()
	archiveDir := t.TempDir()

	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("not-a-tar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:                 clawID,
			UserID:             userID,
			OnboardingComplete: true,
			Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStart,
				Status:    entities.ClawLifecycleOperationStatusPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		queue: []uuid.UUID{opID},
	}
	hostingStub := &workerTestHosting{createContainer: hosting.Container{ID: "ctr-1", ServerID: serverID}}

	svc := NewClaw(
		clawStorage,
		operationStorage,
		&workerTestChannelStorage{},
		&workerTestUserStorage{},
		&workerTestServerStorage{server: entities.Server{ID: serverID, MaxClaws: 1}},
		hostingStub,
		&workerTestKeys{},
		archiveDir,
		GmailWatchConfig{},
	)

	if err := svc.ProcessNextOperation(context.Background()); err != nil {
		t.Fatalf("ProcessNextOperation() error = %v", err)
	}

	if clawStorage.claws[clawID].OnboardingComplete {
		t.Fatal("expected onboarding to be reset after corrupted archive fallback")
	}
}

func writeWorkerArchiveFixture(t *testing.T, archivePath string, root string, files map[string]string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(archivePath), err)
	}

	fd, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Create(%s) error = %v", archivePath, err)
	}
	defer fd.Close()

	tw := tar.NewWriter(fd)
	defer func() {
		if err := tw.Close(); err != nil {
			t.Fatalf("close tar writer: %v", err)
		}
	}()

	if err := tw.WriteHeader(&tar.Header{
		Name:     root + "/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}); err != nil {
		t.Fatalf("write root header: %v", err)
	}

	for relPath, contents := range files {
		fullPath := filepath.ToSlash(filepath.Join(root, relPath))
		if err := tw.WriteHeader(&tar.Header{
			Name:     fullPath,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(contents)),
		}); err != nil {
			t.Fatalf("write header %q: %v", fullPath, err)
		}
		if _, err := tw.Write([]byte(contents)); err != nil {
			t.Fatalf("write file %q: %v", fullPath, err)
		}
	}
}

func readWorkerArchiveFixture(t *testing.T, r io.Reader) map[string][]byte {
	t.Helper()

	files := make(map[string][]byte)
	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("ReadAll(%s) error = %v", hdr.Name, err)
		}
		files[hdr.Name] = data
	}
}

func assertWorkerArchiveConfigEquals(t *testing.T, got []byte, want entities.ClawConfig) {
	t.Helper()

	var gotCfg entities.ClawConfig
	if err := json.Unmarshal(got, &gotCfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	gotJSON, err := json.Marshal(gotCfg)
	if err != nil {
		t.Fatalf("Marshal(got) error = %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal(want) error = %v", err)
	}

	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("openclaw.json = %s, want %s", gotJSON, wantJSON)
	}
}
