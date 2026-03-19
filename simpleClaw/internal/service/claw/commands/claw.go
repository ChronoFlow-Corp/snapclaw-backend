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
	MonthlyBudgetUSD float64
}

type UpdateClaw struct {
	UserID      uuid.UUID
	ClawID      uuid.UUID
	Name        *string
	ChannelIDs  []uuid.UUID
	Model       *string
	ApiKeyLimit ApiKeyLimits
}

type StartClaw struct {
	UserID uuid.UUID
	ClawID uuid.UUID
}

type StopClaw struct {
	UserID uuid.UUID
	ClawID uuid.UUID
}

type DeleteClaw struct {
	UserID       uuid.UUID
	ClawID       uuid.UUID
	DeleteConfig bool
}

type ApprovePairing struct {
	UserID uuid.UUID
	ClawID uuid.UUID
	Code   string
}

type ConnectClaw struct {
	UserID   uuid.UUID
	ClawID   uuid.UUID
	Provider string
}
