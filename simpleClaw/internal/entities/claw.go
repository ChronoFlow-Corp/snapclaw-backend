package entities

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusStop    = "stop"
	StatusRunning = "running"
)

type Claw struct {
	ID          uuid.UUID
	Name        string
	UserID      uuid.UUID
	ServerID    uuid.UUID
	Status      string
	ContainerID string
	Config      ClawConfig
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
