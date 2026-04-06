package commands

import (
	"time"

	"github.com/google/uuid"
)

type Subscribe struct {
	UserID    uuid.UUID
	PlanID    uuid.UUID
	ReturnURL string
	Now       time.Time
}

type ChangePlan struct {
	UserID uuid.UUID
	PlanID uuid.UUID
	Now    time.Time
}

type CancelSubscription struct {
	UserID uuid.UUID
	Now    time.Time
}
