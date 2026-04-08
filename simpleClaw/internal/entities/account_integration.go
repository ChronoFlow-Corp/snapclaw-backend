package entities

import (
	"time"

	"github.com/google/uuid"
)

type AccountIntegrationStatus string

const (
	AccountIntegrationStatusActive   AccountIntegrationStatus = "active"
	AccountIntegrationStatusDisabled AccountIntegrationStatus = "disabled"
)

type AccountIntegration struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	CapabilityID      CapabilityID
	Provider          string
	ExternalAccountID string
	DisplayName       string
	Status            AccountIntegrationStatus
	SecretPayload     map[string]any
	Metadata          map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
