package config

import "testing"

func TestMaxClawsUnmarshalText(t *testing.T) {
	t.Parallel()

	t.Run("auto", func(t *testing.T) {
		t.Parallel()

		var got MaxClaws
		if err := got.UnmarshalText([]byte("auto")); err != nil {
			t.Fatalf("unmarshal auto: %v", err)
		}

		if !got.Auto {
			t.Fatal("expected auto mode")
		}
		if got.Value != 0 {
			t.Fatalf("value = %d, want 0", got.Value)
		}
	})

	t.Run("manual", func(t *testing.T) {
		t.Parallel()

		var got MaxClaws
		if err := got.UnmarshalText([]byte("7")); err != nil {
			t.Fatalf("unmarshal manual: %v", err)
		}

		if got.Auto {
			t.Fatal("expected manual mode")
		}
		if got.Value != 7 {
			t.Fatalf("value = %d, want 7", got.Value)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()

		var got MaxClaws
		if err := got.UnmarshalText([]byte("0")); err == nil {
			t.Fatal("expected validation error")
		}
	})
}

func TestMaxClawsResolve(t *testing.T) {
	t.Parallel()

	t.Run("manual returns configured value", func(t *testing.T) {
		t.Parallel()

		got, err := MaxClaws{Value: 5}.Resolve(16*1024*1024*1024, 2*1024*1024*1024)
		if err != nil {
			t.Fatalf("resolve manual: %v", err)
		}
		if got != 5 {
			t.Fatalf("got %d, want 5", got)
		}
	})

	t.Run("auto uses memtotal minus reserve", func(t *testing.T) {
		t.Parallel()

		got, err := MaxClaws{Auto: true}.Resolve(10*1024*1024*1024, 2*1024*1024*1024)
		if err != nil {
			t.Fatalf("resolve auto: %v", err)
		}
		if got != 10 {
			t.Fatalf("got %d, want 10", got)
		}
	})
}
