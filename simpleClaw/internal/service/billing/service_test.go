package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"shared/pkg/observability"

	"simpleClaw/internal/entities"
	infraSQL "simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/billing/commands"
)

func TestCreatePlanWithMetrics(t *testing.T) {
	t.Parallel()

	reg := observability.NewPrometheusRegistry()
	metrics, err := observability.NewOperationMetrics(reg, "simpleclaw")
	if err != nil {
		t.Fatalf("NewOperationMetrics() error = %v", err)
	}

	store := newFakePlanStorage()
	service := NewService(store, nil, nil, nil, nil, nil, metrics)

	plan, err := service.CreatePlan(context.Background(), commands.CreatePlan{
		Code:               "starter",
		Name:               "Starter",
		BillingAmountMinor: 99000,
		BalanceCreditMinor: 150000,
		Currency:           entities.RUB,
	})
	if err != nil {
		t.Fatalf("CreatePlan() error = %v", err)
	}

	if plan.ID == uuid.Nil {
		t.Fatal("CreatePlan() returned zero ID")
	}
}

func TestCreatePlan(t *testing.T) {
	t.Parallel()

	store := newFakePlanStorage()
	service := NewService(store, nil, nil, nil, nil, nil)

	plan, err := service.CreatePlan(context.Background(), commands.CreatePlan{
		Code:               "starter",
		Name:               "Starter",
		BillingAmountMinor: 99000,
		BalanceCreditMinor: 150000,
		Currency:           entities.RUB,
	})
	if err != nil {
		t.Fatalf("CreatePlan() error = %v", err)
	}

	if plan.ID == uuid.Nil {
		t.Fatal("CreatePlan() returned zero ID")
	}

	if plan.Code != "starter" {
		t.Fatalf("CreatePlan() code = %q, want %q", plan.Code, "starter")
	}
}

func TestSubscribeCreatesPendingSubscription(t *testing.T) {
	t.Parallel()

	planStorage := newFakePlanStorage()
	subscriptionStorage := newFakeSubscriptionStorage()
	plan := newActivePlan()
	planStorage.plans[plan.ID] = plan
	paymentStore := &fakePaymentStorage{}
	paymentInfra := &fakePaymentInfra{
		createPaymentResult: entities.Payment{
			ID: "pay_subscription",
			Confirmation: entities.Confirmation{
				Type:            entities.ConfirmationTypeRedirect,
				ConfirmationURL: strPtr("http://example.com/checkout/subscription"),
			},
		},
	}
	service := NewService(planStorage, subscriptionStorage, nil, nil, paymentStore, paymentInfra)

	confirmationURL, err := service.Subscribe(context.Background(), commands.Subscribe{
		UserID: uuid.New(),
		PlanID: plan.ID,
		Now:    time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if confirmationURL != "http://example.com/checkout/subscription" {
		t.Fatalf("Subscribe() confirmationURL = %q", confirmationURL)
	}

	if len(subscriptionStorage.byUser) != 1 {
		t.Fatalf("subscription count = %d, want 1", len(subscriptionStorage.byUser))
	}

	for userID, subscription := range subscriptionStorage.byUser {
		if userID == uuid.Nil {
			t.Fatal("stored userID = nil")
		}
		if subscription.Status != entities.SubscriptionStatusPending {
			t.Fatalf("Subscribe() status = %q, want %q", subscription.Status, entities.SubscriptionStatusPending)
		}
	}
}

func TestApplySuccessfulTopUpIsIdempotentAndCreditsAmount(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	balanceStore := newFakeBalanceEntryStorage()
	balanceStore.balance[userID] = 1000
	service := NewService(nil, nil, balanceStore, &fakeUserStorage{
		users: map[uuid.UUID]entities.User{
			userID: {ID: userID, BalanceMinor: 1000},
		},
	}, &fakePaymentStorage{}, &fakePaymentInfra{})

	payment := entities.Payment{
		ID:      "pay_top_up",
		UserID:  userID,
		Purpose: entities.PaymentPurposeTopUp,
		Status:  entities.Succeeded,
		Paid:    true,
		Amount: entities.Amount{
			Value:    "250.50",
			Currency: entities.RUB,
		},
		CreatedAt: time.Now().UTC(),
	}

	if err := service.ApplySuccessfulTopUp(context.Background(), payment); err != nil {
		t.Fatalf("ApplySuccessfulTopUp() first error = %v", err)
	}

	if err := service.ApplySuccessfulTopUp(context.Background(), payment); err != nil {
		t.Fatalf("ApplySuccessfulTopUp() second error = %v", err)
	}

	if len(balanceStore.entries[userID]) != 1 {
		t.Fatalf("entry count = %d, want 1", len(balanceStore.entries[userID]))
	}

	if balanceStore.balance[userID] != 26050 {
		t.Fatalf("balance = %d, want %d", balanceStore.balance[userID], 26050)
	}
}

func TestApplySuccessfulSubscriptionPaymentCreditsPlanBalance(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	plan := newActivePlan()
	plan.BalanceCreditMinor = 200000

	planStorage := newFakePlanStorage()
	planStorage.plans[plan.ID] = plan

	subscription := entities.UserSubscription{
		ID:                 uuid.New(),
		UserID:             userID,
		PlanID:             plan.ID,
		Status:             entities.SubscriptionStatusActive,
		StartedAt:          time.Now().UTC(),
		CurrentPeriodStart: time.Now().UTC(),
		CurrentPeriodEnd:   time.Now().UTC().Add(30 * 24 * time.Hour),
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}

	subscriptionStore := newFakeSubscriptionStorage()
	subscriptionStore.byUser[userID] = subscription
	balanceStore := newFakeBalanceEntryStorage()

	payment := entities.Payment{
		ID:        "pay_subscription",
		UserID:    userID,
		Purpose:   entities.PaymentPurposeSubscription,
		Status:    entities.Succeeded,
		Paid:      true,
		CreatedAt: time.Now().UTC(),
	}

	service := NewService(planStorage, subscriptionStore, balanceStore, &fakeUserStorage{
		users: map[uuid.UUID]entities.User{
			userID: {ID: userID, BalanceMinor: 0},
		},
	}, &fakePaymentStorage{}, &fakePaymentInfra{
		captureResult: payment,
	})

	if err := service.ApplySuccessfulSubscriptionPayment(context.Background(), payment); err != nil {
		t.Fatalf("ApplySuccessfulSubscriptionPayment() error = %v", err)
	}

	if balanceStore.balance[userID] != 200000 {
		t.Fatalf("balance = %d, want %d", balanceStore.balance[userID], 200000)
	}
}

func TestChargeUsageReturnsInsufficientBalance(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	balanceStore := newFakeBalanceEntryStorage()
	balanceStore.balance[userID] = 100

	service := NewService(nil, nil, balanceStore, &fakeUserStorage{
		users: map[uuid.UUID]entities.User{
			userID: {ID: userID, BalanceMinor: 100},
		},
	}, &fakePaymentStorage{}, nil)

	_, err := service.ChargeUsage(context.Background(), commands.ChargeUsage{
		UserID:      userID,
		AmountMinor: 250,
		Description: "usage",
		Now:         time.Now().UTC(),
	})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("ChargeUsage() error = %v, want ErrInsufficientBalance", err)
	}
}

func TestHandleOpenRouterUsageWebhookChargesUsageOnce(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	balanceStore := newFakeBalanceEntryStorage()
	balanceStore.balance[userID] = 100
	service := NewService(nil, nil, balanceStore, &fakeUserStorage{
		users: map[uuid.UUID]entities.User{
			userID: {ID: userID, BalanceMinor: 100},
		},
	}, &fakePaymentStorage{}, nil)

	event := commands.OpenRouterUsageEvent{
		TraceID:    "trace-1",
		SpanID:     "span-1",
		APIKeyName: "snapclaw+" + userID.String(),
		Model:      "openai/gpt-4.1-mini",
		TotalCost:  "0.12",
		OccurredAt: time.Now().UTC(),
	}

	if err := service.HandleOpenRouterUsageWebhook(context.Background(), event); err != nil {
		t.Fatalf("HandleOpenRouterUsageWebhook() first error = %v", err)
	}

	if err := service.HandleOpenRouterUsageWebhook(context.Background(), event); err != nil {
		t.Fatalf("HandleOpenRouterUsageWebhook() second error = %v", err)
	}

	if balanceStore.balance[userID] != 88 {
		t.Fatalf("balance = %d, want 88", balanceStore.balance[userID])
	}
}

func TestHandleOpenRouterUsageWebhookRejectsMalformedAPIKeyName(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, newFakeBalanceEntryStorage(), &fakeUserStorage{
		users: map[uuid.UUID]entities.User{},
	}, &fakePaymentStorage{}, nil)

	err := service.HandleOpenRouterUsageWebhook(context.Background(), commands.OpenRouterUsageEvent{
		TraceID:    "trace-1",
		SpanID:     "span-1",
		APIKeyName: "snapclaw-without-user-id",
		TotalCost:  "0.12",
		OccurredAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("HandleOpenRouterUsageWebhook() error = nil, want non-nil")
	}
}

func TestHandleOpenRouterUsageWebhookReturnsInsufficientBalance(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	balanceStore := newFakeBalanceEntryStorage()
	balanceStore.balance[userID] = 5
	service := NewService(nil, nil, balanceStore, &fakeUserStorage{
		users: map[uuid.UUID]entities.User{
			userID: {ID: userID, BalanceMinor: 5},
		},
	}, &fakePaymentStorage{}, nil)

	err := service.HandleOpenRouterUsageWebhook(context.Background(), commands.OpenRouterUsageEvent{
		TraceID:    "trace-1",
		SpanID:     "span-1",
		APIKeyName: "snapclaw+" + userID.String(),
		TotalCost:  "0.12",
		OccurredAt: time.Now().UTC(),
	})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("HandleOpenRouterUsageWebhook() error = %v, want ErrInsufficientBalance", err)
	}
}

func TestGetBillingSummaryUsesLatestSubscriptionPaymentForNextCharge(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	plan := newActivePlan()
	planStorage := newFakePlanStorage()
	planStorage.plans[plan.ID] = plan

	subscription := entities.UserSubscription{
		ID:                 uuid.New(),
		UserID:             userID,
		PlanID:             plan.ID,
		Status:             entities.SubscriptionStatusActive,
		StartedAt:          time.Now().UTC(),
		CurrentPeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		CurrentPeriodEnd:   time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}

	lastPaymentTime := time.Date(2026, time.March, 10, 15, 0, 0, 0, time.UTC)
	paymentStore := &fakePaymentStorage{
		latestSucceeded: map[uuid.UUID]entities.Payment{
			userID: {
				ID:        "pay_subscription",
				UserID:    userID,
				Purpose:   entities.PaymentPurposeSubscription,
				Status:    entities.Succeeded,
				Paid:      true,
				CreatedAt: lastPaymentTime,
			},
		},
	}

	service := NewService(planStorage, &fakeSubscriptionStorage{
		byUser: map[uuid.UUID]entities.UserSubscription{userID: subscription},
	}, &fakeBalanceEntryStorage{
		balance: map[uuid.UUID]int64{userID: 5000},
	}, &fakeUserStorage{
		users: map[uuid.UUID]entities.User{userID: {ID: userID, BalanceMinor: 5000}},
	}, paymentStore, nil)

	summary, err := service.GetBillingSummary(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetBillingSummary() error = %v", err)
	}

	if summary.NextChargeAt == nil {
		t.Fatal("NextChargeAt = nil, want non-nil")
	}

	want := lastPaymentTime.AddDate(0, 1, 0)
	if !summary.NextChargeAt.Equal(want) {
		t.Fatalf("NextChargeAt = %s, want %s", summary.NextChargeAt, want)
	}
}

func newActivePlan() entities.Plan {
	now := time.Now().UTC()

	return entities.Plan{
		ID:                 uuid.New(),
		Code:               "starter",
		Name:               "Starter",
		BillingAmountMinor: 99000,
		BalanceCreditMinor: 150000,
		Currency:           entities.RUB,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

type fakePlanStorage struct {
	plans map[uuid.UUID]entities.Plan
}

func newFakePlanStorage() *fakePlanStorage {
	return &fakePlanStorage{plans: make(map[uuid.UUID]entities.Plan)}
}

func (s *fakePlanStorage) Create(_ context.Context, plan entities.Plan) error {
	s.plans[plan.ID] = plan
	return nil
}

func (s *fakePlanStorage) GetByID(_ context.Context, id uuid.UUID) (entities.Plan, error) {
	plan, ok := s.plans[id]
	if !ok {
		return entities.Plan{}, infraSQL.ErrNotFound
	}
	return plan, nil
}

func (s *fakePlanStorage) List(_ context.Context, includeInactive bool) ([]entities.Plan, error) {
	plans := make([]entities.Plan, 0, len(s.plans))
	for _, plan := range s.plans {
		if !includeInactive && !plan.IsActive {
			continue
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func (s *fakePlanStorage) Update(_ context.Context, plan entities.Plan) error {
	s.plans[plan.ID] = plan
	return nil
}

func (s *fakePlanStorage) Deactivate(_ context.Context, id uuid.UUID) error {
	plan, ok := s.plans[id]
	if !ok {
		return infraSQL.ErrNotFound
	}
	plan.IsActive = false
	s.plans[id] = plan
	return nil
}

type fakeSubscriptionStorage struct {
	byID   map[uuid.UUID]entities.UserSubscription
	byUser map[uuid.UUID]entities.UserSubscription
}

func newFakeSubscriptionStorage() *fakeSubscriptionStorage {
	return &fakeSubscriptionStorage{
		byID:   make(map[uuid.UUID]entities.UserSubscription),
		byUser: make(map[uuid.UUID]entities.UserSubscription),
	}
}

func (s *fakeSubscriptionStorage) Create(_ context.Context, subscription entities.UserSubscription) error {
	s.byID[subscription.ID] = subscription
	s.byUser[subscription.UserID] = subscription
	return nil
}

func (s *fakeSubscriptionStorage) GetByID(_ context.Context, id uuid.UUID) (entities.UserSubscription, error) {
	subscription, ok := s.byID[id]
	if !ok {
		return entities.UserSubscription{}, infraSQL.ErrNotFound
	}
	return subscription, nil
}

func (s *fakeSubscriptionStorage) GetActiveByUserID(_ context.Context, userID uuid.UUID) (entities.UserSubscription, error) {
	subscription, ok := s.byUser[userID]
	if !ok || subscription.Status != entities.SubscriptionStatusActive {
		return entities.UserSubscription{}, infraSQL.ErrNotFound
	}
	return subscription, nil
}

func (s *fakeSubscriptionStorage) Update(_ context.Context, subscription entities.UserSubscription) error {
	s.byID[subscription.ID] = subscription
	s.byUser[subscription.UserID] = subscription
	return nil
}

func (s *fakeSubscriptionStorage) Cancel(_ context.Context, id, userID uuid.UUID, canceledAt time.Time) error {
	subscription, ok := s.byID[id]
	if !ok || subscription.UserID != userID {
		return infraSQL.ErrNotFound
	}
	subscription.Status = entities.SubscriptionStatusCanceled
	subscription.CanceledAt = &canceledAt
	subscription.UpdatedAt = canceledAt
	s.byID[id] = subscription
	s.byUser[userID] = subscription
	return nil
}

type fakeBalanceEntryStorage struct {
	entries   map[uuid.UUID][]entities.UserBalanceEntry
	balance   map[uuid.UUID]int64
	processed map[string]entities.OpenRouterUsageEvent
}

func newFakeBalanceEntryStorage() *fakeBalanceEntryStorage {
	return &fakeBalanceEntryStorage{
		entries: make(map[uuid.UUID][]entities.UserBalanceEntry),
		balance: make(map[uuid.UUID]int64),
	}
}

func (s *fakeBalanceEntryStorage) ListByUserID(_ context.Context, userID uuid.UUID, _ int) ([]entities.UserBalanceEntry, error) {
	if s.entries == nil {
		return nil, nil
	}
	return append([]entities.UserBalanceEntry(nil), s.entries[userID]...), nil
}

func (s *fakeBalanceEntryStorage) ApplyCredit(_ context.Context, entry entities.UserBalanceEntry, _ entities.Payment) (int64, error) {
	if s.entries == nil {
		s.entries = make(map[uuid.UUID][]entities.UserBalanceEntry)
	}
	if s.balance == nil {
		s.balance = make(map[uuid.UUID]int64)
	}

	s.entries[entry.UserID] = append(s.entries[entry.UserID], entry)
	s.balance[entry.UserID] += entry.AmountMinor
	return s.balance[entry.UserID], nil
}

func (s *fakeBalanceEntryStorage) ApplyUsageDebit(_ context.Context, entry entities.UserBalanceEntry) (int64, error) {
	if s.balance[entry.UserID] < entry.AmountMinor {
		return 0, ErrInsufficientBalance
	}

	if s.entries == nil {
		s.entries = make(map[uuid.UUID][]entities.UserBalanceEntry)
	}
	if s.balance == nil {
		s.balance = make(map[uuid.UUID]int64)
	}

	s.entries[entry.UserID] = append(s.entries[entry.UserID], entry)
	s.balance[entry.UserID] -= entry.AmountMinor
	return s.balance[entry.UserID], nil
}

func (s *fakeBalanceEntryStorage) ApplyUsageDebitOnce(
	_ context.Context,
	entry entities.UserBalanceEntry,
	event entities.OpenRouterUsageEvent,
) (int64, bool, error) {
	if s.processed == nil {
		s.processed = make(map[string]entities.OpenRouterUsageEvent)
	}

	key := event.Provider + ":" + event.TraceID + ":" + event.SpanID
	if _, ok := s.processed[key]; ok {
		return s.balance[entry.UserID], false, nil
	}

	balance, err := s.ApplyUsageDebit(context.Background(), entry)
	if err != nil {
		return 0, false, err
	}

	s.processed[key] = event

	return balance, true, nil
}

type fakeUserStorage struct {
	users map[uuid.UUID]entities.User
}

func (s *fakeUserStorage) GetByID(_ context.Context, id uuid.UUID) (entities.User, error) {
	user, ok := s.users[id]
	if !ok {
		return entities.User{}, infraSQL.ErrNotFound
	}
	return user, nil
}

type fakePaymentStorage struct {
	latestSucceeded map[uuid.UUID]entities.Payment
	byID            map[string]entities.Payment
	byUser          map[uuid.UUID][]entities.Payment
}

func (s *fakePaymentStorage) GetLatestSucceededByPurpose(_ context.Context, userID uuid.UUID, _ entities.PaymentPurpose) (entities.Payment, error) {
	if s == nil || s.latestSucceeded == nil {
		return entities.Payment{}, infraSQL.ErrNotFound
	}

	payment, ok := s.latestSucceeded[userID]
	if !ok {
		return entities.Payment{}, infraSQL.ErrNotFound
	}

	return payment, nil
}

func (s *fakePaymentStorage) Create(_ context.Context, payment entities.Payment) error {
	if s.byID == nil {
		s.byID = make(map[string]entities.Payment)
	}
	if s.byUser == nil {
		s.byUser = make(map[uuid.UUID][]entities.Payment)
	}
	s.byID[payment.ID] = payment
	s.byUser[payment.UserID] = append(s.byUser[payment.UserID], payment)
	return nil
}

func (s *fakePaymentStorage) GetByID(_ context.Context, id string) (entities.Payment, error) {
	if s.byID == nil {
		return entities.Payment{}, infraSQL.ErrNotFound
	}
	payment, ok := s.byID[id]
	if !ok {
		return entities.Payment{}, infraSQL.ErrNotFound
	}
	return payment, nil
}

func (s *fakePaymentStorage) GetByUserID(_ context.Context, userID uuid.UUID) ([]entities.Payment, error) {
	if s.byUser == nil {
		return nil, nil
	}
	return append([]entities.Payment(nil), s.byUser[userID]...), nil
}

func (s *fakePaymentStorage) Update(_ context.Context, payment entities.Payment) error {
	if s.byID == nil {
		s.byID = make(map[string]entities.Payment)
	}
	s.byID[payment.ID] = payment
	return nil
}

func (s *fakePaymentStorage) Delete(_ context.Context, id string, userID uuid.UUID) error {
	if s.byID == nil {
		return infraSQL.ErrNotFound
	}
	payment, ok := s.byID[id]
	if !ok || payment.UserID != userID {
		return infraSQL.ErrNotFound
	}
	delete(s.byID, id)
	return nil
}

type fakePaymentInfra struct {
	createPaymentResult entities.Payment
	createPaymentErr    error
	captureResult       entities.Payment
	captureErr          error
}

func (f *fakePaymentInfra) CreatePayment(_ context.Context, amount entities.Amount) (entities.Payment, error) {
	if f.createPaymentErr != nil {
		return entities.Payment{}, f.createPaymentErr
	}

	result := f.createPaymentResult
	result.Amount = amount

	return result, nil
}

func (f *fakePaymentInfra) Capture(_ context.Context, payment *entities.Payment) (entities.Payment, error) {
	if f.captureErr != nil {
		return entities.Payment{}, f.captureErr
	}

	if f.captureResult.ID != "" {
		return f.captureResult, nil
	}

	return *payment, nil
}

func strPtr(v string) *string {
	return &v
}
