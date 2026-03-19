package service

import "testing"

func TestGogWatchPortFromGateway(t *testing.T) {
	t.Run("returns gateway plus one", func(t *testing.T) {
		got, err := GogWatchPortFromGateway("12000")
		if err != nil {
			t.Fatalf("GogWatchPortFromGateway() error = %v", err)
		}

		if got != "12001" {
			t.Fatalf("unexpected watch port: %q", got)
		}
	})

	t.Run("fails for invalid gateway port", func(t *testing.T) {
		if _, err := GogWatchPortFromGateway("abc"); err == nil {
			t.Fatalf("expected error for non-numeric gateway port")
		}
	})

	t.Run("fails when watch port overflows", func(t *testing.T) {
		if _, err := GogWatchPortFromGateway("65535"); err == nil {
			t.Fatalf("expected error for out-of-range watch port")
		}
	})
}
