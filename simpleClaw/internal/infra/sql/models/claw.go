package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
)

type Claw struct {
	ID                 uuid.UUID      `gorm:"primary_key;type:uuid;default:gen_random_uuid()"`
	Name               string         `gorm:"type:varchar(255);not null"`
	Config             datatypes.JSON `gorm:"type:jsonb"`
	UserID             uuid.UUID
	ServerID           *uuid.UUID
	ContainerID        string `gorm:"type:varchar(255)"`
	DesiredState       string `gorm:"type:varchar(32);not null;default:stopped"`
	ObservedState      string `gorm:"type:varchar(32);not null;default:unknown"`
	LifecycleStatus    string `gorm:"type:varchar(32);not null;default:idle"`
	LastLifecycleError string `gorm:"type:text"`
	LastRuntimeSyncAt  *time.Time
	OnboardingComplete bool           `gorm:"not null;default:false"`
	CurrentOperationID *uuid.UUID     `gorm:"type:uuid"`
	Vars               pq.StringArray `gorm:"type:text[]"`
	Channels           []*Channel     `gorm:"many2many:claw_channels;"`
	CreatedAt          time.Time      `gorm:"autoCreateTime"`
	UpdatedAt          time.Time      `gorm:"autoUpdateTime"`
}
