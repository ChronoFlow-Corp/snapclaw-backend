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
	BalanceMinor        string                `json:"balance_minor"`
	CurrentSubscription *SubscriptionResponse `json:"current_subscription,omitempty"`
	NextChargeAt        *time.Time            `json:"next_charge_at,omitempty"`
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
