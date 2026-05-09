package commands

import "github.com/google/uuid"

type CreateClaw struct {
	UserID       uuid.UUID
	Name         string
	ChannelIDs   []uuid.UUID
	Model        string
	ApiKeyLimit  ApiKeyLimits
	Capabilities CreateCapabilitySet
}

type CreateCapabilitySet struct {
	WebSearch      *WebSearchCapabilityInput
	FilesImages    *ToggleCapabilityInput
	Memory         *ToggleCapabilityInput
	Gmail          *IntegrationBoundCapabilityInput
	GoogleCalendar *IntegrationBoundCapabilityInput
	Sheets         *IntegrationBoundCapabilityInput
}

type WebSearchCapabilityInput struct {
	Enabled  bool
	Provider string
}

type ToggleCapabilityInput struct {
	Enabled bool
}

type IntegrationBoundCapabilityInput struct {
	Enabled              bool
	Provider             string
	AccountIntegrationID *uuid.UUID
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

type RestartClaw struct {
	UserID uuid.UUID
	ClawID uuid.UUID
}

type DeleteClaw struct {
	UserID       uuid.UUID
	ClawID       uuid.UUID
	DeleteConfig bool
}

type ApprovePairing struct {
	UserID      uuid.UUID
	ClawID      uuid.UUID
	Code        string
	ChannelType string
}

type ConnectClaw struct {
	UserID   uuid.UUID
	ClawID   uuid.UUID
	Provider string
}
