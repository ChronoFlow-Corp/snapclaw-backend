package hosting

import (
	"time"

	"github.com/google/uuid"
)

const (
	fileTypeJSON       = "json"
	openClawConfigName = "openclaw"
	defaultHTTPTimeout = 15 * time.Second
)

// Container represents the runtime container that is provisioned for a claw.
type Container struct {
	// ID is the containerManager container record ID (DB primary key).
	ID       string
	ServerID uuid.UUID
	Status   string
}
