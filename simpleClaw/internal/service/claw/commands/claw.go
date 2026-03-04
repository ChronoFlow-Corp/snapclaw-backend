package commands

import "github.com/google/uuid"

type CreateClaw struct {
	UserID      uuid.UUID
	Name        string
	ChannelIDs  []uuid.UUID
	Model       string
	ApiKeyLimit ApiKeyLimits
}

type ApiKeyLimits struct {
	RequestsPerMinute int
	MonthlyBudgetUSD  float64
}
