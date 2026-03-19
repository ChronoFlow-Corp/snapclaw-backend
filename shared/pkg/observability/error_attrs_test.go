package observability

import (
	"errors"
	"fmt"
	"testing"
)

func TestNormalizeResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "success", input: "success", want: ResultSuccess},
		{name: "validation error", input: " validation_error ", want: ResultValidationError},
		{name: "empty falls back", input: "", want: ResultError},
		{name: "unknown falls back", input: "boom", want: ResultError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NormalizeResult(tt.input); got != tt.want {
				t.Fatalf("NormalizeResult(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyErrorWrappedValidation(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("service.Claw.Create: %w", errors.New("invalid data"))

	attrs := ClassifyError(DecorateError(err, ErrorAttrs{
		Result: ResultValidationError,
		Kind:   ErrorKindValidation,
		Source: ErrorSourceService,
	}))

	if attrs.Result != ResultValidationError {
		t.Fatalf("result = %q, want %q", attrs.Result, ResultValidationError)
	}
	if attrs.Kind != ErrorKindValidation {
		t.Fatalf("kind = %q, want %q", attrs.Kind, ErrorKindValidation)
	}
	if attrs.Source != ErrorSourceService {
		t.Fatalf("source = %q, want %q", attrs.Source, ErrorSourceService)
	}
}

func TestClassifyErrorWrappedStorageUnavailable(t *testing.T) {
	t.Parallel()

	err := DecorateError(errors.New("storage unavailable"), ErrorAttrs{
		Result: ResultError,
		Kind:   ErrorKindStorageUnavailable,
		Source: ErrorSourceStorage,
	})

	attrs := ClassifyError(fmt.Errorf("service.Claw.Create: %w", err))

	if attrs.Kind != ErrorKindStorageUnavailable {
		t.Fatalf("kind = %q, want %q", attrs.Kind, ErrorKindStorageUnavailable)
	}
	if attrs.Source != ErrorSourceStorage {
		t.Fatalf("source = %q, want %q", attrs.Source, ErrorSourceStorage)
	}
}

func TestClassifyErrorUnknownFallback(t *testing.T) {
	t.Parallel()

	attrs := ClassifyError(errors.New("boom"))

	if attrs.Result != ResultError {
		t.Fatalf("result = %q, want %q", attrs.Result, ResultError)
	}
	if attrs.Kind != ErrorKindUnexpected {
		t.Fatalf("kind = %q, want %q", attrs.Kind, ErrorKindUnexpected)
	}
	if attrs.Source != ErrorSourceInternal {
		t.Fatalf("source = %q, want %q", attrs.Source, ErrorSourceInternal)
	}
}
