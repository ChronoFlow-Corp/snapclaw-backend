package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"containermanager/internal/entities"
)

type runtimeWatcher struct {
	repo           ClawRepository
	runtime        containerRuntime
	interval       time.Duration
	inspectTimeout time.Duration
	running        atomic.Bool
}

func NewRuntimeWatcher(
	repo ClawRepository,
	runtime containerRuntime,
	interval time.Duration,
	inspectTimeout time.Duration,
) *runtimeWatcher {
	if interval <= 0 {
		interval = 15 * time.Second
	}

	if inspectTimeout <= 0 {
		inspectTimeout = 3 * time.Second
	}

	return &runtimeWatcher{
		repo:           repo,
		runtime:        runtime,
		interval:       interval,
		inspectTimeout: inspectTimeout,
	}
}

func (w *runtimeWatcher) Run(ctx context.Context) {
	if w == nil {
		return
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.syncOnce(ctx); err != nil {
				slog.Default().Error("runtime watcher sync failed", slog.Any("err", err))
			}
		}
	}
}

func (w *runtimeWatcher) syncOnce(ctx context.Context) error {
	if w == nil {
		return nil
	}

	if !w.running.CompareAndSwap(false, true) {
		return nil
	}
	defer w.running.Store(false)

	containers, err := w.repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("runtime watcher list containers: %w", err)
	}

	for _, cont := range containers {
		if cont.ContainerID == "" {
			continue
		}

		status, err := w.syncContainer(ctx, cont)
		if err != nil {
			return err
		}

		if status == cont.Status {
			continue
		}

		cont.Status = status
		if err := w.repo.Update(ctx, cont); err != nil {
			return fmt.Errorf("runtime watcher update container %s: %w", cont.ID, err)
		}
	}

	return nil
}

func (w *runtimeWatcher) syncContainer(ctx context.Context, cont entities.Container) (string, error) {
	inspectCtx, cancel := context.WithTimeout(ctx, w.inspectTimeout)
	defer cancel()

	exists, running, _, err := w.runtime.Inspect(inspectCtx, cont.ContainerID)
	if err != nil {
		return "", fmt.Errorf("inspect container %s: %w", cont.ContainerID, err)
	}

	switch {
	case !exists:
		return entities.ContainerStatusError, nil
	case running:
		return entities.ContainerStatusRunning, nil
	default:
		return entities.ContainerStatusStop, nil
	}
}
