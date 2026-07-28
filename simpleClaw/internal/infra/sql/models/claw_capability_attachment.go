package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type ClawCapabilityAttachment struct {
	ID                   uuid.UUID      `gorm:"primary_key;type:uuid;default:gen_random_uuid()"`
	ClawID               uuid.UUID      `gorm:"type:uuid;not null;index"`
	UserID               uuid.UUID      `gorm:"type:uuid;not null;index"`
	CapabilityID         string         `gorm:"type:varchar(64);not null"`
	Provider             string         `gorm:"type:varchar(64)"`
	AccountIntegrationID *uuid.UUID     `gorm:"type:uuid"`
	Enabled              bool           `gorm:"not null;default:true"`
	Settings             datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt            time.Time      `gorm:"autoCreateTime"`
	UpdatedAt            time.Time      `gorm:"autoUpdateTime"`
}
