package commands

import "github.com/google/uuid"

type Logout struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
}
