package service

import (
	"context"
	"testing"
	"time"

	"containermanager/internal/entities"

	"github.com/google/uuid"
)

func TestRuntimeWatcherSyncOnceMarksRunningContainer(t *testing.T) {
	t.Parallel()

	repo := &fakeClawRepository{
		getAllResult: []entities.Container{{
			ID:          uuid.New(),
			UserID:      "user-1",
			ClawID:      "claw-1",
			ContainerID: "docker-1",
			Status:      entities.ContainerStatusStop,
			Port:        "8080",
		}},
	}

	runtime := &fakeRuntime{
		inspectExists:  true,
		inspectRunning: true,
		inspectStatus:  "running",
	}

	watcher := NewRuntimeWatcher(repo, runtime, time.Second, time.Second)

	if err := watcher.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce() error = %v", err)
	}

	if !repo.updateCalled {
		t.Fatal("expected repository update")
	}

	if repo.updated.Status != entities.ContainerStatusRunning {
		t.Fatalf("status = %q, want %q", repo.updated.Status, entities.ContainerStatusRunning)
	}
}

func TestRuntimeWatcherSyncOnceMarksStoppedContainer(t *testing.T) {
	t.Parallel()

	repo := &fakeClawRepository{
		getAllResult: []entities.Container{{
			ID:          uuid.New(),
			UserID:      "user-1",
			ClawID:      "claw-1",
			ContainerID: "docker-1",
			Status:      entities.ContainerStatusRunning,
			Port:        "8080",
		}},
	}

	runtime := &fakeRuntime{
		inspectExists:  true,
		inspectRunning: false,
		inspectStatus:  "exited",
	}

	watcher := NewRuntimeWatcher(repo, runtime, time.Second, time.Second)

	if err := watcher.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce() error = %v", err)
	}

	if !repo.updateCalled {
		t.Fatal("expected repository update")
	}

	if repo.updated.Status != entities.ContainerStatusStop {
		t.Fatalf("status = %q, want %q", repo.updated.Status, entities.ContainerStatusStop)
	}
}

func TestRuntimeWatcherSyncOnceSkipsUnchangedStatus(t *testing.T) {
	t.Parallel()

	repo := &fakeClawRepository{
		getAllResult: []entities.Container{{
			ID:          uuid.New(),
			UserID:      "user-1",
			ClawID:      "claw-1",
			ContainerID: "docker-1",
			Status:      entities.ContainerStatusRunning,
			Port:        "8080",
		}},
	}

	runtime := &fakeRuntime{
		inspectExists:  true,
		inspectRunning: true,
		inspectStatus:  "running",
	}

	watcher := NewRuntimeWatcher(repo, runtime, time.Second, time.Second)

	if err := watcher.syncOnce(context.Background()); err != nil {
		t.Fatalf("syncOnce() error = %v", err)
	}

	if repo.updateCalled {
		t.Fatal("expected unchanged status to skip repository update")
	}
}
