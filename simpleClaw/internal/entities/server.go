package entities

import (
	"time"

	"github.com/google/uuid"
)

type Server struct {
	ID        uuid.UUID
	Name      string
	IP        string
	URL       string
	Status    string
	CreatedAt time.Time
}
