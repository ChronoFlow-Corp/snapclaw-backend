package entities

import (
	"time"

	"github.com/google/uuid"
)

type OpenRouterUsageEvent struct {
	ID         uuid.UUID
	Provider   string
	TraceID    string
	SpanID     string
	UserID     uuid.UUID
	Model      string
	APIKeyName string
	CreatedAt  time.Time
}
