package claw

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"

	"github.com/google/uuid"
)

const defaultRuntimeSyncInterval = 30 * time.Second

func (s *Service) RunRuntimeSync(
	ctx context.Context,
	interval time.Duration,
	timeout time.Duration,
	batchSize int,
	workerCount int,
) {
	if interval <= 0 {
		interval = defaultRuntimeSyncInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.syncRuntimeBatch(ctx, timeout, batchSize, workerCount); err != nil {
				slog.Default().Error("runtime sync iteration failed", slog.Any("err", err))
			}
		}
	}
}

func (s *Service) syncRuntimeBatch(
	ctx context.Context,
	timeout time.Duration,
	batchSize int,
	workerCount int,
) error {
	if !s.runtimeSyncRunning.CompareAndSwap(false, true) {
		return nil
	}
	defer s.runtimeSyncRunning.Store(false)

	claws, err := s.claws.ListRuntimeSyncCandidates(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("list runtime sync candidates: %w", err)
	}

	if len(claws) == 0 {
		return nil
	}

	if workerCount <= 0 {
		workerCount = 1
	}

	jobs := make(chan entities.Claw)
	errCh := make(chan error, 1)

	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()

		for cl := range jobs {
			if err := s.syncClawRuntimeState(ctx, cl, timeout); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker()
	}

	for _, cl := range claws {
		jobs <- cl
	}
	close(jobs)
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (s *Service) syncClawRuntimeState(ctx context.Context, cl entities.Claw, timeout time.Duration) error {
	if cl.ServerID == uuid.Nil {
		return nil
	}

	srv, err := s.servers.GetByID(ctx, cl.ServerID)
	if err != nil {
		return fmt.Errorf("load server for claw %s: %w", cl.ID, err)
	}

	stateCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		stateCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	state, err := s.hosting.State(stateCtx, cl, srv)
	if errors.Is(err, hosting.ErrRuntimeNotFound) {
		return s.claws.UpdateRuntime(ctx, cl.ID, entities.ClawRuntimeUpdate{
			ServerID:           uuid.Nil,
			ContainerRecordID:  "",
			DesiredState:       cl.DesiredState,
			ObservedState:      entities.ClawObservedStateStopped,
			LifecycleStatus:    cl.LifecycleStatus,
			LastLifecycleError: "",
		})
	}

	if err != nil {
		return s.claws.UpdateRuntime(ctx, cl.ID, entities.ClawRuntimeUpdate{
			ServerID:           cl.ServerID,
			ContainerRecordID:  cl.ContainerID,
			DesiredState:       cl.DesiredState,
			ObservedState:      entities.ClawObservedStateUnknown,
			LifecycleStatus:    cl.LifecycleStatus,
			LastLifecycleError: err.Error(),
		})
	}

	update := entities.ClawRuntimeUpdate{
		ServerID:           cl.ServerID,
		ContainerRecordID:  cl.ContainerID,
		DesiredState:       cl.DesiredState,
		ObservedState:      mapObservedState(state),
		LifecycleStatus:    cl.LifecycleStatus,
		LastLifecycleError: state.LastError,
	}

	if update.ObservedState == entities.ClawObservedStateMissing {
		update.ServerID = uuid.Nil
		update.ContainerRecordID = ""
		update.ObservedState = entities.ClawObservedStateStopped
		update.LastLifecycleError = ""
	} else if state.RuntimeRecordID != "" {
		update.ContainerRecordID = state.RuntimeRecordID
	}

	if update.ObservedState == entities.ClawObservedStateRunning || update.ObservedState == entities.ClawObservedStateStopped {
		update.LastLifecycleError = ""
	}

	return s.claws.UpdateRuntime(ctx, cl.ID, update)
}
