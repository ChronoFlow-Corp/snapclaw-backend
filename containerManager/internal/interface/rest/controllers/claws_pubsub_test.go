package controllers

import (
	"testing"
	"time"
)

func TestPubSubMessageKey(t *testing.T) {
	t.Run("uses messageId when present", func(t *testing.T) {
		body := []byte(`{"message":{"messageId":"abc-123"}}`)

		got := pubSubMessageKey(body)
		if got != "msg:abc-123" {
			t.Fatalf("unexpected key: %q", got)
		}
	})

	t.Run("falls back to hash when id missing", func(t *testing.T) {
		body := []byte(`{"message":{"data":"xyz"}}`)

		got := pubSubMessageKey(body)
		if got == "" || got == "msg:" {
			t.Fatalf("unexpected empty key: %q", got)
		}

		if len(got) < len("sha256:") || got[:7] != "sha256:" {
			t.Fatalf("expected sha256 fallback, got: %q", got)
		}
	})
}

func TestPubSubDeduper(t *testing.T) {
	d := newPubSubDeduper(time.Minute)

	if ok := d.Add("key-1"); !ok {
		t.Fatalf("expected first add to pass")
	}

	if ok := d.Add("key-1"); ok {
		t.Fatalf("expected duplicate key to be rejected")
	}

	if ok := d.Add("key-2"); !ok {
		t.Fatalf("expected second unique key to pass")
	}
}
