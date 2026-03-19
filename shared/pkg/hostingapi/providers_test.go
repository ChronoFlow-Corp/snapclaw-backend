package hostingapi

import "testing"

func TestNormalizeProvider(t *testing.T) {
	t.Parallel()

	got, err := NormalizeProvider("  GmAiL ")
	if err != nil {
		t.Fatalf("NormalizeProvider() error = %v", err)
	}

	if got != ProviderGmail {
		t.Fatalf("NormalizeProvider() = %q, want %q", got, ProviderGmail)
	}
}

func TestNormalizeProviderUnknown(t *testing.T) {
	t.Parallel()

	_, err := NormalizeProvider("unknown")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
