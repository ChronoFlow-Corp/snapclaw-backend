package entities

import (
	"time"

	"github.com/google/uuid"
)

type PaymentMethod struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Title      string     `json:"title"`
	IsDefault  bool       `json:"is_default"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}
