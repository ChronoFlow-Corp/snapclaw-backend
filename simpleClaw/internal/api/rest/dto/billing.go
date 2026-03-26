package dto

import "time"

type SubscribeRequest struct {
	PlanID string `json:"plan_id"`
}

type ChangeSubscriptionRequest struct {
	PlanID string `json:"plan_id"`
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
	BalanceMinor        int64                 `json:"balance_minor"`
	CurrentSubscription *SubscriptionResponse `json:"current_subscription,omitempty"`
	NextChargeAt        *time.Time            `json:"next_charge_at,omitempty"`
}

type TopUpRequest struct {
	Amount int64 `json:"amount"`
}
