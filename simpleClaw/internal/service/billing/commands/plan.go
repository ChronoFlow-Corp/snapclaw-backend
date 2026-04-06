package commands

import "github.com/google/uuid"

type CreatePlan struct {
	Code               string
	Name               string
	Interval           string
	BillingAmountMinor int64
	BalanceCreditMinor int64
	Currency           string
}

type UpdatePlan struct {
	ID                 uuid.UUID
	Code               string
	Name               string
	Interval           string
	BillingAmountMinor int64
	BalanceCreditMinor int64
	Currency           string
	IsActive           bool
}
