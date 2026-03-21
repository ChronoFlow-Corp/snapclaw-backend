package config

import "testing"

func TestMaxClawsResolve(t *testing.T) {
	t.Parallel()

	t.Run("manual returns configured value", func(t *testing.T) {
		t.Parallel()

		cl := new(MaxClaws)

		got, err := cl.Resolve(16*1024*1024*1024, 2*1024*1024*1024)
		if err != nil {
			t.Fatalf("resolve manual: %v", err)
		}

		if got != 5 {
			t.Fatalf("got %d, want 5", got)
		}
	})

	t.Run("auto uses memtotal minus reserve", func(t *testing.T) {
		t.Parallel()

		cl := new(MaxClaws)

		got, err := cl.Resolve(10*1024*1024*1024, 2*1024*1024*1024)
		if err != nil {
			t.Fatalf("resolve auto: %v", err)
		}

		if got != 10 {
			t.Fatalf("got %d, want 10", got)
		}
	})
}
