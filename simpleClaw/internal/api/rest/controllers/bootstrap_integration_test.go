package controllers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	billingresult "simpleClaw/internal/service/billing/result"
)

func TestRoutesIntegrationBootstrap_Unauthenticated(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	rr := env.request(t, http.MethodGet, "/api/me/bootstrap", nil)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
}

func TestRoutesIntegrationBootstrap_EligibleSubscriptionAndResumeState(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	clawID := uuid.New()
	periodEnd := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	env.billingService.bootstrapResults[env.user.ID] = billingresult.Bootstrap{
		DashboardAllowed: true,
		Subscription: &billingresult.BootstrapSubscription{
			Status:           "canceled",
			CurrentPeriodEnd: &periodEnd,
			AccessActive:     true,
		},
		Onboarding: billingresult.BootstrapOnboarding{
			Required: true,
			Step:     billingresult.OnboardingStepTelegramChoice,
			ClawID:   &clawID,
		},
	}

	rr := env.request(t, http.MethodGet, "/api/me/bootstrap", nil, accessCookie(env.accessToken))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		DashboardAllowed bool `json:"dashboard_allowed"`
		Subscription     struct {
			Status           string     `json:"status"`
			CurrentPeriodEnd *time.Time `json:"current_period_end"`
			AccessActive     bool       `json:"access_active"`
		} `json:"subscription"`
		Onboarding struct {
			Required        bool   `json:"required"`
			Step            string `json:"step"`
			ClawID          string `json:"claw_id"`
			TelegramManager *struct {
				ID string `json:"id"`
			} `json:"telegram_manager"`
		} `json:"onboarding"`
	}
	decodeJSON(t, rr, &payload)

	if !payload.DashboardAllowed {
		t.Fatal("DashboardAllowed = false, want true")
	}
	if payload.Subscription.Status != "canceled" {
		t.Fatalf("Subscription.Status = %q, want canceled", payload.Subscription.Status)
	}
	if payload.Subscription.CurrentPeriodEnd == nil || !payload.Subscription.CurrentPeriodEnd.Equal(periodEnd) {
		t.Fatalf("Subscription.CurrentPeriodEnd = %v, want %s", payload.Subscription.CurrentPeriodEnd, periodEnd)
	}
	if !payload.Subscription.AccessActive {
		t.Fatal("Subscription.AccessActive = false, want true")
	}
	if payload.Onboarding.Step != billingresult.OnboardingStepTelegramChoice {
		t.Fatalf("Onboarding.Step = %q, want %q", payload.Onboarding.Step, billingresult.OnboardingStepTelegramChoice)
	}
	if payload.Onboarding.ClawID != clawID.String() {
		t.Fatalf("Onboarding.ClawID = %q, want %q", payload.Onboarding.ClawID, clawID.String())
	}
	if payload.Onboarding.TelegramManager != nil {
		t.Fatalf("Onboarding.TelegramManager = %#v, want nil", payload.Onboarding.TelegramManager)
	}
}

func TestRoutesIntegrationBootstrap_NoEligibleSubscription(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	clawID := uuid.New()
	env.billingService.bootstrapResults[env.user.ID] = billingresult.Bootstrap{
		DashboardAllowed: false,
		Onboarding: billingresult.BootstrapOnboarding{
			Required: true,
			Step:     billingresult.OnboardingStepSubscriptionRequired,
			ClawID:   &clawID,
		},
	}

	rr := env.request(t, http.MethodGet, "/api/me/bootstrap", nil, accessCookie(env.accessToken))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		DashboardAllowed bool `json:"dashboard_allowed"`
		Onboarding       struct {
			Step   string `json:"step"`
			ClawID string `json:"claw_id"`
		} `json:"onboarding"`
	}
	decodeJSON(t, rr, &payload)

	if payload.DashboardAllowed {
		t.Fatal("DashboardAllowed = true, want false")
	}
	if payload.Onboarding.Step != billingresult.OnboardingStepSubscriptionRequired {
		t.Fatalf("Onboarding.Step = %q, want %q", payload.Onboarding.Step, billingresult.OnboardingStepSubscriptionRequired)
	}
	if payload.Onboarding.ClawID != clawID.String() {
		t.Fatalf("Onboarding.ClawID = %q, want %q", payload.Onboarding.ClawID, clawID.String())
	}
}

func TestRoutesIntegrationBootstrap_TelegramManagerPayload(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	clawID := uuid.New()
	managedBotID := uuid.New()
	channelID := uuid.New()
	linkExpiresAt := time.Date(2026, time.May, 17, 12, 30, 0, 0, time.UTC)
	env.billingService.bootstrapResults[env.user.ID] = billingresult.Bootstrap{
		DashboardAllowed: true,
		Onboarding: billingresult.BootstrapOnboarding{
			Required: true,
			Step:     billingresult.OnboardingStepTelegramManagerLink,
			ClawID:   &clawID,
			TelegramManager: &billingresult.BootstrapTelegramManager{
				ID:            managedBotID,
				Status:        "pending_link",
				DeepLinkURL:   "https://t.me/simpleclaw_manager_bot?start=resume-code",
				LinkExpiresAt: &linkExpiresAt,
				ChannelID:     &channelID,
				LastError:     "telegram rate limited",
			},
		},
	}

	rr := env.request(t, http.MethodGet, "/api/me/bootstrap", nil, accessCookie(env.accessToken))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Onboarding struct {
			Step            string `json:"step"`
			TelegramManager *struct {
				ID            string     `json:"id"`
				Status        string     `json:"status"`
				DeepLinkURL   string     `json:"deep_link_url"`
				LinkExpiresAt *time.Time `json:"link_expires_at"`
				ChannelID     string     `json:"channel_id"`
				LastError     string     `json:"last_error"`
			} `json:"telegram_manager"`
		} `json:"onboarding"`
	}
	decodeJSON(t, rr, &payload)

	if payload.Onboarding.Step != billingresult.OnboardingStepTelegramManagerLink {
		t.Fatalf("Onboarding.Step = %q, want %q", payload.Onboarding.Step, billingresult.OnboardingStepTelegramManagerLink)
	}

	if payload.Onboarding.TelegramManager == nil {
		t.Fatal("expected onboarding.telegram_manager")
	}

	if payload.Onboarding.TelegramManager.ID != managedBotID.String() {
		t.Fatalf("TelegramManager.ID = %q, want %q", payload.Onboarding.TelegramManager.ID, managedBotID.String())
	}

	if payload.Onboarding.TelegramManager.Status != "pending_link" {
		t.Fatalf("TelegramManager.Status = %q", payload.Onboarding.TelegramManager.Status)
	}

	if payload.Onboarding.TelegramManager.DeepLinkURL == "" {
		t.Fatal("expected deep_link_url")
	}

	if payload.Onboarding.TelegramManager.LinkExpiresAt == nil || !payload.Onboarding.TelegramManager.LinkExpiresAt.Equal(linkExpiresAt) {
		t.Fatalf("TelegramManager.LinkExpiresAt = %v, want %s", payload.Onboarding.TelegramManager.LinkExpiresAt, linkExpiresAt)
	}

	if payload.Onboarding.TelegramManager.ChannelID != channelID.String() {
		t.Fatalf("TelegramManager.ChannelID = %q, want %q", payload.Onboarding.TelegramManager.ChannelID, channelID.String())
	}

	if payload.Onboarding.TelegramManager.LastError != "telegram rate limited" {
		t.Fatalf("TelegramManager.LastError = %q", payload.Onboarding.TelegramManager.LastError)
	}
}

func TestRoutesIntegrationBootstrap_TelegramManagerProvisioningPayload(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	clawID := uuid.New()
	managedBotID := uuid.New()
	env.billingService.bootstrapResults[env.user.ID] = billingresult.Bootstrap{
		DashboardAllowed: true,
		Onboarding: billingresult.BootstrapOnboarding{
			Required: true,
			Step:     billingresult.OnboardingStepTelegramManagerProvisioning,
			ClawID:   &clawID,
			TelegramManager: &billingresult.BootstrapTelegramManager{
				ID:     managedBotID,
				Status: "linked",
			},
		},
	}

	rr := env.request(t, http.MethodGet, "/api/me/bootstrap", nil, accessCookie(env.accessToken))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Onboarding struct {
			Step            string `json:"step"`
			TelegramManager *struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"telegram_manager"`
		} `json:"onboarding"`
	}
	decodeJSON(t, rr, &payload)

	if payload.Onboarding.Step != billingresult.OnboardingStepTelegramManagerProvisioning {
		t.Fatalf("Onboarding.Step = %q, want %q", payload.Onboarding.Step, billingresult.OnboardingStepTelegramManagerProvisioning)
	}

	if payload.Onboarding.TelegramManager == nil {
		t.Fatal("expected onboarding.telegram_manager")
	}

	if payload.Onboarding.TelegramManager.ID != managedBotID.String() {
		t.Fatalf("TelegramManager.ID = %q, want %q", payload.Onboarding.TelegramManager.ID, managedBotID.String())
	}

	if payload.Onboarding.TelegramManager.Status != "linked" {
		t.Fatalf("TelegramManager.Status = %q", payload.Onboarding.TelegramManager.Status)
	}
}

func TestRoutesIntegrationBootstrap_TelegramManualConnectPayload(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	clawID := uuid.New()
	env.billingService.bootstrapResults[env.user.ID] = billingresult.Bootstrap{
		DashboardAllowed: true,
		Onboarding: billingresult.BootstrapOnboarding{
			Required: true,
			Step:     billingresult.OnboardingStepTelegramManualConnect,
			ClawID:   &clawID,
			TelegramManager: &billingresult.BootstrapTelegramManager{
				ID:        uuid.New(),
				Status:    "failed",
				LastError: "telegram manager provisioning failed",
			},
		},
	}

	rr := env.request(t, http.MethodGet, "/api/me/bootstrap", nil, accessCookie(env.accessToken))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Onboarding struct {
			Step            string `json:"step"`
			TelegramManager *struct {
				Status    string `json:"status"`
				LastError string `json:"last_error"`
			} `json:"telegram_manager"`
		} `json:"onboarding"`
	}
	decodeJSON(t, rr, &payload)

	if payload.Onboarding.Step != billingresult.OnboardingStepTelegramManualConnect {
		t.Fatalf("Onboarding.Step = %q, want %q", payload.Onboarding.Step, billingresult.OnboardingStepTelegramManualConnect)
	}

	if payload.Onboarding.TelegramManager == nil {
		t.Fatal("expected onboarding.telegram_manager")
	}

	if payload.Onboarding.TelegramManager.Status != "failed" {
		t.Fatalf("TelegramManager.Status = %q", payload.Onboarding.TelegramManager.Status)
	}

	if payload.Onboarding.TelegramManager.LastError != "telegram manager provisioning failed" {
		t.Fatalf("TelegramManager.LastError = %q", payload.Onboarding.TelegramManager.LastError)
	}
}
