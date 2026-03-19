package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type ctxActionKey struct{}
type ctxFlowKey struct{}

// TracingConfig configures OpenTelemetry tracing bootstrap.
type TracingConfig struct {
	ServiceName string
	Environment string

	Enabled     bool
	Endpoint    string
	Insecure    bool
	SampleRatio float64
}

// WithAction sets action name in context for structured logging/metrics.
func WithAction(ctx context.Context, action string) context.Context {
	action = strings.TrimSpace(action)
	if action == "" {
		return ctx
	}

	return context.WithValue(ctx, ctxActionKey{}, action)
}

// Action returns action name from context.
func Action(ctx context.Context) string {
	v, _ := ctx.Value(ctxActionKey{}).(string)
	return strings.TrimSpace(v)
}

// WithFlow sets flow name in context for structured logging/metrics.
func WithFlow(ctx context.Context, flow string) context.Context {
	flow = strings.TrimSpace(flow)
	if flow == "" {
		return ctx
	}

	return context.WithValue(ctx, ctxFlowKey{}, flow)
}

// Flow returns flow name from context.
func Flow(ctx context.Context) string {
	v, _ := ctx.Value(ctxFlowKey{}).(string)
	return strings.TrimSpace(v)
}

// WithActionFlow sets both action and flow.
func WithActionFlow(ctx context.Context, action, flow string) context.Context {
	ctx = WithAction(ctx, action)
	ctx = WithFlow(ctx, flow)
	return ctx
}

// EnrichLogger injects action/flow/trace ids from context into logger.
func EnrichLogger(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}

	attrs := make([]any, 0, 8)

	if action := Action(ctx); action != "" {
		attrs = append(attrs, "action", action)
	}

	if flow := Flow(ctx); flow != "" {
		attrs = append(attrs, "flow", flow)
	}

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		attrs = append(attrs, "trace_id", spanCtx.TraceID().String())
		attrs = append(attrs, "span_id", spanCtx.SpanID().String())
	}

	if len(attrs) == 0 {
		return logger
	}

	return logger.With(attrs...)
}

// StartFlow marks flow start in logs and returns completion callback.
func StartFlow(
	ctx context.Context,
	logger *slog.Logger,
	flow string,
	action string,
) (context.Context, *slog.Logger, func(err error)) {
	ctx = WithActionFlow(ctx, action, flow)
	logger = EnrichLogger(ctx, logger)

	startedAt := time.Now()
	logger.Info("flow_started")

	var done int32
	finish := func(err error) {
		if !atomic.CompareAndSwapInt32(&done, 0, 1) {
			return
		}

		result := "success"
		attrs := []any{
			"result", result,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		}

		if err != nil {
			result = "error"
			attrs[1] = result
			attrs = append(attrs, "error_kind", errorKind(err))
		}

		logger.Info("flow_finished", attrs...)
	}

	return ctx, logger, finish
}

func errorKind(err error) string {
	if err == nil {
		return ""
	}

	var target interface{ Kind() string }
	if errors.As(err, &target) {
		return target.Kind()
	}

	return reflect.TypeOf(err).String()
}

// SetupTracing configures global OTel tracer provider and returns shutdown callback.
func SetupTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	serviceName := strings.TrimSpace(cfg.ServiceName)
	if serviceName == "" {
		return nil, fmt.Errorf("service name is required")
	}

	attrs := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
	}
	if env := strings.TrimSpace(cfg.Environment); env != "" {
		attrs = append(attrs, attribute.String("deployment.environment", env))
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(normalizeSampleRatio(cfg.SampleRatio)))

	if !cfg.Enabled {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.NeverSample()),
			sdktrace.WithResource(res),
		)

		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))

		return tp.Shutdown, nil
	}

	exporter, err := newOTLPTraceExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exporter),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// NewHTTPClient returns tracing-aware HTTP client.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

// WrapHTTPHandler wraps incoming HTTP server handler for OTel tracing.
func WrapHTTPHandler(handler http.Handler, operation string) http.Handler {
	return otelhttp.NewHandler(handler, operation)
}

func newOTLPTraceExporter(ctx context.Context, cfg TracingConfig) (sdktrace.SpanExporter, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required when tracing is enabled")
	}

	opts := []otlptracehttp.Option{}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		opts = append(opts, otlptracehttp.WithEndpointURL(endpoint))
	} else {
		opts = append(opts, otlptracehttp.WithEndpoint(endpoint))
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
	}

	return otlptracehttp.New(ctx, opts...)
}

func normalizeSampleRatio(v float64) float64 {
	switch {
	case v <= 0:
		return 1.0
	case v > 1:
		return 1.0
	default:
		return v
	}
}
