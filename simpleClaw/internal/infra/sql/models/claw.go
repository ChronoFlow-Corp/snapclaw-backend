package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Claw struct {
	ID          uuid.UUID      `gorm:"primary_key;type:uuid;default:gen_random_uuid()"`
	Name        string         `gorm:"type:varchar(255);not null"`
	Config      datatypes.JSON `gorm:"type:jsonb"`
	UserID      uuid.UUID
	Status      string `gorm:"type:varchar(255);not null"`
	ServerID    uuid.UUID
	ContainerID string     `gorm:"type:varchar(255)"`
	Channels    []*Channel `gorm:"many2many:claw_channels;"`
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
}
