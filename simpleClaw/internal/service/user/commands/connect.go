package commands

import (
	"time"

	"github.com/google/uuid"
)

type ConnectCommand struct {
	Provider     string
	UserID       uuid.UUID
	Email        string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}
