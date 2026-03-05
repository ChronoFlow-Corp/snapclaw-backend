package entities

import (
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	RefreshToken string
	CreatedAt    time.Time
}

func NewSession(userID uuid.UUID) Session {
	return Session{
		ID:        uuid.New(),
		UserID:    userID,
		CreatedAt: time.Now(),
	}
}
