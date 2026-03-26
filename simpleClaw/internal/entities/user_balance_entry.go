package entities

import (
	"time"

	"github.com/google/uuid"
)

const (
	BalanceEntryTypeTopUpCredit        = "top_up_credit"
	BalanceEntryTypeSubscriptionCredit = "subscription_credit"
	BalanceEntryTypeUsageDebit         = "usage_debit"
	BalanceEntryTypeRefundCredit       = "refund_credit"
	BalanceEntryTypeManualAdjustment   = "manual_adjustment"
)

type UserBalanceEntry struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Type           string
	AmountMinor    int64
	PaymentID      *string
	SubscriptionID *uuid.UUID
	Description    string
	CreatedAt      time.Time
}
