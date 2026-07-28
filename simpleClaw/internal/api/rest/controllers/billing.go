package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"shared/pkg/jwt"
	"shared/pkg/response"
	"sort"
	"strconv"
	"strings"
	"time"

	"simpleClaw/internal/service/billing/result"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/api/rest/middleware"
	"simpleClaw/internal/entities"
	billingsvc "simpleClaw/internal/service/billing"
	"simpleClaw/internal/service/billing/commands"
)

type billingService interface {
	CreatePlan(ctx context.Context, cmd commands.CreatePlan) (entities.Plan, error)
	GetPlan(ctx context.Context, id uuid.UUID) (entities.Plan, error)
	ListPlans(ctx context.Context, includeInactive bool) ([]entities.Plan, error)
	UpdatePlan(ctx context.Context, cmd commands.UpdatePlan) (entities.Plan, error)
	DeactivatePlan(ctx context.Context, id uuid.UUID) error
	Subscribe(ctx context.Context, cmd commands.Subscribe) (result.SubscriptionCheckout, error)
	GetSubscriptionCheckoutStatus(
		ctx context.Context,
		userID, subscriptionID uuid.UUID,
	) (result.SubscriptionCheckoutStatus, error)
	ChangePlan(ctx context.Context, cmd commands.ChangePlan) (entities.UserSubscription, error)
	CancelSubscription(ctx context.Context, cmd commands.CancelSubscription) error
	GetCurrentSubscription(
		ctx context.Context,
		userID uuid.UUID,
	) (*billingsvc.SubscriptionSummary, error)
	GetBillingSummary(ctx context.Context, userID uuid.UUID) (billingsvc.BillingSummary, error)
	GetBootstrap(ctx context.Context, userID uuid.UUID) (result.Bootstrap, error)
	EventPayment(ctx context.Context, cm commands.PaymentEvent) (err error)
	HandleOpenRouterUsageWebhook(ctx context.Context, event commands.OpenRouterUsageEvent) error
	TopUp(ctx context.Context, cm commands.TopUp) (confirmURL string, err error)
	ExpanseAnalyze(
		ctx context.Context,
		cm commands.Expanse,
	) (result.Expanses, error)
}

type Billing struct {
	service                 billingService
	users                   adminService
	j                       jwt.JWT
	frontendURL             string
	openRouterWebhookSecret string
}

func NewBilling(
	service billingService,
	users adminService,
	j jwt.JWT,
	frontendURL string,
	openRouterWebhookSecret string,
) *Billing {
	return &Billing{
		service:                 service,
		users:                   users,
		j:                       j,
		frontendURL:             strings.TrimSpace(frontendURL),
		openRouterWebhookSecret: strings.TrimSpace(openRouterWebhookSecret),
	}
}

func (b *Billing) Register(r chi.Router) {
	r.Post("/billing/webhook/yookassa", b.HandleYooKassaWebhook)
	r.Post("/billing/webhook/openrouter", b.HandleOpenRouterWebhook)
	r.Get("/plans/public", b.ListPublicPlans)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthJwt(b.j))
		r.Use(middleware.AdminOnly(b.users))
		r.Get("/plans", b.ListPlans)
		r.Get("/plans/{id}", b.GetPlan)
		r.Post("/plans", b.CreatePlan)
		r.Patch("/plans/{id}", b.UpdatePlan)
		r.Delete("/plans/{id}", b.DeactivatePlan)
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthJwt(b.j))
		r.Get("/me/bootstrap", b.GetBootstrap)
		r.Get("/me/billing", b.GetBilling)
		r.Get("/me/subscription", b.GetSubscription)
		r.Get("/me/subscription/{id}/checkout-status", b.GetSubscriptionCheckoutStatus)
		r.Post("/me/subscription", b.Subscribe)
		r.Patch("/me/subscription", b.ChangePlan)
		r.Delete("/me/subscription", b.CancelSubscription)
		r.Post("/me/topUp", b.TopUp)
		r.Get("/me/expanse", b.ExpanseAnalyze)
	})
}

func (b *Billing) HandleYooKassaWebhook(w http.ResponseWriter, r *http.Request) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnsupportedMediaType,
			Message: "content type must be application/json",
		})
		return
	}

	var req dto.YooKassaWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})
		return
	}

	var payment entities.Payment
	if err := json.Unmarshal(req.Object, &payment); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid payment payload",
		})
		return
	}

	if err := b.service.EventPayment(r.Context(), commands.PaymentEvent{
		PaymentEvent: entities.PaymentEvent{
			Type:   req.Type,
			Event:  req.Event,
			Object: payment,
		},
	}); err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, map[string]string{"status": "ok"})
}

func (b *Billing) HandleOpenRouterWebhook(w http.ResponseWriter, r *http.Request) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnsupportedMediaType,
			Message: "content type must be application/json",
		})
		return
	}

	if b.openRouterWebhookSecret == "" ||
		strings.TrimSpace(r.Header.Get("Authorization")) != b.openRouterWebhookSecret {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid webhook authorization",
		})
		return
	}

	var req dto.OpenRouterWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})
		return
	}

	for _, resourceSpan := range req.ResourceSpans {
		for _, scopeSpan := range resourceSpan.ScopeSpans {
			for _, span := range scopeSpan.Spans {
				event, ok := openRouterUsageEventFromSpan(span)
				if !ok {
					continue
				}

				if err := b.service.HandleOpenRouterUsageWebhook(r.Context(), event); err != nil {
					respondServiceError(w, err)
					return
				}
			}
		}
	}

	response.RespondOK(w, map[string]string{"status": "ok"})
}

func openRouterUsageEventFromSpan(span dto.OpenRouterSpan) (commands.OpenRouterUsageEvent, bool) {
	var (
		apiKeyName  string
		model       string
		totalCost   string
		inputTokens int
		outputToken int
	)

	for _, attr := range span.Attributes {
		switch attr.Key {
		case "trace.metadata.openrouter.api_key_name":
			apiKeyName = strings.TrimSpace(attr.Value.StringValue)
		case "gen_ai.response.model":
			model = strings.TrimSpace(attr.Value.StringValue)
		case "gen_ai.usage.total_cost":
			totalCost = strconv.FormatFloat(attr.Value.DoubleValue, 'f', -1, 64)
		case "gen_ai.usage.input_tokens":
			inputTokens = attr.Value.IntValue
		case "gen_ai.usage.output_tokens":
			outputToken = attr.Value.IntValue
		}
	}

	if strings.TrimSpace(span.TraceID) == "" || strings.TrimSpace(span.SpanID) == "" ||
		apiKeyName == "" ||
		totalCost == "" {
		return commands.OpenRouterUsageEvent{}, false
	}

	occurredAt, err := parseUnixNanoTime(span.EndTimeUnixNano)
	if err != nil {
		return commands.OpenRouterUsageEvent{}, false
	}

	return commands.OpenRouterUsageEvent{
		TraceID:     strings.TrimSpace(span.TraceID),
		SpanID:      strings.TrimSpace(span.SpanID),
		APIKeyName:  apiKeyName,
		Model:       model,
		TotalCost:   totalCost,
		InputTokens: inputTokens,
		OutputToken: outputToken,
		OccurredAt:  occurredAt,
	}, true
}

func parseUnixNanoTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, strconv.ErrSyntax
	}

	nanos, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(0, nanos).UTC(), nil
}

func (b *Billing) CreatePlan(w http.ResponseWriter, r *http.Request) {
	var req dto.CreatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid request body"},
		)
		return
	}

	plan, err := b.service.CreatePlan(r.Context(), commands.CreatePlan{
		Code:               req.Code,
		Name:               req.Name,
		Interval:           req.Interval,
		BillingAmountMinor: req.BillingAmountMinor,
		BalanceCreditMinor: req.BalanceCreditMinor,
		Currency:           req.Currency,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, planToResponse(plan))
}

func (b *Billing) ListPublicPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := b.service.ListPlans(r.Context(), false)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	grouped := make(map[string]dto.PublicPlanGroupResponse, len(plans))
	for _, plan := range plans {
		group := grouped[plan.Code]
		if group.Code == "" {
			group.Code = plan.Code
			group.Name = plan.Name
		}

		variant := &dto.PublicPlanVariantResponse{
			PlanID:             plan.ID.String(),
			BillingAmountMinor: plan.BillingAmountMinor,
			BalanceCreditMinor: plan.BalanceCreditMinor,
			Currency:           plan.Currency,
		}

		switch plan.Interval {
		case entities.PlanIntervalMonthly:
			group.Monthly = variant
		case entities.PlanIntervalYearly:
			group.Yearly = variant
		}

		grouped[plan.Code] = group
	}

	responses := make([]dto.PublicPlanGroupResponse, 0, len(grouped))
	for _, group := range grouped {
		responses = append(responses, group)
	}

	sort.Slice(responses, func(i, j int) bool {
		return responses[i].Code < responses[j].Code
	})

	response.RespondOK(w, responses)
}

func (b *Billing) ListPlans(w http.ResponseWriter, r *http.Request) {
	includeInactive, _ := strconv.ParseBool(r.URL.Query().Get("include_inactive"))

	plans, err := b.service.ListPlans(r.Context(), includeInactive)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	responses := make([]dto.PlanResponse, 0, len(plans))
	for _, plan := range plans {
		responses = append(responses, planToResponse(plan))
	}

	response.RespondOK(w, responses)
}

func (b *Billing) GetPlan(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid plan id"},
		)
		return
	}

	plan, err := b.service.GetPlan(r.Context(), id)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, planToResponse(plan))
}

func (b *Billing) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid plan id"},
		)
		return
	}

	var req dto.UpdatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid request body"},
		)
		return
	}

	plan, err := b.service.UpdatePlan(r.Context(), commands.UpdatePlan{
		ID:                 id,
		Code:               req.Code,
		Name:               req.Name,
		Interval:           req.Interval,
		BillingAmountMinor: req.BillingAmountMinor,
		BalanceCreditMinor: req.BalanceCreditMinor,
		Currency:           req.Currency,
		IsActive:           req.IsActive,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, planToResponse(plan))
}

func (b *Billing) DeactivatePlan(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid plan id"},
		)
		return
	}

	if err := b.service.DeactivatePlan(r.Context(), id); err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, map[string]bool{"deactivated": true})
}

func (b *Billing) GetBilling(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	summary, err := b.service.GetBillingSummary(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, billingSummaryResponse(summary))
}

func (b *Billing) GetBootstrap(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	bootstrap, err := b.service.GetBootstrap(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, bootstrapResponse(bootstrap))
}

func (b *Billing) GetSubscription(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	summary, err := b.service.GetCurrentSubscription(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	if summary == nil {
		response.RespondOK(w, map[string]any{"subscription": nil})
		return
	}

	response.RespondOK(w, subscriptionToResponse(summary.Subscription))
}

func (b *Billing) Subscribe(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	var req dto.SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid request body"},
		)
		return
	}

	planID, err := uuid.Parse(req.PlanID)
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid plan id"},
		)
		return
	}

	checkout, err := b.service.Subscribe(r.Context(), commands.Subscribe{
		UserID:    userID,
		PlanID:    planID,
		ReturnURL: subscriptionReturnURLBase(b.frontendURL),
		Now:       time.Now().UTC(),
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, dto.SubscriptionCheckoutResponse{
		SubscriptionID:  checkout.SubscriptionID.String(),
		PaymentID:       checkout.PaymentID,
		Status:          checkout.Status,
		ConfirmationURL: checkout.ConfirmationURL,
		ReturnURL:       checkout.ReturnURL,
	})
}

func (b *Billing) TopUp(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	var req dto.TopUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid request body"},
		)

		return
	}

	valAmount, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "amount must be number"},
		)

		return
	}

	if valAmount <= 0 {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "amount must be greater than 0"},
		)
		return
	}

	u, err := b.service.TopUp(r.Context(), commands.TopUp{
		UserID:          userID,
		PaymentMethodID: req.PaymentMethodID,
		Amount:          req.Amount,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, dto.TopUpResponse{ConfirmationURL: u})
}

func (b *Billing) GetSubscriptionCheckoutStatus(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	subscriptionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid subscription id"},
		)
		return
	}

	status, err := b.service.GetSubscriptionCheckoutStatus(r.Context(), userID, subscriptionID)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, dto.SubscriptionCheckoutStatusResponse{
		SubscriptionID:     status.SubscriptionID.String(),
		SubscriptionStatus: status.SubscriptionStatus,
		PaymentStatus:      status.PaymentStatus,
		PlanID:             status.PlanID.String(),
		NextChargeAt:       status.NextChargeAt,
	})
}

func (b *Billing) ChangePlan(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	var req dto.ChangeSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid request body"},
		)
		return
	}

	planID, err := uuid.Parse(req.PlanID)
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusBadRequest, Message: "invalid plan id"},
		)
		return
	}

	subscription, err := b.service.ChangePlan(r.Context(), commands.ChangePlan{
		UserID: userID,
		PlanID: planID,
		Now:    time.Now().UTC(),
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, subscriptionToResponse(subscription))
}

func (b *Billing) ExpanseAnalyze(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	res, err := b.service.ExpanseAnalyze(r.Context(), commands.Expanse{UserID: userID})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, dto.ExpanseAnalyzeResponse{
		Today:   res.Today,
		Weekly:  res.Week,
		Monthly: res.Month,
		Daily:   res.Day,
	})
}

func (b *Billing) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(
			w,
			response.Error{Code: http.StatusUnauthorized, Message: "invalid user id"},
		)
		return
	}

	if err := b.service.CancelSubscription(r.Context(), commands.CancelSubscription{
		UserID: userID,
		Now:    time.Now().UTC(),
	}); err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, map[string]bool{"canceled": true})
}

func planToResponse(plan entities.Plan) dto.PlanResponse {
	return dto.PlanResponse{
		ID:                 plan.ID.String(),
		Code:               plan.Code,
		Name:               plan.Name,
		Interval:           plan.Interval,
		BillingAmountMinor: plan.BillingAmountMinor,
		BalanceCreditMinor: plan.BalanceCreditMinor,
		Currency:           plan.Currency,
		IsActive:           plan.IsActive,
		CreatedAt:          plan.CreatedAt,
		UpdatedAt:          plan.UpdatedAt,
	}
}

func subscriptionToResponse(subscription entities.UserSubscription) dto.SubscriptionResponse {
	return dto.SubscriptionResponse{
		ID:                 subscription.ID.String(),
		PlanID:             subscription.PlanID.String(),
		Status:             subscription.Status,
		StartedAt:          subscription.StartedAt,
		CurrentPeriodStart: subscription.CurrentPeriodStart,
		CurrentPeriodEnd:   subscription.CurrentPeriodEnd,
		CanceledAt:         subscription.CanceledAt,
	}
}

func billingSummaryResponse(summary billingsvc.BillingSummary) dto.BillingSummaryResponse {
	summaryResponse := dto.BillingSummaryResponse{
		Balance:      summary.BalanceMinor,
		NextChargeAt: summary.NextChargeAt,
	}

	if summary.CurrentSubscription != nil {
		subscription := subscriptionToResponse(summary.CurrentSubscription.Subscription)
		summaryResponse.CurrentSubscription = &subscription
	}

	return summaryResponse
}

func bootstrapResponse(bootstrap result.Bootstrap) dto.BootstrapResponse {
	resp := dto.BootstrapResponse{
		DashboardAllowed: bootstrap.DashboardAllowed,
		Onboarding: dto.BootstrapOnboardingResponse{
			Required: bootstrap.Onboarding.Required,
			Step:     bootstrap.Onboarding.Step,
		},
	}

	if bootstrap.Subscription != nil {
		resp.Subscription = &dto.BootstrapSubscriptionDTO{
			Status:           bootstrap.Subscription.Status,
			CurrentPeriodEnd: bootstrap.Subscription.CurrentPeriodEnd,
			AccessActive:     bootstrap.Subscription.AccessActive,
		}
	}

	if bootstrap.Onboarding.ClawID != nil {
		resp.Onboarding.ClawID = bootstrap.Onboarding.ClawID.String()
	}

	if bootstrap.Onboarding.TelegramManager != nil {
		resp.Onboarding.TelegramManager = &dto.BootstrapTelegramManagerDTO{
			ID:            bootstrap.Onboarding.TelegramManager.ID.String(),
			Status:        bootstrap.Onboarding.TelegramManager.Status,
			DeepLinkURL:   bootstrap.Onboarding.TelegramManager.DeepLinkURL,
			LinkExpiresAt: bootstrap.Onboarding.TelegramManager.LinkExpiresAt,
			LastError:     bootstrap.Onboarding.TelegramManager.LastError,
		}
		if bootstrap.Onboarding.TelegramManager.ChannelID != nil {
			resp.Onboarding.TelegramManager.ChannelID = bootstrap.Onboarding.TelegramManager.ChannelID.String()
		}
	}

	return resp
}

func subscriptionReturnURLBase(frontendURL string) string {
	frontendURL = strings.TrimSpace(frontendURL)
	if frontendURL == "" {
		return ""
	}

	parsed, err := url.Parse(frontendURL)
	if err != nil {
		return ""
	}

	parsed.Path = path.Join(parsed.Path, "onboard")
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}
