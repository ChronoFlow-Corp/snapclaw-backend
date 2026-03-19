package hostingapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	WriteError(rec, http.StatusBadRequest, ErrCodeInvalidCode, "invalid code")

	res := rec.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}

	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want %q", got, "application/json")
	}

	var body ErrorResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}

	if body.Code != ErrCodeInvalidCode {
		t.Fatalf("code = %q, want %q", body.Code, ErrCodeInvalidCode)
	}
	if body.Message != "invalid code" {
		t.Fatalf("message = %q, want %q", body.Message, "invalid code")
	}
}

func TestParseErrorResponse(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"code":"validation","message":"userId is required"}`)

	parsed, ok := ParseErrorResponse(raw)
	if !ok {
		t.Fatal("expected ParseErrorResponse to succeed")
	}

	if parsed.Code != "validation" {
		t.Fatalf("code = %q, want %q", parsed.Code, "validation")
	}
	if parsed.Message != "userId is required" {
		t.Fatalf("message = %q, want %q", parsed.Message, "userId is required")
	}
}
