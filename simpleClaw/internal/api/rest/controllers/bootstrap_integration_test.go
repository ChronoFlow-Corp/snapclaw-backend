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
			Required bool   `json:"required"`
			Step     string `json:"step"`
			ClawID   string `json:"claw_id"`
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
}

func TestRoutesIntegrationBootstrap_NoEligibleSubscription(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	env.billingService.bootstrapResults[env.user.ID] = billingresult.Bootstrap{
		DashboardAllowed: false,
		Onboarding: billingresult.BootstrapOnboarding{
			Required: true,
			Step:     billingresult.OnboardingStepSubscriptionRequired,
		},
	}

	rr := env.request(t, http.MethodGet, "/api/me/bootstrap", nil, accessCookie(env.accessToken))
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		DashboardAllowed bool `json:"dashboard_allowed"`
		Onboarding       struct {
			Step string `json:"step"`
		} `json:"onboarding"`
	}
	decodeJSON(t, rr, &payload)

	if payload.DashboardAllowed {
		t.Fatal("DashboardAllowed = true, want false")
	}
	if payload.Onboarding.Step != billingresult.OnboardingStepSubscriptionRequired {
		t.Fatalf("Onboarding.Step = %q, want %q", payload.Onboarding.Step, billingresult.OnboardingStepSubscriptionRequired)
	}
}
