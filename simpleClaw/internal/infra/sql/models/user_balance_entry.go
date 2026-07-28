package models

import (
	"time"

	"github.com/google/uuid"
)

type UserBalanceEntry struct {
	ID             uuid.UUID `gorm:"primaryKey;type:uuid"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index"`
	User           User      `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
	Type           string    `gorm:"type:varchar(64);not null;index"`
	AmountMinor    int64     `gorm:"not null"`
	PaymentID      *string   `gorm:"type:varchar(255);index"`
	SubscriptionID *uuid.UUID
	Description    string    `gorm:"type:text"`
	CreatedAt      time.Time `gorm:"not null"`
}
