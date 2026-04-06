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
