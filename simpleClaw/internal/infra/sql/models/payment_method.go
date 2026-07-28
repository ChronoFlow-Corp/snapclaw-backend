package models

import (
	"time"

	"github.com/google/uuid"
)

type PaymentMethod struct {
	ID         uuid.UUID  `gorm:"primaryKey;type:uuid"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index"`
	User       User       `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
	Title      string     `gorm:"type:varchar(255);not null"`
	IsDefault  bool       `gorm:"type:boolean;not null;default:false"`
	CreatedAt  time.Time  `gorm:"not null"`
	LastUsedAt *time.Time `gorm:"column:last_used_at"`
}
