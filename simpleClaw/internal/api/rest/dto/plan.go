package dto

import "time"

type CreatePlanRequest struct {
	Code               string `json:"code"`
	Name               string `json:"name"`
	BillingAmountMinor int64  `json:"billing_amount_minor"`
	BalanceCreditMinor int64  `json:"balance_credit_minor"`
	Currency           string `json:"currency"`
}

type UpdatePlanRequest struct {
	Code               string `json:"code"`
	Name               string `json:"name"`
	BillingAmountMinor int64  `json:"billing_amount_minor"`
	BalanceCreditMinor int64  `json:"balance_credit_minor"`
	Currency           string `json:"currency"`
	IsActive           bool   `json:"is_active"`
}

type PlanResponse struct {
	ID                 string    `json:"id"`
	Code               string    `json:"code"`
	Name               string    `json:"name"`
	BillingAmountMinor int64     `json:"billing_amount_minor"`
	BalanceCreditMinor int64     `json:"balance_credit_minor"`
	Currency           string    `json:"currency"`
	IsActive           bool      `json:"is_active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
