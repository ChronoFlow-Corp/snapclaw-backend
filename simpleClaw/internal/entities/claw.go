package entities

import (
	"time"

	"github.com/google/uuid"
)

type Claw struct {
	ID uuid.UUID

	Name   string
	UserID uuid.UUID

	ServerID    uuid.UUID
	ContainerID string

	ClawLifecycleState

	Config ClawConfig

	OnboardingComplete bool

	CreatedAt time.Time
	UpdatedAt time.Time
}
