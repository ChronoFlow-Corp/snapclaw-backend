package service

import (
	"context"

	"containermanager/internal/entities"
)

type ClawRepository interface {
	Create(ctx context.Context, cl entities.Container) error
	GetByUserID(ctx context.Context, uID string) (entities.Container, error)
	GetAll(ctx context.Context) ([]entities.Container, error)
	Update(ctx context.Context, cl entities.Container) error
	Remove(ctx context.Context, cl entities.Container) error
	GetByID(ctx context.Context, id string) (entities.Container, error)
}
