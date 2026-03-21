package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"containermanager/internal/service"
	"shared/pkg/hostingapi"
)

func TestMapApproveError(t *testing.T) {
	t.Parallel()

	t.Run("invalid code", func(t *testing.T) {
		t.Parallel()

		code, payload := mapApproveError(
			fmt.Errorf("approve failed: %w", service.ErrInvalidCode),
		)

		if code != http.StatusBadRequest {
			t.Fatalf("unexpected code: %d", code)
		}

		if payload.Code != hostingapi.ErrCodeInvalidCode {
			t.Fatalf("unexpected payload code: %q", payload.Code)
		}

		if payload.Message != service.ErrInvalidCode.Error() {
			t.Fatalf("unexpected payload message: %q", payload.Message)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		t.Parallel()

		code, payload := mapApproveError(fmt.Errorf("boom"))

		if code != http.StatusInternalServerError {
			t.Fatalf("unexpected code: %d", code)
		}

		if payload.Code != hostingapi.ErrCodeInternal {
			t.Fatalf("unexpected payload code: %q", payload.Code)
		}

		if payload.Message != "boom" {
			t.Fatalf("unexpected payload message: %q", payload.Message)
		}
	})
}
