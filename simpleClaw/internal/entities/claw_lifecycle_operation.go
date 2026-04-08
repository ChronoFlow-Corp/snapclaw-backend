package entities

import (
	"time"

	"github.com/google/uuid"
)

type ClawLifecycleOperationType string

const (
	ClawLifecycleOperationTypeStart     ClawLifecycleOperationType = "start"
	ClawLifecycleOperationTypeStop      ClawLifecycleOperationType = "stop"
	ClawLifecycleOperationTypeRestart   ClawLifecycleOperationType = "restart"
	ClawLifecycleOperationTypeDelete    ClawLifecycleOperationType = "delete"
	ClawLifecycleOperationTypeReconcile ClawLifecycleOperationType = "reconcile"
)

type ClawLifecycleOperationStatus string

const (
	ClawLifecycleOperationStatusPending        ClawLifecycleOperationStatus = "pending"
	ClawLifecycleOperationStatusRunning        ClawLifecycleOperationStatus = "running"
	ClawLifecycleOperationStatusSucceeded      ClawLifecycleOperationStatus = "succeeded"
	ClawLifecycleOperationStatusFailed         ClawLifecycleOperationStatus = "failed"
	ClawLifecycleOperationStatusRetryScheduled ClawLifecycleOperationStatus = "retry_scheduled"
	ClawLifecycleOperationStatusAbandoned      ClawLifecycleOperationStatus = "abandoned"
)

type ClawLifecycleOperation struct {
	ID                uuid.UUID
	ClawID            uuid.UUID
	Type              ClawLifecycleOperationType
	Status            ClawLifecycleOperationStatus
	Stage             string
	Attempt           int
	LastError         string
	ServerID          uuid.UUID
	ContainerRecordID string
	DockerContainerID string
	NextRetryAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
