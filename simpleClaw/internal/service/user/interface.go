package user

import (
	"context"

	"simpleClaw/internal/entities"

	"github.com/google/uuid"
)

type UStorage interface {
	Create(ctx context.Context, u entities.User) error
	Delete(ctx context.Context, id uuid.UUID) error
	CreateSession(ctx context.Context, session entities.Session) error
	DeleteSession(ctx context.Context, session entities.Session) error
	GetSessions(ctx context.Context, userID uuid.UUID) ([]entities.Session, error)
	GetSession(ctx context.Context, id uuid.UUID) (entities.Session, error)
	GetByEmail(ctx context.Context, email string) (entities.User, error)
}

type ChannelStorage interface {
	Create(ctx context.Context, channel entities.Channel) error
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.Channel, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Channel, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	Update(ctx context.Context, ch entities.Channel) error
}
