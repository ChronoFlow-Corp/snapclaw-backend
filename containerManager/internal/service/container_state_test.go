package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/service/commands"

	"github.com/google/uuid"
)

func TestContainerRuntimeStateReturnsContainer(t *testing.T) {
	t.Parallel()

	want := entities.Container{
		ID:             uuid.New(),
		UserID:         "user-1",
		ClawID:         "claw-1",
		ContainerID:    "docker-1",
		Status:         entities.ContainerStatusRunning,
		Port:           "8080",
		HasStartedOnce: true,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	svc := &Container{
		clRepo: &fakeClawRepository{
			getByUserClawIDResult: want,
		},
	}

	got, err := svc.RuntimeState(context.Background(), commands.StateClaw{UserID: want.UserID, ClawID: want.ClawID})
	if err != nil {
		t.Fatalf("RuntimeState() error = %v", err)
	}

	if got != want {
		t.Fatalf("RuntimeState() = %+v, want %+v", got, want)
	}
}

func TestContainerRuntimeStateReturnsNotFound(t *testing.T) {
	t.Parallel()

	svc := &Container{
		clRepo: &fakeClawRepository{
			getByUserClawIDErr: storage.ErrNotFound,
		},
	}

	_, err := svc.RuntimeState(context.Background(), commands.StateClaw{UserID: "user-1", ClawID: "claw-1"})
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("RuntimeState() error = %v, want ErrNotFound", err)
	}
}
