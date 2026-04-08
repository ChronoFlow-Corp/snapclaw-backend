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

func TestNormalizeApproveChannelType(t *testing.T) {
	t.Parallel()

	got, err := NormalizeApproveChannelType("  TeLeGrAm ")
	if err != nil {
		t.Fatalf("NormalizeApproveChannelType() error = %v", err)
	}

	if got != "telegram" {
		t.Fatalf("NormalizeApproveChannelType() = %q, want %q", got, "telegram")
	}
}

func TestNormalizeApproveChannelTypeUnknown(t *testing.T) {
	t.Parallel()

	_, err := NormalizeApproveChannelType("unknown")
	if err == nil {
		t.Fatal("expected error for unknown channel type")
	}
}
