package entities

import (
	"time"

	"github.com/google/uuid"
)

type ClawCapabilityAttachment struct {
	ID                   uuid.UUID
	ClawID               uuid.UUID
	UserID               uuid.UUID
	CapabilityID         CapabilityID
	Provider             string
	AccountIntegrationID *uuid.UUID
	Enabled              bool
	Settings             map[string]any
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
