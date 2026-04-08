package hostingapi

type LifecycleCommandRequest struct {
	OperationID    string `json:"operationId"`
	IdempotencyKey string `json:"idempotencyKey"`
	UserID         string `json:"userId"`
	ClawID         string `json:"clawId"`
}

type ClawConfigFile struct {
	Name     string `json:"name"`
	FileType string `json:"fileType"`
	Data     string `json:"data"`
}

type EnsureRuntimeRequest struct {
	UserID     string           `json:"userId"`
	ClawID     string           `json:"clawId"`
	Vars       []string         `json:"vars,omitempty"`
	ClawConfig []ClawConfigFile `json:"clawConfig,omitempty"`
}

type EnsureRuntimeResponse struct {
	RuntimeRecordID   string       `json:"runtimeRecordId,omitempty"`
	DockerContainerID string       `json:"dockerContainerId,omitempty"`
	Port              BoundTCPPort `json:"port,omitempty"`
}

// RuntimeObservedState is the last lifecycle state observed from the execution plane.
// It is a snapshot of what the host reported, not a desired state.
type RuntimeObservedState string

// RuntimeExecutionStatus describes the controller's normalized status for the runtime snapshot.
type RuntimeExecutionStatus string

// BoundTCPPort is the bound TCP port value for the runtime container, encoded as a decimal string.
type BoundTCPPort string

type RuntimeStateResponse struct {
	RuntimeRecordID   string `json:"runtimeRecordId,omitempty"`
	DockerContainerID string `json:"dockerContainerId,omitempty"`
	// ObservedState records the lifecycle phase reported by the execution plane.
	ObservedState RuntimeObservedState `json:"observedState"`
	// RuntimeStatus records the normalized controller status for the runtime snapshot.
	RuntimeStatus RuntimeExecutionStatus `json:"runtimeStatus"`
	// Port is the bound TCP port value as a string.
	Port      BoundTCPPort `json:"port,omitempty"`
	LastError string       `json:"lastError,omitempty"`
}

type CapacityResponse struct {
	MaxClaws int `json:"maxClaws"`
}
