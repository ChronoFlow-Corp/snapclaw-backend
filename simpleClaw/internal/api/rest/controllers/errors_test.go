package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"simpleClaw/internal/service/claw"
)

func TestMapServiceError_InvalidPairingCode(t *testing.T) {
	t.Parallel()

	code, message := mapServiceError(
		fmt.Errorf("approve failed: %w", claw.ErrPairingCodeInvalid),
	)

	if code != http.StatusBadRequest {
		t.Fatalf("unexpected code: %d", code)
	}

	if message != "invalid code" {
		t.Fatalf("unexpected message: %q", message)
	}
}
