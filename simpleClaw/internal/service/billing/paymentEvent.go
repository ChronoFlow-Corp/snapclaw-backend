package billing

import (
	"context"
	"fmt"
	"log/slog"
	"shared/pkg/observability"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/billing/commands"
)

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
		return nil
	case entities.Succeeded:
		return nil
	case entities.Canceled:
		return nil
	case entities.Pending:
		return nil
	default:
		return fmt.Errorf("unknown status: %s", cm.Object.Status)
	}
}

func (s *Service) succeededPayment(ctx context.Context, p entities.Payment) error {
	const op = "service.billing.SucceededPayment"

	_, err := s.payments.GetByID(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *Service) updatePayment(ctx context.Context, old, new entities.Payment) error {
	const op = "service.Billing.UpdatePayment"

	return nil
}
