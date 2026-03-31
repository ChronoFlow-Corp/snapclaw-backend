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

type paymentStorage interface {
	Create(ctx context.Context, payment entities.Payment) error
	GetByID(
		ctx context.Context,
		id string,
		userID uuid.UUID,
	) (entities.Payment, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Payment, error)
	Update(ctx context.Context, payment entities.Payment) error
	Delete(ctx context.Context, id string, userID uuid.UUID) error
}

type paymentMethodStorage interface {
	Create(ctx context.Context, method entities.PaymentMethod) error
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.PaymentMethod, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.PaymentMethod, error)
	UpdateDefault(ctx context.Context, id, userID uuid.UUID, isDefault bool) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	ClearDefaultByUserID(ctx context.Context, userID uuid.UUID) error
	CountByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
}

type apiKeyManager interface {
	Create(
		ctx context.Context,
		userID uuid.UUID,
		monthlyBudgetUSD float64,
	) (entities.OpenRouterKey, error)
}
