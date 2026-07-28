package admin

import (
	"context"
	"testing"
	"time"

	"simpleClaw/internal/entities"
	billingcommands "simpleClaw/internal/service/billing/commands"
	billingresult "simpleClaw/internal/service/billing/result"

	"github.com/google/uuid"
)

type stubStats struct {
	counts    entities.AdminUserCounts
	listItems []entities.AdminUserListItem
	listTotal int64
}

func (s stubStats) CountUsers(context.Context) (int64, error) { return s.counts.Total, nil }
func (s stubStats) CountUsersByRole(context.Context, string) (int64, error) {
	return s.counts.Admins, nil
}

func (s stubStats) CountUsersWithActiveSubscription(context.Context) (int64, error) {
	return s.counts.WithActiveSubscription, nil
}

func (s stubStats) CountUsersWithIssues(context.Context) (int64, error) {
	return s.counts.WithIssues, nil
}

func (s stubStats) ListUsers(
	context.Context,
	entities.AdminUserFilter,
) ([]entities.AdminUserListItem, int64, error) {
	return s.listItems, s.listTotal, nil
}

type stubUsers struct{ user entities.User }

func (s stubUsers) GetByID(context.Context, uuid.UUID) (entities.User, error) { return s.user, nil }

type stubClaws struct {
	list []entities.Claw
	one  entities.Claw
}

func (s stubClaws) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return s.list, nil
}
func (s stubClaws) GetBySystemID(context.Context, uuid.UUID) (entities.Claw, error) {
	return s.one, nil
}

type stubCaps struct {
	atts []entities.ClawCapabilityAttachment
}

func (s stubCaps) ListByClawID(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) ([]entities.ClawCapabilityAttachment, error) {
	return s.atts, nil
}

type stubSubs struct{ list []entities.UserSubscription }

func (s stubSubs) ListByUserID(context.Context, uuid.UUID) ([]entities.UserSubscription, error) {
	return s.list, nil
}

type stubPayments struct{ list []entities.Payment }

func (s stubPayments) GetByUserID(context.Context, uuid.UUID) ([]entities.Payment, error) {
	return s.list, nil
}

type stubMethods struct{ list []entities.PaymentMethod }

func (s stubMethods) GetByUserID(context.Context, uuid.UUID) ([]entities.PaymentMethod, error) {
	return s.list, nil
}

type stubBalance struct{ list []entities.UserBalanceEntry }

func (s stubBalance) ListByUserID(context.Context, uuid.UUID, int) ([]entities.UserBalanceEntry, error) {
	return s.list, nil
}

type stubIntegrations struct{ list []entities.AccountIntegration }

func (s stubIntegrations) ListByUserID(context.Context, uuid.UUID) ([]entities.AccountIntegration, error) {
	return s.list, nil
}

type stubTelegram struct{ list []entities.TelegramManagedBot }

func (s stubTelegram) ListManagedBotsByUserID(
	context.Context,
	uuid.UUID,
) ([]entities.TelegramManagedBot, error) {
	return s.list, nil
}

type stubUsage struct{ res billingresult.Expanses }

func (s stubUsage) ExpanseAnalyze(
	context.Context,
	billingcommands.Expanse,
) (billingresult.Expanses, error) {
	return s.res, nil
}

func clawWith(observed entities.ClawObservedState, lifecycle entities.ClawLifecycleStatus) entities.Claw {
	return entities.Claw{
		ID: uuid.New(),
		ClawLifecycleState: entities.ClawLifecycleState{
			ObservedState:   observed,
			LifecycleStatus: lifecycle,
		},
	}
}

func TestCounts(t *testing.T) {
	svc := New(Deps{Stats: stubStats{counts: entities.AdminUserCounts{
		Total:                  42,
		WithActiveSubscription: 10,
		WithIssues:             3,
		Admins:                 2,
	}}})

	got, err := svc.Counts(context.Background())
	if err != nil {
		t.Fatalf("Counts() error = %v", err)
	}

	want := entities.AdminUserCounts{Total: 42, WithActiveSubscription: 10, WithIssues: 3, Admins: 2}
	if got != want {
		t.Fatalf("Counts() = %+v, want %+v", got, want)
	}
}

func TestUserDetailSummary(t *testing.T) {
	userID := uuid.New()
	periodEnd := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	svc := New(Deps{
		Stats: stubStats{},
		Users: stubUsers{user: entities.User{ID: userID, Email: "a@b.c", Role: entities.UserRole}},
		Claws: stubClaws{list: []entities.Claw{
			clawWith(entities.ClawObservedStateRunning, entities.ClawLifecycleStatusIdle),
			clawWith(entities.ClawObservedStateError, entities.ClawLifecycleStatusIdle),
			clawWith(entities.ClawObservedStateStopped, entities.ClawLifecycleStatusFailed),
			clawWith(entities.ClawObservedStateStopped, entities.ClawLifecycleStatusIdle),
		}},
		Payments: stubPayments{list: []entities.Payment{
			{Paid: true, Amount: entities.Amount{Value: "100.00", Currency: entities.RUB}},
			{Paid: false, Amount: entities.Amount{Value: "999.00", Currency: entities.RUB}},
			{Paid: true, Amount: entities.Amount{Value: "0.50", Currency: entities.RUB}},
		}},
		// Newest-first: a newer canceled row plus an older active row. The summary
		// must prefer the active one (consistent with has_active_subscription).
		Subscriptions: stubSubs{list: []entities.UserSubscription{
			{Status: entities.SubscriptionStatusCanceled},
			{Status: entities.SubscriptionStatusActive, CurrentPeriodEnd: periodEnd},
		}},
		Usage: stubUsage{res: billingresult.Expanses{Month: "123.45"}},
	})

	_, summary, err := svc.UserDetail(context.Background(), userID)
	if err != nil {
		t.Fatalf("UserDetail() error = %v", err)
	}

	if summary.ClawsTotal != 4 {
		t.Errorf("ClawsTotal = %d, want 4", summary.ClawsTotal)
	}
	if summary.ClawsRunning != 1 {
		t.Errorf("ClawsRunning = %d, want 1", summary.ClawsRunning)
	}
	if summary.ClawsError != 2 {
		t.Errorf("ClawsError = %d, want 2 (one error observed_state + one failed lifecycle)", summary.ClawsError)
	}
	if summary.PaymentsTotalMinor != 10050 {
		t.Errorf("PaymentsTotalMinor = %d, want 10050 (paid only)", summary.PaymentsTotalMinor)
	}
	if summary.SubscriptionStatus != entities.SubscriptionStatusActive {
		t.Errorf("SubscriptionStatus = %q, want %q (prefer active)", summary.SubscriptionStatus, entities.SubscriptionStatusActive)
	}
	if summary.CurrentPeriodEnd == nil || !summary.CurrentPeriodEnd.Equal(periodEnd) {
		t.Errorf("CurrentPeriodEnd = %v, want %v", summary.CurrentPeriodEnd, periodEnd)
	}
	if summary.UsageMonth != "123.45" {
		t.Errorf("UsageMonth = %q, want %q", summary.UsageMonth, "123.45")
	}
}

func TestUserDetailNoSubscription(t *testing.T) {
	userID := uuid.New()

	svc := New(Deps{
		Users:         stubUsers{user: entities.User{ID: userID}},
		Claws:         stubClaws{},
		Payments:      stubPayments{},
		Subscriptions: stubSubs{},
		Usage:         stubUsage{},
	})

	_, summary, err := svc.UserDetail(context.Background(), userID)
	if err != nil {
		t.Fatalf("UserDetail() error = %v", err)
	}

	if summary.SubscriptionStatus != "" {
		t.Errorf("SubscriptionStatus = %q, want empty", summary.SubscriptionStatus)
	}
	if summary.CurrentPeriodEnd != nil {
		t.Errorf("CurrentPeriodEnd = %v, want nil", summary.CurrentPeriodEnd)
	}
}

func TestUserDetailRequiresID(t *testing.T) {
	svc := New(Deps{})

	if _, _, err := svc.UserDetail(context.Background(), uuid.Nil); err == nil {
		t.Fatal("UserDetail(nil) expected error, got nil")
	}
}

func TestUserClawsEnabledCapabilitiesOnly(t *testing.T) {
	clawID := uuid.New()

	svc := New(Deps{
		Claws: stubClaws{list: []entities.Claw{{ID: clawID}}},
		Capabilities: stubCaps{atts: []entities.ClawCapabilityAttachment{
			{CapabilityID: entities.CapabilityWebSearch, Enabled: true},
			{CapabilityID: entities.CapabilityGmail, Enabled: false},
			{CapabilityID: entities.CapabilityMemory, Enabled: true},
		}},
	})

	views, err := svc.UserClaws(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("UserClaws() error = %v", err)
	}

	if len(views) != 1 {
		t.Fatalf("len(views) = %d, want 1", len(views))
	}

	got := views[0].Capabilities
	if len(got) != 2 {
		t.Fatalf("capabilities = %v, want 2 enabled", got)
	}
	if got[0] != entities.CapabilityWebSearch || got[1] != entities.CapabilityMemory {
		t.Errorf("capabilities = %v, want [web_search memory]", got)
	}
}

func TestUserSubResourcePassthrough(t *testing.T) {
	userID := uuid.New()
	ctx := context.Background()

	svc := New(Deps{
		Subscriptions:  stubSubs{list: []entities.UserSubscription{{ID: uuid.New()}}},
		Payments:       stubPayments{list: []entities.Payment{{ID: "p1"}}},
		PaymentMethods: stubMethods{list: []entities.PaymentMethod{{ID: uuid.New()}}},
		Balance:        stubBalance{list: []entities.UserBalanceEntry{{ID: uuid.New()}, {ID: uuid.New()}}},
		Integrations:   stubIntegrations{list: []entities.AccountIntegration{{ID: uuid.New()}}},
		Telegram:       stubTelegram{list: []entities.TelegramManagedBot{{ID: uuid.New()}}},
		Usage:          stubUsage{res: billingresult.Expanses{Today: "1", Week: "2", Month: "3", Day: "4"}},
	})

	if subs, err := svc.UserSubscriptions(ctx, userID); err != nil || len(subs) != 1 {
		t.Errorf("UserSubscriptions => len=%d err=%v, want 1/nil", len(subs), err)
	}
	if payments, err := svc.UserPayments(ctx, userID); err != nil || len(payments) != 1 {
		t.Errorf("UserPayments => len=%d err=%v, want 1/nil", len(payments), err)
	}
	if methods, err := svc.UserPaymentMethods(ctx, userID); err != nil || len(methods) != 1 {
		t.Errorf("UserPaymentMethods => len=%d err=%v, want 1/nil", len(methods), err)
	}
	if entries, err := svc.UserBalanceEntries(ctx, userID); err != nil || len(entries) != 2 {
		t.Errorf("UserBalanceEntries => len=%d err=%v, want 2/nil", len(entries), err)
	}
	if integrations, err := svc.UserIntegrations(ctx, userID); err != nil || len(integrations) != 1 {
		t.Errorf("UserIntegrations => len=%d err=%v, want 1/nil", len(integrations), err)
	}
	if bots, err := svc.UserTelegramBots(ctx, userID); err != nil || len(bots) != 1 {
		t.Errorf("UserTelegramBots => len=%d err=%v, want 1/nil", len(bots), err)
	}
	if usage, err := svc.UserUsage(ctx, userID); err != nil || usage.Month != "3" {
		t.Errorf("UserUsage => %+v err=%v, want Month=3", usage, err)
	}

	// Every per-user method must reject a nil user id.
	if _, err := svc.UserPayments(ctx, uuid.Nil); err == nil {
		t.Error("UserPayments(nil) expected error")
	}
}

func TestPaidPaymentsTotalMinorSkipsUnpaidAndUnparseable(t *testing.T) {
	got := paidPaymentsTotalMinor([]entities.Payment{
		{Paid: true, Amount: entities.Amount{Value: "100.00", Currency: entities.RUB}},
		{Paid: false, Amount: entities.Amount{Value: "50.00", Currency: entities.RUB}},
		{Paid: true, Amount: entities.Amount{Value: "0.50", Currency: entities.RUB}},
		{Paid: true, Amount: entities.Amount{Value: "oops", Currency: entities.RUB}},
		{Paid: true, Amount: entities.Amount{Value: "5.00", Currency: "X"}},
	})

	if got != 10050 {
		t.Fatalf("paidPaymentsTotalMinor = %d, want 10050 (paid+parseable only)", got)
	}
}
