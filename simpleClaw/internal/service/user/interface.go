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
	UpdateSessionRefresh(
		ctx context.Context,
		sessionID, userID uuid.UUID,
		refreshToken string,
	) error
	GetByEmail(ctx context.Context, email string) (entities.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (entities.User, error)
	UpdateRole(ctx context.Context, id uuid.UUID, role string) error
	UpdateOpenRouterKey(ctx context.Context, id uuid.UUID, key entities.OpenRouterKey) error
	GetGmailToken(ctx context.Context, userID uuid.UUID) (entities.GmailToken, error)
	UpsertGmailToken(ctx context.Context, userID uuid.UUID, token entities.GmailToken) error
}

type ChannelStorage interface {
	Create(ctx context.Context, channel entities.Channel) error
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.Channel, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Channel, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	Update(ctx context.Context, ch entities.Channel) error
}

type apiKeyManager interface {
	Create(
		ctx context.Context,
		userID uuid.UUID,
		label string,
		monthlyBudgetUSD float64,
	) (entities.OpenRouterKey, error)
}
