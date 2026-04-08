package claw

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"shared/pkg/observability"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/storages/claws"

	"github.com/google/uuid"
)

const (
	stageArchiveRuntime = "archive_runtime"
	stageEnsureRuntime = "ensure_runtime"
	stageStartRuntime  = "start_runtime"
	stageDeleteRuntime = "delete_runtime"
	stageDeleteClaw    = "delete_claw"

	defaultWorkerInterval = 10 * time.Second
)

var errArchiveRestoreFallback = errors.New("archived runtime config is unavailable")

func (s *Service) RunLifecycleWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultWorkerInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := s.ProcessNextOperation(ctx)
		if err != nil && !errors.Is(err, sql.ErrNotFound) {
			slog.Default().Error("lifecycle worker iteration failed", slog.Any("err", err))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) ProcessNextOperation(ctx context.Context) error {
	const opName = "service.Claw.ProcessNextOperation"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.claw",
		"claw.process_operation",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	if s.operations == nil {
		return fmt.Errorf("%s: %w", opName, ErrOperationStorageRequired)
	}

	op, err := s.operations.LockNextRunnable(ctx, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("%s: %w", opName, err)
	}

	s.logLifecycleEvent("picked_operation", op, slog.String("status", string(op.Status)))

	switch op.Type {
	case entities.ClawLifecycleOperationTypeStart:
		err = s.processStartOperation(ctx, op)
	case entities.ClawLifecycleOperationTypeStop:
		err = s.processStopOperation(ctx, op)
	case entities.ClawLifecycleOperationTypeRestart:
		err = s.processRestartOperation(ctx, op)
	case entities.ClawLifecycleOperationTypeDelete:
		err = s.processDeleteOperation(ctx, op)
	default:
		err = s.failOperation(
			ctx,
			op,
			fmt.Errorf("unsupported lifecycle operation type %q", op.Type),
		)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", opName, err)
	}

	s.logLifecycleEvent("processed_operation", op, slog.String("status", string(op.Status)))

	return nil
}

func (s *Service) processStartOperation(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
) error {
	cl, err := s.claws.GetBySystemID(ctx, op.ClawID)
	if err != nil {
		return s.retryOperation(ctx, op, entities.Claw{}, err)
	}

	srv, err := s.serverForStart(ctx, cl)
	if err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	if cl.ContainerID == "" {
		if err := s.restoreArchivedRuntimeConfig(ctx, cl, srv); err != nil {
			if errors.Is(err, errArchiveRestoreFallback) {
				if markErr := s.markOnboardingIncomplete(ctx, cl); markErr != nil {
					return s.retryOperation(ctx, op, cl, markErr)
				}
			} else {
				return s.retryOperation(ctx, op, cl, err)
			}
		}

		if err := s.updateOperationStage(ctx, &op, stageEnsureRuntime); err != nil {
			return err
		}

		container, err := s.hosting.Create(ctx, cl, srv)
		if err != nil {
			return s.retryOperation(ctx, op, cl, err)
		}

		cl.ServerID = srv.ID
		if container.ServerID != uuid.Nil {
			cl.ServerID = container.ServerID
		}
		cl.ContainerID = container.ID

		if err := s.claws.UpdateRuntime(ctx, cl.ID, entities.ClawRuntimeUpdate{
			ServerID:          cl.ServerID,
			ContainerRecordID: cl.ContainerID,
			DesiredState:      entities.ClawDesiredStateRunning,
			ObservedState:     cl.ObservedState,
			LifecycleStatus:   entities.ClawLifecycleStatusStartPending,
		}); err != nil {
			return s.retryOperation(ctx, op, cl, err)
		}
	}

	if err := s.updateOperationStage(ctx, &op, stageStartRuntime); err != nil {
		return err
	}

	if err := s.hosting.Start(ctx, cl, srv); err != nil {
		return s.reconcileStartFailure(ctx, op, cl, srv, err)
	}

	return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
		ServerID:          cl.ServerID,
		ContainerRecordID: cl.ContainerID,
		DesiredState:      entities.ClawDesiredStateRunning,
		ObservedState:     entities.ClawObservedStateRunning,
		LifecycleStatus:   entities.ClawLifecycleStatusIdle,
	})
}

func (s *Service) processStopOperation(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
) error {
	cl, err := s.claws.GetBySystemID(ctx, op.ClawID)
	if err != nil {
		return s.retryOperation(ctx, op, entities.Claw{}, err)
	}

	if cl.ContainerID == "" || cl.ServerID == uuid.Nil {
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

	if err := s.updateOperationStage(ctx, &op, stageDeleteRuntime); err != nil {
		return err
	}

	if err := s.archiveRuntimeConfig(ctx, cl, srv); err != nil {
		if markErr := s.markOnboardingIncomplete(ctx, cl); markErr != nil {
			return s.retryOperation(ctx, op, cl, markErr)
		}
		return s.retryOperation(ctx, op, cl, err)
	}

	if err := s.hosting.Delete(ctx, cl, srv, true); err != nil {
		return s.reconcileDeleteLikeFailure(ctx, op, cl, srv, err, false)
	}

	return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
		DesiredState:      entities.ClawDesiredStateStopped,
		ObservedState:     entities.ClawObservedStateStopped,
		LifecycleStatus:   entities.ClawLifecycleStatusIdle,
		ContainerRecordID: "",
	})
}

func (s *Service) processRestartOperation(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
) error {
	cl, err := s.claws.GetBySystemID(ctx, op.ClawID)
	if err != nil {
		return s.retryOperation(ctx, op, entities.Claw{}, err)
	}

	if cl.ContainerID != "" && cl.ServerID != uuid.Nil {
		srv, err := s.servers.GetByID(ctx, cl.ServerID)
		if err != nil {
			return s.retryOperation(ctx, op, cl, err)
		}

		if err := s.updateOperationStage(ctx, &op, stageArchiveRuntime); err != nil {
			return err
		}

		if err := s.archiveRuntimeConfig(ctx, cl, srv); err != nil {
			if markErr := s.markOnboardingIncomplete(ctx, cl); markErr != nil {
				return s.retryOperation(ctx, op, cl, markErr)
			}
			return s.retryOperation(ctx, op, cl, err)
		}

		if err := s.updateOperationStage(ctx, &op, stageDeleteRuntime); err != nil {
			return err
		}

		if err := s.hosting.Delete(ctx, cl, srv, true); err != nil {
			state, stateErr := s.hosting.State(ctx, cl, srv)
			if !errors.Is(err, hosting.ErrRuntimeNotFound) &&
				!errors.Is(stateErr, hosting.ErrRuntimeNotFound) &&
				(stateErr != nil || mapObservedState(state) != entities.ClawObservedStateMissing) {
				return s.retryOperation(ctx, op, cl, err)
			}
		}

		if err := s.claws.UpdateRuntime(ctx, cl.ID, entities.ClawRuntimeUpdate{
			ServerID:          cl.ServerID,
			ContainerRecordID: "",
			DesiredState:      entities.ClawDesiredStateRunning,
			ObservedState:     entities.ClawObservedStateStopped,
			LifecycleStatus:   entities.ClawLifecycleStatusRestartPending,
		}); err != nil {
			return s.retryOperation(ctx, op, cl, err)
		}
	}

	return s.processStartOperation(ctx, op)
}

func (s *Service) archiveRuntimeConfig(
	ctx context.Context,
	cl entities.Claw,
	srv entities.Server,
) error {
	if cl.ContainerID == "" || cl.ServerID == uuid.Nil {
		return nil
	}

	if s.hosting == nil {
		return ErrHostingMissing
	}

	if s.archives == nil {
		return fmt.Errorf("archive store is not configured")
	}

	body, err := s.hosting.ConfigArchive(ctx, cl, srv, false)
	if err != nil {
		return err
	}
	if body == nil {
		return fmt.Errorf("config archive body is required")
	}
	defer body.Close()

	return s.archives.Save(ctx, cl.UserID, cl.ID, body)
}

func (s *Service) restoreArchivedRuntimeConfig(
	ctx context.Context,
	cl entities.Claw,
	srv entities.Server,
) error {
	if s.hosting == nil || s.archives == nil {
		return errArchiveRestoreFallback
	}

	body, err := s.archives.Open(cl.UserID, cl.ID)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return errArchiveRestoreFallback
	case errors.Is(err, ErrArchiveStoreUnavailable):
		return errArchiveRestoreFallback
	case err != nil:
		return err
	}
	defer body.Close()

	var rewritten bytes.Buffer
	if err := hosting.RewriteConfigArchive(body, cl.ID.String(), cl.Config, &rewritten); err != nil {
		slog.Default().Warn(
			"stored archive is unreadable, falling back to ensure",
			slog.String("claw_id", cl.ID.String()),
			slog.Any("err", err),
		)
		return errArchiveRestoreFallback
	}

	if err := s.hosting.RestoreConfigArchive(ctx, cl, srv, bytes.NewReader(rewritten.Bytes())); err != nil {
		return err
	}

	return nil
}

func (s *Service) markOnboardingIncomplete(ctx context.Context, cl entities.Claw) error {
	if !cl.OnboardingComplete {
		return nil
	}

	return s.claws.UpdateLifecycle(ctx, cl.ID, claws.LifecycleUpdate{
		DesiredState:       cl.DesiredState,
		ObservedState:      cl.ObservedState,
		LifecycleStatus:    cl.LifecycleStatus,
		CurrentOperationID: cl.CurrentOperationID,
		LastLifecycleError: cl.LastError,
		OnboardingComplete: boolPtr(false),
	})
}

func (s *Service) processDeleteOperation(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
) error {
	cl, err := s.claws.GetBySystemID(ctx, op.ClawID)
	if err != nil {
		if errors.Is(err, sql.ErrNotFound) {
			return s.markOperationSucceeded(ctx, op)
		}

		return s.retryOperation(ctx, op, entities.Claw{}, err)
	}

	if cl.ContainerID != "" && cl.ServerID != uuid.Nil {
		srv, err := s.servers.GetByID(ctx, cl.ServerID)
		if err != nil {
			return s.retryOperation(ctx, op, cl, err)
		}

		if err := s.updateOperationStage(ctx, &op, stageDeleteRuntime); err != nil {
			return err
		}

		if err := s.hosting.Delete(ctx, cl, srv, true); err != nil {
			return s.reconcileDeleteLikeFailure(ctx, op, cl, srv, err, true)
		}
	}

	if err := s.updateOperationStage(ctx, &op, stageDeleteClaw); err != nil {
		return err
	}

	if err := s.claws.Delete(ctx, cl.ID, cl.UserID); err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	return nil
}

func (s *Service) serverForStart(ctx context.Context, cl entities.Claw) (entities.Server, error) {
	if cl.ServerID != uuid.Nil {
		return s.servers.GetByID(ctx, cl.ServerID)
	}

	return s.selectAvailableServer(ctx)
}

func (s *Service) completeRuntimeOperation(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
	cl entities.Claw,
	update entities.ClawRuntimeUpdate,
) error {
	if err := s.claws.UpdateRuntime(ctx, cl.ID, update); err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	if err := s.claws.UpdateLifecycle(ctx, cl.ID, claws.LifecycleUpdate{
		DesiredState:       update.DesiredState,
		ObservedState:      update.ObservedState,
		LifecycleStatus:    entities.ClawLifecycleStatusIdle,
		CurrentOperationID: nil,
		LastLifecycleError: "",
	}); err != nil {
		return s.retryOperation(ctx, op, cl, err)
	}

	return s.markOperationSucceeded(ctx, op)
}

func (s *Service) reconcileStartFailure(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
	cl entities.Claw,
	srv entities.Server,
	cause error,
) error {
	state, err := s.hosting.State(ctx, cl, srv)
	if err == nil && mapObservedState(state) == entities.ClawObservedStateRunning {
		return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
			ServerID:          cl.ServerID,
			ContainerRecordID: cl.ContainerID,
			DesiredState:      entities.ClawDesiredStateRunning,
			ObservedState:     entities.ClawObservedStateRunning,
			LifecycleStatus:   entities.ClawLifecycleStatusIdle,
		})
	}

	return s.retryOperation(ctx, op, cl, cause)
}

func (s *Service) reconcileDeleteLikeFailure(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
	cl entities.Claw,
	srv entities.Server,
	cause error,
	deleting bool,
) error {
	state, err := s.hosting.State(ctx, cl, srv)
	if errors.Is(err, hosting.ErrRuntimeNotFound) ||
		(err == nil && mapObservedState(state) == entities.ClawObservedStateMissing) {
		if deleting {
			if err := s.claws.Delete(ctx, cl.ID, cl.UserID); err != nil {
				return s.retryOperation(ctx, op, cl, err)
			}

			return nil
		}

		return s.completeRuntimeOperation(ctx, op, cl, entities.ClawRuntimeUpdate{
			DesiredState:      entities.ClawDesiredStateStopped,
			ObservedState:     entities.ClawObservedStateStopped,
			LifecycleStatus:   entities.ClawLifecycleStatusIdle,
			ContainerRecordID: "",
		})
	}

	return s.retryOperation(ctx, op, cl, cause)
}

func (s *Service) retryOperation(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
	cl entities.Claw,
	cause error,
) error {
	retryAt := time.Now().UTC().Add(nextRetryDelay(op.Attempt))
	op.Attempt++
	op.Status = entities.ClawLifecycleOperationStatusRetryScheduled
	op.LastError = cause.Error()
	op.NextRetryAt = &retryAt
	op.UpdatedAt = time.Now().UTC()

	if err := s.operations.Update(ctx, op); err != nil {
		return err
	}

	if cl.ID != uuid.Nil {
		if err := s.claws.UpdateLifecycle(ctx, cl.ID, claws.LifecycleUpdate{
			DesiredState:       desiredStateForOperation(op.Type, cl.DesiredState),
			ObservedState:      cl.ObservedState,
			LifecycleStatus:    entities.ClawLifecycleStatusReconcilePending,
			CurrentOperationID: &op.ID,
			LastLifecycleError: cause.Error(),
		}); err != nil {
			return err
		}
	}

	s.logLifecycleEvent(
		"retry_scheduled",
		op,
		slog.String("next_retry_at", retryAt.Format(time.RFC3339Nano)),
		slog.String("error", cause.Error()),
	)

	return nil
}

func (s *Service) failOperation(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
	cause error,
) error {
	op.Status = entities.ClawLifecycleOperationStatusFailed
	op.LastError = cause.Error()
	op.NextRetryAt = nil
	op.UpdatedAt = time.Now().UTC()

	if err := s.operations.Update(ctx, op); err != nil {
		return err
	}

	s.logLifecycleEvent("failed_operation", op, slog.String("error", cause.Error()))

	return nil
}

func (s *Service) markOperationSucceeded(
	ctx context.Context,
	op entities.ClawLifecycleOperation,
) error {
	op.Status = entities.ClawLifecycleOperationStatusSucceeded
	op.LastError = ""
	op.NextRetryAt = nil
	op.UpdatedAt = time.Now().UTC()

	if err := s.operations.Update(ctx, op); err != nil {
		return err
	}

	s.logLifecycleEvent("succeeded_operation", op)

	return nil
}

func (s *Service) updateOperationStage(
	ctx context.Context,
	op *entities.ClawLifecycleOperation,
	stage string,
) error {
	op.Stage = stage
	op.UpdatedAt = time.Now().UTC()
	if err := s.operations.Update(ctx, *op); err != nil {
		return err
	}

	s.logLifecycleEvent("stage_changed", *op, slog.String("stage", stage))

	return nil
}

func nextRetryDelay(attempt int) time.Duration {
	switch {
	case attempt <= 0:
		return 2 * time.Second
	case attempt == 1:
		return 5 * time.Second
	case attempt == 2:
		return 15 * time.Second
	default:
		return 30 * time.Second
	}
}

func desiredStateForOperation(
	opType entities.ClawLifecycleOperationType,
	fallback entities.ClawDesiredState,
) entities.ClawDesiredState {
	switch opType {
	case entities.ClawLifecycleOperationTypeStart:
		return entities.ClawDesiredStateRunning
	case entities.ClawLifecycleOperationTypeRestart:
		return entities.ClawDesiredStateRunning
	case entities.ClawLifecycleOperationTypeStop:
		return entities.ClawDesiredStateStopped
	case entities.ClawLifecycleOperationTypeDelete:
		return entities.ClawDesiredStateDeleted
	default:
		return fallback
	}
}

func mapObservedState(state hosting.RuntimeState) entities.ClawObservedState {
	switch state.ObservedState {
	case string(entities.ClawObservedStateRunning):
		return entities.ClawObservedStateRunning
	case string(entities.ClawObservedStateStopped), "stop":
		return entities.ClawObservedStateStopped
	case string(entities.ClawObservedStateMissing):
		return entities.ClawObservedStateMissing
	default:
		return entities.ClawObservedStateUnknown
	}
}

func (s *Service) logLifecycleEvent(
	message string,
	op entities.ClawLifecycleOperation,
	attrs ...slog.Attr,
) {
	base := []slog.Attr{
		slog.String("operation_id", op.ID.String()),
		slog.String("claw_id", op.ClawID.String()),
		slog.String("type", string(op.Type)),
		slog.String("status", string(op.Status)),
		slog.Int("attempt", op.Attempt),
	}
	if op.Stage != "" {
		base = append(base, slog.String("stage", op.Stage))
	}

	base = append(base, attrs...)
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, message, base...)
}
