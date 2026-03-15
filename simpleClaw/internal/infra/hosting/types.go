package hosting

import (
	"time"

	"github.com/google/uuid"
)

const (
	fileTypeJSON        = "json"
	openClawConfigName  = "openclaw"
	clawsEndpoint       = "/claws"
	approveEndpoint     = "/approve"
	clawsStartEndpoint  = "/claws/start"
	clawsStopEndpoint   = "/claws/stop"
	clawsConfigEndpoint = "/claws/config"
	defaultHTTPTimeout  = 15 * time.Second
)

// Container represents the runtime container that is provisioned for a claw.
type Container struct {
	// ID is the containerManager container record ID (DB primary key).
	ID       string
	ServerID uuid.UUID
	Status   string
}

type createClawRequest struct {
	UserID     string           `json:"userId"`
	ClawID     string           `json:"clawId"`
	Vars       []string         `json:"vars,omitempty"`
	ClawConfig []clawConfigFile `json:"clawConfig"`
}

type updateClawRequest struct {
	UserID     string           `json:"userId"`
	ClawID     string           `json:"clawId"`
	Vars       []string         `json:"vars,omitempty"`
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
