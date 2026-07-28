package models

import (
	"time"

	"github.com/google/uuid"
)

type UserSubscription struct {
	ID                 uuid.UUID `gorm:"primaryKey;type:uuid"`
	UserID             uuid.UUID `gorm:"type:uuid;not null;index"`
	User               User      `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
	PlanID             uuid.UUID `gorm:"type:uuid;not null;index"`
	Plan               Plan      `gorm:"constraint:OnDelete:RESTRICT;foreignKey:PlanID;references:ID"`
	Status             string    `gorm:"type:varchar(32);not null;index"`
	StartedAt          time.Time `gorm:"not null"`
	CurrentPeriodStart time.Time `gorm:"not null"`
	CurrentPeriodEnd   time.Time `gorm:"not null"`
	CanceledAt         *time.Time
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}
