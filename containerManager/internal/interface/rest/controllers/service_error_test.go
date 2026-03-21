package controllers

import (
	"net/http"
	"testing"

	"containermanager/internal/service"
	"shared/pkg/hostingapi"
)

func TestMapServiceError(t *testing.T) {
	t.Parallel()

	t.Run("capacity exceeded", func(t *testing.T) {
		t.Parallel()

		code, payload := mapServiceError(service.ErrServerCapacityExceeded)
		if code != http.StatusConflict {
			t.Fatalf("code = %d, want %d", code, http.StatusConflict)
		}

		if payload.Code != hostingapi.ErrCodeServerCapacityExceeded {
			t.Fatalf("payload code = %q", payload.Code)
		}
	})

	t.Run("memory unavailable", func(t *testing.T) {
		t.Parallel()

		code, payload := mapServiceError(service.ErrServerMemoryUnavailable)
		if code != http.StatusConflict {
			t.Fatalf("code = %d, want %d", code, http.StatusConflict)
		}

		if payload.Code != hostingapi.ErrCodeServerMemoryUnavailable {
			t.Fatalf("payload code = %q", payload.Code)
		}
	})
}
