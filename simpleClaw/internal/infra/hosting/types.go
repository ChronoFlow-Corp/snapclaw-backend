package hosting

import (
	"time"

	"github.com/google/uuid"
)

const (
	fileTypeJSON       = "json"
	openClawConfigName = "openclaw"
	clawsEndpoint      = "/claws"
	defaultHTTPTimeout = 15 * time.Second
)

// Container represents the runtime container that is provisioned for a claw.
type Container struct {
	ID       string
	ServerID uuid.UUID
	Status   string
}

type createClawRequest struct {
	UserID     string           `json:"userId"`
	ClawConfig []clawConfigFile `json:"clawConfig"`
}

type clawConfigFile struct {
	Name       string           `json:"name"`
	Data       string           `json:"data,omitempty"`
	FileType   string           `json:"fileType"`
	ClawConfig []clawConfigFile `json:"clawConfig,omitempty"`
}

type createClawResponse struct {
	ContainerID string `json:"containerId"`
	ServerID    string `json:"serverId,omitempty"`
	Status      string `json:"status,omitempty"`
}
