package models

import (
	"time"

	"github.com/google/uuid"
)

type TelegramAccountLink struct {
	ID               uuid.UUID `gorm:"primary_key;type:uuid"`
	UserID           uuid.UUID `gorm:"type:uuid;not null;index"`
	LinkCodeHash     string    `gorm:"type:varchar(255);not null;uniqueIndex"`
	TelegramUserID   int64     `gorm:"index"`
	TelegramChatID   int64
	TelegramUsername string    `gorm:"type:varchar(255)"`
	Status           string    `gorm:"type:varchar(32);not null;index"`
	ExpiresAt        time.Time `gorm:"index"`
	CreatedAt        time.Time `gorm:"autoCreateTime"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}

type TelegramWebhookUpdate struct {
	UpdateID   int64     `gorm:"primary_key;autoIncrement:false"`
	ReceivedAt time.Time `gorm:"autoCreateTime"`
}

type TelegramManagedBot struct {
	ID                  uuid.UUID  `gorm:"primary_key;type:uuid"`
	UserID              uuid.UUID  `gorm:"type:uuid;not null;index"`
	ClawID              *uuid.UUID `gorm:"type:uuid;index"`
	TelegramOwnerUserID *int64     `gorm:"index"`
	ManagedBotUserID    *int64     `gorm:"uniqueIndex"`
	ManagedBotUsername  string     `gorm:"type:varchar(255);index"`
	ManagedBotName      string     `gorm:"type:varchar(255)"`
	DeepLinkURL         string     `gorm:"type:text"`
	LinkExpiresAt       *time.Time `gorm:"index"`
	ChannelID           *uuid.UUID `gorm:"type:uuid"`
	Status              string     `gorm:"type:varchar(32);not null;index"`
	LastError           string     `gorm:"type:text"`
	CreatedAt           time.Time  `gorm:"autoCreateTime"`
	UpdatedAt           time.Time  `gorm:"autoUpdateTime"`
}
