package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"shared/pkg/observability"
	"sort"
	"strings"
	"time"

	"simpleClaw/internal/service/billing/result"

	"simpleClaw/config"

	entitychannels "simpleClaw/internal/entities/channels"
	"simpleClaw/internal/service/pkg/minor"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/billing/commands"

	"github.com/google/uuid"
)

const defaultOpenRouterCostCurrency = "USD"

type Service struct {
	env           string
	plans         planStorage
	subscriptions subscriptionStorage
	balance       balanceEntryStorage
	payMethod     paymentMethodStorage
	keyManager    keyManager
	users         userStorage
	payments      paymentStorage
	payInfra      paymentInfra
	usageAmounts  usageAmountConverter
	bootstrapClaw bootstrapClawReader
	bootstrapBot  bootstrapManagedBotReader
	metrics       *observability.OperationMetrics
}

type SubscriptionSummary struct {
	Subscription entities.UserSubscription
	Plan         entities.Plan
}

type BillingSummary struct {
	BalanceMinor        string
	CurrentSubscription *SubscriptionSummary
	NextChargeAt        *time.Time
}

func NewService(
	env string,
	plans planStorage,
	subscriptions subscriptionStorage,
	balance balanceEntryStorage,
	users userStorage,
	payments paymentStorage,
	payInfra paymentInfra,
	usageAmounts usageAmountConverter,
	payMethod paymentMethodStorage,
	keyManager keyManager,
	metrics ...*observability.OperationMetrics,
) *Service {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Service{
		env:           env,
		plans:         plans,
		subscriptions: subscriptions,
		balance:       balance,
		users:         users,
		payments:      payments,
		payInfra:      payInfra,
		payMethod:     payMethod,
		keyManager:    keyManager,
		usageAmounts:  usageAmounts,
		metrics:       opMetrics,
	}
}

func (s *Service) WithBootstrapClawReader(reader bootstrapClawReader) *Service {
	s.bootstrapClaw = reader

	return s
}

func (s *Service) WithBootstrapManagedBotReader(reader bootstrapManagedBotReader) *Service {
	s.bootstrapBot = reader

	return s
}

func (s *Service) CreatePlan(
	ctx context.Context,
	cmd commands.CreatePlan,
) (plan entities.Plan, err error) {
	const op = "service.billing.CreatePlan"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"plan.create",
		"billing_plan",
	)

	defer func() { finish(err) }()

	if s.plans == nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	now := time.Now().UTC()
	plan = entities.Plan{
		ID:                 uuid.New(),
		Code:               strings.TrimSpace(cmd.Code),
		Name:               strings.TrimSpace(cmd.Name),
		Interval:           strings.TrimSpace(cmd.Interval),
		BillingAmountMinor: cmd.BillingAmountMinor,
		BalanceCreditMinor: cmd.BalanceCreditMinor,
		Currency:           strings.TrimSpace(cmd.Currency),
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := validatePlan(plan); err != nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.plans.Create(ctx, plan); err != nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, err)
	}

	return plan, nil
}

func (s *Service) GetPlan(ctx context.Context, id uuid.UUID) (plan entities.Plan, err error) {
	const op = "service.billing.GetPlan"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"plan.get",
		"billing_plan",
	)

	defer func() { finish(err) }()

	if s.plans == nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if id == uuid.Nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	plan, err = s.plans.GetByID(ctx, id)
	if err != nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, err)
	}

	return plan, nil
}

func (s *Service) ListPlans(
	ctx context.Context,
	includeInactive bool,
) (plans []entities.Plan, err error) {
	const op = "service.billing.ListPlans"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"plan.list",
		"billing_plan",
	)

	defer func() { finish(err) }()

	if s.plans == nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	plans, err = s.plans.List(ctx, includeInactive)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return plans, nil
}

func (s *Service) UpdatePlan(
	ctx context.Context,
	cmd commands.UpdatePlan,
) (plan entities.Plan, err error) {
	const op = "service.billing.UpdatePlan"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"plan.update",
		"billing_plan",
	)

	defer func() { finish(err) }()

	if s.plans == nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	plan, err = s.plans.GetByID(ctx, cmd.ID)
	if err != nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, err)
	}

	plan.Code = strings.TrimSpace(cmd.Code)
	plan.Name = strings.TrimSpace(cmd.Name)
	plan.Interval = strings.TrimSpace(cmd.Interval)
	plan.BillingAmountMinor = cmd.BillingAmountMinor
	plan.BalanceCreditMinor = cmd.BalanceCreditMinor
	plan.Currency = strings.TrimSpace(cmd.Currency)
	plan.IsActive = cmd.IsActive
	plan.UpdatedAt = time.Now().UTC()

	if err := validatePlan(plan); err != nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.plans.Update(ctx, plan); err != nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, err)
	}

	return plan, nil
}

func (s *Service) DeactivatePlan(ctx context.Context, id uuid.UUID) (err error) {
	const op = "service.billing.DeactivatePlan"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"plan.deactivate",
		"billing_plan",
	)

	defer func() { finish(err) }()

	if s.plans == nil {
		return fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if id == uuid.Nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	if err := s.plans.Deactivate(ctx, id); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) Subscribe(
	ctx context.Context,
	cmd commands.Subscribe,
) (checkout result.SubscriptionCheckout, err error) {
	const op = "service.billing.Subscribe"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"subscription.subscribe",
		"billing_subscription",
	)

	defer func() { finish(err) }()

	if s.plans == nil || s.subscriptions == nil {
		return result.SubscriptionCheckout{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if cmd.UserID == uuid.Nil || cmd.PlanID == uuid.Nil || cmd.Now.IsZero() {
		return result.SubscriptionCheckout{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	plan, err := s.plans.GetByID(ctx, cmd.PlanID)
	if err != nil {
		return result.SubscriptionCheckout{}, fmt.Errorf("%s: load plan %s: %w", op, cmd.PlanID, err)
	}

	if !plan.IsActive {
		return result.SubscriptionCheckout{}, fmt.Errorf(
			"%s: validate plan %s: %w",
			op,
			plan.ID,
			decorateValidation(ErrPlanInactive),
		)
	}

	periodEnd, err := nextChargeAt(plan, cmd.Now)
	if err != nil {
		return result.SubscriptionCheckout{}, fmt.Errorf(
			"%s: calculate subscription period plan=%s: %w",
			op,
			plan.ID,
			err,
		)
	}

	subscription := entities.UserSubscription{
		ID:                 uuid.New(),
		UserID:             cmd.UserID,
		PlanID:             plan.ID,
		Status:             entities.SubscriptionStatusPending,
		StartedAt:          cmd.Now,
		CurrentPeriodStart: cmd.Now,
		CurrentPeriodEnd:   periodEnd,
		CreatedAt:          cmd.Now,
		UpdatedAt:          cmd.Now,
	}

	if err := s.subscriptions.Create(ctx, subscription); err != nil {
		return result.SubscriptionCheckout{}, fmt.Errorf(
			"%s: create pending subscription user=%s plan=%s subscription=%s: %w",
			op,
			cmd.UserID,
			plan.ID,
			subscription.ID,
			err,
		)
	}

	val, err := minor.MinorToString(plan.BillingAmountMinor, plan.Currency)
	if err != nil {
		return result.SubscriptionCheckout{}, fmt.Errorf("%s: %w", op, err)
	}

	returnURL, err := subscriptionReturnURL(cmd.ReturnURL, subscription.ID)
	if err != nil {
		return result.SubscriptionCheckout{}, fmt.Errorf("%s: %w", op, err)
	}

	payment, err := s.payInfra.CreatePayment(
		ctx,
		entities.Amount{
			Value:    val,
			Currency: plan.Currency,
		},
		cmd.UserID, true, nil, returnURL,
	)
	if err != nil {
		return result.SubscriptionCheckout{}, fmt.Errorf(
			"%s: create subscription payment user=%s plan=%s subscription=%s: %w",
			op,
			cmd.UserID,
			plan.ID,
			subscription.ID,
			decorateExternal(err),
		)
	}

	if payment.Confirmation.ConfirmationURL == nil {
		return result.SubscriptionCheckout{}, fmt.Errorf(
			"%s: create subscription payment user=%s plan=%s subscription=%s: %w",
			op,
			cmd.UserID,
			plan.ID,
			subscription.ID,
			decorateInternal(ErrPaymentConfirmationMissing),
		)
	}

	payment.SubscriptionID = &subscription.ID
	payment.UserID = cmd.UserID
	payment.Purpose = entities.PaymentPurposeSubscription

	err = s.payments.Create(ctx, payment)
	if err != nil {
		return result.SubscriptionCheckout{}, fmt.Errorf(
			"%s: persist subscription payment %s user=%s subscription=%s: %w",
			op,
			payment.ID,
			cmd.UserID,
			subscription.ID,
			err,
		)
	}

	return result.SubscriptionCheckout{
		SubscriptionID:  subscription.ID,
		PaymentID:       payment.ID,
		Status:          subscription.Status,
		ConfirmationURL: *payment.Confirmation.ConfirmationURL,
		ReturnURL:       returnURL,
	}, nil
}

func (s *Service) ChangePlan(
	ctx context.Context,
	cmd commands.ChangePlan,
) (subscription entities.UserSubscription, err error) {
	const op = "service.billing.ChangePlan"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"subscription.change_plan",
		"billing_subscription",
	)

	defer func() { finish(err) }()

	if s.plans == nil || s.subscriptions == nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if cmd.UserID == uuid.Nil || cmd.PlanID == uuid.Nil || cmd.Now.IsZero() {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	plan, err := s.plans.GetByID(ctx, cmd.PlanID)
	if err != nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: load plan %s: %w", op, cmd.PlanID, err)
	}

	if !plan.IsActive {
		return entities.UserSubscription{}, fmt.Errorf(
			"%s: validate plan %s: %w",
			op,
			plan.ID,
			decorateValidation(ErrPlanInactive),
		)
	}

	subscription, err = s.subscriptions.GetActiveByUserID(ctx, cmd.UserID)
	if err != nil {
		return entities.UserSubscription{}, fmt.Errorf(
			"%s: load active subscription user=%s: %w",
			op,
			cmd.UserID,
			err,
		)
	}

	if subscription.Status == entities.SubscriptionStatusActive {
		return entities.UserSubscription{}, fmt.Errorf(
			"%s: validate subscription %s: %w",
			op,
			subscription.ID,
			decorateValidation(ErrSubscriptionChangeWhileActive),
		)
	}

	subscription.PlanID = cmd.PlanID
	subscription.UpdatedAt = cmd.Now

	if err := s.subscriptions.Update(ctx, subscription); err != nil {
		return entities.UserSubscription{}, fmt.Errorf(
			"%s: update subscription %s to plan %s: %w",
			op,
			subscription.ID,
			cmd.PlanID,
			err,
		)
	}

	return subscription, nil
}

func (s *Service) GetSubscriptionCheckoutStatus(
	ctx context.Context,
	userID, subscriptionID uuid.UUID,
) (status result.SubscriptionCheckoutStatus, err error) {
	const op = "service.billing.GetSubscriptionCheckoutStatus"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"subscription.checkout_status.get",
		"billing_subscription",
	)

	defer func() { finish(err) }()

	if s.subscriptions == nil || s.payments == nil || s.plans == nil {
		return result.SubscriptionCheckoutStatus{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if userID == uuid.Nil || subscriptionID == uuid.Nil {
		return result.SubscriptionCheckoutStatus{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	subscription, err := s.subscriptions.GetByIDAndUserID(ctx, subscriptionID, userID)
	if err != nil {
		return result.SubscriptionCheckoutStatus{}, fmt.Errorf(
			"%s: load subscription %s user=%s: %w",
			op,
			subscriptionID,
			userID,
			err,
		)
	}

	payment, err := s.payments.GetLatestBySubscriptionID(ctx, subscriptionID, userID)
	if err != nil {
		return result.SubscriptionCheckoutStatus{}, fmt.Errorf(
			"%s: load latest payment subscription=%s user=%s: %w",
			op,
			subscriptionID,
			userID,
			err,
		)
	}

	if _, err := s.plans.GetByID(ctx, subscription.PlanID); err != nil {
		return result.SubscriptionCheckoutStatus{}, fmt.Errorf(
			"%s: load plan %s for subscription %s: %w",
			op,
			subscription.PlanID,
			subscriptionID,
			err,
		)
	}

	status = result.SubscriptionCheckoutStatus{
		SubscriptionID:     subscription.ID,
		SubscriptionStatus: subscription.Status,
		PaymentStatus:      string(payment.Status),
		PlanID:             subscription.PlanID,
	}

	if subscription.Status == entities.SubscriptionStatusActive {
		nextChargeAt := subscription.CurrentPeriodEnd
		status.NextChargeAt = &nextChargeAt
	}

	return status, nil
}

func (s *Service) CancelSubscription(
	ctx context.Context,
	cmd commands.CancelSubscription,
) (err error) {
	const op = "service.billing.CancelSubscription"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"subscription.cancel",
		"billing_subscription",
	)

	defer func() { finish(err) }()

	if s.subscriptions == nil {
		return fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if cmd.UserID == uuid.Nil || cmd.Now.IsZero() {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	subscription, err := s.subscriptions.GetActiveByUserID(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("%s: load active subscription user=%s: %w", op, cmd.UserID, err)
	}

	if err := s.subscriptions.Cancel(ctx, subscription.ID, cmd.UserID, cmd.Now); err != nil {
		return fmt.Errorf(
			"%s: cancel subscription %s user=%s: %w",
			op,
			subscription.ID,
			cmd.UserID,
			err,
		)
	}

	err = s.payMethod.ClearDefaultByUserID(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("%s: get methods by user=%s: %w", op, cmd.UserID, err)
	}

	return nil
}

func (s *Service) GetCurrentSubscription(
	ctx context.Context,
	userID uuid.UUID,
) (summary *SubscriptionSummary, err error) {
	const op = "service.billing.GetCurrentSubscription"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"subscription.get",
		"billing_subscription",
	)

	defer func() { finish(err) }()

	if s.subscriptions == nil || s.plans == nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	subscription, err := s.subscriptions.GetActiveByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	plan, err := s.plans.GetByID(ctx, subscription.PlanID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &SubscriptionSummary{
		Subscription: subscription,
		Plan:         plan,
	}, nil
}

func (s *Service) ListBalanceEntries(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
) (entries []entities.UserBalanceEntry, err error) {
	const op = "service.billing.ListBalanceEntries"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"balance.entry.list",
		"billing_balance",
	)

	defer func() { finish(err) }()

	if s.balance == nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	entries, err = s.balance.ListByUserID(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return entries, nil
}

func (s *Service) ApplySuccessfulTopUp(ctx context.Context, payment entities.Payment) (err error) {
	const op = "service.billing.ApplySuccessfulTopUp"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"payment.apply_top_up",
		"billing_payment",
	)

	defer func() { finish(err) }()

	if s.balance == nil {
		return fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if err := validateSuccessfulPayment(payment, entities.PaymentPurposeTopUp); err != nil {
		return fmt.Errorf(
			"%s: validate top-up payment %s user=%s: %w",
			op,
			payment.ID,
			payment.UserID,
			err,
		)
	}

	processed, err := s.alreadyProcessed(
		ctx,
		payment.UserID,
		payment.ID,
		entities.BalanceEntryTypeTopUpCredit,
	)
	if err != nil {
		return fmt.Errorf(
			"%s: check top-up payment dedup payment=%s user=%s: %w",
			op,
			payment.ID,
			payment.UserID,
			err,
		)
	}
	if processed {
		return nil
	}

	amountMinor, err := minor.StringToMinor(payment.Amount.Value, payment.Amount.Currency)
	if err != nil {
		return fmt.Errorf(
			"%s: parse top-up amount payment=%s value=%q: %w",
			op,
			payment.ID,
			payment.Amount.Value,
			err,
		)
	}

	payment, err = s.payInfra.Capture(ctx, &payment)
	if err != nil {
		return fmt.Errorf(
			"%s: capture top-up payment %s user=%s: %w",
			op,
			payment.ID,
			payment.UserID,
			decorateExternal(err),
		)
	}

	paymentID := payment.ID
	_, err = s.balance.ApplyCredit(ctx, entities.UserBalanceEntry{
		ID:          uuid.New(),
		UserID:      payment.UserID,
		Type:        entities.BalanceEntryTypeTopUpCredit,
		AmountMinor: amountMinor,
		PaymentID:   &paymentID,
		Description: payment.Description,
		CreatedAt:   payment.CreatedAt,
	}, payment)
	if err != nil {
		return fmt.Errorf(
			"%s: apply top-up credit payment=%s user=%s: %w",
			op,
			payment.ID,
			payment.UserID,
			err,
		)
	}

	return nil
}

func (s *Service) ApplySuccessfulSubscriptionPayment(
	ctx context.Context,
	payment entities.Payment,
) (err error) {
	const op = "service.billing.ApplySuccessfulSubscriptionPayment"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"payment.apply_subscription",
		"billing_payment",
	)

	defer func() { finish(err) }()

	if s.plans == nil || s.subscriptions == nil || s.balance == nil {
		return fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if err := validateSuccessfulPayment(payment, entities.PaymentPurposeSubscription); err != nil {
		return fmt.Errorf(
			"%s: validate subscription payment %s user=%s: %w",
			op,
			payment.ID,
			payment.UserID,
			err,
		)
	}

	processed, err := s.alreadyProcessed(
		ctx,
		payment.UserID,
		payment.ID,
		entities.BalanceEntryTypeSubscriptionCredit,
	)
	if err != nil {
		return fmt.Errorf(
			"%s: check subscription payment dedup payment=%s user=%s: %w",
			op,
			payment.ID,
			payment.UserID,
			err,
		)
	}
	if processed {
		return nil
	}

	if payment.SubscriptionID == nil {
		return fmt.Errorf(
			"%s: validate subscription payment %s user=%s: %w",
			op,
			payment.ID,
			payment.UserID,
			decorateInternal(ErrPaymentSubscriptionMissing),
		)
	}

	subscription, err := s.subscriptions.GetByID(ctx, *payment.SubscriptionID)
	if err != nil {
		return fmt.Errorf(
			"%s: load subscription %s for payment %s: %w",
			op,
			*payment.SubscriptionID,
			payment.ID,
			err,
		)
	}

	if subscription.Status != entities.SubscriptionStatusPending {
		if subscription.Status == entities.SubscriptionStatusActive &&
			payment.Status == entities.Succeeded {
			_, err := s.balance.GetByPaymentID(ctx, payment.ID)
			if err != nil && !errors.Is(err, sql.ErrNotFound) {
				return fmt.Errorf("%s: %w", op, err)
			}

			if err == nil {
				return nil
			}

			plan, err := s.plans.GetByID(ctx, subscription.PlanID)
			if err != nil {
				return fmt.Errorf(
					"%s: load plan %s for subscription %s: %w",
					op,
					subscription.PlanID,
					subscription.ID,
					err,
				)
			}

			_, err = s.balance.ApplyCredit(ctx, entities.UserBalanceEntry{
				ID:             uuid.New(),
				UserID:         payment.UserID,
				Type:           entities.BalanceEntryTypeSubscriptionCredit,
				AmountMinor:    plan.BalanceCreditMinor,
				PaymentID:      &payment.ID,
				SubscriptionID: &subscription.ID,
				Description:    payment.Description,
				CreatedAt:      payment.CreatedAt,
			}, payment)
			if err != nil {
				return fmt.Errorf(
					"%s: apply subscription credit payment=%s subscription=%s user=%s: %w",
					op,
					payment.ID,
					subscription.ID,
					payment.UserID,
					err,
				)
			}

			return nil
		}
		return fmt.Errorf(
			"%s: validate subscription %s for payment %s: %w",
			op,
			subscription.ID,
			payment.ID,
			decorateValidation(ErrSubscriptionNotPending),
		)
	}

	plan, err := s.plans.GetByID(ctx, subscription.PlanID)
	if err != nil {
		return fmt.Errorf(
			"%s: load plan %s for subscription %s: %w",
			op,
			subscription.PlanID,
			subscription.ID,
			err,
		)
	}

	subscription.Status = entities.SubscriptionStatusActive

	err = s.subscriptions.Update(ctx, subscription)
	if err != nil {
		return fmt.Errorf(
			"%s: activate subscription %s for payment %s: %w",
			op,
			subscription.ID,
			payment.ID,
			err,
		)
	}

	payment, err = s.payInfra.Capture(ctx, &payment)
	if err != nil {
		return fmt.Errorf(
			"%s: capture subscription payment %s subscription=%s: %w",
			op,
			payment.ID,
			subscription.ID,
			decorateExternal(err),
		)
	}

	paymentID := payment.ID
	subscriptionID := subscription.ID
	_, err = s.balance.ApplyCredit(ctx, entities.UserBalanceEntry{
		ID:             uuid.New(),
		UserID:         payment.UserID,
		Type:           entities.BalanceEntryTypeSubscriptionCredit,
		AmountMinor:    plan.BalanceCreditMinor,
		PaymentID:      &paymentID,
		SubscriptionID: &subscriptionID,
		Description:    payment.Description,
		CreatedAt:      payment.CreatedAt,
	}, payment)
	if err != nil {
		return fmt.Errorf(
			"%s: apply subscription credit payment=%s subscription=%s user=%s: %w",
			op,
			payment.ID,
			subscription.ID,
			payment.UserID,
			err,
		)
	}

	return nil
}

func (s *Service) TopUp(ctx context.Context, cm commands.TopUp) (confirmURL string, err error) {
	const op = "service.billing.TopUp"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"balance.charge_usage",
		"billing_balance",
	)

	defer func() { finish(err) }()

	p, err := s.payInfra.CreatePayment(ctx, entities.Amount{
		Value:    cm.Amount,
		Currency: entities.RUB,
	}, cm.UserID, false, nil, "")
	if err != nil {
		return "", fmt.Errorf(
			"%s: create top-up payment user=%s amount=%q: %w",
			op,
			cm.UserID,
			cm.Amount,
			decorateExternal(err),
		)
	}

	if p.Confirmation.ConfirmationURL == nil {
		return "", fmt.Errorf(
			"%s: create top-up payment user=%s amount=%q: %w",
			op,
			cm.UserID,
			cm.Amount,
			decorateInternal(ErrPaymentConfirmationMissing),
		)
	}

	p.UserID = cm.UserID
	p.Purpose = entities.PaymentPurposeTopUp

	err = s.payments.Create(ctx, p)
	if err != nil {
		return "", fmt.Errorf("%s: persist top-up payment %s user=%s: %w", op, p.ID, cm.UserID, err)
	}

	return *p.Confirmation.ConfirmationURL, nil
}

func (s *Service) HandleOpenRouterUsageWebhook(
	ctx context.Context,
	event commands.OpenRouterUsageEvent,
) (err error) {
	const op = "service.billing.HandleOpenRouterUsageWebhook"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"usage.openrouter_webhook",
		"billing_usage",
	)

	defer func() { finish(err) }()

	if s.balance == nil {
		return fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if strings.TrimSpace(event.TraceID) == "" ||
		strings.TrimSpace(event.SpanID) == "" ||
		strings.TrimSpace(event.APIKeyName) == "" ||
		strings.TrimSpace(event.TotalCost) == "" ||
		event.OccurredAt.IsZero() {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	userID, err := parseUserIDFromOpenRouterAPIKeyName(event.APIKeyName)
	if err != nil {
		return fmt.Errorf(
			"%s: parse api key name %q trace=%s span=%s: %w",
			op,
			sanitizeAPIKeyName(event.APIKeyName),
			strings.TrimSpace(event.TraceID),
			strings.TrimSpace(event.SpanID),
			err,
		)
	}

	if s.env == config.EnvDevelopment {
		devModelsCosts(event.InputTokens, event.OutputToken)
	}

	amountMinor, err := s.usageAmounts.ToMinor(
		event.TotalCost,
		entities.USD,
		entities.RUB,
	)
	if err != nil {
		return fmt.Errorf(
			"%s: convert usage amount trace=%s span=%s total_cost=%q: %w",
			op,
			strings.TrimSpace(event.TraceID),
			strings.TrimSpace(event.SpanID),
			event.TotalCost,
			err,
		)
	}

	if amountMinor <= 0 {
		return nil
	}

	description := strings.TrimSpace(event.Description)
	if description == "" {
		description = fmt.Sprintf(
			"openrouter usage model=%s trace=%s span=%s",
			strings.TrimSpace(event.Model),
			strings.TrimSpace(event.TraceID),
			strings.TrimSpace(event.SpanID),
		)
	}

	_, applied, err := s.balance.ApplyUsageDebitOnce(ctx, entities.UserBalanceEntry{
		ID:          uuid.New(),
		UserID:      userID,
		Type:        entities.BalanceEntryTypeUsageDebit,
		AmountMinor: amountMinor,
		Description: description,
		CreatedAt:   event.OccurredAt,
	}, entities.OpenRouterUsageEvent{
		ID:         uuid.New(),
		Provider:   "openrouter",
		TraceID:    strings.TrimSpace(event.TraceID),
		SpanID:     strings.TrimSpace(event.SpanID),
		UserID:     userID,
		Model:      strings.TrimSpace(event.Model),
		APIKeyName: strings.TrimSpace(event.APIKeyName),
		CreatedAt:  event.OccurredAt,
	})
	if err != nil {
		if isInsufficientBalanceErr(err) {
			u, err := s.users.GetByID(ctx, userID)
			if err != nil {
				return fmt.Errorf("%s: get user by id: %w", op, err)
			}

			err = s.keyManager.DisableKey(ctx, u.OpenRouterKeyID)
			if err != nil {
				// TODO: stop container
				return fmt.Errorf("%s: disable key: %w", op, err)
			}

			return fmt.Errorf("%s: %w", op, decorateValidation(ErrInsufficientBalance))
		}
		return fmt.Errorf(
			"%s: apply usage debit trace=%s span=%s user=%s: %w",
			op,
			strings.TrimSpace(event.TraceID),
			strings.TrimSpace(event.SpanID),
			userID,
			err,
		)
	}

	if !applied {
		return nil
	}

	return nil
}

func (s *Service) GetBillingSummary(
	ctx context.Context,
	userID uuid.UUID,
) (summary BillingSummary, err error) {
	const op = "service.billing.GetBillingSummary"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"billing.summary.get",
		"billing_balance",
	)

	defer func() { finish(err) }()

	if s.users == nil {
		return BillingSummary{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if userID == uuid.Nil {
		return BillingSummary{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return BillingSummary{}, fmt.Errorf("%s: load user %s: %w", op, userID, err)
	}

	b, err := minor.MinorToString(user.BalanceMinor, entities.RUB)
	if err != nil {
		return BillingSummary{}, fmt.Errorf("%s: parse minor %s: %w", op, userID, err)
	}

	summary = BillingSummary{BalanceMinor: b}

	if s.subscriptions != nil && s.plans != nil {
		subscription, err := s.subscriptions.GetActiveByUserID(ctx, userID)
		if err == nil {
			plan, planErr := s.plans.GetByID(ctx, subscription.PlanID)
			if planErr != nil {
				return BillingSummary{}, fmt.Errorf(
					"%s: load plan %s for subscription %s: %w",
					op,
					subscription.PlanID,
					subscription.ID,
					planErr,
				)
			}

			summary.CurrentSubscription = &SubscriptionSummary{
				Subscription: subscription,
				Plan:         plan,
			}
		} else if !errors.Is(err, sql.ErrNotFound) {
			return BillingSummary{}, fmt.Errorf("%s: load active subscription user=%s: %w", op, userID, err)
		}
	}

	if s.payments != nil {
		payment, err := s.payments.GetLatestSucceededByPurpose(
			ctx,
			userID,
			entities.PaymentPurposeSubscription,
		)
		if err == nil {
			nextCharge, chargeErr := nextChargeAt(summary.CurrentSubscription.Plan, payment.CreatedAt)
			if chargeErr != nil {
				return BillingSummary{}, fmt.Errorf(
					"%s: calculate next charge plan=%s: %w",
					op,
					summary.CurrentSubscription.Plan.ID,
					chargeErr,
				)
			}
			summary.NextChargeAt = &nextCharge
		} else if !errors.Is(err, sql.ErrNotFound) {
			return BillingSummary{}, fmt.Errorf("%s: load latest subscription payment user=%s: %w", op, userID, err)
		}
	}

	return summary, nil
}

func (s *Service) GetBootstrap(
	ctx context.Context,
	userID uuid.UUID,
) (bootstrap result.Bootstrap, err error) {
	const op = "service.billing.GetBootstrap"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.billing",
		"billing.bootstrap.get",
		"billing_subscription",
	)

	defer func() { finish(err) }()

	if s.users == nil || s.subscriptions == nil {
		return result.Bootstrap{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	if userID == uuid.Nil {
		return result.Bootstrap{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	if _, err := s.users.GetByID(ctx, userID); err != nil {
		return result.Bootstrap{}, fmt.Errorf("%s: load user %s: %w", op, userID, err)
	}

	subscription, err := s.subscriptions.GetLatestByUserID(ctx, userID)
	switch {
	case err == nil:
		bootstrap.Subscription = bootstrapSubscription(subscription, time.Now().UTC())
	case errors.Is(err, sql.ErrNotFound):
	default:
		return result.Bootstrap{}, fmt.Errorf("%s: load latest subscription user=%s: %w", op, userID, err)
	}

	bootstrap.DashboardAllowed = bootstrap.Subscription != nil && bootstrap.Subscription.AccessActive

	var claws []entities.Claw
	if s.bootstrapClaw != nil {
		claws, err = s.bootstrapClaw.GetByUserID(ctx, userID)
		if err != nil {
			return result.Bootstrap{}, fmt.Errorf("%s: load claws user=%s: %w", op, userID, err)
		}
	}

	selectedClaw := selectBootstrapClaw(claws)

	var managedBot *entities.TelegramManagedBot
	if selectedClaw != nil && s.bootstrapBot != nil {
		bot, botErr := s.bootstrapBot.GetLatestManagedBotByClawID(ctx, userID, selectedClaw.ID)
		switch {
		case botErr == nil:
			managedBot = &bot
		case errors.Is(botErr, sql.ErrNotFound):
		default:
			return result.Bootstrap{}, fmt.Errorf(
				"%s: load managed bot user=%s claw=%s: %w",
				op,
				userID,
				selectedClaw.ID,
				botErr,
			)
		}
	}

	bootstrap.Onboarding = deriveBootstrapOnboarding(bootstrap.DashboardAllowed, selectedClaw, managedBot)

	return bootstrap, nil
}

func bootstrapSubscription(subscription entities.UserSubscription, now time.Time) *result.BootstrapSubscription {
	currentPeriodEnd := subscription.CurrentPeriodEnd

	return &result.BootstrapSubscription{
		Status:           subscription.Status,
		CurrentPeriodEnd: &currentPeriodEnd,
		AccessActive:     subscriptionAllowsDashboard(subscription, now),
	}
}

func subscriptionAllowsDashboard(subscription entities.UserSubscription, now time.Time) bool {
	switch subscription.Status {
	case entities.SubscriptionStatusActive:
		return true
	case entities.SubscriptionStatusCanceled:
		if subscription.CurrentPeriodEnd.IsZero() {
			return false
		}

		return subscription.CurrentPeriodEnd.After(now)
	default:
		return false
	}
}

func deriveBootstrapOnboarding(
	dashboardAllowed bool,
	cl *entities.Claw,
	managedBot *entities.TelegramManagedBot,
) result.BootstrapOnboarding {
	if cl == nil {
		return result.BootstrapOnboarding{
			Required: true,
			Step:     result.OnboardingStepSubscriptionRequired,
		}
	}

	if !dashboardAllowed {
		return result.BootstrapOnboarding{
			Required: true,
			Step:     result.OnboardingStepSubscriptionRequired,
			ClawID:   &cl.ID,
		}
	}

	if cl.OnboardingComplete {
		return result.BootstrapOnboarding{
			Required: false,
			Step:     result.OnboardingStepDashboardReady,
			ClawID:   &cl.ID,
		}
	}

	if managedBot != nil {
		payload := bootstrapTelegramManager(managedBot)

		switch managedBot.Status {
		case entities.TelegramManagedBotStatusPendingLink:
			return result.BootstrapOnboarding{
				Required:        true,
				Step:            result.OnboardingStepTelegramManagerLink,
				ClawID:          &cl.ID,
				TelegramManager: payload,
			}
		case entities.TelegramManagedBotStatusLinked, entities.TelegramManagedBotStatusWaitingCreation:
			return result.BootstrapOnboarding{
				Required:        true,
				Step:            result.OnboardingStepTelegramManagerProvisioning,
				ClawID:          &cl.ID,
				TelegramManager: payload,
			}
		case entities.TelegramManagedBotStatusFailed:
			return result.BootstrapOnboarding{
				Required:        true,
				Step:            result.OnboardingStepTelegramManualConnect,
				ClawID:          &cl.ID,
				TelegramManager: payload,
			}
		case entities.TelegramManagedBotStatusReady:
			return result.BootstrapOnboarding{
				Required:        false,
				Step:            result.OnboardingStepDashboardReady,
				ClawID:          &cl.ID,
				TelegramManager: payload,
			}
		}
	}

	if requiresTelegramConfirm(cl.Config) {
		return result.BootstrapOnboarding{
			Required: true,
			Step:     result.OnboardingStepTelegramConfirm,
			ClawID:   &cl.ID,
		}
	}

	return result.BootstrapOnboarding{
		Required: true,
		Step:     result.OnboardingStepTelegramChoice,
		ClawID:   &cl.ID,
	}
}

func bootstrapTelegramManager(managedBot *entities.TelegramManagedBot) *result.BootstrapTelegramManager {
	if managedBot == nil {
		return nil
	}

	payload := &result.BootstrapTelegramManager{
		ID:          managedBot.ID,
		Status:      string(managedBot.Status),
		DeepLinkURL: managedBot.DeepLinkURL,
		LastError:   managedBot.LastError,
	}
	if !managedBot.LinkExpiresAt.IsZero() {
		linkExpiresAt := managedBot.LinkExpiresAt
		payload.LinkExpiresAt = &linkExpiresAt
	}
	if managedBot.ChannelID != uuid.Nil {
		channelID := managedBot.ChannelID
		payload.ChannelID = &channelID
	}

	return payload
}

func selectBootstrapClaw(claws []entities.Claw) *entities.Claw {
	candidates := make([]entities.Claw, 0, len(claws))
	for _, cl := range claws {
		if cl.DesiredState == entities.ClawDesiredStateDeleted {
			continue
		}

		candidates = append(candidates, cl)
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].OnboardingComplete != candidates[j].OnboardingComplete {
			return !candidates[i].OnboardingComplete
		}
		if !candidates[i].UpdatedAt.Equal(candidates[j].UpdatedAt) {
			return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt)
		}
		if !candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
		}

		return candidates[i].ID.String() > candidates[j].ID.String()
	})

	return &candidates[0]
}

func requiresTelegramConfirm(cfg entities.ClawConfig) bool {
	return cfg.Channels != nil &&
		cfg.Channels.Telegram != nil &&
		cfg.Channels.Telegram.Enabled &&
		cfg.Channels.Telegram.DmPolicy == entitychannels.DmPairing
}

func (s *Service) ExpanseAnalyze(
	ctx context.Context,
	cm commands.Expanse,
) (result.Expanses, error) {
	return s.expanseAnalyzeAt(ctx, cm, time.Now().UTC())
}

func (s *Service) expanseAnalyzeAt(
	ctx context.Context,
	cm commands.Expanse,
	now time.Time,
) (result.Expanses, error) {
	const op = "service.billing.ExpanseAnalyze"

	if s.balance == nil {
		return result.Expanses{}, fmt.Errorf("%s: %w", op, sql.ErrUnavailable)
	}

	now = now.UTC()
	todayStart := startOfDayUTC(now)
	weekStart := startOfISOWeekUTC(now)
	monthStart := startOfMonthUTC(now)

	todaySpent, err := s.balance.SumUsageDebitByUserIDInRange(ctx, cm.UserID, todayStart, now)
	if err != nil {
		return result.Expanses{}, fmt.Errorf("%s: sum usage debit for today: %w", op, err)
	}

	dayTotal, err := s.balance.SumUsageDebitByUserIDInRange(
		ctx,
		cm.UserID,
		todayStart.AddDate(0, 0, -30),
		todayStart,
	)
	if err != nil {
		return result.Expanses{}, fmt.Errorf("%s: sum usage debit for daily average: %w", op, err)
	}

	weekTotal, err := s.balance.SumUsageDebitByUserIDInRange(
		ctx,
		cm.UserID,
		weekStart.AddDate(0, 0, -56),
		weekStart,
	)
	if err != nil {
		return result.Expanses{}, fmt.Errorf("%s: sum usage debit for weekly average: %w", op, err)
	}

	monthTotal, err := s.balance.SumUsageDebitByUserIDInRange(
		ctx,
		cm.UserID,
		monthStart.AddDate(0, -12, 0),
		monthStart,
	)
	if err != nil {
		return result.Expanses{}, fmt.Errorf("%s: sum usage debit for monthly average: %w", op, err)
	}

	today, err := formatMinorRUB(todaySpent)
	if err != nil {
		return result.Expanses{}, fmt.Errorf("%s: format today spent: %w", op, err)
	}

	day, err := formatMinorRUB(dayTotal / 30)
	if err != nil {
		return result.Expanses{}, fmt.Errorf("%s: format daily average: %w", op, err)
	}

	week, err := formatMinorRUB(weekTotal / 8)
	if err != nil {
		return result.Expanses{}, fmt.Errorf("%s: format weekly average: %w", op, err)
	}

	month, err := formatMinorRUB(monthTotal / 12)
	if err != nil {
		return result.Expanses{}, fmt.Errorf("%s: format monthly average: %w", op, err)
	}

	return result.Expanses{
		Today: today,
		Month: month,
		Week:  week,
		Day:   day,
	}, nil
}

func startOfDayUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func startOfISOWeekUTC(t time.Time) time.Time {
	t = startOfDayUTC(t)

	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	return t.AddDate(0, 0, -(weekday - 1))
}

func startOfMonthUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func formatMinorRUB(amountMinor int64) (string, error) {
	return minor.MinorToString(amountMinor, entities.RUB)
}
