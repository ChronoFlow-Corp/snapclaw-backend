package commands

import (
	"containermanager/internal/entities"
)

type CreateClaw struct {
	UserID string
	Config []entities.ClawConfig
}

type StartClaw struct {
	UserID      string
	ContainerID string
}

type StopClaw struct {
	UserID      string
	ContainerID string
}

type DeleteClaw struct {
	UserID      string
	ContainerID string
}
