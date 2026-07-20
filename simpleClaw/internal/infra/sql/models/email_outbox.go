package models

import (
	"time"

	"github.com/google/uuid"
)

type EmailOutbox struct {
	ID                uuid.UUID  `gorm:"primary_key;type:uuid"`
	ToAddress         string     `gorm:"type:varchar(320);not null;index"`
	Subject           string     `gorm:"type:varchar(998)"`
	HTMLBody          string     `gorm:"type:text"`
	TextBody          string     `gorm:"type:text"`
	Headers           string     `gorm:"type:text"`
	Category          string     `gorm:"type:varchar(64);index"`
	Status            string     `gorm:"type:varchar(16);not null;default:'pending';index:idx_email_outbox_due,priority:1"`
	Attempts          int        `gorm:"not null;default:0"`
	MaxAttempts       int        `gorm:"not null;default:5"`
	LastError         string     `gorm:"type:text"`
	ProviderMessageID string     `gorm:"type:varchar(128)"`
	NextAttemptAt     time.Time  `gorm:"index:idx_email_outbox_due,priority:2"`
	CreatedAt         time.Time  `gorm:"autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"autoUpdateTime"`
	SentAt            *time.Time `gorm:"index"`
}
