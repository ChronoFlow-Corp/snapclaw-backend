package billing

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"shared/pkg/observability"

	"simpleClaw/internal/entities"
	infraSQL "simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/billing/commands"
	"simpleClaw/internal/service/pkg/minor"
)

func TestCreatePlanWithMetrics(t *testing.T) {
	t.Parallel()

	reg := observability.NewPrometheusRegistry()
	metrics, err := observability.NewOperationMetrics(reg, "simpleclaw")
	if err != nil {
		t.Fatalf("NewOperationMetrics() error = %v", err)
	}

	store := newFakePlanStorage()
	service := newTestService(store, nil, nil, nil, nil, nil, metrics)

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
	service := newTestService(store, nil, nil, nil, nil, nil)

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
	service := newTestService(planStorage, subscriptionStorage, nil, nil, paymentStore, paymentInfra)

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

func TestSubscribeReturnsPlanInactive(t *testing.T) {
	t.Parallel()

	planStorage := newFakePlanStorage()
	subscriptionStorage := newFakeSubscriptionStorage()
	plan := newActivePlan()
	plan.IsActive = false
	planStorage.plans[plan.ID] = plan

	service := newTestService(
		planStorage,
		subscriptionStorage,
		nil,
		nil,
		&fakePaymentStorage{},
		&fakePaymentInfra{},
	)

	_, err := service.Subscribe(context.Background(), commands.Subscribe{
		UserID: uuid.New(),
		PlanID: plan.ID,
		Now:    time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, ErrPlanInactive) {
		t.Fatalf("Subscribe() error = %v, want ErrPlanInactive", err)
	}
}

func TestSubscribeIncludesPlanLoadStageInError(t *testing.T) {
	t.Parallel()

	service := newTestService(
		newFakePlanStorage(),
		newFakeSubscriptionStorage(),
		nil,
		nil,
		&fakePaymentStorage{},
		&fakePaymentInfra{},
	)

	_, err := service.Subscribe(context.Background(), commands.Subscribe{
		UserID: uuid.New(),
		PlanID: uuid.New(),
		Now:    time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("Subscribe() error = nil, want non-nil")
	}

	if !strings.Contains(err.Error(), "load plan") {
		t.Fatalf("Subscribe() error = %q, want to contain %q", err.Error(), "load plan")
	}
}

func TestApplySuccessfulTopUpIsIdempotentAndCreditsAmount(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	balanceStore := newFakeBalanceEntryStorage()
	balanceStore.balance[userID] = 1000
	service := newTestService(nil, nil, balanceStore, &fakeUserStorage{
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
		Status:             entities.SubscriptionStatusPending,
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
		ID:             "pay_subscription",
		UserID:         userID,
		Purpose:        entities.PaymentPurposeSubscription,
		Status:         entities.Succeeded,
		Paid:           true,
		SubscriptionID: &subscription.ID,
		CreatedAt:      time.Now().UTC(),
	}

	service := newTestService(planStorage, subscriptionStore, balanceStore, &fakeUserStorage{
		users: map[uuid.UUID]entities.User{
			userID: {ID: userID, BalanceMinor: 0},
		},
	}, &fakePaymentStorage{}, &fakePaymentInfra{
		captureResult: payment,
	})

	subscriptionStore.byID[subscription.ID] = subscription

	if err := service.ApplySuccessfulSubscriptionPayment(context.Background(), payment); err != nil {
		t.Fatalf("ApplySuccessfulSubscriptionPayment() error = %v", err)
	}

	if balanceStore.balance[userID] != 200000 {
		t.Fatalf("balance = %d, want %d", balanceStore.balance[userID], 200000)
	}
}

func TestHandleOpenRouterUsageWebhookChargesUsageOnce(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	balanceStore := newFakeBalanceEntryStorage()
	balanceStore.balance[userID] = 100
	service := newTestService(nil, nil, balanceStore, &fakeUserStorage{
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

	service := newTestService(nil, nil, newFakeBalanceEntryStorage(), &fakeUserStorage{
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

	if !errors.Is(err, ErrOpenRouterAPIKeyInvalid) {
		t.Fatalf("HandleOpenRouterUsageWebhook() error = %v, want ErrOpenRouterAPIKeyInvalid", err)
	}

	if !strings.Contains(err.Error(), "parse api key name") {
		t.Fatalf("HandleOpenRouterUsageWebhook() error = %q, want to contain %q", err.Error(), "parse api key name")
	}
}

func TestHandleOpenRouterUsageWebhookReturnsInsufficientBalance(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	balanceStore := newFakeBalanceEntryStorage()
	balanceStore.balance[userID] = 5
	service := newTestService(nil, nil, balanceStore, &fakeUserStorage{
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

	service := newTestService(planStorage, &fakeSubscriptionStorage{
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

func TestEventPaymentReturnsInvalidEventType(t *testing.T) {
	t.Parallel()

	service := newTestService(nil, nil, nil, nil, &fakePaymentStorage{}, &fakePaymentInfra{})

	err := service.EventPayment(context.Background(), commands.PaymentEvent{
		PaymentEvent: entities.PaymentEvent{
			Type:  "unexpected",
			Event: "payment.succeeded",
			Object: entities.Payment{
				ID: "pay_123",
			},
		},
	})
	if !errors.Is(err, ErrPaymentEventTypeInvalid) {
		t.Fatalf("EventPayment() error = %v, want ErrPaymentEventTypeInvalid", err)
	}
}

func TestBillingErrorClassificationPlanInactive(t *testing.T) {
	t.Parallel()

	planStorage := newFakePlanStorage()
	plan := newActivePlan()
	plan.IsActive = false
	planStorage.plans[plan.ID] = plan

	service := newTestService(
		planStorage,
		newFakeSubscriptionStorage(),
		nil,
		nil,
		&fakePaymentStorage{},
		&fakePaymentInfra{},
	)

	_, err := service.Subscribe(context.Background(), commands.Subscribe{
		UserID: uuid.New(),
		PlanID: plan.ID,
		Now:    time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("Subscribe() error = nil, want non-nil")
	}

	attrs := observability.ClassifyError(err)
	if attrs.Result != observability.ResultValidationError {
		t.Fatalf("result = %q, want %q", attrs.Result, observability.ResultValidationError)
	}

	if attrs.Source != observability.ErrorSourceService {
		t.Fatalf("source = %q, want %q", attrs.Source, observability.ErrorSourceService)
	}
}

func TestBillingErrorClassificationExternalPaymentFailure(t *testing.T) {
	t.Parallel()

	planStorage := newFakePlanStorage()
	plan := newActivePlan()
	planStorage.plans[plan.ID] = plan

	service := newTestService(
		planStorage,
		newFakeSubscriptionStorage(),
		nil,
		nil,
		&fakePaymentStorage{},
		&fakePaymentInfra{createPaymentErr: errors.New("provider down")},
	)

	_, err := service.Subscribe(context.Background(), commands.Subscribe{
		UserID: uuid.New(),
		PlanID: plan.ID,
		Now:    time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("Subscribe() error = nil, want non-nil")
	}

	attrs := observability.ClassifyError(err)
	if attrs.Source != observability.ErrorSourceExternal {
		t.Fatalf("source = %q, want %q", attrs.Source, observability.ErrorSourceExternal)
	}

	if attrs.Result != observability.ResultError {
		t.Fatalf("result = %q, want %q", attrs.Result, observability.ResultError)
	}
}

func TestExpanseAnalyzeAtReturnsZeroesForEmptyHistory(t *testing.T) {
	t.Parallel()

	service := newTestService(nil, nil, newFakeBalanceEntryStorage(), nil, nil, nil)

	got, err := service.expanseAnalyzeAt(
		context.Background(),
		commands.Expanse{UserID: uuid.New()},
		time.Date(2026, time.April, 3, 15, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("expanseAnalyzeAt() error = %v", err)
	}

	if got.Today != "0.00" {
		t.Fatalf("Today = %q, want %q", got.Today, "0.00")
	}
	if got.Day != "0.00" {
		t.Fatalf("Day = %q, want %q", got.Day, "0.00")
	}
	if got.Week != "0.00" {
		t.Fatalf("Week = %q, want %q", got.Week, "0.00")
	}
	if got.Month != "0.00" {
		t.Fatalf("Month = %q, want %q", got.Month, "0.00")
	}
}

func TestExpanseAnalyzeAtUsesUTCPeriodSums(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 3, 15, 0, 0, 0, time.UTC)
	userID := uuid.New()
	todayStart := testStartOfDayUTC(now)
	weekStart := testStartOfISOWeekUTC(now)
	monthStart := testStartOfMonthUTC(now)

	entries := []entities.UserBalanceEntry{
		newUsageEntry(userID, 5000, monthStart.AddDate(0, -12, 0).Add(-time.Minute)),
		newUsageEntry(userID, 12000, monthStart.AddDate(0, -12, 0)),
		newUsageEntry(userID, 800, weekStart.AddDate(0, 0, -56)),
		newUsageEntry(userID, 900, todayStart.AddDate(0, 0, -30)),
		newUsageEntry(userID, 700, weekStart),
		newUsageEntry(userID, 500, monthStart.AddDate(0, 0, 1).Add(12*time.Hour)),
		newUsageEntry(userID, 25, todayStart),
		newUsageEntry(userID, 275, todayStart.Add(10*time.Hour)),
		newUsageEntry(userID, 999, now),
	}

	service := newTestService(
		nil,
		nil,
		&fakeBalanceEntryStorage{
			entries: map[uuid.UUID][]entities.UserBalanceEntry{
				userID: entries,
			},
			balance: map[uuid.UUID]int64{userID: 100000},
		},
		nil,
		nil,
		nil,
	)

	got, err := service.expanseAnalyzeAt(context.Background(), commands.Expanse{UserID: userID}, now)
	if err != nil {
		t.Fatalf("expanseAnalyzeAt() error = %v", err)
	}

	dailyStart := todayStart.AddDate(0, 0, -30)
	weeklyStart := weekStart.AddDate(0, 0, -56)
	monthlyStart := monthStart.AddDate(0, -12, 0)

	wantToday := mustMinorString(t, sumEntriesInRange(entries, todayStart, now))
	wantDay := mustMinorString(t, sumEntriesInRange(entries, dailyStart, todayStart)/30)
	wantWeek := mustMinorString(t, sumEntriesInRange(entries, weeklyStart, weekStart)/8)
	wantMonth := mustMinorString(t, sumEntriesInRange(entries, monthlyStart, monthStart)/12)

	if got.Today != wantToday {
		t.Fatalf("Today = %q, want %q", got.Today, wantToday)
	}
	if got.Day != wantDay {
		t.Fatalf("Day = %q, want %q", got.Day, wantDay)
	}
	if got.Week != wantWeek {
		t.Fatalf("Week = %q, want %q", got.Week, wantWeek)
	}
	if got.Month != wantMonth {
		t.Fatalf("Month = %q, want %q", got.Month, wantMonth)
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

func (s *fakeBalanceEntryStorage) GetByPaymentID(
	_ context.Context,
	paymentID string,
) (entities.UserBalanceEntry, error) {
	for _, entries := range s.entries {
		for _, entry := range entries {
			if entry.PaymentID != nil && *entry.PaymentID == paymentID {
				return entry, nil
			}
		}
	}

	return entities.UserBalanceEntry{}, infraSQL.ErrNotFound
}

func (s *fakeBalanceEntryStorage) SumUsageDebitByUserIDInRange(
	_ context.Context,
	userID uuid.UUID,
	start, end time.Time,
) (int64, error) {
	return sumEntriesInRange(s.entries[userID], start, end), nil
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

func (f *fakePaymentInfra) CreatePayment(
	_ context.Context,
	amount entities.Amount,
	_ uuid.UUID,
	_ bool,
	_ *uuid.UUID,
) (entities.Payment, error) {
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

func (f *fakePaymentInfra) Cancel(_ context.Context, paymentID string) (entities.Payment, error) {
	if f.captureErr != nil {
		return entities.Payment{}, f.captureErr
	}

	if f.captureResult.ID != "" {
		return f.captureResult, nil
	}

	return entities.Payment{ID: paymentID}, nil
}

func strPtr(v string) *string {
	return &v
}

type fakeUsageAmountConverter struct{}

func (fakeUsageAmountConverter) ToMinor(totalCost string, _, targetCurrency string) (int64, error) {
	return minor.StringToMinor(totalCost, targetCurrency)
}

type noopKeyManager struct{}

func (noopKeyManager) DisableKey(context.Context, string) error {
	return nil
}

func newTestService(
	plans planStorage,
	subscriptions subscriptionStorage,
	balance balanceEntryStorage,
	users userStorage,
	payments paymentStorage,
	payInfra paymentInfra,
	metrics ...*observability.OperationMetrics,
) *Service {
	return NewService(
		"",
		plans,
		subscriptions,
		balance,
		users,
		payments,
		payInfra,
		fakeUsageAmountConverter{},
		nil,
		noopKeyManager{},
		metrics...,
	)
}

func newUsageEntry(userID uuid.UUID, amountMinor int64, createdAt time.Time) entities.UserBalanceEntry {
	return entities.UserBalanceEntry{
		ID:          uuid.New(),
		UserID:      userID,
		Type:        entities.BalanceEntryTypeUsageDebit,
		AmountMinor: amountMinor,
		CreatedAt:   createdAt,
	}
}

func testStartOfDayUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func testStartOfISOWeekUTC(t time.Time) time.Time {
	t = testStartOfDayUTC(t)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return t.AddDate(0, 0, -(weekday - 1))
}

func testStartOfMonthUTC(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func sumEntriesInRange(entries []entities.UserBalanceEntry, start, end time.Time) int64 {
	var total int64
	for _, entry := range entries {
		if entry.Type != entities.BalanceEntryTypeUsageDebit {
			continue
		}
		if entry.CreatedAt.Before(start) || !entry.CreatedAt.Before(end) {
			continue
		}
		total += entry.AmountMinor
	}
	return total
}

func mustMinorString(t *testing.T, amount int64) string {
	t.Helper()

	formatted, err := minor.MinorToString(amount, entities.RUB)
	if err != nil {
		t.Fatalf("MinorToString(%s) error = %v", strconv.FormatInt(amount, 10), err)
	}

	return formatted
}
