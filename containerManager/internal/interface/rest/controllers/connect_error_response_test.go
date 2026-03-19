package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shared/pkg/hostingapi"
	"testing"
)

func TestConnectValidationErrorResponse(t *testing.T) {
	t.Parallel()

	c := &Claw{}
	req := httptest.NewRequest(http.MethodPost, "/connect?userId=u1&clawId=c1", nil)
	rec := httptest.NewRecorder()

	c.Connect(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}

	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want %q", got, "application/json")
	}

	var payload hostingapi.ErrorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if payload.Code != hostingapi.ErrCodeValidation {
		t.Fatalf("code = %q, want %q", payload.Code, hostingapi.ErrCodeValidation)
	}
	if payload.Message != "provider is required" {
		t.Fatalf("message = %q, want %q", payload.Message, "provider is required")
	}
}
