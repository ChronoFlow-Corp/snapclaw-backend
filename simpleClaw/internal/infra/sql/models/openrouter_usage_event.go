package models

import (
	"time"

	"github.com/google/uuid"
)

type OpenRouterUsageEvent struct {
	ID         uuid.UUID `gorm:"primaryKey;type:uuid"`
	Provider   string    `gorm:"type:varchar(32);not null;uniqueIndex:idx_openrouter_usage_events_ref"`
	TraceID    string    `gorm:"type:varchar(128);not null;uniqueIndex:idx_openrouter_usage_events_ref"`
	SpanID     string    `gorm:"type:varchar(128);not null;uniqueIndex:idx_openrouter_usage_events_ref"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index"`
	User       User      `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
	Model      string    `gorm:"type:varchar(255)"`
	APIKeyName string    `gorm:"type:varchar(255);not null"`
	CreatedAt  time.Time `gorm:"not null"`
}
