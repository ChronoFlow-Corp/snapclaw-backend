package hosting

import (
	"fmt"
	"time"

	"shared/pkg/hostingapi"

	"github.com/google/uuid"
)

const (
	fileTypeJSON        = "json"
	openClawConfigName  = "openclaw"
	runtimeBindingsName = "runtime-bindings"
	defaultHTTPTimeout  = 15 * time.Second
	clawsConfigEndpoint = "/claws/config"
	queryDeleteAfter    = "deleteAfter"
)

// Container represents the runtime container that is provisioned for a claw.
type Container struct {
	// ID is the containerManager container record ID (DB primary key).
	ID       string
	ServerID uuid.UUID
}

type RuntimeState struct {
	RuntimeRecordID   string
	DockerContainerID string
	ObservedState     string
	RuntimeStatus     string
	Port              string
	LastError         string
}

type RuntimeSecrets struct {
	BraveAPIKey string
}

func newLifecycleCommandRequest(userID, clawID, action string) hostingapi.LifecycleCommandRequest {
	key := fmt.Sprintf("%s:%s", clawID, action)

	return hostingapi.LifecycleCommandRequest{
		OperationID:    key,
		IdempotencyKey: key,
		UserID:         userID,
		ClawID:         clawID,
	}
}
