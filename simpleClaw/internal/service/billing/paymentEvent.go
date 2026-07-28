package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"shared/pkg/observability"
	"time"

	"simpleClaw/internal/service/pkg/minor"

	"simpleClaw/internal/infra/sql"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/billing/commands"

	"github.com/google/uuid"
)

var errPaymentNotSubscription = errors.New("payment is not subscription")

func (s *Service) EventPayment(ctx context.Context, cm commands.PaymentEvent) (err error) {
	const op = "service.Billing.EventPayment"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"payment.event_handler",
		"billing_payment",
	)

	defer func() { finish(err) }()

	switch cm.Object.Status {
	case entities.WaitingForCapture:
		err = s.captureHandle(ctx, cm.Object)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		return nil
	case entities.Succeeded:
		err = s.succeededHandle(ctx, cm.Object)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		return nil
	case entities.Canceled:
		err = s.canceledHandle(ctx, cm.Object)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		return nil
	case entities.Pending:
		err = s.pendingHandle(ctx, cm.Object)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		return nil
	default:
		return fmt.Errorf("unknown status: %s", cm.Object.Status)
	}
}

func (s *Service) succeededHandle(ctx context.Context, p entities.Payment) error {
	const op = "service.billing.SucceededPayment"

	p, err := s.succeededPayment(ctx, p)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	err = s.succeededSubscription(ctx, p)
	if err != nil && !errors.Is(err, errPaymentNotSubscription) {
		return fmt.Errorf("%s: %w", op, err)
	}

	err = s.succeededBalance(ctx, p)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) pendingHandle(ctx context.Context, p entities.Payment) error {
	const op = "service.billing.pendingPayment"

	_, err := s.payments.GetByID(ctx, p.ID)
	switch {
	case err != nil && !errors.Is(err, sql.ErrNotFound):
	case err != nil && errors.Is(err, sql.ErrNotFound):
		_, err := s.payInfra.Cancel(ctx, p.ID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

func (s *Service) canceledHandle(ctx context.Context, p entities.Payment) error {
	const op = "service.billing.canceledPayment"

	pDb, err := s.payments.GetByID(ctx, p.ID)
	switch {
	case err != nil && !errors.Is(err, sql.ErrNotFound):
		return fmt.Errorf("%s: %w", op, err)
	case err != nil && errors.Is(err, sql.ErrNotFound):
		err = s.payments.Create(ctx, p)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	pDb = applyPaymentEventSnapshot(pDb, p)
	err = s.payments.Update(ctx, pDb)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) captureHandle(ctx context.Context, p entities.Payment) error {
	const op = "service.billing.capturePayment"

	pDb, err := s.payments.GetByID(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	pDb = applyPaymentEventSnapshot(pDb, p)
	err = s.payments.Update(ctx, pDb)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if pDb.Purpose == entities.PaymentPurposeSubscription && pDb.SubscriptionID != nil {
		err = s.ApplySuccessfulSubscriptionPayment(ctx, pDb)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		return nil
	}

	if pDb.Purpose == entities.PaymentPurposeTopUp && pDb.SubscriptionID == nil {
		err = s.ApplySuccessfulTopUp(ctx, pDb)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		return nil
	}

	return fmt.Errorf(
		"%s: unexcpected payment state, purpose=%s, subID=%s",
		op,
		pDb.Purpose,
		pDb.SubscriptionID,
	)
}

func (s *Service) succeededPayment(
	ctx context.Context,
	p entities.Payment,
) (entities.Payment, error) {
	const op = "service.Billing.completePayment"

	pDb, err := s.payments.GetByID(ctx, p.ID)
	switch {
	case err != nil && !errors.Is(err, sql.ErrNotFound):
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	case err != nil && errors.Is(err, sql.ErrNotFound):
		err = s.payments.Create(ctx, p)
		if err != nil {
			return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	if pDb.Status != p.Status {
		p = applyPaymentEventSnapshot(pDb, p)

		err = s.payments.Update(ctx, p)
		if err != nil {
			return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	return p, nil
}

func (s *Service) succeededSubscription(ctx context.Context, p entities.Payment) error {
	const op = "service.Billing.completeSubscription"

	if p.Status != entities.Succeeded {
		return errors.New("payment is not succeeded")
	}

	if p.Purpose != entities.PaymentPurposeSubscription {
		return fmt.Errorf("%s: %w", op, errPaymentNotSubscription)
	}

	if p.SubscriptionID == nil {
		return nil
	}

	sub, err := s.subscriptions.GetByID(ctx, *p.SubscriptionID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if sub.Status != entities.SubscriptionStatusActive {
		sub.Status = entities.SubscriptionStatusActive
		err = s.subscriptions.Update(ctx, sub)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	pDb, err := s.payments.GetByID(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if p.PaymentMethod != nil {
		_, err := s.payments.GetByID(ctx, p.PaymentMethod.ID)
		switch {
		case err != nil && !errors.Is(err, sql.ErrNotFound):
		case err != nil && errors.Is(err, sql.ErrNotFound):
			if p.PaymentMethod.Saved {
				now := time.Now()
				err = s.payMethod.Create(ctx, entities.PaymentMethod{
					ID:         uuid.New(),
					UserID:     pDb.UserID,
					Title:      p.PaymentMethod.Title,
					IsDefault:  true,
					CreatedAt:  now,
					LastUsedAt: &now,
				})
			}
		}
	}

	return nil
}

func (s *Service) succeededBalance(ctx context.Context, p entities.Payment) error {
	const op = "service.Billing.succeededBalance"

	if p.Status != entities.Succeeded {
		return errors.New("payment is not succeeded")
	}

	var amount int64
	if p.SubscriptionID != nil {
		sub, err := s.subscriptions.GetByID(ctx, *p.SubscriptionID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		plan, err := s.plans.GetByID(ctx, sub.PlanID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		amount = plan.BalanceCreditMinor
	} else {
		m, err := minor.StringToMinor(p.Amount.Value, p.Amount.Currency)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		amount = m
	}

	_, err := s.balance.GetByPaymentID(ctx, p.ID)
	switch {
	case err != nil && !errors.Is(err, sql.ErrNotFound):
		return fmt.Errorf("%s: %w", op, err)
	case err != nil && errors.Is(err, sql.ErrNotFound):
		entryType := entities.BalanceEntryTypeTopUpCredit
		if p.SubscriptionID != nil {
			entryType = entities.BalanceEntryTypeSubscriptionCredit
		}
		_, err = s.balance.ApplyCredit(ctx, entities.UserBalanceEntry{
			ID:             uuid.New(),
			UserID:         p.UserID,
			Type:           entryType,
			AmountMinor:    amount,
			PaymentID:      &p.ID,
			SubscriptionID: p.SubscriptionID,
			Description:    p.Description,
			CreatedAt:      time.Now(),
		}, p)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}
