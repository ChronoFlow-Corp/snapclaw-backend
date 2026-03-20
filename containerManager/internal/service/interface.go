package service

import (
	"context"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/pkg/docker"
)

type ClawRepository interface {
	Create(ctx context.Context, cl entities.Container) error
	GetByUserClawID(ctx context.Context, uID, cID string) (entities.Container, error)
	GetAll(ctx context.Context) ([]entities.Container, error)
	Update(ctx context.Context, cl entities.Container) error
	Remove(ctx context.Context, cl entities.Container) error
	GetByID(ctx context.Context, id string) (entities.Container, error)
}

type containerRuntime interface {
	Create(ctx context.Context, opts docker.CreateOptions) (string, error)
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string) error
	Remove(ctx context.Context, containerID string) error
	ExecGmail(ctx context.Context, containerID string, token []byte, opts docker.ExecGmailOptions) error
	StartGmailWatch(ctx context.Context, containerID string, opts docker.ExecGmailWatchStartOptions) error
	StartGmailWatcher(ctx context.Context, containerID string, opts docker.ExecGmailWatcherOptions) error
}
