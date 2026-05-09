package controllers_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"shared/pkg/jwt"
	"simpleClaw/config"
	"simpleClaw/internal/api/rest/controllers"
	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/entities"
	entitychannels "simpleClaw/internal/entities/channels"
	"simpleClaw/internal/infra/sql"
	billingservice "simpleClaw/internal/service/billing"
	billingcommands "simpleClaw/internal/service/billing/commands"
	billingresult "simpleClaw/internal/service/billing/result"
	clawservice "simpleClaw/internal/service/claw"
	clawcommands "simpleClaw/internal/service/claw/commands"
	clawcapabilityservice "simpleClaw/internal/service/clawcapability"
	integrationservice "simpleClaw/internal/service/integrations"
	"simpleClaw/internal/service/integrations/googleoauth"
	servercommands "simpleClaw/internal/service/server/commands"
	usercommands "simpleClaw/internal/service/user/commands"
)

type testEnv struct {
	router                  http.Handler
	user                    entities.User
	sessionID               uuid.UUID
	accessToken             string
	refreshToken            string
	openRouterWebhookSecret string
	userService             *fakeUserService
	clawService             *fakeClawService
	integrationService      *fakeIntegrationService
	googleOAuthService      *fakeGoogleOAuthService
	clawCapabilityService   *fakeClawCapabilityService
	serverService           *fakeServerService
	billingService          *fakeBillingService
}

func TestRoutesIntegration(t *testing.T) {
	t.Run("POST /api/auth/refresh", func(t *testing.T) {
		env := newTestEnv(t)

		rr := env.request(
			t,
			http.MethodPost,
			"/api/auth/refresh",
			nil,
			refreshCookie(env.refreshToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload map[string]bool
		decodeJSON(t, rr, &payload)

		if !payload["refreshed"] {
			t.Fatalf("expected refreshed=true, got payload=%v", payload)
		}

		cookies := rr.Result().Cookies()
		if !hasCookie(cookies, "access_token") {
			t.Fatalf("expected access_token cookie in response")
		}

		if !hasCookie(cookies, "refresh_token") {
			t.Fatalf("expected refresh_token cookie in response")
		}
	})

	t.Run("POST /api/auth/refresh clears cookies when refresh token expired", func(t *testing.T) {
		env := newTestEnv(t)
		env.userService.refreshErr = jwt.ErrExpired

		rr := env.request(
			t,
			http.MethodPost,
			"/api/auth/refresh",
			nil,
			refreshCookie(env.refreshToken),
		)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		accessCookie := findCookie(rr.Result().Cookies(), "access_token")
		if accessCookie == nil || accessCookie.Value != "" || accessCookie.MaxAge != -1 {
			t.Fatalf("expected cleared access_token cookie, got %#v", accessCookie)
		}

		refreshCookie := findCookie(rr.Result().Cookies(), "refresh_token")
		if refreshCookie == nil || refreshCookie.Value != "" || refreshCookie.MaxAge != -1 {
			t.Fatalf("expected cleared refresh_token cookie, got %#v", refreshCookie)
		}
	})

	t.Run("POST /api/auth/logout", func(t *testing.T) {
		env := newTestEnv(t)

		rr := env.request(
			t,
			http.MethodPost,
			"/api/auth/logout",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		if len(env.userService.logoutCalls) != 1 {
			t.Fatalf("expected single logout call, got %d", len(env.userService.logoutCalls))
		}

		call := env.userService.logoutCalls[0]
		if call.UserID != env.user.ID {
			t.Fatalf("logout user id = %s, want %s", call.UserID, env.user.ID)
		}

		if call.SessionID != env.sessionID {
			t.Fatalf("logout session id = %s, want %s", call.SessionID, env.sessionID)
		}

		clearedAccessCookie := findCookie(rr.Result().Cookies(), "access_token")
		if clearedAccessCookie == nil || clearedAccessCookie.Value != "" || clearedAccessCookie.MaxAge != -1 {
			t.Fatalf("expected cleared access_token cookie, got %#v", clearedAccessCookie)
		}

		clearedRefreshCookie := findCookie(rr.Result().Cookies(), "refresh_token")
		if clearedRefreshCookie == nil || clearedRefreshCookie.Value != "" || clearedRefreshCookie.MaxAge != -1 {
			t.Fatalf("expected cleared refresh_token cookie, got %#v", clearedRefreshCookie)
		}
	})

	t.Run("GET /api/me/user-info", func(t *testing.T) {
		env := newTestEnv(t)

		rr := env.request(
			t,
			http.MethodGet,
			"/api/me/user-info",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != env.user.ID.String() {
			t.Fatalf("unexpected user id: %s", payload.ID)
		}

		if payload.Email != env.user.Email {
			t.Fatalf("unexpected email: %s", payload.Email)
		}
	})

	t.Run("POST /api/me/channel", func(t *testing.T) {
		env := newTestEnv(t)

		body := map[string]any{
			"name": "alerts",
			"telegramChannel": map[string]any{
				"dmPolicy":  "allowlist",
				"botToken":  "test-bot-token",
				"allowFrom": []string{"1001", "1002"},
			},
		}

		rr := env.request(t, http.MethodPost, "/api/me/channel", body, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			BotToken  string   `json:"botToken"`
			DmPolicy  string   `json:"dmPolicy"`
			AllowFrom []string `json:"allowFrom"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID == "" {
			t.Fatalf("expected non-empty channel ID")
		}

		if payload.Name != "alerts" {
			t.Fatalf("unexpected channel name: %s", payload.Name)
		}

		if payload.BotToken != "test-bot-token" {
			t.Fatalf("unexpected bot token: %s", payload.BotToken)
		}

		if payload.DmPolicy != "allowlist" {
			t.Fatalf("unexpected dm policy: %s", payload.DmPolicy)
		}

		if len(payload.AllowFrom) != 2 || payload.AllowFrom[0] != "1001" || payload.AllowFrom[1] != "1002" {
			t.Fatalf("unexpected allowFrom: %#v", payload.AllowFrom)
		}
	})

	t.Run("POST /api/me/integrations/google/oauth/start requires auth", func(t *testing.T) {
		env := newTestEnv(t)

		rr := env.request(
			t,
			http.MethodPost,
			"/api/me/integrations/google/oauth/start",
			map[string]any{"capabilities": []string{"gmail"}},
		)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("POST /api/me/integrations/google/oauth/start returns auth url", func(t *testing.T) {
		env := newTestEnv(t)
		env.googleOAuthService.startResult = googleoauth.StartResult{
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth?state=test-state",
			Capabilities: []string{"gmail", "google_calendar"},
		}

		rr := env.request(
			t,
			http.MethodPost,
			"/api/me/integrations/google/oauth/start",
			map[string]any{
				"capabilities": []string{"gmail", "google_calendar"},
				"returnTo":     "/onboard/google",
			},
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}

		var payload struct {
			AuthURL      string   `json:"authUrl"`
			Capabilities []string `json:"capabilities"`
		}
		decodeJSON(t, rr, &payload)

		if payload.AuthURL != env.googleOAuthService.startResult.AuthURL {
			t.Fatalf("auth url = %q", payload.AuthURL)
		}

		if !reflect.DeepEqual(payload.Capabilities, env.googleOAuthService.startResult.Capabilities) {
			t.Fatalf("capabilities = %#v", payload.Capabilities)
		}
	})

	t.Run("GET /api/auth/google/integrations/callback rejects bad state", func(t *testing.T) {
		env := newTestEnv(t)
		env.googleOAuthService.completeErr = googleoauth.ErrStateInvalid

		rr := env.request(
			t,
			http.MethodGet,
			"/api/auth/google/integrations/callback?state=bad&code=test-code",
			nil,
		)

		if rr.Code != http.StatusFound {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		if got := rr.Header().Get("Location"); got != "http://example.com/onboard?integration_oauth=error" {
			t.Fatalf("location = %q", got)
		}
	})

	t.Run("GET /api/auth/google/integrations/callback handles access_denied", func(t *testing.T) {
		env := newTestEnv(t)
		env.googleOAuthService.completeErr = errors.New("complete should not be called")

		rr := env.request(
			t,
			http.MethodGet,
			"/api/auth/google/integrations/callback?state=test-state&error=access_denied",
			nil,
		)

		if rr.Code != http.StatusFound {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		if env.googleOAuthService.completeCalls != 0 {
			t.Fatalf("complete calls = %d, want 0", env.googleOAuthService.completeCalls)
		}

		if got := rr.Header().Get("Location"); got != "http://example.com/onboard?integration_oauth=error" {
			t.Fatalf("location = %q", got)
		}
	})

	t.Run("GET /api/auth/google/integrations/callback redirects success", func(t *testing.T) {
		env := newTestEnv(t)
		env.googleOAuthService.completeResult = googleoauth.CompleteResult{
			ReturnTo: "/onboard/google",
			Integrations: []entities.AccountIntegration{
				{ID: uuid.New()},
			},
		}

		rr := env.request(
			t,
			http.MethodGet,
			"/api/auth/google/integrations/callback?state=test-state&code=test-code",
			nil,
		)

		if rr.Code != http.StatusFound {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		if got := rr.Header().Get("Location"); got != "http://example.com/onboard/google?integration_oauth=success" {
			t.Fatalf("location = %q", got)
		}
	})

	t.Run("GET /api/me/payment-method", func(t *testing.T) {
		env := newTestEnv(t)
		method := entities.PaymentMethod{
			ID:        uuid.New(),
			UserID:    env.user.ID,
			Title:     "Primary card",
			IsDefault: true,
			CreatedAt: time.Now().UTC(),
		}
		env.userService.paymentMethods[method.ID] = method

		rr := env.request(t, http.MethodGet, "/api/me/payment-method", nil, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload []struct {
			ID string `json:"id"`
		}
		decodeJSON(t, rr, &payload)

		if len(payload) != 1 || payload[0].ID != method.ID.String() {
			t.Fatalf("unexpected payment methods payload: %#v", payload)
		}
	})

	t.Run("GET /api/me/payment-method/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		method := entities.PaymentMethod{
			ID:        uuid.New(),
			UserID:    env.user.ID,
			Title:     "Primary card",
			IsDefault: true,
			CreatedAt: time.Now().UTC(),
		}
		env.userService.paymentMethods[method.ID] = method

		rr := env.request(
			t,
			http.MethodGet,
			"/api/me/payment-method/"+method.ID.String(),
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID string `json:"id"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != method.ID.String() {
			t.Fatalf("unexpected payment method id: %s", payload.ID)
		}
	})

	t.Run("PATCH /api/me/payment-method/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		first := entities.PaymentMethod{
			ID:        uuid.New(),
			UserID:    env.user.ID,
			Title:     "First",
			IsDefault: true,
			CreatedAt: time.Now().UTC().Add(-time.Minute),
		}
		second := entities.PaymentMethod{
			ID:        uuid.New(),
			UserID:    env.user.ID,
			Title:     "Second",
			IsDefault: false,
			CreatedAt: time.Now().UTC(),
		}
		env.userService.paymentMethods[first.ID] = first
		env.userService.paymentMethods[second.ID] = second

		patchRR := env.request(
			t,
			http.MethodPatch,
			"/api/me/payment-method/"+second.ID.String(),
			map[string]any{"is_default": true},
			accessCookie(env.accessToken),
		)
		if patchRR.Code != http.StatusOK {
			t.Fatalf("unexpected patch status: %d", patchRR.Code)
		}

		if env.userService.paymentMethods[first.ID].IsDefault {
			t.Fatal("first method should no longer be default")
		}
		if !env.userService.paymentMethods[second.ID].IsDefault {
			t.Fatal("second method should become default")
		}
	})

	t.Run("PATCH /api/me/payment-method/{id} rejects false", func(t *testing.T) {
		env := newTestEnv(t)
		method := entities.PaymentMethod{
			ID:        uuid.New(),
			UserID:    env.user.ID,
			Title:     "Primary",
			IsDefault: true,
			CreatedAt: time.Now().UTC(),
		}
		env.userService.paymentMethods[method.ID] = method

		rr := env.request(
			t,
			http.MethodPatch,
			"/api/me/payment-method/"+method.ID.String(),
			map[string]any{"is_default": false},
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d", rr.Code)
		}
	})

	t.Run("DELETE /api/me/payment-method/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		method := entities.PaymentMethod{
			ID:        uuid.New(),
			UserID:    env.user.ID,
			Title:     "Primary",
			IsDefault: true,
			CreatedAt: time.Now().UTC(),
		}
		env.userService.paymentMethods[method.ID] = method

		rr := env.request(
			t,
			http.MethodDelete,
			"/api/me/payment-method/"+method.ID.String(),
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		if _, ok := env.userService.paymentMethods[method.ID]; ok {
			t.Fatalf("payment method %s should be removed", method.ID)
		}
	})

	t.Run("GET /api/me/billing", func(t *testing.T) {
		env := newTestEnv(t)

		rr := env.request(t, http.MethodGet, "/api/me/billing", nil, accessCookie(env.accessToken))
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			BalanceMinor int64 `json:"balance_minor"`
		}
		decodeJSON(t, rr, &payload)

		if payload.BalanceMinor != env.billingService.balance[env.user.ID] {
			t.Fatalf("unexpected balance: %d", payload.BalanceMinor)
		}
	})

	t.Run("POST /api/billing/webhook/openrouter", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/openrouter",
			bytes.NewBufferString(`{
				"resourceSpans": [{
					"scopeSpans": [{
						"spans": [{
							"traceId": "trace-route",
							"spanId": "span-route",
							"endTimeUnixNano": "1774526400000000000",
							"attributes": [
								{"key":"trace.metadata.openrouter.api_key_name","value":{"stringValue":"snapclaw+2dd0b4f0-6f6a-4f4c-b4de-8e5fa6db6d66"}},
								{"key":"gen_ai.usage.total_cost","value":{"doubleValue":0.01}}
							]
						}]
					}]
				}]
			}`),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", env.openRouterWebhookSecret)

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		if env.billingService.lastOpenRouterWebhook.TraceID != "trace-route" {
			t.Fatalf("unexpected trace id: %s", env.billingService.lastOpenRouterWebhook.TraceID)
		}
	})

	t.Run("POST /api/me/subscription", func(t *testing.T) {
		env := newTestEnv(t)

		plan := env.billingService.seedPlan("starter-monthly")

		rr := env.request(
			t,
			http.MethodPost,
			"/api/me/subscription",
			map[string]any{"plan_id": plan.ID.String()},
			accessCookie(env.accessToken),
		)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			SubscriptionID  string `json:"subscription_id"`
			PaymentID       string `json:"payment_id"`
			Status          string `json:"status"`
			ConfirmationURL string `json:"confirmation_url"`
			ReturnURL       string `json:"return_url"`
		}
		decodeJSON(t, rr, &payload)

		if payload.SubscriptionID == "" {
			t.Fatal("expected non-empty subscription id")
		}
		if payload.PaymentID == "" {
			t.Fatal("expected non-empty payment id")
		}
		if payload.Status != entities.SubscriptionStatusPending {
			t.Fatalf("unexpected status payload: %s", payload.Status)
		}
		if payload.ConfirmationURL == "" {
			t.Fatal("expected confirmation_url")
		}
		if !strings.Contains(payload.ReturnURL, "subscription_id="+payload.SubscriptionID) {
			t.Fatalf("unexpected return_url: %s", payload.ReturnURL)
		}
		if location := rr.Header().Get("Location"); location != "" {
			t.Fatalf("unexpected redirect location: %s", location)
		}
	})

	t.Run("POST /api/me/subscription maps inactive plan error", func(t *testing.T) {
		env := newTestEnv(t)
		plan := env.billingService.seedPlan("starter-monthly")
		env.billingService.subscribeErr = billingservice.ErrPlanInactive

		rr := env.request(
			t,
			http.MethodPost,
			"/api/me/subscription",
			map[string]any{"plan_id": plan.ID.String()},
			accessCookie(env.accessToken),
		)
		if rr.Code != http.StatusConflict {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		decodeJSON(t, rr, &payload)

		if payload.Message != "plan is inactive" {
			t.Fatalf("unexpected message: %s", payload.Message)
		}
	})

	t.Run("PATCH /api/me/subscription rejects active plan change", func(t *testing.T) {
		env := newTestEnv(t)
		currentPlan := env.billingService.seedPlan("starter-monthly")
		targetPlan := env.billingService.seedPlan("starter-yearly")

		now := time.Now().UTC()
		subscription := entities.UserSubscription{
			ID:                 uuid.New(),
			UserID:             env.user.ID,
			PlanID:             currentPlan.ID,
			Status:             entities.SubscriptionStatusActive,
			StartedAt:          now,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.AddDate(0, 1, 0),
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		env.billingService.subscriptions[env.user.ID] = subscription

		rr := env.request(
			t,
			http.MethodPatch,
			"/api/me/subscription",
			map[string]any{"plan_id": targetPlan.ID.String()},
			accessCookie(env.accessToken),
		)
		if rr.Code != http.StatusConflict {
			t.Fatalf("unexpected status: %d", rr.Code)
		}
	})

	t.Run("GET /api/me/subscription/{id}/checkout-status", func(t *testing.T) {
		env := newTestEnv(t)
		plan := env.billingService.seedPlan("starter-monthly")
		now := time.Now().UTC()
		subscription := entities.UserSubscription{
			ID:                 uuid.New(),
			UserID:             env.user.ID,
			PlanID:             plan.ID,
			Status:             entities.SubscriptionStatusPending,
			StartedAt:          now,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.AddDate(0, 1, 0),
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		env.billingService.subscriptions[env.user.ID] = subscription
		env.billingService.paymentStatusBySubscription[subscription.ID] = string(entities.Pending)

		rr := env.request(
			t,
			http.MethodGet,
			"/api/me/subscription/"+subscription.ID.String()+"/checkout-status",
			nil,
			accessCookie(env.accessToken),
		)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			SubscriptionID     string  `json:"subscription_id"`
			SubscriptionStatus string  `json:"subscription_status"`
			PaymentStatus      string  `json:"payment_status"`
			PlanID             string  `json:"plan_id"`
			NextChargeAt       *string `json:"next_charge_at"`
		}
		decodeJSON(t, rr, &payload)

		if payload.SubscriptionID != subscription.ID.String() {
			t.Fatalf("unexpected subscription id: %s", payload.SubscriptionID)
		}
		if payload.SubscriptionStatus != entities.SubscriptionStatusPending {
			t.Fatalf("unexpected subscription status: %s", payload.SubscriptionStatus)
		}
		if payload.PaymentStatus != string(entities.Pending) {
			t.Fatalf("unexpected payment status: %s", payload.PaymentStatus)
		}
		if payload.PlanID != plan.ID.String() {
			t.Fatalf("unexpected plan id: %s", payload.PlanID)
		}
		if payload.NextChargeAt != nil {
			t.Fatalf("unexpected next_charge_at: %v", payload.NextChargeAt)
		}
	})

	t.Run("GET /api/me/subscription/{id}/checkout-status returns not found for another user", func(t *testing.T) {
		env := newTestEnv(t)
		plan := env.billingService.seedPlan("starter-monthly")
		now := time.Now().UTC()
		subscription := entities.UserSubscription{
			ID:                 uuid.New(),
			UserID:             uuid.New(),
			PlanID:             plan.ID,
			Status:             entities.SubscriptionStatusPending,
			StartedAt:          now,
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.AddDate(0, 1, 0),
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		env.billingService.subscriptions[subscription.UserID] = subscription
		env.billingService.paymentStatusBySubscription[subscription.ID] = string(entities.Pending)

		rr := env.request(
			t,
			http.MethodGet,
			"/api/me/subscription/"+subscription.ID.String()+"/checkout-status",
			nil,
			accessCookie(env.accessToken),
		)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("unexpected status: %d", rr.Code)
		}
	})

	t.Run("DELETE /api/me/subscription", func(t *testing.T) {
		env := newTestEnv(t)
		plan := env.billingService.seedPlan("starter-monthly")
		_, err := env.billingService.Subscribe(context.Background(), billingcommands.Subscribe{
			UserID: env.user.ID,
			PlanID: plan.ID,
			Now:    time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("seed subscription: %v", err)
		}

		rr := env.request(
			t,
			http.MethodDelete,
			"/api/me/subscription",
			nil,
			accessCookie(env.accessToken),
		)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}
	})

	t.Run("POST /api/plans", func(t *testing.T) {
		env := newAdminTestEnv(t)

		rr := env.request(
			t,
			http.MethodPost,
			"/api/plans",
			map[string]any{
				"code":                 "starter",
				"name":                 "Starter",
				"interval":             "monthly",
				"billing_amount_minor": 99000,
				"balance_credit_minor": 150000,
				"currency":             "RUB",
			},
			accessCookie(env.accessToken),
		)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID == "" {
			t.Fatal("expected non-empty plan id")
		}

		if payload.Code != "starter" {
			t.Fatalf("unexpected code: %s", payload.Code)
		}
	})

	t.Run("GET /api/plans", func(t *testing.T) {
		env := newAdminTestEnv(t)
		env.billingService.seedPlan("starter-monthly")
		env.billingService.seedPlan("pro-monthly")

		rr := env.request(t, http.MethodGet, "/api/plans", nil, accessCookie(env.accessToken))
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload []struct {
			ID string `json:"id"`
		}
		decodeJSON(t, rr, &payload)

		if len(payload) != 2 {
			t.Fatalf("expected 2 plans, got %d", len(payload))
		}
	})

	t.Run("GET /api/plans/public", func(t *testing.T) {
		env := newTestEnv(t)

		starterMonthly := env.billingService.seedPlan("starter-monthly")
		starterYearly := env.billingService.seedPlan("starter-yearly")
		proMonthly := env.billingService.seedPlan("pro-monthly")
		inactive := env.billingService.seedPlan("legacy-yearly")
		inactive.IsActive = false
		env.billingService.plans[inactive.ID] = inactive

		rr := env.request(t, http.MethodGet, "/api/plans/public", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload []struct {
			Code    string `json:"code"`
			Name    string `json:"name"`
			Monthly *struct {
				PlanID string `json:"plan_id"`
			} `json:"monthly"`
			Yearly *struct {
				PlanID string `json:"plan_id"`
			} `json:"yearly"`
		}
		decodeJSON(t, rr, &payload)

		if len(payload) != 2 {
			t.Fatalf("expected 2 public groups, got %d", len(payload))
		}

		if payload[0].Code != "pro" {
			t.Fatalf("payload[0].Code = %q, want %q", payload[0].Code, "pro")
		}
		if payload[0].Monthly == nil || payload[0].Monthly.PlanID != proMonthly.ID.String() {
			t.Fatal("pro group should contain monthly variant")
		}
		if payload[0].Yearly != nil {
			t.Fatal("pro group should not contain yearly variant")
		}

		if payload[1].Code != "starter" {
			t.Fatalf("payload[1].Code = %q, want %q", payload[1].Code, "starter")
		}
		if payload[1].Monthly == nil || payload[1].Monthly.PlanID != starterMonthly.ID.String() {
			t.Fatal("starter group should contain monthly variant")
		}
		if payload[1].Yearly == nil || payload[1].Yearly.PlanID != starterYearly.ID.String() {
			t.Fatal("starter group should contain yearly variant")
		}
	})

	t.Run("GET /api/claws", func(t *testing.T) {
		env := newTestEnv(t)
		env.clawService.seed(env.user.ID, "first")
		env.clawService.seed(env.user.ID, "second")

		rr := env.request(t, http.MethodGet, "/api/claws", nil, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload []struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			ModelKey        string `json:"modelKey"`
			ResolvedModel   string `json:"resolvedModel"`
			DesiredState    string `json:"desiredState"`
			ObservedState   string `json:"observedState"`
			LifecycleStatus string `json:"lifecycleStatus"`
		}
		decodeJSON(t, rr, &payload)

		if len(payload) != 2 {
			t.Fatalf("expected 2 claws, got %d", len(payload))
		}

		if payload[0].DesiredState != string(entities.ClawDesiredStateStopped) {
			t.Fatalf("unexpected desired state: %s", payload[0].DesiredState)
		}

		if payload[0].ObservedState != string(entities.ClawObservedStateUnknown) {
			t.Fatalf("unexpected observed state: %s", payload[0].ObservedState)
		}

		if payload[0].LifecycleStatus != string(entities.ClawLifecycleStatusIdle) {
			t.Fatalf("unexpected lifecycle status: %s", payload[0].LifecycleStatus)
		}

		if payload[0].ModelKey != "openai-gpt-5-4" {
			t.Fatalf("unexpected model key: %s", payload[0].ModelKey)
		}

		if payload[0].ResolvedModel != "openrouter/openai/gpt-5.4" {
			t.Fatalf("unexpected resolved model: %s", payload[0].ResolvedModel)
		}
	})

	t.Run("GET /api/claws/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "single")

		rr := env.request(
			t,
			http.MethodGet,
			"/api/claws/"+cl.ID.String(),
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			ModelKey        string `json:"modelKey"`
			ResolvedModel   string `json:"resolvedModel"`
			DesiredState    string `json:"desiredState"`
			ObservedState   string `json:"observedState"`
			LifecycleStatus string `json:"lifecycleStatus"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != cl.ID.String() {
			t.Fatalf("unexpected claw id: %s", payload.ID)
		}

		if payload.Name != "single" {
			t.Fatalf("unexpected claw name: %s", payload.Name)
		}

		if payload.DesiredState != string(entities.ClawDesiredStateStopped) {
			t.Fatalf("unexpected desired state: %s", payload.DesiredState)
		}

		if payload.ObservedState != string(entities.ClawObservedStateUnknown) {
			t.Fatalf("unexpected observed state: %s", payload.ObservedState)
		}

		if payload.LifecycleStatus != string(entities.ClawLifecycleStatusIdle) {
			t.Fatalf("unexpected lifecycle status: %s", payload.LifecycleStatus)
		}

		if payload.ModelKey != "openai-gpt-5-4" {
			t.Fatalf("unexpected model key: %s", payload.ModelKey)
		}

		if payload.ResolvedModel != "openrouter/openai/gpt-5.4" {
			t.Fatalf("unexpected resolved model: %s", payload.ResolvedModel)
		}
	})

	t.Run("POST /api/claws", func(t *testing.T) {
		env := newTestEnv(t)

		body := map[string]any{
			"name":       "new-claw",
			"model":      "openai/gpt-4.1-mini",
			"channelIds": []string{uuid.NewString()},
			"apiLimits": map[string]any{
				"requestsPerMinute": 120,
				"monthlyBudgetUsd":  15.5,
			},
		}

		rr := env.request(t, http.MethodPost, "/api/claws", body, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			DesiredState    string `json:"desiredState"`
			ObservedState   string `json:"observedState"`
			LifecycleStatus string `json:"lifecycleStatus"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID == "" {
			t.Fatalf("expected non-empty claw ID")
		}

		if payload.Name != "new-claw" {
			t.Fatalf("unexpected claw name: %s", payload.Name)
		}

		if payload.DesiredState != string(entities.ClawDesiredStateStopped) {
			t.Fatalf("unexpected desired state: %s", payload.DesiredState)
		}

		if payload.ObservedState != string(entities.ClawObservedStateUnknown) {
			t.Fatalf("unexpected observed state: %s", payload.ObservedState)
		}

		if payload.LifecycleStatus != string(entities.ClawLifecycleStatusIdle) {
			t.Fatalf("unexpected lifecycle status: %s", payload.LifecycleStatus)
		}
	})

	t.Run("PUT /api/claws/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "before-update")

		body := map[string]any{
			"name":       "after-update",
			"model":      "openai/gpt-4.1-mini",
			"channelIds": []string{uuid.NewString()},
			"apiLimits": map[string]any{
				"requestsPerMinute": 90,
				"monthlyBudgetUsd":  9.99,
			},
		}

		rr := env.request(
			t,
			http.MethodPut,
			"/api/claws/"+cl.ID.String(),
			body,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID   string `json:"ID"`
			Name string `json:"Name"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != cl.ID.String() {
			t.Fatalf("unexpected claw id: %s", payload.ID)
		}

		if payload.Name != "after-update" {
			t.Fatalf("unexpected claw name: %s", payload.Name)
		}
	})

	t.Run("PUT /api/claws/{id} partial without name/model", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "before-partial-update")

		body := map[string]any{
			"channelIds": []string{},
		}

		rr := env.request(
			t,
			http.MethodPut,
			"/api/claws/"+cl.ID.String(),
			body,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID   string `json:"ID"`
			Name string `json:"Name"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != cl.ID.String() {
			t.Fatalf("unexpected claw id: %s", payload.ID)
		}

		if payload.Name != "before-partial-update" {
			t.Fatalf("unexpected claw name: %s", payload.Name)
		}
	})

	t.Run("POST /api/claws/{id}/start", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "to-start")

		rr := env.request(
			t,
			http.MethodPost,
			"/api/claws/"+cl.ID.String()+"/start",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload dto.ClawResponse
		decodeJSON(t, rr, &payload)

		if payload.LifecycleStatus != string(entities.ClawLifecycleStatusStartPending) {
			t.Fatalf("unexpected lifecycle status: %s", payload.LifecycleStatus)
		}

		updated, err := env.clawService.GetByID(context.Background(), cl.ID, env.user.ID)
		if err != nil {
			t.Fatalf("get claw after start: %v", err)
		}

		if updated.DesiredState != entities.ClawDesiredStateRunning {
			t.Fatalf("unexpected desired state after start: %s", updated.DesiredState)
		}

		if updated.ObservedState != entities.ClawObservedStateUnknown {
			t.Fatalf("unexpected observed state after start: %s", updated.ObservedState)
		}
	})

	t.Run("POST /api/claws/{id}/start returns conflict when lifecycle operation is active", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "already-starting")
		opID := uuid.New()
		cl.CurrentOperationID = &opID
		cl.LifecycleStatus = entities.ClawLifecycleStatusStartPending
		env.clawService.claws[cl.ID] = cl

		rr := env.request(
			t,
			http.MethodPost,
			"/api/claws/"+cl.ID.String()+"/start",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusConflict {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		decodeJSON(t, rr, &payload)

		if payload.Message != "lifecycle operation already in progress" {
			t.Fatalf("unexpected message: %s", payload.Message)
		}
	})

	t.Run("POST /api/claws/{id}/stop", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "to-stop")
		cl.DesiredState = entities.ClawDesiredStateRunning
		cl.ObservedState = entities.ClawObservedStateRunning
		env.clawService.claws[cl.ID] = cl

		rr := env.request(
			t,
			http.MethodPost,
			"/api/claws/"+cl.ID.String()+"/stop",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload dto.ClawResponse
		decodeJSON(t, rr, &payload)

		if payload.LifecycleStatus != string(entities.ClawLifecycleStatusStopPending) {
			t.Fatalf("unexpected lifecycle status: %s", payload.LifecycleStatus)
		}

		updated, err := env.clawService.GetByID(context.Background(), cl.ID, env.user.ID)
		if err != nil {
			t.Fatalf("get claw after stop: %v", err)
		}

		if updated.DesiredState != entities.ClawDesiredStateStopped {
			t.Fatalf("unexpected desired state after stop: %s", updated.DesiredState)
		}

		if updated.ObservedState != entities.ClawObservedStateRunning {
			t.Fatalf("unexpected observed state after stop: %s", updated.ObservedState)
		}
	})

	t.Run("POST /api/claws/{id}/restart", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "to-restart")
		cl.DesiredState = entities.ClawDesiredStateStopped
		cl.ObservedState = entities.ClawObservedStateStopped
		env.clawService.claws[cl.ID] = cl

		rr := env.request(
			t,
			http.MethodPost,
			"/api/claws/"+cl.ID.String()+"/restart",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload dto.ClawResponse
		decodeJSON(t, rr, &payload)

		if payload.ID != cl.ID.String() {
			t.Fatalf("unexpected claw id: %s", payload.ID)
		}

		if payload.LifecycleStatus != string(entities.ClawLifecycleStatusRestartPending) {
			t.Fatalf("unexpected lifecycle status: %s", payload.LifecycleStatus)
		}
	})

	t.Run("DELETE /api/claws/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "to-delete")

		rr := env.request(
			t,
			http.MethodDelete,
			"/api/claws/"+cl.ID.String(),
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload dto.ClawResponse
		decodeJSON(t, rr, &payload)

		if payload.LifecycleStatus != string(entities.ClawLifecycleStatusDeletePending) {
			t.Fatalf("unexpected lifecycle status: %s", payload.LifecycleStatus)
		}

		if !env.clawService.exists(cl.ID) {
			t.Fatalf("claw %s should still exist while delete is pending", cl.ID)
		}
	})

	t.Run("GET /api/servers requires admin", func(t *testing.T) {
		env := newTestEnv(t)

		rr := env.request(t, http.MethodGet, "/api/servers", nil, accessCookie(env.accessToken))

		if rr.Code != http.StatusForbidden {
			t.Fatalf("unexpected status: %d, body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("POST /api/servers", func(t *testing.T) {
		env := newAdminTestEnv(t)

		body := map[string]any{
			"name":      "alpha",
			"ip":        "10.0.0.5",
			"url":       "http://alpha.internal",
			"proxyUrl":  "http://alpha.internal/gmail-pubsub",
			"status":    "ready",
			"secretKey": "top-secret",
		}

		rr := env.request(t, http.MethodPost, "/api/servers", body, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d, body=%s", rr.Code, rr.Body.String())
		}

		var payload struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			IP        string `json:"ip"`
			URL       string `json:"url"`
			ProxyURL  string `json:"proxyUrl"`
			Status    string `json:"status"`
			SecretKey string `json:"secretKey"`
			MaxClaws  int    `json:"maxClaws"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID == "" {
			t.Fatalf("expected non-empty server ID")
		}

		if payload.Name != "alpha" || payload.URL != "http://alpha.internal" {
			t.Fatalf("unexpected payload: %+v", payload)
		}

		if payload.ProxyURL != "http://alpha.internal/gmail-pubsub" {
			t.Fatalf("unexpected payload: %+v", payload)
		}

		if payload.MaxClaws != 7 {
			t.Fatalf("unexpected maxClaws: %+v", payload)
		}
	})

	t.Run("GET /api/servers", func(t *testing.T) {
		env := newAdminTestEnv(t)
		env.serverService.seed("first")
		env.serverService.seed("second")

		rr := env.request(t, http.MethodGet, "/api/servers", nil, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d, body=%s", rr.Code, rr.Body.String())
		}

		var payload []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			MaxClaws int    `json:"maxClaws"`
		}
		decodeJSON(t, rr, &payload)

		if len(payload) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(payload))
		}

		if payload[0].MaxClaws != 7 || payload[1].MaxClaws != 7 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	})

	t.Run("GET /api/servers/{id}", func(t *testing.T) {
		env := newAdminTestEnv(t)
		srv := env.serverService.seed("single")

		rr := env.request(
			t,
			http.MethodGet,
			"/api/servers/"+srv.ID.String(),
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d, body=%s", rr.Code, rr.Body.String())
		}

		var payload struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			MaxClaws int    `json:"maxClaws"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != srv.ID.String() || payload.Name != "single" {
			t.Fatalf("unexpected payload: %+v", payload)
		}

		if payload.MaxClaws != 7 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	})

	t.Run("PUT /api/servers/{id}", func(t *testing.T) {
		env := newAdminTestEnv(t)
		srv := env.serverService.seed("before-update")

		body := map[string]any{
			"name":      "after-update",
			"ip":        "10.0.0.44",
			"url":       "http://updated.internal",
			"proxyUrl":  "http://updated.internal/gmail-pubsub",
			"status":    "busy",
			"secretKey": "updated-secret",
		}

		rr := env.request(
			t,
			http.MethodPut,
			"/api/servers/"+srv.ID.String(),
			body,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d, body=%s", rr.Code, rr.Body.String())
		}

		var payload struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			URL       string `json:"url"`
			ProxyURL  string `json:"proxyUrl"`
			SecretKey string `json:"secretKey"`
			MaxClaws  int    `json:"maxClaws"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != srv.ID.String() || payload.Name != "after-update" {
			t.Fatalf("unexpected payload: %+v", payload)
		}

		if payload.URL != "http://updated.internal" || payload.SecretKey != "updated-secret" {
			t.Fatalf("unexpected payload: %+v", payload)
		}

		if payload.ProxyURL != "http://updated.internal/gmail-pubsub" {
			t.Fatalf("unexpected payload: %+v", payload)
		}

		if payload.MaxClaws != 7 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	})

	t.Run("DELETE /api/servers/{id}", func(t *testing.T) {
		env := newAdminTestEnv(t)
		srv := env.serverService.seed("to-delete")

		rr := env.request(
			t,
			http.MethodDelete,
			"/api/servers/"+srv.ID.String(),
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d, body=%s", rr.Code, rr.Body.String())
		}

		var payload map[string]bool
		decodeJSON(t, rr, &payload)

		if !payload["deleted"] {
			t.Fatalf("expected deleted=true, got payload=%v", payload)
		}

		if env.serverService.exists(srv.ID) {
			t.Fatalf("server %s should be deleted", srv.ID)
		}
	})
}

func newTestEnv(t *testing.T) *testEnv {
	return newTestEnvWithRole(t, entities.UserRole)
}

func newAdminTestEnv(t *testing.T) *testEnv {
	return newTestEnvWithRole(t, entities.AdminRole)
}

func newTestEnvWithRole(t *testing.T, role string) *testEnv {
	t.Helper()

	j := newTestJWT(t)

	user := entities.NewUser(
		"integration-user",
		"integration-user",
		"",
		"integration@example.com",
		role,
	)
	sessionID := uuid.New()

	access, refresh, err := j.GeneratePair(user.ID, sessionID)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	uService := newFakeUserService(user, j, sessionID, refresh.Raw)
	cService := newFakeClawService()
	iService := newFakeIntegrationService()
	gOAuthService := newFakeGoogleOAuthService()
	ccService := newFakeClawCapabilityService()
	sService := newFakeServerService()
	bService := newFakeBillingService(user.ID)
	openRouterWebhookSecret := "test-openrouter-webhook-secret"

	api := chi.NewRouter()
	controllers.NewUser(config.EnvDevelopment, uService, iService, gOAuthService, j, "http://example.com").Register(api)
	controllers.NewClaw(cService, ccService, j).Register(api)
	controllers.NewServer(sService, uService, j).Register(api)
	controllers.NewBilling(
		bService,
		uService,
		j,
		"http://example.com",
		openRouterWebhookSecret,
	).Register(api)

	root := chi.NewRouter()
	controllers.NewPubSubProxy(sService, controllers.PubSubProxyOptions{}).Register(root)
	root.Mount("/api", api)

	return &testEnv{
		router:                  root,
		user:                    user,
		sessionID:               sessionID,
		accessToken:             access.Raw,
		refreshToken:            refresh.Raw,
		openRouterWebhookSecret: openRouterWebhookSecret,
		userService:             uService,
		clawService:             cService,
		integrationService:      iService,
		googleOAuthService:      gOAuthService,
		clawCapabilityService:   ccService,
		serverService:           sService,
		billingService:          bService,
	}
}

func newTestJWT(t *testing.T) jwt.JWT {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey),
	})

	return jwt.New(privatePEM, publicPEM, []byte("test-refresh-secret"), time.Hour, 24*time.Hour)
}

func (e *testEnv) request(
	t *testing.T,
	method, path string,
	body any,
	cookies ...*http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()

	var (
		reqBody []byte
		err     error
	)

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for _, c := range cookies {
		req.AddCookie(c)
	}

	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, req)

	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, dst any) {
	t.Helper()

	err := json.NewDecoder(rr.Body).Decode(dst)
	if err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rr.Body.String())
	}
}

func accessCookie(token string) *http.Cookie {
	return &http.Cookie{Name: "access_token", Value: token}
}

func refreshCookie(token string) *http.Cookie {
	return &http.Cookie{Name: "refresh_token", Value: token}
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	for _, c := range cookies {
		if c.Name == name && c.Value != "" {
			return true
		}
	}

	return false
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}

	return nil
}

type fakeUserService struct {
	j              jwt.JWT
	sessionID      uuid.UUID
	users          map[uuid.UUID]entities.User
	channels       map[uuid.UUID]entities.Channel
	paymentMethods map[uuid.UUID]entities.PaymentMethod
	lastRefresh    map[uuid.UUID]string
	refreshErr     error
	logoutCalls    []usercommands.Logout
	channelOrder   []uuid.UUID
}

func newFakeUserService(
	user entities.User,
	j jwt.JWT,
	sessionID uuid.UUID,
	refreshToken string,
) *fakeUserService {
	return &fakeUserService{
		j:              j,
		sessionID:      sessionID,
		users:          map[uuid.UUID]entities.User{user.ID: user},
		channels:       map[uuid.UUID]entities.Channel{},
		paymentMethods: map[uuid.UUID]entities.PaymentMethod{},
		lastRefresh:    map[uuid.UUID]string{user.ID: refreshToken},
	}
}

func (s *fakeUserService) SignIn(
	_ context.Context,
	_ usercommands.SignIn,
) (jwt.AccessToken, jwt.RefreshToken, error) {
	return jwt.AccessToken{}, jwt.RefreshToken{}, errors.New(
		"oauth routes are not covered in this suite",
	)
}

func (s *fakeUserService) Refresh(
	_ context.Context,
	rawRefresh string,
) (jwt.AccessToken, jwt.RefreshToken, error) {
	if s.refreshErr != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, s.refreshErr
	}

	token, err := s.j.ParseRefresh(rawRefresh)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, err
	}

	expectedRefresh, ok := s.lastRefresh[token.Claims.UserID]
	if !ok || expectedRefresh != rawRefresh || token.Claims.SessionID != s.sessionID {
		return jwt.AccessToken{}, jwt.RefreshToken{}, jwt.ErrInvalid
	}

	access, refresh, err := s.j.GeneratePair(token.Claims.UserID, token.Claims.SessionID)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, err
	}

	s.lastRefresh[token.Claims.UserID] = refresh.Raw

	return access, refresh, nil
}

func (s *fakeUserService) Logout(_ context.Context, cmd usercommands.Logout) error {
	s.logoutCalls = append(s.logoutCalls, cmd)

	return nil
}

func (s *fakeUserService) UserInfo(_ context.Context, userID uuid.UUID) (entities.User, error) {
	user, ok := s.users[userID]
	if !ok {
		return entities.User{}, sql.ErrNotFound
	}

	return user, nil
}

func (s *fakeUserService) AddChannel(
	_ context.Context,
	cm usercommands.AddChannel,
) (entities.Channel, error) {
	if _, ok := s.users[cm.UserID]; !ok {
		return entities.Channel{}, sql.ErrNotFound
	}

	cfg := entities.ClawChannels{}

	if cm.Telegram != nil {
		cfg.Telegram = &entitychannels.TelegramConfig{
			DmPolicy:  entitychannels.DmPolicy(cm.Telegram.DmPolicy),
			AllowFrom: append([]string(nil), cm.Telegram.AllowFrom...),
			Enabled:   true,
			BotToken:  cm.Telegram.BotToken,
		}
	}

	ch := entities.NewChannel(entities.ChannelTelegramType, cm.Name, cfg, cm.UserID)
	s.channels[ch.ID] = ch
	s.channelOrder = append(s.channelOrder, ch.ID)

	return ch, nil
}

func (s *fakeUserService) AddPaymentMethod(
	_ context.Context,
	cm usercommands.AddPaymentMethod,
) (entities.PaymentMethod, error) {
	if _, ok := s.users[cm.UserID]; !ok {
		return entities.PaymentMethod{}, sql.ErrNotFound
	}

	method := entities.PaymentMethod{
		ID:        uuid.New(),
		UserID:    cm.UserID,
		Title:     cm.Title,
		IsDefault: cm.IsDefault,
		CreatedAt: time.Now().UTC(),
	}

	hasMethods := false
	for id, existing := range s.paymentMethods {
		if existing.UserID != cm.UserID {
			continue
		}

		hasMethods = true
		if cm.IsDefault {
			existing.IsDefault = false
			s.paymentMethods[id] = existing
		}
	}

	if !hasMethods {
		method.IsDefault = true
	}

	s.paymentMethods[method.ID] = method
	return method, nil
}

func (s *fakeUserService) GetPaymentMethod(
	_ context.Context,
	methodID, userID uuid.UUID,
) (entities.PaymentMethod, error) {
	method, ok := s.paymentMethods[methodID]
	if !ok || method.UserID != userID {
		return entities.PaymentMethod{}, sql.ErrNotFound
	}

	return method, nil
}

func (s *fakeUserService) GetPaymentMethods(
	_ context.Context,
	userID uuid.UUID,
) ([]entities.PaymentMethod, error) {
	methods := make([]entities.PaymentMethod, 0, len(s.paymentMethods))
	for _, method := range s.paymentMethods {
		if method.UserID == userID {
			methods = append(methods, method)
		}
	}

	sort.Slice(methods, func(i, j int) bool {
		if methods[i].IsDefault != methods[j].IsDefault {
			return methods[i].IsDefault
		}

		return methods[i].CreatedAt.After(methods[j].CreatedAt)
	})

	return methods, nil
}

func (s *fakeUserService) SetDefaultPaymentMethod(
	_ context.Context,
	cm usercommands.SetDefaultPaymentMethod,
) error {
	method, ok := s.paymentMethods[cm.PaymentMethodID]
	if !ok || method.UserID != cm.UserID {
		return sql.ErrNotFound
	}

	for id, existing := range s.paymentMethods {
		if existing.UserID != cm.UserID {
			continue
		}

		existing.IsDefault = false
		s.paymentMethods[id] = existing
	}

	method.IsDefault = true
	s.paymentMethods[method.ID] = method
	return nil
}

func (s *fakeUserService) RemovePaymentMethod(
	_ context.Context,
	methodID, userID uuid.UUID,
) error {
	method, ok := s.paymentMethods[methodID]
	if !ok || method.UserID != userID {
		return sql.ErrNotFound
	}

	delete(s.paymentMethods, methodID)
	return nil
}

func (s *fakeUserService) Connect(
	_ context.Context,
	_ usercommands.ConnectCommand,
) error {
	return errors.New("oauth routes are not covered in this suite")
}

type fakeClawService struct {
	claws map[uuid.UUID]entities.Claw
}

type fakeIntegrationService struct {
	items map[uuid.UUID]entities.AccountIntegration
}

type fakeGoogleOAuthService struct {
	startResult    googleoauth.StartResult
	startErr       error
	completeResult googleoauth.CompleteResult
	completeErr    error
	completeCalls  int
}

func newFakeIntegrationService() *fakeIntegrationService {
	return &fakeIntegrationService{items: map[uuid.UUID]entities.AccountIntegration{}}
}

func newFakeGoogleOAuthService() *fakeGoogleOAuthService {
	return &fakeGoogleOAuthService{}
}

func (s *fakeIntegrationService) List(_ context.Context, userID uuid.UUID) ([]entities.AccountIntegration, error) {
	out := make([]entities.AccountIntegration, 0, len(s.items))
	for _, item := range s.items {
		if item.UserID == userID {
			out = append(out, item)
		}
	}

	return out, nil
}

func (s *fakeIntegrationService) Connect(
	_ context.Context,
	cmd integrationservice.ConnectCommand,
) (entities.AccountIntegration, error) {
	id := cmd.ID
	if id == uuid.Nil {
		id = uuid.New()
	}

	capability := integrationservice.CapabilityFromProvider(cmd.Provider)
	integration := entities.AccountIntegration{
		ID:                id,
		UserID:            cmd.UserID,
		CapabilityID:      capability,
		Provider:          cmd.Provider,
		ExternalAccountID: cmd.ExternalAccountID,
		DisplayName:       cmd.DisplayName,
		Status:            entities.AccountIntegrationStatusActive,
		SecretPayload:     cmd.SecretPayload,
		Metadata:          cmd.Metadata,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	s.items[id] = integration

	return integration, nil
}

func (s *fakeGoogleOAuthService) Start(
	_ context.Context,
	_ googleoauth.StartCommand,
) (googleoauth.StartResult, error) {
	return s.startResult, s.startErr
}

func (s *fakeGoogleOAuthService) Complete(
	_ context.Context,
	_ googleoauth.CompleteCommand,
) (googleoauth.CompleteResult, error) {
	s.completeCalls++
	return s.completeResult, s.completeErr
}

type fakeClawCapabilityService struct{}

func newFakeClawCapabilityService() *fakeClawCapabilityService {
	return &fakeClawCapabilityService{}
}

func (s *fakeClawCapabilityService) Attach(
	_ context.Context,
	_ clawcapabilityservice.AttachCommand,
) error {
	return nil
}

func (s *fakeClawCapabilityService) Detach(
	_ context.Context,
	_ uuid.UUID,
	_ uuid.UUID,
	_ entities.CapabilityID,
) error {
	return nil
}

func newFakeClawService() *fakeClawService {
	return &fakeClawService{
		claws: map[uuid.UUID]entities.Claw{},
	}
}

func fakeClawConfig() entities.ClawConfig {
	return entities.NewDefaultClawConfig("openrouter/openai/gpt-5.4")
}

func (s *fakeClawService) seed(userID uuid.UUID, name string) entities.Claw {
	now := time.Now()
	cl := entities.Claw{
		ID:                 uuid.New(),
		Name:               name,
		UserID:             userID,
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             fakeClawConfig(),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.claws[cl.ID] = cl

	return cl
}

func (s *fakeClawService) exists(id uuid.UUID) bool {
	_, ok := s.claws[id]

	return ok
}

func (s *fakeClawService) Create(
	_ context.Context,
	cm clawcommands.CreateClaw,
) (entities.Claw, error) {
	now := time.Now()
	cl := entities.Claw{
		ID:                 uuid.New(),
		Name:               cm.Name,
		UserID:             cm.UserID,
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             fakeClawConfig(),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.claws[cl.ID] = cl

	return cl, nil
}

func (s *fakeClawService) GetByID(
	_ context.Context,
	clawID uuid.UUID,
	userID uuid.UUID,
) (entities.Claw, error) {
	cl, ok := s.claws[clawID]
	if !ok || cl.UserID != userID {
		return entities.Claw{}, sql.ErrNotFound
	}

	return cl, nil
}

func (s *fakeClawService) GetByUserID(
	_ context.Context,
	userID uuid.UUID,
) ([]entities.Claw, error) {
	result := make([]entities.Claw, 0, len(s.claws))
	for _, cl := range s.claws {
		if cl.UserID == userID {
			result = append(result, cl)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID.String() < result[j].ID.String()
	})

	return result, nil
}

func (s *fakeClawService) Update(
	_ context.Context,
	cm clawcommands.UpdateClaw,
) (entities.Claw, error) {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return entities.Claw{}, sql.ErrNotFound
	}

	if cm.Name != nil {
		existing.Name = *cm.Name
	}

	existing.UpdatedAt = time.Now()
	s.claws[existing.ID] = existing

	return existing, nil
}

func (s *fakeClawService) Start(
	_ context.Context,
	cm clawcommands.StartClaw,
) (entities.Claw, error) {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return entities.Claw{}, sql.ErrNotFound
	}
	if existing.CurrentOperationID != nil {
		return entities.Claw{}, clawservice.ErrLifecycleOperationInProgress
	}

	existing.DesiredState = entities.ClawDesiredStateRunning
	existing.LifecycleStatus = entities.ClawLifecycleStatusStartPending
	opID := uuid.New()
	existing.CurrentOperationID = &opID
	existing.UpdatedAt = time.Now()
	s.claws[existing.ID] = existing

	return existing, nil
}

func (s *fakeClawService) Stop(
	_ context.Context,
	cm clawcommands.StopClaw,
) (entities.Claw, error) {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return entities.Claw{}, sql.ErrNotFound
	}
	if existing.CurrentOperationID != nil {
		return entities.Claw{}, clawservice.ErrLifecycleOperationInProgress
	}

	existing.DesiredState = entities.ClawDesiredStateStopped
	existing.LifecycleStatus = entities.ClawLifecycleStatusStopPending
	opID := uuid.New()
	existing.CurrentOperationID = &opID
	existing.UpdatedAt = time.Now()
	s.claws[existing.ID] = existing

	return existing, nil
}

func (s *fakeClawService) Restart(
	_ context.Context,
	cm clawcommands.RestartClaw,
) (entities.Claw, error) {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return entities.Claw{}, sql.ErrNotFound
	}
	if existing.CurrentOperationID != nil {
		return entities.Claw{}, clawservice.ErrLifecycleOperationInProgress
	}

	existing.DesiredState = entities.ClawDesiredStateRunning
	existing.LifecycleStatus = entities.ClawLifecycleStatusRestartPending
	opID := uuid.New()
	existing.CurrentOperationID = &opID
	existing.UpdatedAt = time.Now()
	s.claws[existing.ID] = existing

	return existing, nil
}

func (s *fakeClawService) ApprovePairing(
	_ context.Context,
	_ clawcommands.ApprovePairing,
) error {
	return errors.New("approve pairing is not covered in this suite")
}

func (s *fakeClawService) Connect(
	_ context.Context,
	_ clawcommands.ConnectClaw,
) error {
	return errors.New("connect is not covered in this suite")
}

func (s *fakeClawService) Delete(
	_ context.Context,
	cm clawcommands.DeleteClaw,
) (entities.Claw, error) {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return entities.Claw{}, sql.ErrNotFound
	}
	if existing.CurrentOperationID != nil {
		return entities.Claw{}, clawservice.ErrLifecycleOperationInProgress
	}

	existing.DesiredState = entities.ClawDesiredStateDeleted
	existing.LifecycleStatus = entities.ClawLifecycleStatusDeletePending
	opID := uuid.New()
	existing.CurrentOperationID = &opID
	existing.UpdatedAt = time.Now()
	s.claws[existing.ID] = existing

	return existing, nil
}

type fakeBillingService struct {
	plans                       map[uuid.UUID]entities.Plan
	subscriptions               map[uuid.UUID]entities.UserSubscription
	bootstrapResults            map[uuid.UUID]billingresult.Bootstrap
	paymentStatusBySubscription map[uuid.UUID]string
	balance                     map[uuid.UUID]int64
	lastWebhook                 billingcommands.PaymentEvent
	lastOpenRouterWebhook       billingcommands.OpenRouterUsageEvent
	subscribeErr                error
	webhookErr                  error
	openRouterWebhookErr        error
}

func newFakeBillingService(userID uuid.UUID) *fakeBillingService {
	return &fakeBillingService{
		plans:                       map[uuid.UUID]entities.Plan{},
		subscriptions:               map[uuid.UUID]entities.UserSubscription{},
		bootstrapResults:            map[uuid.UUID]billingresult.Bootstrap{},
		paymentStatusBySubscription: map[uuid.UUID]string{},
		balance:                     map[uuid.UUID]int64{userID: 0},
	}
}

func (s *fakeBillingService) seedPlan(code string) entities.Plan {
	now := time.Now().UTC()
	groupCode := code
	interval := entities.PlanIntervalMonthly
	switch {
	case strings.HasSuffix(code, "-monthly"):
		groupCode = strings.TrimSuffix(code, "-monthly")
		interval = entities.PlanIntervalMonthly
	case strings.HasSuffix(code, "-yearly"):
		groupCode = strings.TrimSuffix(code, "-yearly")
		interval = entities.PlanIntervalYearly
	}
	plan := entities.Plan{
		ID:                 uuid.New(),
		Code:               groupCode,
		Name:               "Plan " + groupCode,
		Interval:           interval,
		BillingAmountMinor: 99000,
		BalanceCreditMinor: 150000,
		Currency:           entities.RUB,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.plans[plan.ID] = plan

	return plan
}

func (s *fakeBillingService) CreatePlan(_ context.Context, cm billingcommands.CreatePlan) (entities.Plan, error) {
	now := time.Now().UTC()
	plan := entities.Plan{
		ID:                 uuid.New(),
		Code:               cm.Code,
		Name:               cm.Name,
		Interval:           cm.Interval,
		BillingAmountMinor: cm.BillingAmountMinor,
		BalanceCreditMinor: cm.BalanceCreditMinor,
		Currency:           cm.Currency,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.plans[plan.ID] = plan

	return plan, nil
}

func (s *fakeBillingService) GetPlan(_ context.Context, id uuid.UUID) (entities.Plan, error) {
	plan, ok := s.plans[id]
	if !ok {
		return entities.Plan{}, sql.ErrNotFound
	}

	return plan, nil
}

func (s *fakeBillingService) ListPlans(_ context.Context, includeInactive bool) ([]entities.Plan, error) {
	plans := make([]entities.Plan, 0, len(s.plans))
	for _, plan := range s.plans {
		if !includeInactive && !plan.IsActive {
			continue
		}
		plans = append(plans, plan)
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Code < plans[j].Code
	})

	return plans, nil
}

func (s *fakeBillingService) UpdatePlan(_ context.Context, cm billingcommands.UpdatePlan) (entities.Plan, error) {
	plan, ok := s.plans[cm.ID]
	if !ok {
		return entities.Plan{}, sql.ErrNotFound
	}

	plan.Code = cm.Code
	plan.Name = cm.Name
	plan.Interval = cm.Interval
	plan.BillingAmountMinor = cm.BillingAmountMinor
	plan.BalanceCreditMinor = cm.BalanceCreditMinor
	plan.Currency = cm.Currency
	plan.IsActive = cm.IsActive
	plan.UpdatedAt = time.Now().UTC()
	s.plans[plan.ID] = plan

	return plan, nil
}

func (s *fakeBillingService) DeactivatePlan(_ context.Context, id uuid.UUID) error {
	plan, ok := s.plans[id]
	if !ok {
		return sql.ErrNotFound
	}

	plan.IsActive = false
	s.plans[id] = plan
	return nil
}

func (s *fakeBillingService) Subscribe(
	_ context.Context,
	cm billingcommands.Subscribe,
) (billingresult.SubscriptionCheckout, error) {
	if s.subscribeErr != nil {
		return billingresult.SubscriptionCheckout{}, s.subscribeErr
	}

	if _, ok := s.plans[cm.PlanID]; !ok {
		return billingresult.SubscriptionCheckout{}, sql.ErrNotFound
	}

	subscription := entities.UserSubscription{
		ID:                 uuid.New(),
		UserID:             cm.UserID,
		PlanID:             cm.PlanID,
		Status:             entities.SubscriptionStatusPending,
		StartedAt:          cm.Now,
		CurrentPeriodStart: cm.Now,
		CurrentPeriodEnd:   cm.Now.AddDate(0, 1, 0),
		CreatedAt:          cm.Now,
		UpdatedAt:          cm.Now,
	}
	s.subscriptions[cm.UserID] = subscription
	s.paymentStatusBySubscription[subscription.ID] = string(entities.Pending)
	return billingresult.SubscriptionCheckout{
		SubscriptionID:  subscription.ID,
		PaymentID:       "pay_" + subscription.ID.String(),
		Status:          subscription.Status,
		ConfirmationURL: "http://example.com/checkout/" + subscription.ID.String(),
		ReturnURL:       cm.ReturnURL + "?subscription_id=" + subscription.ID.String(),
	}, nil
}

func (s *fakeBillingService) GetSubscriptionCheckoutStatus(
	_ context.Context,
	userID, subscriptionID uuid.UUID,
) (billingresult.SubscriptionCheckoutStatus, error) {
	for _, subscription := range s.subscriptions {
		if subscription.ID != subscriptionID || subscription.UserID != userID {
			continue
		}

		paymentStatus := s.paymentStatusBySubscription[subscriptionID]
		if paymentStatus == "" {
			paymentStatus = string(entities.Pending)
		}

		var nextChargeAt *time.Time
		if subscription.Status == entities.SubscriptionStatusActive {
			next := subscription.CurrentPeriodEnd
			nextChargeAt = &next
		}

		return billingresult.SubscriptionCheckoutStatus{
			SubscriptionID:     subscription.ID,
			SubscriptionStatus: subscription.Status,
			PaymentStatus:      paymentStatus,
			PlanID:             subscription.PlanID,
			NextChargeAt:       nextChargeAt,
		}, nil
	}

	return billingresult.SubscriptionCheckoutStatus{}, sql.ErrNotFound
}

func (s *fakeBillingService) ChangePlan(_ context.Context, cm billingcommands.ChangePlan) (entities.UserSubscription, error) {
	subscription, ok := s.subscriptions[cm.UserID]
	if !ok {
		return entities.UserSubscription{}, sql.ErrNotFound
	}

	if subscription.Status == entities.SubscriptionStatusActive {
		return entities.UserSubscription{}, billingservice.ErrSubscriptionChangeWhileActive
	}

	subscription.PlanID = cm.PlanID
	subscription.UpdatedAt = cm.Now
	s.subscriptions[cm.UserID] = subscription
	return subscription, nil
}

func (s *fakeBillingService) CancelSubscription(_ context.Context, cm billingcommands.CancelSubscription) error {
	subscription, ok := s.subscriptions[cm.UserID]
	if !ok {
		return sql.ErrNotFound
	}

	subscription.Status = entities.SubscriptionStatusCanceled
	subscription.CanceledAt = &cm.Now
	subscription.UpdatedAt = cm.Now
	s.subscriptions[cm.UserID] = subscription
	return nil
}

func (s *fakeBillingService) GetCurrentSubscription(_ context.Context, userID uuid.UUID) (*billingservice.SubscriptionSummary, error) {
	subscription, ok := s.subscriptions[userID]
	if !ok || subscription.Status != entities.SubscriptionStatusActive {
		return nil, nil
	}

	plan, ok := s.plans[subscription.PlanID]
	if !ok {
		return nil, sql.ErrNotFound
	}

	return &billingservice.SubscriptionSummary{
		Subscription: subscription,
		Plan:         plan,
	}, nil
}

func (s *fakeBillingService) GetBillingSummary(_ context.Context, userID uuid.UUID) (billingservice.BillingSummary, error) {
	var nextChargeAt *time.Time
	if subscription, ok := s.subscriptions[userID]; ok && subscription.Status == entities.SubscriptionStatusActive {
		next := subscription.CurrentPeriodEnd
		nextChargeAt = &next
	}

	summary := billingservice.BillingSummary{
		BalanceMinor: strconv.FormatInt(s.balance[userID], 10),
		NextChargeAt: nextChargeAt,
	}

	current, err := s.GetCurrentSubscription(context.Background(), userID)
	if err != nil {
		return billingservice.BillingSummary{}, err
	}
	summary.CurrentSubscription = current

	return summary, nil
}

func (s *fakeBillingService) GetBootstrap(_ context.Context, userID uuid.UUID) (billingresult.Bootstrap, error) {
	if bootstrap, ok := s.bootstrapResults[userID]; ok {
		return bootstrap, nil
	}

	return billingresult.Bootstrap{
		DashboardAllowed: false,
		Onboarding: billingresult.BootstrapOnboarding{
			Required: true,
			Step:     billingresult.OnboardingStepSubscriptionRequired,
		},
	}, nil
}

func (s *fakeBillingService) EventPayment(_ context.Context, event billingcommands.PaymentEvent) error {
	s.lastWebhook = event
	return s.webhookErr
}

func (s *fakeBillingService) HandleOpenRouterUsageWebhook(
	_ context.Context,
	event billingcommands.OpenRouterUsageEvent,
) error {
	s.lastOpenRouterWebhook = event
	return s.openRouterWebhookErr
}

func (s *fakeBillingService) TopUp(_ context.Context, cm billingcommands.TopUp) (string, error) {
	if cm.UserID == uuid.Nil || strings.TrimSpace(cm.Amount) == "" {
		return "", sql.ErrInvalid
	}

	return "http://example.com/topup", nil
}

func (s *fakeBillingService) ExpanseAnalyze(
	_ context.Context,
	_ billingcommands.Expanse,
) (billingresult.Expanses, error) {
	return billingresult.Expanses{}, nil
}

type fakeServerService struct {
	servers map[uuid.UUID]entities.Server
}

func newFakeServerService() *fakeServerService {
	return &fakeServerService{
		servers: map[uuid.UUID]entities.Server{},
	}
}

func (s *fakeServerService) seed(name string) entities.Server {
	srv := entities.NewServer(
		name,
		"10.0.0.1",
		"http://"+name+".internal",
		"http://"+name+".internal/gmail-pubsub",
		"ready",
		"secret-"+name,
	)
	srv.MaxClaws = 7
	s.servers[srv.ID] = srv

	return srv
}

func (s *fakeServerService) exists(id uuid.UUID) bool {
	_, ok := s.servers[id]

	return ok
}

func (s *fakeServerService) Create(
	_ context.Context,
	cm servercommands.CreateServer,
) (entities.Server, error) {
	srv := entities.NewServer(cm.Name, cm.IP, cm.URL, cm.ProxyURL, cm.Status, cm.SecretKey)
	srv.MaxClaws = 7
	s.servers[srv.ID] = srv

	return srv, nil
}

func (s *fakeServerService) GetAll(_ context.Context) ([]entities.Server, error) {
	result := make([]entities.Server, 0, len(s.servers))
	for _, srv := range s.servers {
		result = append(result, srv)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID.String() < result[j].ID.String()
	})

	return result, nil
}

func (s *fakeServerService) GetByID(
	_ context.Context,
	id uuid.UUID,
) (entities.Server, error) {
	srv, ok := s.servers[id]
	if !ok {
		return entities.Server{}, sql.ErrNotFound
	}

	return srv, nil
}

func (s *fakeServerService) Update(
	_ context.Context,
	cm servercommands.UpdateServer,
) (entities.Server, error) {
	srv, ok := s.servers[cm.ID]
	if !ok {
		return entities.Server{}, sql.ErrNotFound
	}

	srv.Name = cm.Name
	srv.IP = cm.IP
	srv.URL = cm.URL
	srv.ProxyURL = cm.ProxyURL
	srv.Status = cm.Status
	srv.SecretKey = cm.SecretKey
	srv.MaxClaws = 7
	s.servers[srv.ID] = srv

	return srv, nil
}

func (s *fakeServerService) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := s.servers[id]; !ok {
		return sql.ErrNotFound
	}

	delete(s.servers, id)

	return nil
}
