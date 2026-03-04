package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Channel struct {
	ID             uuid.UUID `gorm:"primary_key;type:uuid;default:gen_random_uuid()"`
	ChannelType    string    `gorm:"type:varchar(64);not null"`
	Name           string    `gorm:"type:varchar(255)"`
	UserID         uuid.UUID
	Claws          []*Claw        `gorm:"many2many:claw_channels;"`
	OpenClawConfig datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt      time.Time      `gorm:"autoUpdateTime"`
}
