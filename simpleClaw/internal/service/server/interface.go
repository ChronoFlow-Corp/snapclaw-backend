package server

import (
	"context"

	"simpleClaw/internal/entities"

	"github.com/google/uuid"
)

type storage interface {
	Create(ctx context.Context, srv entities.Server) error
	GetAll(ctx context.Context) ([]entities.Server, error)
	GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error)
	Update(ctx context.Context, srv entities.Server) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type capacityResolver interface {
	Capacity(ctx context.Context, srv entities.Server) (int, error)
}
