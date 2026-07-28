package admin

import (
	"context"

	"simpleClaw/internal/entities"
	billingcommands "simpleClaw/internal/service/billing/commands"
	billingresult "simpleClaw/internal/service/billing/result"

	"github.com/google/uuid"
)

// statsStorage runs the aggregate dashboard/listing queries that have no
// per-domain equivalent elsewhere.
type statsStorage interface {
	CountUsers(ctx context.Context) (int64, error)
	CountUsersByRole(ctx context.Context, role string) (int64, error)
	CountUsersWithActiveSubscription(ctx context.Context) (int64, error)
	CountUsersWithIssues(ctx context.Context) (int64, error)
	ListUsers(ctx context.Context, filter entities.AdminUserFilter) ([]entities.AdminUserListItem, int64, error)
}

type userReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (entities.User, error)
}

type clawReader interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Claw, error)
	GetBySystemID(ctx context.Context, id uuid.UUID) (entities.Claw, error)
}

type capabilityReader interface {
	ListByClawID(ctx context.Context, clawID, userID uuid.UUID) ([]entities.ClawCapabilityAttachment, error)
}

type subscriptionReader interface {
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]entities.UserSubscription, error)
}

type paymentReader interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Payment, error)
}

type paymentMethodReader interface {
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.PaymentMethod, error)
}

type balanceReader interface {
	ListByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]entities.UserBalanceEntry, error)
}

type integrationReader interface {
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]entities.AccountIntegration, error)
}

type telegramReader interface {
	ListManagedBotsByUserID(ctx context.Context, userID uuid.UUID) ([]entities.TelegramManagedBot, error)
}

// usageAnalyzer reuses the billing expanse aggregation so admin usage figures
// stay identical to what the user sees in their own dashboard.
type usageAnalyzer interface {
	ExpanseAnalyze(ctx context.Context, cm billingcommands.Expanse) (billingresult.Expanses, error)
}
