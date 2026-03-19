package observability

import (
	"testing"
	"time"
)

func TestOperationMetricsObserveError(t *testing.T) {
	t.Parallel()

	reg := NewPrometheusRegistry()
	m, err := NewOperationMetrics(reg, "simpleclaw")
	if err != nil {
		t.Fatalf("NewOperationMetrics() error = %v", err)
	}

	m.Observe(OperationObservation{
		Component:   "service.claw",
		Action:      "claw.create",
		Flow:        "claw_lifecycle",
		Result:      "error",
		ErrorKind:   "storage_unavailable",
		ErrorSource: "storage",
		Duration:    150 * time.Millisecond,
	})

	scrapeBody := scrapeMetrics(t, Handler(reg))
	assertContains(t, scrapeBody, "operation_total")
	assertContains(t, scrapeBody, "operation_duration_seconds")
	assertContains(t, scrapeBody, `component="service.claw"`)
	assertContains(t, scrapeBody, `action="claw.create"`)
	assertContains(t, scrapeBody, `flow="claw_lifecycle"`)
	assertContains(t, scrapeBody, `result="error"`)
	assertContains(t, scrapeBody, `error_kind="storage_unavailable"`)
	assertContains(t, scrapeBody, `error_source="storage"`)
}
