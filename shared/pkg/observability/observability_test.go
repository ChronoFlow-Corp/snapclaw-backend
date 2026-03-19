package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestActionFlowContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithAction(ctx, "claw.start")
	ctx = WithFlow(ctx, "claw_lifecycle")

	if got := Action(ctx); got != "claw.start" {
		t.Fatalf("Action() = %q, want %q", got, "claw.start")
	}

	if got := Flow(ctx); got != "claw_lifecycle" {
		t.Fatalf("Flow() = %q, want %q", got, "claw_lifecycle")
	}
}

func TestWithActionFlow(t *testing.T) {
	ctx := WithActionFlow(context.Background(), "pubsub.forward", "gmail_pubsub_fanout")

	if got := Action(ctx); got != "pubsub.forward" {
		t.Fatalf("Action() = %q, want %q", got, "pubsub.forward")
	}

	if got := Flow(ctx); got != "gmail_pubsub_fanout" {
		t.Fatalf("Flow() = %q, want %q", got, "gmail_pubsub_fanout")
	}
}

func TestEnrichLoggerAddsActionFlowAndTraceIDs(t *testing.T) {
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{9, 8, 7, 6, 5, 4, 3, 2},
		TraceFlags: trace.FlagsSampled,
	})

	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)
	ctx = WithActionFlow(ctx, "claw.create", "claw_lifecycle")

	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))

	EnrichLogger(ctx, base).Info("hello")

	entry := parseSingleJSONLine(t, buf.String())

	if got := entry["action"]; got != "claw.create" {
		t.Fatalf("action = %#v, want %q", got, "claw.create")
	}

	if got := entry["flow"]; got != "claw_lifecycle" {
		t.Fatalf("flow = %#v, want %q", got, "claw_lifecycle")
	}

	if got := entry["trace_id"]; got == "" {
		t.Fatalf("trace_id is empty")
	}

	if got := entry["span_id"]; got == "" {
		t.Fatalf("span_id is empty")
	}
}

func TestStartFlowLifecycleLogs(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))

	ctx, logger, finish := StartFlow(context.Background(), base, "gmail_pubsub_fanout", "pubsub.forward")
	logger.Info("forwarding started")
	finish(nil)

	lines := splitJSONLines(buf.String())
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 log lines, got %d", len(lines))
	}

	start := parseJSONLine(t, lines[0])
	done := parseJSONLine(t, lines[len(lines)-1])

	if got := start["msg"]; got != "flow_started" {
		t.Fatalf("start msg = %#v, want %q", got, "flow_started")
	}

	if got := start["flow"]; got != "gmail_pubsub_fanout" {
		t.Fatalf("start flow = %#v", got)
	}

	if got := start["action"]; got != "pubsub.forward" {
		t.Fatalf("start action = %#v", got)
	}

	if got := done["msg"]; got != "flow_finished" {
		t.Fatalf("finish msg = %#v, want %q", got, "flow_finished")
	}

	if got := done["result"]; got != "success" {
		t.Fatalf("result = %#v, want %q", got, "success")
	}

	if got := done["duration_ms"]; got == nil {
		t.Fatalf("duration_ms is missing")
	}

	if got := Action(ctx); got != "pubsub.forward" {
		t.Fatalf("context action = %q", got)
	}

	if got := Flow(ctx); got != "gmail_pubsub_fanout" {
		t.Fatalf("context flow = %q", got)
	}
}

func TestStartFlowLifecycleError(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))

	_, _, finish := StartFlow(context.Background(), base, "claw_lifecycle", "claw.start")
	finish(errors.New("timeout"))

	lines := splitJSONLines(buf.String())
	done := parseJSONLine(t, lines[len(lines)-1])

	if got := done["result"]; got != "error" {
		t.Fatalf("result = %#v, want %q", got, "error")
	}

	if got := done["error_kind"]; got != "*errors.errorString" {
		t.Fatalf("error_kind = %#v, want %q", got, "*errors.errorString")
	}
}

func parseSingleJSONLine(t *testing.T, s string) map[string]any {
	t.Helper()
	lines := splitJSONLines(s)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	return parseJSONLine(t, lines[0])
}

func splitJSONLines(s string) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func parseJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()

	var out map[string]any
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("failed to unmarshal %q: %v", line, err)
	}

	return out
}
