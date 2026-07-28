package entities

import (
	"testing"
)

func TestClawLifecycleDefaults(t *testing.T) {
	cl := NewClawLifecycleState()

	if cl.DesiredState != ClawDesiredStateStopped {
		t.Fatalf("unexpected desired state: %s", cl.DesiredState)
	}

	if cl.ObservedState != ClawObservedStateUnknown {
		t.Fatalf("unexpected observed state: %s", cl.ObservedState)
	}

	if cl.LifecycleStatus != ClawLifecycleStatusIdle {
		t.Fatalf("unexpected lifecycle status: %s", cl.LifecycleStatus)
	}

	if cl.CurrentOperationID != nil {
		t.Fatalf("expected nil current operation, got %v", cl.CurrentOperationID)
	}

	if cl.LastError != "" {
		t.Fatalf("unexpected last error: %q", cl.LastError)
	}
}
