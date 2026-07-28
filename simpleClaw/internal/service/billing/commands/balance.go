package commands

import (
	"time"

	"github.com/google/uuid"
)

type ChargeUsage struct {
	UserID      uuid.UUID
	AmountMinor int64
	Description string
	Now         time.Time
}

type TopUp struct {
	Amount          string
	PaymentMethodID *uuid.UUID
	UserID          uuid.UUID
}
