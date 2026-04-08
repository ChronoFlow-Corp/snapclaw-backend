package claw

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"shared/pkg/observability"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/storages/claws"

	"github.com/google/uuid"
)

const defaultReconcilerInterval = 3 * time.Second

func (s *Service) RunReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultReconcilerInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := s.ProcessNextReconcile(ctx)
		if err != nil && !errors.Is(err, sql.ErrNotFound) {
			slog.Default().Error("lifecycle reconciler iteration failed", slog.Any("err", err))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) ProcessNextReconcile(ctx context.Context) error {
	const opName = "service.Claw.ProcessNextReconcile"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.reconcile",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	if s.operations == nil {
		return fmt.Errorf("%s: %w", opName, ErrOperationStorageRequired)
	}

	cl, err := s.claws.GetNextReconcilePending(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", opName, err)
	}

	active, err := s.operations.GetActiveByClawID(ctx, cl.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNotFound) {
			err = s.claws.UpdateLifecycle(ctx, cl.ID, claws.LifecycleUpdate{
				DesiredState:       cl.DesiredState,
				ObservedState:      cl.ObservedState,
				LifecycleStatus:    entities.ClawLifecycleStatusFailed,
				CurrentOperationID: nil,
				LastLifecycleError: "missing lifecycle operation for reconciliation",
			})
		}
		if err != nil {
			return fmt.Errorf("%s: %w", opName, err)
		}

		return nil
	}

	s.logLifecycleEvent(
		"picked_reconcile_candidate",
		active,
		slog.String("desired_state", string(cl.DesiredState)),
		slog.String("observed_state", string(cl.ObservedState)),
		slog.String("lifecycle_status", string(cl.LifecycleStatus)),
	)

	err = s.reconcileClaw(ctx, cl, active)
	if err != nil {
		return fmt.Errorf("%s: %w", opName, err)
	}

	return nil
}

func (s *Service) reconcileClaw(ctx context.Context, cl entities.Claw, op entities.ClawLifecycleOperation) error {
	switch cl.DesiredState {
	case entities.ClawDesiredStateRunning:
		return s.reconcileRunningClaw(ctx, cl, op)
	case entities.ClawDesiredStateStopped:
		return s.reconcileStoppedClaw(ctx, cl, op)
	case entities.ClawDesiredStateDeleted:
		return s.reconcileDeletedClaw(ctx, cl, op)
	default:
		return s.failOperation(ctx, op, fmt.Errorf("unsupported desired state %q", cl.DesiredState))
	}
}

func (s *Service) reconcileRunningClaw(ctx context.Context, cl entities.Claw, op entities.ClawLifecycleOperation) error {
	srv, err := s.serverForStart(ctx, cl)
	if err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	if cl.ContainerID == "" {
		return s.retryOperation(ctx, op, cl, hosting.ErrRuntimeNotFound)
	}

	state, err := s.hosting.State(ctx, cl, srv)
	if errors.Is(err, hosting.ErrRuntimeNotFound) {
		return s.retryOperation(ctx, op, cl, err)
	}
	if err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	switch mapObservedState(state) {
	case entities.ClawObservedStateRunning:
		if state.RuntimeRecordID != "" {
			cl.ContainerID = state.RuntimeRecordID
		}

		s.logLifecycleEvent("reconcile_detected_running_runtime", op)
		return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
			ServerID:          cl.ServerID,
			ContainerRecordID: cl.ContainerID,
			DesiredState:      entities.ClawDesiredStateRunning,
			ObservedState:     entities.ClawObservedStateRunning,
			LifecycleStatus:   entities.ClawLifecycleStatusIdle,
		})
	case entities.ClawObservedStateStopped:
		s.logLifecycleEvent("reconcile_restart_runtime", op)
		if err := s.hosting.Start(ctx, cl, srv); err != nil {
			return s.retryOperation(ctx, op, cl, err)
		}

		return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
			ServerID:          cl.ServerID,
			ContainerRecordID: cl.ContainerID,
			DesiredState:      entities.ClawDesiredStateRunning,
			ObservedState:     entities.ClawObservedStateRunning,
			LifecycleStatus:   entities.ClawLifecycleStatusIdle,
		})
	default:
		return s.retryOperation(ctx, op, cl, fmt.Errorf("runtime state is not reconciled yet"))
	}
}

func (s *Service) reconcileStoppedClaw(ctx context.Context, cl entities.Claw, op entities.ClawLifecycleOperation) error {
	if cl.ServerID == uuid.Nil || cl.ContainerID == "" {
		return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
			DesiredState:      entities.ClawDesiredStateStopped,
			ObservedState:     entities.ClawObservedStateStopped,
			LifecycleStatus:   entities.ClawLifecycleStatusIdle,
			ContainerRecordID: "",
		})
	}

	srv, err := s.servers.GetByID(ctx, cl.ServerID)
	if err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	state, err := s.hosting.State(ctx, cl, srv)
	if errors.Is(err, hosting.ErrRuntimeNotFound) {
		s.logLifecycleEvent("reconcile_detected_missing_runtime", op)
		return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
			DesiredState:      entities.ClawDesiredStateStopped,
			ObservedState:     entities.ClawObservedStateStopped,
			LifecycleStatus:   entities.ClawLifecycleStatusIdle,
			ContainerRecordID: "",
		})
	}
	if err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	if mapObservedState(state) == entities.ClawObservedStateMissing {
		s.logLifecycleEvent("reconcile_detected_missing_runtime", op)
		return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
			DesiredState:      entities.ClawDesiredStateStopped,
			ObservedState:     entities.ClawObservedStateStopped,
			LifecycleStatus:   entities.ClawLifecycleStatusIdle,
			ContainerRecordID: "",
		})
	}

	if err := s.hosting.Delete(ctx, cl, srv, true); err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
		DesiredState:      entities.ClawDesiredStateStopped,
		ObservedState:     entities.ClawObservedStateStopped,
		LifecycleStatus:   entities.ClawLifecycleStatusIdle,
		ContainerRecordID: "",
	})
}

func (s *Service) reconcileDeletedClaw(ctx context.Context, cl entities.Claw, op entities.ClawLifecycleOperation) error {
	if cl.ServerID == uuid.Nil || cl.ContainerID == "" {
		if err := s.claws.Delete(ctx, cl.ID, cl.UserID); err != nil {
			return s.retryOperation(ctx, op, cl, err)
		}

		return s.markOperationSucceeded(ctx, op)
	}

	srv, err := s.servers.GetByID(ctx, cl.ServerID)
	if err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	state, err := s.hosting.State(ctx, cl, srv)
	if errors.Is(err, hosting.ErrRuntimeNotFound) || (err == nil && mapObservedState(state) == entities.ClawObservedStateMissing) {
		s.logLifecycleEvent("reconcile_delete_completed", op)
		if err := s.claws.Delete(ctx, cl.ID, cl.UserID); err != nil {
			return s.retryOperation(ctx, op, cl, err)
		}

		return s.markOperationSucceeded(ctx, op)
	}
	if err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	if err := s.hosting.Delete(ctx, cl, srv, true); err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}
	s.logLifecycleEvent("reconcile_delete_runtime", op)

	if err := s.claws.Delete(ctx, cl.ID, cl.UserID); err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	return s.markOperationSucceeded(ctx, op)
}
