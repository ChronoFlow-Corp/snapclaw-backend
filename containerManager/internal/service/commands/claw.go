package commands

import (
	"containermanager/internal/entities"
)

type CreateClaw struct {
	UserID string
	ClawID string
	Vars   []string
	Config []entities.ClawConfig
}

type StartClaw struct {
	UserID string
	ClawID string
}

type StopClaw struct {
	UserID string
	ClawID string
}

type DeleteClaw struct {
	UserID       string
	ClawID       string
	DeleteConfig bool
}

type UpdateClaw struct {
	UserID string
	ClawID string
	Vars   []string
	Config []entities.ClawConfig
}

type ConfigArchive struct {
	UserID      string
	ClawID      string
	DeleteAfter bool
}

type RestoreConfig struct {
	UserID string
	ClawID string
}
