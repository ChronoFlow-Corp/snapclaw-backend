package observability

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPubSubFanoutMetricsObserve(t *testing.T) {
	reg := NewPrometheusRegistry()
	m, err := NewPubSubFanoutMetrics(reg, "simpleclaw")
	if err != nil {
		t.Fatalf("NewPubSubFanoutMetrics() error = %v", err)
	}

	m.Observe("gmail_pubsub_fanout", 5, 3, nil, 150*time.Millisecond)
	m.Observe("gmail_pubsub_fanout", 4, 0, errors.New("all failed"), 210*time.Millisecond)

	body := scrapeMetricsBody(t, Handler(reg))

	assertContainsMetric(t, body, "simpleclaw_pubsub_fanout_total")
	assertContainsMetric(t, body, `flow="gmail_pubsub_fanout"`)
	assertContainsMetric(t, body, `result="success"`)
	assertContainsMetric(t, body, `result="error"`)
	assertContainsMetric(t, body, "simpleclaw_pubsub_fanout_duration_seconds")
	assertContainsMetric(t, body, "simpleclaw_pubsub_fanout_targets")
	assertContainsMetric(t, body, "simpleclaw_pubsub_fanout_success_targets")
}

func scrapeMetricsBody(t *testing.T, h http.Handler) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	data, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	return string(data)
}

func assertContainsMetric(t *testing.T, body, sub string) {
	t.Helper()
	if !strings.Contains(body, sub) {
		t.Fatalf("metrics body does not contain %q\nbody:\n%s", sub, body)
	}
}
