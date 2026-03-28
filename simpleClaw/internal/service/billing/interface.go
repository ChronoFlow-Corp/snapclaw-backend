package billing

import (
	"context"
	"time"

	"github.com/google/uuid"

	"simpleClaw/internal/entities"
)

type planStorage interface {
	Create(ctx context.Context, plan entities.Plan) error
	GetByID(ctx context.Context, id uuid.UUID) (entities.Plan, error)
	List(ctx context.Context, includeInactive bool) ([]entities.Plan, error)
	Update(ctx context.Context, plan entities.Plan) error
	Deactivate(ctx context.Context, id uuid.UUID) error
}

type subscriptionStorage interface {
	Create(ctx context.Context, subscription entities.UserSubscription) error
	GetByID(ctx context.Context, id uuid.UUID) (entities.UserSubscription, error)
	GetActiveByUserID(ctx context.Context, userID uuid.UUID) (entities.UserSubscription, error)
	Update(ctx context.Context, subscription entities.UserSubscription) error
	Cancel(ctx context.Context, id, userID uuid.UUID, canceledAt time.Time) error
}

type balanceEntryStorage interface {
	ListByUserID(
		ctx context.Context,
		userID uuid.UUID,
		limit int,
	) ([]entities.UserBalanceEntry, error)
	ApplyCredit(
		ctx context.Context,
		entry entities.UserBalanceEntry,
		payment entities.Payment,
	) (int64, error)
	ApplyUsageDebit(ctx context.Context, entry entities.UserBalanceEntry) (int64, error)
	ApplyUsageDebitOnce(
		ctx context.Context,
		entry entities.UserBalanceEntry,
		event entities.OpenRouterUsageEvent,
	) (balance int64, applied bool, err error)
	GetByPaymentID(
		ctx context.Context,
		paymentID string,
	) (entities.UserBalanceEntry, error)
}

type userStorage interface {
	GetByID(ctx context.Context, id uuid.UUID) (entities.User, error)
}

type paymentStorage interface {
	GetLatestSucceededByPurpose(
		ctx context.Context,
		userID uuid.UUID,
		purpose entities.PaymentPurpose,
	) (entities.Payment, error)
	Create(ctx context.Context, payment entities.Payment) error
	GetByID(
		ctx context.Context,
		id string,
	) (entities.Payment, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Payment, error)
	Update(ctx context.Context, payment entities.Payment) error
	Delete(ctx context.Context, id string, userID uuid.UUID) error
}

type paymentInfra interface {
	CreatePayment(
		ctx context.Context,
		amount entities.Amount,
	) (entities.Payment, error)
	Capture(
		ctx context.Context,
		payment *entities.Payment,
	) (entities.Payment, error)
}

type usageAmountConverter interface {
	ToMinor(totalCost string, sourceCurrency, targetCurrency string) (int64, error)
}
