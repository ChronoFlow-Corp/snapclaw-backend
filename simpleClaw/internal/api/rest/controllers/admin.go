package controllers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"shared/pkg/jwt"
	"shared/pkg/response"
	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/api/rest/middleware"
	"simpleClaw/internal/entities"
	adminservice "simpleClaw/internal/service/admin"
	billingresult "simpleClaw/internal/service/billing/result"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	adminUsersDefaultPageSize = 20
	adminUsersMaxPageSize     = 100
)

var (
	errInvalidBoolFilter = &adminFilterError{message: "invalid boolean filter value"}
	errInvalidTimeFilter = &adminFilterError{message: "invalid date filter value; use RFC3339 or YYYY-MM-DD"}
)

type adminFilterError struct {
	message string
}

func (e *adminFilterError) Error() string { return e.message }

type adminReadService interface {
	Counts(ctx context.Context) (entities.AdminUserCounts, error)
	ListUsers(ctx context.Context, filter entities.AdminUserFilter) (entities.AdminUserPage, error)
	UserDetail(ctx context.Context, userID uuid.UUID) (entities.User, entities.AdminUserSummary, error)
	UserClaws(ctx context.Context, userID uuid.UUID) ([]adminservice.ClawView, error)
	UserSubscriptions(ctx context.Context, userID uuid.UUID) ([]entities.UserSubscription, error)
	UserPayments(ctx context.Context, userID uuid.UUID) ([]entities.Payment, error)
	UserPaymentMethods(ctx context.Context, userID uuid.UUID) ([]entities.PaymentMethod, error)
	UserBalanceEntries(ctx context.Context, userID uuid.UUID) ([]entities.UserBalanceEntry, error)
	UserIntegrations(ctx context.Context, userID uuid.UUID) ([]entities.AccountIntegration, error)
	UserTelegramBots(ctx context.Context, userID uuid.UUID) ([]entities.TelegramManagedBot, error)
	UserUsage(ctx context.Context, userID uuid.UUID) (billingresult.Expanses, error)
	Claw(ctx context.Context, clawID uuid.UUID) (adminservice.ClawView, error)
}

// Admin serves the back-office read API. Every route is gated by AuthJwt +
// AdminOnly, so only users whose role is admin can reach it.
type Admin struct {
	service adminReadService
	users   adminService
	j       jwt.JWT
}

func NewAdmin(service adminReadService, users adminService, j jwt.JWT) *Admin {
	return &Admin{
		service: service,
		users:   users,
		j:       j,
	}
}

func (a *Admin) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthJwt(a.j))
		r.Use(middleware.AdminOnly(a.users))

		r.Get("/admin/users", a.ListUsers)
		r.Get("/admin/users/counts", a.UserCounts)
		r.Get("/admin/users/{id}", a.UserDetail)
		r.Get("/admin/users/{id}/claws", a.UserClaws)
		r.Get("/admin/users/{id}/subscription", a.UserSubscriptions)
		r.Get("/admin/users/{id}/payments", a.UserPayments)
		r.Get("/admin/users/{id}/payment-methods", a.UserPaymentMethods)
		r.Get("/admin/users/{id}/balance-entries", a.UserBalanceEntries)
		r.Get("/admin/users/{id}/integrations", a.UserIntegrations)
		r.Get("/admin/users/{id}/telegram-bots", a.UserTelegramBots)
		r.Get("/admin/users/{id}/usage", a.UserUsage)

		r.Get("/admin/claws/{id}", a.Claw)
	})
}

func (a *Admin) UserCounts(w http.ResponseWriter, r *http.Request) {
	counts, err := a.service.Counts(r.Context())
	if err != nil {
		respondServiceError(w, err)

		return
	}

	response.RespondOK(w, dto.AdminUserCountsResponse{
		Total:                  counts.Total,
		WithActiveSubscription: counts.WithActiveSubscription,
		WithIssues:             counts.WithIssues,
		Admins:                 counts.Admins,
	})
}

func (a *Admin) ListUsers(w http.ResponseWriter, r *http.Request) {
	filter, err := parseAdminUserFilter(r)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})

		return
	}

	page, err := a.service.ListUsers(r.Context(), filter)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	items := make([]dto.AdminUserListItemResponse, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, adminUserListItemResponse(item))
	}

	response.RespondOK(w, dto.AdminUserListResponse{
		Items:    items,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	})
}

func (a *Admin) UserDetail(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	user, summary, err := a.service.UserDetail(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	response.RespondOK(w, dto.AdminUserDetailResponse{
		ID:           user.ID.String(),
		Email:        user.Email,
		Name:         user.Name,
		Nickname:     user.Nickname,
		AvatarURL:    user.AvatarURL,
		Role:         user.Role,
		BalanceMinor: user.BalanceMinor,
		HasIssues:    summary.ClawsError > 0,
		CreatedAt:    user.CreatedAt,
		Summary:      adminUserSummaryResponse(summary),
	})
}

func (a *Admin) UserClaws(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	views, err := a.service.UserClaws(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	resp := make([]dto.AdminClawResponse, 0, len(views))
	for _, view := range views {
		resp = append(resp, adminClawResponse(view))
	}

	response.RespondOK(w, resp)
}

func (a *Admin) UserSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	subscriptions, err := a.service.UserSubscriptions(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	resp := make([]dto.SubscriptionResponse, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		resp = append(resp, subscriptionToResponse(subscription))
	}

	response.RespondOK(w, resp)
}

func (a *Admin) UserPayments(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	payments, err := a.service.UserPayments(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	resp := make([]dto.AdminPaymentResponse, 0, len(payments))
	for _, payment := range payments {
		resp = append(resp, adminPaymentResponse(payment))
	}

	response.RespondOK(w, resp)
}

func (a *Admin) UserPaymentMethods(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	methods, err := a.service.UserPaymentMethods(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	resp := make([]dto.PaymentMethodResponse, 0, len(methods))
	for _, method := range methods {
		resp = append(resp, paymentMethodResponse(method))
	}

	response.RespondOK(w, resp)
}

func (a *Admin) UserBalanceEntries(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	entries, err := a.service.UserBalanceEntries(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	resp := make([]dto.AdminBalanceEntryResponse, 0, len(entries))
	for _, entry := range entries {
		resp = append(resp, adminBalanceEntryResponse(entry))
	}

	response.RespondOK(w, resp)
}

func (a *Admin) UserIntegrations(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	integrations, err := a.service.UserIntegrations(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	resp := make([]dto.AdminIntegrationResponse, 0, len(integrations))
	for _, integration := range integrations {
		resp = append(resp, adminIntegrationResponse(integration))
	}

	response.RespondOK(w, resp)
}

func (a *Admin) UserTelegramBots(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	bots, err := a.service.UserTelegramBots(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	resp := make([]dto.AdminTelegramBotResponse, 0, len(bots))
	for _, bot := range bots {
		resp = append(resp, adminTelegramBotResponse(bot))
	}

	response.RespondOK(w, resp)
}

func (a *Admin) UserUsage(w http.ResponseWriter, r *http.Request) {
	userID, ok := adminPathID(w, r, "invalid user id")
	if !ok {
		return
	}

	usage, err := a.service.UserUsage(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	response.RespondOK(w, dto.ExpanseAnalyzeResponse{
		Today:   usage.Today,
		Weekly:  usage.Week,
		Monthly: usage.Month,
		Daily:   usage.Day,
	})
}

func (a *Admin) Claw(w http.ResponseWriter, r *http.Request) {
	clawID, ok := adminPathID(w, r, "invalid claw id")
	if !ok {
		return
	}

	view, err := a.service.Claw(r.Context(), clawID)
	if err != nil {
		respondServiceError(w, err)

		return
	}

	response.RespondOK(w, adminClawResponse(view))
}

func adminPathID(w http.ResponseWriter, r *http.Request, message string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: message,
		})

		return uuid.Nil, false
	}

	return id, true
}

func parseAdminUserFilter(r *http.Request) (entities.AdminUserFilter, error) {
	query := r.URL.Query()

	filter := entities.AdminUserFilter{
		Page:     parsePositiveInt(query.Get("page"), 1),
		PageSize: clampPageSize(parsePositiveInt(query.Get("page_size"), adminUsersDefaultPageSize)),
		Sort:     parseAdminUserSort(query.Get("sort")),
		Query:    strings.TrimSpace(query.Get("q")),
		Role:     strings.TrimSpace(query.Get("role")),
	}

	hasActiveSubscription, err := parseOptionalBool(query.Get("has_active_subscription"))
	if err != nil {
		return entities.AdminUserFilter{}, err
	}

	filter.HasActiveSubscription = hasActiveSubscription

	hasIssues, err := parseOptionalBool(query.Get("has_issues"))
	if err != nil {
		return entities.AdminUserFilter{}, err
	}

	filter.HasIssues = hasIssues

	registeredFrom, err := parseTimeBound(query.Get("registered_from"), false)
	if err != nil {
		return entities.AdminUserFilter{}, err
	}

	filter.RegisteredFrom = registeredFrom

	registeredTo, err := parseTimeBound(query.Get("registered_to"), true)
	if err != nil {
		return entities.AdminUserFilter{}, err
	}

	filter.RegisteredTo = registeredTo

	return filter, nil
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}

	return value
}

func clampPageSize(size int) int {
	if size > adminUsersMaxPageSize {
		return adminUsersMaxPageSize
	}

	return size
}

func parseAdminUserSort(raw string) entities.AdminUserSort {
	switch entities.AdminUserSort(raw) {
	case entities.AdminUserSortCreatedAtAsc,
		entities.AdminUserSortBalanceDesc,
		entities.AdminUserSortBalanceAsc,
		entities.AdminUserSortEmailAsc:
		return entities.AdminUserSort(raw)
	default:
		return entities.AdminUserSortCreatedAtDesc
	}
}

func parseOptionalBool(raw string) (*bool, error) {
	if raw == "" {
		return nil, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, errInvalidBoolFilter
	}

	return &value, nil
}

// parseTimeBound parses an RFC3339 timestamp or a bare YYYY-MM-DD date. When a
// date-only value is used as an inclusive upper bound (endOfDayIfDateOnly), it is
// rolled to the last instant of that day so the whole day is included by `<=`.
func parseTimeBound(raw string, endOfDayIfDateOnly bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return &parsed, nil
	}

	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDayIfDateOnly {
			parsed = parsed.Add(24*time.Hour - time.Nanosecond)
		}

		return &parsed, nil
	}

	return nil, errInvalidTimeFilter
}

func adminUserListItemResponse(item entities.AdminUserListItem) dto.AdminUserListItemResponse {
	return dto.AdminUserListItemResponse{
		ID:                    item.ID.String(),
		Email:                 item.Email,
		Name:                  item.Name,
		Nickname:              item.Nickname,
		AvatarURL:             item.AvatarURL,
		Role:                  item.Role,
		BalanceMinor:          item.BalanceMinor,
		HasActiveSubscription: item.HasActiveSubscription,
		ClawsCount:            item.ClawsCount,
		HasIssues:             item.HasIssues,
		CreatedAt:             item.CreatedAt,
	}
}

func adminUserSummaryResponse(summary entities.AdminUserSummary) dto.AdminUserSummaryResponse {
	resp := dto.AdminUserSummaryResponse{
		ClawsTotal:         summary.ClawsTotal,
		ClawsRunning:       summary.ClawsRunning,
		ClawsError:         summary.ClawsError,
		CurrentPeriodEnd:   summary.CurrentPeriodEnd,
		PaymentsTotalMinor: summary.PaymentsTotalMinor,
		UsageMonth:         summary.UsageMonth,
	}

	if summary.SubscriptionStatus != "" {
		status := summary.SubscriptionStatus
		resp.SubscriptionStatus = &status
	}

	return resp
}

func adminClawResponse(view adminservice.ClawView) dto.AdminClawResponse {
	capabilities := make([]string, 0, len(view.Capabilities))
	for _, capability := range view.Capabilities {
		capabilities = append(capabilities, string(capability))
	}

	resp := dto.AdminClawResponse{
		ClawResponse: clawResponse(view.Claw),
		Capabilities: capabilities,
		CreatedAt:    view.Claw.CreatedAt,
	}

	if view.Claw.ServerID != uuid.Nil {
		resp.ServerID = view.Claw.ServerID.String()
	}

	return resp
}

func adminPaymentResponse(payment entities.Payment) dto.AdminPaymentResponse {
	return dto.AdminPaymentResponse{
		ID:          payment.ID,
		Purpose:     string(payment.Purpose),
		Status:      string(payment.Status),
		Paid:        payment.Paid,
		Amount:      payment.Amount.Value,
		Currency:    payment.Amount.Currency,
		Description: payment.Description,
		CreatedAt:   payment.CreatedAt,
	}
}

func adminBalanceEntryResponse(entry entities.UserBalanceEntry) dto.AdminBalanceEntryResponse {
	return dto.AdminBalanceEntryResponse{
		ID:          entry.ID.String(),
		Type:        entry.Type,
		AmountMinor: entry.AmountMinor,
		Description: entry.Description,
		CreatedAt:   entry.CreatedAt,
	}
}

func adminIntegrationResponse(integration entities.AccountIntegration) dto.AdminIntegrationResponse {
	return dto.AdminIntegrationResponse{
		ID:                integration.ID.String(),
		Capability:        string(integration.CapabilityID),
		Provider:          integration.Provider,
		ExternalAccountID: integration.ExternalAccountID,
		DisplayName:       integration.DisplayName,
		Status:            string(integration.Status),
		CreatedAt:         integration.CreatedAt,
	}
}

func adminTelegramBotResponse(bot entities.TelegramManagedBot) dto.AdminTelegramBotResponse {
	resp := dto.AdminTelegramBotResponse{
		ID:        bot.ID.String(),
		Status:    string(bot.Status),
		LastError: bot.LastError,
		CreatedAt: bot.CreatedAt,
	}

	if bot.ManagedBotUsername != "" {
		username := bot.ManagedBotUsername
		resp.ManagedBotUsername = &username
	}

	if bot.ChannelID != uuid.Nil {
		channelID := bot.ChannelID.String()
		resp.ChannelID = &channelID
	}

	return resp
}
