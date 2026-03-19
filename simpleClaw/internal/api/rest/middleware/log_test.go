package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggerSkipsSuccessfulMetricsRequest(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(prev)
	})

	h := Logger()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(buf.String(), "request completed") {
		t.Fatalf("unexpected request log for successful /metrics request: %s", buf.String())
	}
}

func TestLoggerLogsNonOKMetricsRequest(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(prev)
	})

	h := Logger()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := buf.String()
	if !strings.Contains(body, "request completed") {
		t.Fatalf("expected request log for non-OK /metrics request, got: %s", body)
	}
	if !strings.Contains(body, `"status":502`) {
		t.Fatalf("expected status in request log, got: %s", body)
	}
	if !strings.Contains(body, `"component":"http.server"`) {
		t.Fatalf("expected component in request log, got: %s", body)
	}
	if !strings.Contains(body, `"result":"error"`) {
		t.Fatalf("expected result in request log, got: %s", body)
	}
}
