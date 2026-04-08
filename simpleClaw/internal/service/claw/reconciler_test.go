package claw

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
)

func TestProcessNextReconcileMarksRunningRuntimeAsRecovered(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:          clawID,
			UserID:      uuid.New(),
			ServerID:    serverID,
			ContainerID: "ctr-1",
			CreatedAt:   now.Add(-time.Minute),
			UpdatedAt:   now.Add(-time.Minute),
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateRunning,
				ObservedState:      entities.ClawObservedStateUnknown,
				LifecycleStatus:    entities.ClawLifecycleStatusReconcilePending,
				CurrentOperationID: &opID,
				LastError:          "timeout",
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeStart,
				Status:    entities.ClawLifecycleOperationStatusRetryScheduled,
				CreatedAt: now.Add(-time.Minute),
				UpdatedAt: now.Add(-time.Minute),
			},
		},
	}
	hostingStub := &workerTestHosting{
		state: hosting.RuntimeState{
			RuntimeRecordID: "ctr-1",
			ObservedState:   "running",
			RuntimeStatus:   "running",
		},
	}

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

	if err := svc.ProcessNextReconcile(context.Background()); err != nil {
		t.Fatalf("ProcessNextReconcile() error = %v", err)
	}

	cl := clawStorage.claws[clawID]
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
		t.Fatalf("current operation id = %v, want nil", cl.CurrentOperationID)
	}
	if cl.LastError != "" {
		t.Fatalf("last error = %q, want empty", cl.LastError)
	}

	op := operationStorage.operations[opID]
	if op.Status != entities.ClawLifecycleOperationStatusSucceeded {
		t.Fatalf("operation status = %q, want %q", op.Status, entities.ClawLifecycleOperationStatusSucceeded)
	}
}

func TestProcessNextReconcileDeletesClawWhenRuntimeIsMissing(t *testing.T) {
	now := time.Now().UTC()
	clawID := uuid.New()
	serverID := uuid.New()
	opID := uuid.New()

	clawStorage := &workerTestClawStorage{claws: map[uuid.UUID]entities.Claw{
		clawID: {
			ID:          clawID,
			UserID:      uuid.New(),
			ServerID:    serverID,
			ContainerID: "ctr-1",
			CreatedAt:   now.Add(-time.Minute),
			UpdatedAt:   now.Add(-time.Minute),
			ClawLifecycleState: entities.ClawLifecycleState{
				DesiredState:       entities.ClawDesiredStateDeleted,
				ObservedState:      entities.ClawObservedStateUnknown,
				LifecycleStatus:    entities.ClawLifecycleStatusReconcilePending,
				CurrentOperationID: &opID,
				LastError:          "delete timeout",
			},
		},
	}}
	operationStorage := &workerTestOperationStorage{
		operations: map[uuid.UUID]entities.ClawLifecycleOperation{
			opID: {
				ID:        opID,
				ClawID:    clawID,
				Type:      entities.ClawLifecycleOperationTypeDelete,
				Status:    entities.ClawLifecycleOperationStatusRetryScheduled,
				CreatedAt: now.Add(-time.Minute),
				UpdatedAt: now.Add(-time.Minute),
			},
		},
	}
	hostingStub := &workerTestHosting{stateErr: hosting.ErrRuntimeNotFound}

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

	if err := svc.ProcessNextReconcile(context.Background()); err != nil {
		t.Fatalf("ProcessNextReconcile() error = %v", err)
	}

	if _, ok := clawStorage.claws[clawID]; ok {
		t.Fatalf("claw %s still exists after reconcile delete", clawID)
	}

	op := operationStorage.operations[opID]
	if op.Status != entities.ClawLifecycleOperationStatusSucceeded {
		t.Fatalf("operation status = %q, want %q", op.Status, entities.ClawLifecycleOperationStatusSucceeded)
	}
}
