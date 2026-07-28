package observability

import (
	"context"
	"errors"
	"strings"
)

const (
	ResultSuccess         = "success"
	ResultError           = "error"
	ResultValidationError = "validation_error"
	ResultNotFound        = "not_found"
	ResultDenied          = "denied"
	ResultPartialSuccess  = "partial_success"
)

const (
	ErrorSourceHTTP     = "http"
	ErrorSourceService  = "service"
	ErrorSourceStorage  = "storage"
	ErrorSourceInfra    = "infra"
	ErrorSourceExternal = "external"
	ErrorSourceInternal = "internal"
)

const (
	ErrorKindUnexpected         = "unexpected"
	ErrorKindValidation         = "validation"
	ErrorKindTimeout            = "timeout"
	ErrorKindStorageUnavailable = "storage_unavailable"
	ErrorKindNotFound           = "not_found"
	ErrorKindDenied             = "denied"
)

type ctxComponentKey struct{}

type ErrorAttrs struct {
	Result string
	Kind   string
	Source string
}

type decoratedError struct {
	err   error
	attrs ErrorAttrs
}

func (e *decoratedError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}

	return e.err.Error()
}

func (e *decoratedError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.err
}

func (e *decoratedError) ObservabilityAttrs() ErrorAttrs {
	if e == nil {
		return ErrorAttrs{}
	}

	return e.attrs
}

type attrsProvider interface {
	ObservabilityAttrs() ErrorAttrs
}

func DecorateError(err error, attrs ErrorAttrs) error {
	if err == nil {
		return nil
	}

	return &decoratedError{
		err: err,
		attrs: ErrorAttrs{
			Result: NormalizeResult(attrs.Result),
			Kind:   NormalizeErrorKind(attrs.Kind),
			Source: NormalizeErrorSource(attrs.Source),
		},
	}
}

func ClassifyError(err error) ErrorAttrs {
	if err == nil {
		return ErrorAttrs{}
	}

	var provider attrsProvider
	if errors.As(err, &provider) {
		attrs := provider.ObservabilityAttrs()
		return ErrorAttrs{
			Result: NormalizeResult(attrs.Result),
			Kind:   NormalizeErrorKind(attrs.Kind),
			Source: NormalizeErrorSource(attrs.Source),
		}
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "not found"):
		return ErrorAttrs{
			Result: ResultNotFound,
			Kind:   ErrorKindNotFound,
			Source: ErrorSourceInternal,
		}
	case strings.Contains(message, "forbidden"), strings.Contains(message, "unauthorized"), strings.Contains(message, "denied"):
		return ErrorAttrs{
			Result: ResultDenied,
			Kind:   ErrorKindDenied,
			Source: ErrorSourceInternal,
		}
	case strings.Contains(message, "timeout"), strings.Contains(message, "deadline exceeded"):
		return ErrorAttrs{
			Result: ResultError,
			Kind:   ErrorKindTimeout,
			Source: ErrorSourceExternal,
		}
	case strings.Contains(message, "invalid"), strings.Contains(message, "required"), strings.Contains(message, "unsupported"):
		return ErrorAttrs{
			Result: ResultValidationError,
			Kind:   ErrorKindValidation,
			Source: ErrorSourceInternal,
		}
	case strings.Contains(message, "storage unavailable"):
		return ErrorAttrs{
			Result: ResultError,
			Kind:   ErrorKindStorageUnavailable,
			Source: ErrorSourceStorage,
		}
	default:
		return ErrorAttrs{
			Result: ResultError,
			Kind:   ErrorKindUnexpected,
			Source: ErrorSourceInternal,
		}
	}
}

func WithComponent(ctx context.Context, component string) context.Context {
	component = NormalizeComponent(component)
	if component == "unknown" {
		return ctx
	}

	return context.WithValue(ctx, ctxComponentKey{}, component)
}

func Component(ctx context.Context) string {
	v, _ := ctx.Value(ctxComponentKey{}).(string)
	return NormalizeComponent(v)
}

func NormalizeComponent(value string) string {
	return normalizeValue(value, "unknown")
}

func NormalizeFlow(value string) string {
	return normalizeValue(value, "unknown")
}

func NormalizeAction(value string) string {
	return normalizeValue(value, "unknown")
}

func NormalizeResult(value string) string {
	switch normalizeValue(value, ResultError) {
	case ResultSuccess, ResultError, ResultValidationError, ResultNotFound, ResultDenied, ResultPartialSuccess:
		return normalizeValue(value, ResultError)
	default:
		return ResultError
	}
}

func NormalizeErrorKind(value string) string {
	switch normalizeValue(value, ErrorKindUnexpected) {
	case ErrorKindUnexpected, ErrorKindValidation, ErrorKindTimeout, ErrorKindStorageUnavailable, ErrorKindNotFound, ErrorKindDenied:
		return normalizeValue(value, ErrorKindUnexpected)
	default:
		return ErrorKindUnexpected
	}
}

func NormalizeErrorSource(value string) string {
	switch normalizeValue(value, ErrorSourceInternal) {
	case ErrorSourceHTTP, ErrorSourceService, ErrorSourceStorage, ErrorSourceInfra, ErrorSourceExternal, ErrorSourceInternal:
		return normalizeValue(value, ErrorSourceInternal)
	default:
		return ErrorSourceInternal
	}
}

func normalizeValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}
