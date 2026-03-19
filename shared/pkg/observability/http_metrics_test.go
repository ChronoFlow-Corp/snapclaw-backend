package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPMetricsMiddlewareRecordsRequest(t *testing.T) {
	reg := NewPrometheusRegistry()

	metrics, err := NewHTTPMetrics(reg)
	if err != nil {
		t.Fatalf("NewHTTPMetrics() error = %v", err)
	}

	classifier := func(method, path string) (string, string) {
		return "claw.start", "claw_lifecycle"
	}
	routeResolver := func(_ *http.Request) string {
		return "/api/claws/{id}/start"
	}

	h := metrics.Middleware(classifier, routeResolver)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/claws/123/start", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	scrapeBody := scrapeMetrics(t, Handler(reg))

	assertContains(t, scrapeBody, "http_requests_total")
	assertContains(t, scrapeBody, "http_request_duration_seconds")
	assertContains(t, scrapeBody, "http_inflight_requests")
	assertContains(t, scrapeBody, `action="claw.start"`)
	assertContains(t, scrapeBody, `flow="claw_lifecycle"`)
	assertContains(t, scrapeBody, `route="/api/claws/{id}/start"`)
	assertContains(t, scrapeBody, `status="204"`)
}

func scrapeMetrics(t *testing.T, handler http.Handler) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("failed to read metrics body: %v", err)
	}

	return string(body)
}

func assertContains(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Fatalf("metrics body does not contain %q\nbody:\n%s", needle, body)
	}
}
