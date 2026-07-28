package entities

import (
	"time"

	"github.com/google/uuid"
)

const (
	SubscriptionStatusActive   = "active"
	SubscriptionStatusPending  = "pending"
	SubscriptionStatusCanceled = "canceled"
	SubscriptionStatusPastDue  = "past_due"
)

type UserSubscription struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	PlanID             uuid.UUID
	Status             string
	StartedAt          time.Time
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CanceledAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
