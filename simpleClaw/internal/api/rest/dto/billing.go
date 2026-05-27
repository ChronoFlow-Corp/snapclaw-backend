package dto

import (
	"time"

	"github.com/google/uuid"
)

type SubscribeRequest struct {
	PlanID string `json:"plan_id"`
}

type ChangeSubscriptionRequest struct {
	PlanID string `json:"plan_id"`
}

type SubscriptionCheckoutResponse struct {
	SubscriptionID  string `json:"subscription_id"`
	PaymentID       string `json:"payment_id"`
	Status          string `json:"status"`
	ConfirmationURL string `json:"confirmation_url"`
	ReturnURL       string `json:"return_url"`
}

type SubscriptionCheckoutStatusResponse struct {
	SubscriptionID     string     `json:"subscription_id"`
	SubscriptionStatus string     `json:"subscription_status"`
	PaymentStatus      string     `json:"payment_status"`
	PlanID             string     `json:"plan_id"`
	NextChargeAt       *time.Time `json:"next_charge_at,omitempty"`
}

type SubscriptionResponse struct {
	ID                 string     `json:"id"`
	PlanID             string     `json:"plan_id"`
	Status             string     `json:"status"`
	StartedAt          time.Time  `json:"started_at"`
	CurrentPeriodStart time.Time  `json:"current_period_start"`
	CurrentPeriodEnd   time.Time  `json:"current_period_end"`
	CanceledAt         *time.Time `json:"canceled_at,omitempty"`
}

type BillingSummaryResponse struct {
	Balance             string                `json:"balance"`
	CurrentSubscription *SubscriptionResponse `json:"current_subscription,omitempty"`
	NextChargeAt        *time.Time            `json:"next_charge_at,omitempty"`
}

type BootstrapResponse struct {
	DashboardAllowed bool                        `json:"dashboard_allowed"`
	Subscription     *BootstrapSubscriptionDTO   `json:"subscription,omitempty"`
	Onboarding       BootstrapOnboardingResponse `json:"onboarding"`
}

type BootstrapSubscriptionDTO struct {
	Status           string     `json:"status"`
	CurrentPeriodEnd *time.Time `json:"current_period_end,omitempty"`
	AccessActive     bool       `json:"access_active"`
}

type BootstrapOnboardingResponse struct {
	Required        bool                         `json:"required"`
	Step            string                       `json:"step"`
	ClawID          string                       `json:"claw_id,omitempty"`
	TelegramManager *BootstrapTelegramManagerDTO `json:"telegram_manager,omitempty"`
}

type BootstrapTelegramManagerDTO struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	DeepLinkURL   string     `json:"deep_link_url,omitempty"`
	LinkExpiresAt *time.Time `json:"link_expires_at,omitempty"`
	ChannelID     string     `json:"channel_id,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type TopUpRequest struct {
	Amount          string     `json:"amount"`
	PaymentMethodID *uuid.UUID `json:"payment_method_id,omitempty"`
}

type ExpanseAnalyzeResponse struct {
	Today   string `json:"today"`
	Weekly  string `json:"weekly"`
	Monthly string `json:"monthly"`
	Daily   string `json:"daily"`
}
