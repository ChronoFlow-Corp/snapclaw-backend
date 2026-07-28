// Package admin implements the control-plane read model for the back-office
// console: dashboard counters, the paginated users listing and per-user 360°
// detail. It is read-only and composes existing storages rather than owning new
// tables, so it never mutates user, billing or claw state.
package admin

import (
	"context"
	"fmt"
	"log/slog"

	"shared/pkg/observability"
	"simpleClaw/internal/entities"
	billingcommands "simpleClaw/internal/service/billing/commands"
	billingresult "simpleClaw/internal/service/billing/result"
	"simpleClaw/internal/service/pkg/minor"

	"github.com/google/uuid"
)

// balanceEntriesLimit caps how many ledger rows the balance tab pulls per user.
const balanceEntriesLimit = 200

// ClawView is a claw together with the capability ids currently enabled on it.
type ClawView struct {
	Claw         entities.Claw
	Capabilities []entities.CapabilityID
}

// Deps groups the read-only collaborators the admin read model composes.
type Deps struct {
	Stats          statsStorage
	Users          userReader
	Claws          clawReader
	Capabilities   capabilityReader
	Subscriptions  subscriptionReader
	Payments       paymentReader
	PaymentMethods paymentMethodReader
	Balance        balanceReader
	Integrations   integrationReader
	Telegram       telegramReader
	Usage          usageAnalyzer
}

type Service struct {
	deps    Deps
	metrics *observability.OperationMetrics
}

func New(deps Deps, metrics ...*observability.OperationMetrics) *Service {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Service{deps: deps, metrics: opMetrics}
}

func (s *Service) start(ctx context.Context, action string) (context.Context, func(error)) {
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "service.admin", action, "admin")

	return ctx, finish
}

// Counts returns the aggregate user counters for the dashboard overview.
func (s *Service) Counts(ctx context.Context) (counts entities.AdminUserCounts, err error) {
	const op = "service.admin.Counts"

	ctx, finish := s.start(ctx, "admin.counts")
	defer func() { finish(err) }()

	counts.Total, err = s.deps.Stats.CountUsers(ctx)
	if err != nil {
		return entities.AdminUserCounts{}, fmt.Errorf("%s: %w", op, err)
	}

	counts.Admins, err = s.deps.Stats.CountUsersByRole(ctx, entities.AdminRole)
	if err != nil {
		return entities.AdminUserCounts{}, fmt.Errorf("%s: %w", op, err)
	}

	counts.WithActiveSubscription, err = s.deps.Stats.CountUsersWithActiveSubscription(ctx)
	if err != nil {
		return entities.AdminUserCounts{}, fmt.Errorf("%s: %w", op, err)
	}

	counts.WithIssues, err = s.deps.Stats.CountUsersWithIssues(ctx)
	if err != nil {
		return entities.AdminUserCounts{}, fmt.Errorf("%s: %w", op, err)
	}

	return counts, nil
}

// ListUsers returns a filtered, sorted and paginated page of users.
func (s *Service) ListUsers(
	ctx context.Context,
	filter entities.AdminUserFilter,
) (page entities.AdminUserPage, err error) {
	const op = "service.admin.ListUsers"

	ctx, finish := s.start(ctx, "admin.list_users")
	defer func() { finish(err) }()

	items, total, err := s.deps.Stats.ListUsers(ctx, filter)
	if err != nil {
		return entities.AdminUserPage{}, fmt.Errorf("%s: %w", op, err)
	}

	return entities.AdminUserPage{
		Items:    items,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

// UserDetail returns the user plus their aggregated 360° summary.
func (s *Service) UserDetail(
	ctx context.Context,
	userID uuid.UUID,
) (user entities.User, summary entities.AdminUserSummary, err error) {
	const op = "service.admin.UserDetail"

	ctx, finish := s.start(ctx, "admin.user_detail")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return entities.User{}, entities.AdminUserSummary{}, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	user, err = s.deps.Users.GetByID(ctx, userID)
	if err != nil {
		return entities.User{}, entities.AdminUserSummary{}, fmt.Errorf("%s: %w", op, err)
	}

	claws, err := s.deps.Claws.GetByUserID(ctx, userID)
	if err != nil {
		return entities.User{}, entities.AdminUserSummary{}, fmt.Errorf("%s: %w", op, err)
	}

	summary.ClawsTotal = int64(len(claws))
	for _, cl := range claws {
		if cl.ObservedState == entities.ClawObservedStateRunning {
			summary.ClawsRunning++
		}

		if isClawInTrouble(cl) {
			summary.ClawsError++
		}
	}

	payments, err := s.deps.Payments.GetByUserID(ctx, userID)
	if err != nil {
		return entities.User{}, entities.AdminUserSummary{}, fmt.Errorf("%s: %w", op, err)
	}

	summary.PaymentsTotalMinor = paidPaymentsTotalMinor(payments)

	subscriptions, err := s.deps.Subscriptions.ListByUserID(ctx, userID)
	if err != nil {
		return entities.User{}, entities.AdminUserSummary{}, fmt.Errorf("%s: %w", op, err)
	}

	if chosen, ok := currentSubscription(subscriptions); ok {
		summary.SubscriptionStatus = chosen.Status
		periodEnd := chosen.CurrentPeriodEnd
		summary.CurrentPeriodEnd = &periodEnd
	}

	usage, err := s.deps.Usage.ExpanseAnalyze(ctx, billingcommands.Expanse{UserID: userID})
	if err != nil {
		return entities.User{}, entities.AdminUserSummary{}, fmt.Errorf("%s: %w", op, err)
	}

	summary.UsageMonth = usage.Month

	return user, summary, nil
}

// UserClaws returns the user's claws, each with its enabled capability ids.
func (s *Service) UserClaws(ctx context.Context, userID uuid.UUID) (views []ClawView, err error) {
	const op = "service.admin.UserClaws"

	ctx, finish := s.start(ctx, "admin.user_claws")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	claws, err := s.deps.Claws.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	views = make([]ClawView, 0, len(claws))
	for _, cl := range claws {
		capabilities, capErr := s.enabledCapabilities(ctx, cl.ID, userID)
		if capErr != nil {
			return nil, fmt.Errorf("%s: %w", op, capErr)
		}

		views = append(views, ClawView{Claw: cl, Capabilities: capabilities})
	}

	return views, nil
}

// Claw returns a single claw addressed by its id alone (admin is not scoped to an
// owner) plus its enabled capability ids.
func (s *Service) Claw(ctx context.Context, clawID uuid.UUID) (view ClawView, err error) {
	const op = "service.admin.Claw"

	ctx, finish := s.start(ctx, "admin.claw")
	defer func() { finish(err) }()

	if clawID == uuid.Nil {
		return ClawView{}, fmt.Errorf("%s: %w", op, ErrClawIDRequired)
	}

	claw, err := s.deps.Claws.GetBySystemID(ctx, clawID)
	if err != nil {
		return ClawView{}, fmt.Errorf("%s: %w", op, err)
	}

	capabilities, err := s.enabledCapabilities(ctx, claw.ID, claw.UserID)
	if err != nil {
		return ClawView{}, fmt.Errorf("%s: %w", op, err)
	}

	return ClawView{Claw: claw, Capabilities: capabilities}, nil
}

// UserSubscriptions returns the user's full subscription history, newest first.
func (s *Service) UserSubscriptions(
	ctx context.Context,
	userID uuid.UUID,
) (subs []entities.UserSubscription, err error) {
	const op = "service.admin.UserSubscriptions"

	ctx, finish := s.start(ctx, "admin.user_subscriptions")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	subs, err = s.deps.Subscriptions.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return subs, nil
}

// UserPayments returns the user's payments, newest first.
func (s *Service) UserPayments(ctx context.Context, userID uuid.UUID) (payments []entities.Payment, err error) {
	const op = "service.admin.UserPayments"

	ctx, finish := s.start(ctx, "admin.user_payments")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	payments, err = s.deps.Payments.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return payments, nil
}

// UserPaymentMethods returns the user's saved payment methods.
func (s *Service) UserPaymentMethods(
	ctx context.Context,
	userID uuid.UUID,
) (methods []entities.PaymentMethod, err error) {
	const op = "service.admin.UserPaymentMethods"

	ctx, finish := s.start(ctx, "admin.user_payment_methods")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	methods, err = s.deps.PaymentMethods.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return methods, nil
}

// UserBalanceEntries returns the user's most recent balance ledger entries.
func (s *Service) UserBalanceEntries(
	ctx context.Context,
	userID uuid.UUID,
) (entries []entities.UserBalanceEntry, err error) {
	const op = "service.admin.UserBalanceEntries"

	ctx, finish := s.start(ctx, "admin.user_balance_entries")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	entries, err = s.deps.Balance.ListByUserID(ctx, userID, balanceEntriesLimit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return entries, nil
}

// UserIntegrations returns the user's account integrations.
func (s *Service) UserIntegrations(
	ctx context.Context,
	userID uuid.UUID,
) (integrations []entities.AccountIntegration, err error) {
	const op = "service.admin.UserIntegrations"

	ctx, finish := s.start(ctx, "admin.user_integrations")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	integrations, err = s.deps.Integrations.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return integrations, nil
}

// UserTelegramBots returns the user's managed Telegram bots.
func (s *Service) UserTelegramBots(
	ctx context.Context,
	userID uuid.UUID,
) (bots []entities.TelegramManagedBot, err error) {
	const op = "service.admin.UserTelegramBots"

	ctx, finish := s.start(ctx, "admin.user_telegram_bots")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	bots, err = s.deps.Telegram.ListManagedBotsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return bots, nil
}

// UserUsage returns the user's usage aggregation (today/week/month/day).
func (s *Service) UserUsage(ctx context.Context, userID uuid.UUID) (usage billingresult.Expanses, err error) {
	const op = "service.admin.UserUsage"

	ctx, finish := s.start(ctx, "admin.user_usage")
	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return billingresult.Expanses{}, fmt.Errorf("%s: %w", op, ErrUserIDRequired)
	}

	usage, err = s.deps.Usage.ExpanseAnalyze(ctx, billingcommands.Expanse{UserID: userID})
	if err != nil {
		return billingresult.Expanses{}, fmt.Errorf("%s: %w", op, err)
	}

	return usage, nil
}

func (s *Service) enabledCapabilities(
	ctx context.Context,
	clawID, userID uuid.UUID,
) ([]entities.CapabilityID, error) {
	attachments, err := s.deps.Capabilities.ListByClawID(ctx, clawID, userID)
	if err != nil {
		return nil, err
	}

	capabilities := make([]entities.CapabilityID, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Enabled {
			capabilities = append(capabilities, attachment.CapabilityID)
		}
	}

	return capabilities, nil
}

func isClawInTrouble(cl entities.Claw) bool {
	return cl.ObservedState == entities.ClawObservedStateError ||
		cl.LifecycleStatus == entities.ClawLifecycleStatusFailed
}

// currentSubscription picks the subscription that best represents the user's
// standing: the active one if present, otherwise the most recent. This keeps the
// detail card's status consistent with the listing's has_active_subscription flag.
// The input is expected newest-first.
func currentSubscription(subscriptions []entities.UserSubscription) (entities.UserSubscription, bool) {
	if len(subscriptions) == 0 {
		return entities.UserSubscription{}, false
	}

	for _, subscription := range subscriptions {
		if subscription.Status == entities.SubscriptionStatusActive {
			return subscription, true
		}
	}

	return subscriptions[0], true
}

// paidPaymentsTotalMinor sums the minor-unit value of the user's paid payments.
// Amounts that fail to parse (e.g. an unknown currency) are skipped rather than
// failing the whole summary.
func paidPaymentsTotalMinor(payments []entities.Payment) int64 {
	var total int64

	for _, payment := range payments {
		if !payment.Paid {
			continue
		}

		value, err := minor.StringToMinor(payment.Amount.Value, payment.Amount.Currency)
		if err != nil {
			continue
		}

		total += value
	}

	return total
}
