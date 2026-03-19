package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HTTPClassifier func(method, path string) (action, flow string)
type RouteResolver func(r *http.Request) string

// HTTPMetrics stores Prometheus vectors for HTTP request monitoring.
type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inflight *prometheus.GaugeVec
}

// NewPrometheusRegistry returns a standalone registry with process and Go runtime metrics.
func NewPrometheusRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
	)
	return reg
}

// NewHTTPMetrics constructs HTTP metrics and registers them in the provided registry.
func NewHTTPMetrics(reg prometheus.Registerer) (*HTTPMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	m := &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "route", "status", "action", "flow", "component", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "action", "flow", "component", "result"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "http_inflight_requests",
			Help: "Current number of in-flight HTTP requests.",
		}, []string{"method", "action", "flow", "component"}),
	}

	if err := reg.Register(m.requests); err != nil {
		return nil, err
	}

	if err := reg.Register(m.duration); err != nil {
		return nil, err
	}

	if err := reg.Register(m.inflight); err != nil {
		return nil, err
	}

	return m, nil
}

// Middleware records request count, latency and in-flight gauge.
func (m *HTTPMetrics) Middleware(classifier HTTPClassifier, routeResolver RouteResolver) func(http.Handler) http.Handler {
	if classifier == nil {
		classifier = func(method, path string) (string, string) {
			return method, "http_request"
		}
	}

	if routeResolver == nil {
		routeResolver = func(r *http.Request) string {
			return r.URL.Path
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isMetricsPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			startedAt := time.Now()

			action, flow := classifier(r.Method, r.URL.Path)
			action = normalizeLabel(action, "unknown")
			flow = normalizeLabel(flow, "unknown")
			component := "http.server"

			inflight := m.inflight.WithLabelValues(r.Method, action, flow, component)
			inflight.Inc()
			defer inflight.Dec()

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			route := normalizeLabel(routeResolver(r), "unknown")
			status := strconv.Itoa(rec.status)
			result := resultFromHTTPStatus(rec.status)

			m.requests.WithLabelValues(r.Method, route, status, action, flow, component, result).Inc()
			m.duration.WithLabelValues(r.Method, route, action, flow, component, result).Observe(time.Since(startedAt).Seconds())
		})
	}
}

func resultFromHTTPStatus(status int) string {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return ResultSuccess
	case status == http.StatusNotFound:
		return ResultNotFound
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ResultDenied
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		return ResultValidationError
	default:
		return ResultError
	}
}

// Handler exposes metrics in Prometheus exposition format.
func Handler(gatherer prometheus.Gatherer) http.Handler {
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}

	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func normalizeLabel(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func isMetricsPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}

	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}

	return path == "/metrics"
}
