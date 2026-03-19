package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID               uuid.UUID   `gorm:"primary_key;type:uuid;default:gen_random_uuid()"`
	Name             string      `gorm:"type:varchar(255)"`
	NickName         string      `gorm:"type:varchar(255)"`
	AvatarURL        string      `gorm:"type:varchar(255)"`
	Email            string      `gorm:"type:varchar(255);unique"`
	Role             string      `gorm:"type:varchar(16)"`
	OpenRouterKeyID  string      `gorm:"type:varchar(128)"`
	OpenRouterApiKey string      `gorm:"type:varchar(355)"`
	CreatedAt        time.Time   `gorm:"autoCreateTime"`
	UpdatedAt        time.Time   `gorm:"autoUpdateTime"`
	Sessions         []Session   `gorm:"constraint:OnDelete:CASCADE"`
	GmailToken       *GmailToken `gorm:"constraint:OnDelete:CASCADE"`
	Channels         []Channel   `gorm:"constraint:OnDelete:CASCADE"`
	Claws            []Claw      `gorm:"constraint:OnDelete:CASCADE"`
}

type Session struct {
	ID           uuid.UUID `gorm:"primary_key;type:uuid;default:gen_random_uuid()"`
	UserID       uuid.UUID
	RefreshToken string    `gorm:"type:text"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}
