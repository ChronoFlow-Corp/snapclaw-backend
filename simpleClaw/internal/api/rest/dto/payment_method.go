package dto

import "time"

type AddPaymentMethodRequest struct {
	Title     string `json:"title"`
	IsDefault bool   `json:"is_default"`
}

type SetDefaultPaymentMethodRequest struct {
	IsDefault bool `json:"is_default"`
}

type PaymentMethodResponse struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	IsDefault  bool       `json:"is_default"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}
