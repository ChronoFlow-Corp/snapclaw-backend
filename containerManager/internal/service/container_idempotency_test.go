package service

import (
	"context"
	"testing"
	"time"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/pkg/configurer"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/service/commands"

	"github.com/google/uuid"
)

func TestContainerStopReturnsNilWhenContainerAlreadyStopped(t *testing.T) {
	t.Parallel()

	runtime := &fakeRuntime{}
	repo := &fakeClawRepository{
		getByUserClawIDResult: entities.Container{
			ID:             uuid.New(),
			UserID:         "user-1",
			ClawID:         "claw-1",
			ContainerID:    "container-1",
			Status:         entities.ContainerStatusStop,
			HasStartedOnce: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
	}

	svc := &Container{
		clRepo:  repo,
		manager: runtime,
		cfg:     configurer.NewClawConfigurer(t.TempDir(), "", -1, -1),
		p:       NewPorter(),
	}

	if err := svc.Stop(context.Background(), commands.StopClaw{UserID: "user-1", ClawID: "claw-1"}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if runtime.stopCalled {
		t.Fatal("expected runtime stop not to be called for an already stopped container")
	}
}

func TestContainerStartReturnsNilWhenContainerAlreadyRunning(t *testing.T) {
	t.Parallel()

	runtime := &fakeRuntime{}
	repo := &fakeClawRepository{
		getByUserClawIDResult: entities.Container{
			ID:             uuid.New(),
			UserID:         "user-1",
			ClawID:         "claw-1",
			ContainerID:    "container-1",
			Status:         entities.ContainerStatusRunning,
			HasStartedOnce: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
	}

	svc := &Container{
		clRepo:              repo,
		manager:             runtime,
		readAvailableMemory: func() (uint64, error) { return warmStartMinAvailableBytes, nil },
		coldStartMinBytes:   coldStartMinAvailableBytes,
		warmStartMinBytes:   warmStartMinAvailableBytes,
	}

	if err := svc.Start(context.Background(), commands.StartClaw{UserID: "user-1", ClawID: "claw-1"}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if runtime.startCalled {
		t.Fatal("expected runtime start not to be called for an already running container")
	}
}

func TestContainerDeleteReturnsNilWhenContainerMissing(t *testing.T) {
	t.Parallel()

	runtime := &fakeRuntime{}
	repo := &fakeClawRepository{
		getByUserClawIDErr: storage.ErrNotFound,
	}

	svc := &Container{
		clRepo:  repo,
		manager: runtime,
		cfg:     configurer.NewClawConfigurer(t.TempDir(), "", -1, -1),
		p:       NewPorter(),
	}

	if err := svc.Delete(context.Background(), commands.DeleteClaw{UserID: "user-1", ClawID: "claw-1", DeleteConfig: true}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if runtime.removeCalled {
		t.Fatal("expected runtime remove not to be called when container is missing")
	}
}

func TestContainerEnsureReturnsExistingRuntimeWithoutCreatingNewOne(t *testing.T) {
	t.Parallel()

	existingID := uuid.New()
	runtime := &fakeRuntime{}
	repo := &fakeClawRepository{
		getByUserClawIDResult: entities.Container{
			ID:             existingID,
			UserID:         "user-1",
			ClawID:         "claw-1",
			ContainerID:    "docker-1",
			Port:           "8080",
			Status:         entities.ContainerStatusStop,
			HasStartedOnce: false,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
	}

	svc := &Container{
		clRepo:  repo,
		manager: runtime,
		cfg:     configurer.NewClawConfigurer(t.TempDir(), "", -1, -1),
		p:       NewPorter(),
	}

	got, err := svc.Ensure(context.Background(), commands.CreateClaw{UserID: "user-1", ClawID: "claw-1"})
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	if got.ID != existingID {
		t.Fatalf("Ensure() id = %s, want %s", got.ID, existingID)
	}

	if runtime.createCalled {
		t.Fatal("expected runtime create not to be called when runtime already exists")
	}
}
