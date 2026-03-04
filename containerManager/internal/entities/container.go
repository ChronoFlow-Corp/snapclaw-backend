package entities

import (
	"time"

	"github.com/google/uuid"
)

const (
	ClawConfigTypeDir  = "dir"
	ClawConfigTypeJson = "json"
	ClawConfigTypeMd   = "md"
)

const (
	ConstainerStatusStop   = "stop"
	ContainerStatusRunning = "running"
	ContainerStatusError   = "error"
)

type ContainerOptions struct {
	ID   string
	Name string
}

type Container struct {
	ID          uuid.UUID
	UserID      string
	ContainerID string
	Status      string
	Port        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ClawConfig struct {
	ClawConfig []ClawConfig
	Name       string
	Data       []byte
	FileType   string
}
