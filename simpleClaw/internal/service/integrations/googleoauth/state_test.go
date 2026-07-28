package googleoauth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestStateCodecRoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	codec := NewStateCodec([]byte("state-secret"), 10*time.Minute, func() time.Time { return now })

	raw, err := codec.Sign(StatePayload{
		UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Capabilities: []string{"gmail", "google_calendar"},
		ReturnTo:     "/onboard/google",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	got, err := codec.Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got.UserID != uuid.MustParse("11111111-1111-1111-1111-111111111111") {
		t.Fatalf("Parse().UserID = %s", got.UserID)
	}

	if got.ReturnTo != "/onboard/google" {
		t.Fatalf("Parse().ReturnTo = %q", got.ReturnTo)
	}

	if len(got.Capabilities) != 2 || got.Capabilities[0] != "gmail" || got.Capabilities[1] != "google_calendar" {
		t.Fatalf("Parse().Capabilities = %#v", got.Capabilities)
	}

	if !got.IssuedAt.Equal(now) {
		t.Fatalf("Parse().IssuedAt = %v, want %v", got.IssuedAt, now)
	}

	if !got.ExpiresAt.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("Parse().ExpiresAt = %v, want %v", got.ExpiresAt, now.Add(10*time.Minute))
	}
}

func TestStateCodecRejectsTampering(t *testing.T) {
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	codec := NewStateCodec([]byte("state-secret"), 10*time.Minute, func() time.Time { return now })

	raw, err := codec.Sign(StatePayload{
		UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Capabilities: []string{"gmail"},
		ReturnTo:     "/onboard",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	raw = raw[:len(raw)-1] + "A"
	if _, err := codec.Parse(raw); err == nil {
		t.Fatal("Parse() error = nil, want tamper rejection")
	}
}

func TestStateCodecRejectsExpiredState(t *testing.T) {
	issuedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	current := issuedAt
	codec := NewStateCodec([]byte("state-secret"), time.Minute, func() time.Time { return current })

	raw, err := codec.Sign(StatePayload{
		UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Capabilities: []string{"gmail"},
		ReturnTo:     "/onboard",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	current = issuedAt.Add(2 * time.Minute)
	if _, err := codec.Parse(raw); err == nil {
		t.Fatal("Parse() error = nil, want expiry rejection")
	}
}

func TestStateCodecRejectsUnsafeReturnTo(t *testing.T) {
	codec := NewStateCodec([]byte("state-secret"), 10*time.Minute, time.Now)

	_, err := codec.Sign(StatePayload{
		UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Capabilities: []string{"gmail"},
		ReturnTo:     "https://evil.example/onboard",
	})
	if err == nil {
		t.Fatal("Sign() error = nil, want unsafe return_to rejection")
	}
}
