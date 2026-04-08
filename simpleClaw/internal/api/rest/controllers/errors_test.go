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

func TestMapServiceError_ApproveChannelRequired(t *testing.T) {
	t.Parallel()

	code, message := mapServiceError(
		fmt.Errorf("approve failed: %w", claw.ErrApproveChannelRequired),
	)

	if code != http.StatusBadRequest {
		t.Fatalf("unexpected code: %d", code)
	}

	if message != "channel type is required" {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestMapServiceError_WebSearchRequired(t *testing.T) {
	t.Parallel()

	code, message := mapServiceError(
		fmt.Errorf("create failed: %w", claw.ErrWebSearchRequired),
	)

	if code != http.StatusBadRequest {
		t.Fatalf("unexpected code: %d", code)
	}

	if message != "web search capability is required" {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestMapServiceError_BraveAPIKeyMissing(t *testing.T) {
	t.Parallel()

	code, message := mapServiceError(
		fmt.Errorf("create failed: %w", claw.ErrBraveAPIKeyMissing),
	)

	if code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected code: %d", code)
	}

	if message != "brave api key is not configured" {
		t.Fatalf("unexpected message: %q", message)
	}
}
