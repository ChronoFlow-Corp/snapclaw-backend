package models

import (
	"time"

	"github.com/google/uuid"
)

type ClawLifecycleOperation struct {
	ID                uuid.UUID  `gorm:"primaryKey;type:uuid"`
	ClawID            uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_claw_lifecycle_active_operation,where:status = 'pending' OR status = 'running' OR status = 'retry_scheduled'"`
	Type              string     `gorm:"type:varchar(32);not null"`
	Status            string     `gorm:"type:varchar(32);not null;index"`
	Stage             string     `gorm:"type:varchar(128);not null;default:''"`
	Attempt           int        `gorm:"not null;default:0"`
	LastError         string     `gorm:"type:text"`
	ServerID          *uuid.UUID `gorm:"type:uuid"`
	ContainerRecordID string     `gorm:"type:varchar(255)"`
	DockerContainerID string     `gorm:"type:varchar(255)"`
	NextRetryAt       *time.Time `gorm:"index"`
	CreatedAt         time.Time  `gorm:"autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime"`
}
