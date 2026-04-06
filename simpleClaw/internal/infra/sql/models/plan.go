package models

import (
	"time"

	"github.com/google/uuid"
)

type Plan struct {
	ID                 uuid.UUID          `gorm:"primaryKey;type:uuid"`
	Code               string             `gorm:"type:varchar(64);not null;uniqueIndex:idx_plan_code_interval"`
	Name               string             `gorm:"type:varchar(255);not null"`
	Interval           string             `gorm:"type:varchar(16);not null;uniqueIndex:idx_plan_code_interval"`
	BillingAmountMinor int64              `gorm:"not null"`
	BalanceCreditMinor int64              `gorm:"not null"`
	Currency           string             `gorm:"type:varchar(8);not null"`
	IsActive           bool               `gorm:"not null"`
	CreatedAt          time.Time          `gorm:"not null"`
	UpdatedAt          time.Time          `gorm:"not null"`
	Subscriptions      []UserSubscription `gorm:"constraint:OnDelete:RESTRICT"`
}
