package controllers

import (
	"fmt"
	"net/http"
	"testing"

	"containermanager/internal/service"
)

func TestMapApproveError(t *testing.T) {
	t.Parallel()

	t.Run("invalid code", func(t *testing.T) {
		t.Parallel()

		code, message := mapApproveError(
			fmt.Errorf("approve failed: %w", service.ErrInvalidCode),
		)

		if code != http.StatusBadRequest {
			t.Fatalf("unexpected code: %d", code)
		}
		if message != service.ErrInvalidCode.Error() {
			t.Fatalf("unexpected message: %q", message)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		t.Parallel()

		code, message := mapApproveError(fmt.Errorf("boom"))

		if code != http.StatusInternalServerError {
			t.Fatalf("unexpected code: %d", code)
		}
		if message != "boom" {
			t.Fatalf("unexpected message: %q", message)
		}
	})
}
