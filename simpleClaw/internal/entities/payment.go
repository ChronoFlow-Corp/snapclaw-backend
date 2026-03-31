package entities

import (
	"time"

	"github.com/google/uuid"
)

const ConfirmationTypeRedirect = "redirect"

type Status string

const (
	// Pending - data is being processed.
	Pending Status = "pending"

	// Waiting for capture.
	WaitingForCapture Status = "waiting_for_capture"

	// Succeeded — receipt successfully registered.
	Succeeded Status = "succeeded"

	// Canceled — receipt was not registered, you need to create it independently.
	Canceled Status = "canceled"
)

type Currency = string

const (
	RUB Currency = "RUB"
	USD Currency = "USD"
)

type PaymentPurpose string

const (
	PaymentPurposeTopUp        PaymentPurpose = "top_up"
	PaymentPurposeSubscription PaymentPurpose = "subscription"
)

type Payment struct {
	ID                   string                `json:"id"`
	UserID               uuid.UUID             `json:"user_id"`
	SubscriptionID       *uuid.UUID            `json:"subscription_id,omitempty"`
	Purpose              PaymentPurpose        `json:"purpose,omitempty"`
	Status               Status                `json:"status"`
	Paid                 bool                  `json:"paid"`
	Amount               Amount                `json:"amount"`
	AuthorizationDetails *AuthorizationDetails `json:"authorization_details,omitempty"`
	CreatedAt            time.Time             `json:"created_at"`
	Description          string                `json:"description,omitempty"`
	Confirmation         Confirmation          `json:"confirmation,omitempty"`
	ExpiresAt            *time.Time            `json:"expires_at,omitempty"`
	Metadata             interface{}           `json:"metadata,omitempty"`
	PaymentMethod        *PaymentMethodDetails `json:"payment_method,omitempty"`
	Recipient            *Recipient            `json:"recipient,omitempty"`
	Refundable           bool                  `json:"refundable"`
	Test                 bool                  `json:"test"`
	IncomeAmount         *Amount               `json:"income_amount,omitempty"`
}

type Amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type AuthorizationDetails struct {
	RRN          string        `json:"rrn,omitempty"`
	AuthCode     string        `json:"auth_code,omitempty"`
	ThreeDSecure *ThreeDSecure `json:"three_d_secure,omitempty"`
}

type ThreeDSecure struct {
	Applied bool `json:"applied"`
}

type PaymentMethodDetails struct {
	Type  string `json:"type"`
	ID    string `json:"id,omitempty"`
	Saved bool   `json:"saved,omitempty"`
	Card  *Card  `json:"card,omitempty"`
	Title string `json:"title,omitempty"`
}

type Card struct {
	First6        string       `json:"first6,omitempty"`
	Last4         string       `json:"last4,omitempty"`
	ExpiryMonth   string       `json:"expiry_month,omitempty"`
	ExpiryYear    string       `json:"expiry_year,omitempty"`
	CardType      string       `json:"card_type,omitempty"`
	CardProduct   *CardProduct `json:"card_product,omitempty"`
	IssuerCountry string       `json:"issuer_country,omitempty"`
	IssuerName    string       `json:"issuer_name,omitempty"`
}

type CardProduct struct {
	Code string `json:"code,omitempty"`
	Name string `json:"name,omitempty"`
}

type Recipient struct {
	AccountID string `json:"account_id"`
	GatewayID string `json:"gateway_id,omitempty"`
}

type Confirmation struct {
	Type            string  `json:"type"`
	ConfirmationURL *string `json:"confirmation_url,omitempty"`
	ReturnURL       *string `json:"return_url,omitempty"`
}

type PaymentEvent struct {
	Type   string
	Event  string
	Object Payment
}
