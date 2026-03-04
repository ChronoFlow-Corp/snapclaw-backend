package hosting

import "github.com/google/uuid"

// Container represents the runtime container that is provisioned for a claw.
type Container struct {
	ID       string
	ServerID uuid.UUID
	Status   string
}
