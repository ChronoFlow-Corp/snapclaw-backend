package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shared/pkg/hostingapi"
	"testing"
)

func TestCapacityResponse(t *testing.T) {
	t.Parallel()

	c := &Claw{capacity: hostingapi.CapacityResponse{MaxClaws: 7}}
	req := httptest.NewRequest(http.MethodGet, "/capacity", nil)
	rec := httptest.NewRecorder()

	c.Capacity(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	var payload hostingapi.CapacityResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if payload.MaxClaws != 7 {
		t.Fatalf("maxClaws = %d, want 7", payload.MaxClaws)
	}
}
