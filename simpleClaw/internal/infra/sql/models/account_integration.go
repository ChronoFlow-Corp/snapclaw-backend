package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type AccountIntegration struct {
	ID                uuid.UUID      `gorm:"primary_key;type:uuid;default:gen_random_uuid()"`
	UserID            uuid.UUID      `gorm:"type:uuid;not null;index"`
	CapabilityID      string         `gorm:"type:varchar(64);not null"`
	Provider          string         `gorm:"type:varchar(64);not null;index"`
	ExternalAccountID string         `gorm:"type:varchar(255)"`
	DisplayName       string         `gorm:"type:varchar(255)"`
	Status            string         `gorm:"type:varchar(32);not null"`
	SecretPayload     datatypes.JSON `gorm:"type:jsonb"`
	Metadata          datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt         time.Time      `gorm:"autoCreateTime"`
	UpdatedAt         time.Time      `gorm:"autoUpdateTime"`
}
