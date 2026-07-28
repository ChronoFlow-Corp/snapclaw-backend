package result

import (
	"time"

	"github.com/google/uuid"
)

type SubscriptionCheckout struct {
	SubscriptionID  uuid.UUID
	PaymentID       string
	Status          string
	ConfirmationURL string
	ReturnURL       string
}

type SubscriptionCheckoutStatus struct {
	SubscriptionID     uuid.UUID
	SubscriptionStatus string
	PaymentStatus      string
	PlanID             uuid.UUID
	NextChargeAt       *time.Time
}

const (
	OnboardingStepSubscriptionRequired        = "subscription_required"
	OnboardingStepTelegramChoice              = "telegram_choice"
	OnboardingStepTelegramManagerLink         = "telegram_manager_link"
	OnboardingStepTelegramManagerProvisioning = "telegram_manager_provisioning"
	OnboardingStepTelegramManualConnect       = "telegram_manual_connect"
	OnboardingStepTelegramConfirm             = "telegram_confirm"
	OnboardingStepDashboardReady              = "dashboard_ready"
)

type Bootstrap struct {
	DashboardAllowed bool
	Subscription     *BootstrapSubscription
	Onboarding       BootstrapOnboarding
}

type BootstrapSubscription struct {
	Status           string
	CurrentPeriodEnd *time.Time
	AccessActive     bool
}

type BootstrapOnboarding struct {
	Required        bool
	Step            string
	ClawID          *uuid.UUID
	TelegramManager *BootstrapTelegramManager
}

type BootstrapTelegramManager struct {
	ID            uuid.UUID
	Status        string
	DeepLinkURL   string
	LinkExpiresAt *time.Time
	ChannelID     *uuid.UUID
	LastError     string
}
