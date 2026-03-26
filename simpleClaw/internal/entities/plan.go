package entities

import (
	"time"

	"github.com/google/uuid"
)

type Plan struct {
	ID                 uuid.UUID
	Code               string
	Name               string
	BillingAmountMinor int64
	BalanceCreditMinor int64
	Currency           string
	IsActive           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
