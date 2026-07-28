package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeGogImportPayload(t *testing.T) {
	t.Run("accepts direct gog payload", func(t *testing.T) {
		raw := []byte(`{
			"email":"user@example.com",
			"client":"default",
			"refresh_token":"rt-1",
			"topic":"projects/demo/topics/gog-gmail-watch",
			"labels":["INBOX","IMPORTANT"]
		}`)

		out, err := normalizeGogImportPayload(raw)
		if err != nil {
			t.Fatalf("normalizeGogImportPayload() error = %v", err)
		}

		var payload gogImportPayload
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}

		if payload.Email != "user@example.com" {
			t.Fatalf("unexpected email: %q", payload.Email)
		}

		if payload.Client != "default" {
			t.Fatalf("unexpected client: %q", payload.Client)
		}

		if payload.RefreshToken != "rt-1" {
			t.Fatalf("unexpected refresh token: %q", payload.RefreshToken)
		}

		if payload.Topic != "projects/demo/topics/gog-gmail-watch" {
			t.Fatalf("unexpected topic: %q", payload.Topic)
		}

		if len(payload.Labels) != 2 {
			t.Fatalf("unexpected labels count: %d", len(payload.Labels))
		}
	})

	t.Run("maps legacy simpleclaw payload", func(t *testing.T) {
		raw := []byte(`{
			"email":"legacy@example.com",
			"client":"",
			"topic":"projects/demo/topics/legacy",
			"labels":["INBOX"],
			"token":{"refresh_token":"legacy-rt"}
		}`)

		out, err := normalizeGogImportPayload(raw)
		if err != nil {
			t.Fatalf("normalizeGogImportPayload() error = %v", err)
		}

		var payload gogImportPayload
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("unmarshal output: %v", err)
		}

		if payload.Email != "legacy@example.com" {
			t.Fatalf("unexpected email: %q", payload.Email)
		}

		if payload.Client != "default" {
			t.Fatalf("unexpected client defaulting: %q", payload.Client)
		}

		if payload.RefreshToken != "legacy-rt" {
			t.Fatalf("unexpected refresh token: %q", payload.RefreshToken)
		}

		if payload.Topic != "projects/demo/topics/legacy" {
			t.Fatalf("unexpected topic: %q", payload.Topic)
		}
	})

	t.Run("fails on missing refresh token", func(t *testing.T) {
		raw := []byte(`{"email":"user@example.com","token":{"refresh_token":""}}`)

		if _, err := normalizeGogImportPayload(raw); err == nil {
			t.Fatalf("expected error for missing refresh token")
		}
	})

	t.Run("fails on missing email", func(t *testing.T) {
		raw := []byte(`{"refresh_token":"rt-1","topic":"projects/demo/topics/gog-gmail-watch"}`)

		if _, err := normalizeGogImportPayload(raw); err == nil {
			t.Fatalf("expected error for missing email")
		}
	})

	t.Run("fails on missing topic", func(t *testing.T) {
		raw := []byte(`{"email":"user@example.com","refresh_token":"rt-1"}`)

		if _, err := normalizeGogImportPayload(raw); err == nil {
			t.Fatalf("expected error for missing topic")
		}
	})
}
