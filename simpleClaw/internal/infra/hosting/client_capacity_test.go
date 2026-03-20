package hosting

import (
	"errors"
	"net/http"
	"shared/pkg/hostingapi"
	"testing"
)

func TestUnexpectedStatusError_MapsCapacityErrors(t *testing.T) {
	t.Parallel()

	err := unexpectedStatusError(
		"test-op",
		http.StatusConflict,
		"409 Conflict",
		[]byte(`{"code":"server_capacity_exceeded","message":"server capacity exceeded"}`),
	)
	if !errors.Is(err, ErrServerCapacityExceeded) {
		t.Fatalf("expected ErrServerCapacityExceeded, got %v", err)
	}

	err = unexpectedStatusError(
		"test-op",
		http.StatusConflict,
		"409 Conflict",
		[]byte(`{"code":"server_memory_unavailable","message":"server memory unavailable"}`),
	)
	if !errors.Is(err, ErrServerMemoryUnavailable) {
		t.Fatalf("expected ErrServerMemoryUnavailable, got %v", err)
	}

	if hostingapi.ErrCodeServerCapacityExceeded == "" || hostingapi.ErrCodeServerMemoryUnavailable == "" {
		t.Fatal("expected shared error codes to be defined")
	}
}
