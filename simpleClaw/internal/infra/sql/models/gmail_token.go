package models

import (
	"time"

	"github.com/google/uuid"
)

type GmailToken struct {
	UserID       uuid.UUID `gorm:"primary_key;type:uuid"`
	Email        string    `gorm:"type:varchar(255)"`
	Client       string    `gorm:"type:varchar(64)"`
	AccessToken  string    `gorm:"type:text"`
	RefreshToken string    `gorm:"type:text"`
	TokenType    string    `gorm:"type:varchar(32)"`
	ExpiresAt    time.Time
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}
