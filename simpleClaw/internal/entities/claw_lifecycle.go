package entities

import "github.com/google/uuid"

type ClawDesiredState string

const (
	ClawDesiredStateRunning ClawDesiredState = "running"
	ClawDesiredStateStopped ClawDesiredState = "stopped"
	ClawDesiredStateDeleted ClawDesiredState = "deleted"
)

type ClawObservedState string

const (
	ClawObservedStateRunning ClawObservedState = "running"
	ClawObservedStateStopped ClawObservedState = "stopped"
	ClawObservedStateMissing ClawObservedState = "missing"
	ClawObservedStateUnknown ClawObservedState = "unknown"
	ClawObservedStateError   ClawObservedState = "error"
)

type ClawLifecycleStatus string

const (
	ClawLifecycleStatusIdle             ClawLifecycleStatus = "idle"
	ClawLifecycleStatusStartPending     ClawLifecycleStatus = "start_pending"
	ClawLifecycleStatusStopPending      ClawLifecycleStatus = "stop_pending"
	ClawLifecycleStatusDeletePending    ClawLifecycleStatus = "delete_pending"
	ClawLifecycleStatusReconcilePending ClawLifecycleStatus = "reconcile_pending"
	ClawLifecycleStatusFailed           ClawLifecycleStatus = "failed"
)

type ClawLifecycleState struct {
	DesiredState       ClawDesiredState
	ObservedState      ClawObservedState
	LifecycleStatus    ClawLifecycleStatus
	CurrentOperationID *uuid.UUID
	LastError          string
}

type ClawRuntimeUpdate struct {
	ServerID           uuid.UUID
	ContainerRecordID  string
	DesiredState       ClawDesiredState
	ObservedState      ClawObservedState
	LifecycleStatus    ClawLifecycleStatus
	LastLifecycleError string
}

func NewClawLifecycleState() ClawLifecycleState {
	return ClawLifecycleState{
		DesiredState:    ClawDesiredStateStopped,
		ObservedState:   ClawObservedStateUnknown,
		LifecycleStatus: ClawLifecycleStatusIdle,
	}
}
