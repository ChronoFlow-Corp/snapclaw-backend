package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/pkg/configurer"
	"github.com/google/uuid"
)

func TestContainerApprove_ExecutesApproveInRuntime(t *testing.T) {
	t.Parallel()

	runtime := &fakeRuntime{}
	repo := &fakeClawRepository{
		getByUserClawIDResult: entities.Container{
			ID:          uuid.New(),
			UserID:      "user-1",
			ClawID:      "claw-1",
			ContainerID: "docker-1",
			Status:      entities.ContainerStatusRunning,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}
	svc := &Container{
		clRepo:  repo,
		manager: runtime,
		cfg:     configurer.NewClawConfigurer(t.TempDir(), "", -1, -1),
	}

	err := svc.Approve(context.Background(), "claw-1", "user-1", "telegram", "123456")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	if !runtime.execPairingApproveCalled {
		t.Fatal("expected pairing approve exec to be called")
	}

	if runtime.execPairingApproveChannel != "telegram" {
		t.Fatalf("channel = %q, want %q", runtime.execPairingApproveChannel, "telegram")
	}

	if runtime.execPairingApproveCode != "123456" {
		t.Fatalf("code = %q, want %q", runtime.execPairingApproveCode, "123456")
	}
}

func TestContainerApprove_InvalidCodeFromExec(t *testing.T) {
	t.Parallel()

	runtime := &fakeRuntime{execPairingApproveErr: errors.New("invalid code")}
	repo := &fakeClawRepository{
		getByUserClawIDResult: entities.Container{
			ID:          uuid.New(),
			UserID:      "user-1",
			ClawID:      "claw-1",
			ContainerID: "docker-1",
			Status:      entities.ContainerStatusRunning,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}
	svc := &Container{
		clRepo:  repo,
		manager: runtime,
		cfg:     configurer.NewClawConfigurer(t.TempDir(), "", -1, -1),
	}

	err := svc.Approve(context.Background(), "claw-1", "user-1", "telegram", "wrong")
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("Approve() error = %v, want ErrInvalidCode", err)
	}
}
