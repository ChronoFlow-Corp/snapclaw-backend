package observability

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PubSubFanoutMetrics captures pubsub fanout quality and latency.
type PubSubFanoutMetrics struct {
	total          *prometheus.CounterVec
	duration       *prometheus.HistogramVec
	targets        *prometheus.HistogramVec
	successTargets *prometheus.HistogramVec
}

// NewPubSubFanoutMetrics registers pubsub fanout metrics in provided registry.
func NewPubSubFanoutMetrics(reg prometheus.Registerer, namespace string) (*PubSubFanoutMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	namespace = normalizeMetricNamespace(namespace)

	m := &PubSubFanoutMetrics{
		total: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "pubsub_fanout_total",
			Help:      "Total number of pubsub fanout operations.",
		}, []string{"flow", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "pubsub_fanout_duration_seconds",
			Help:      "Duration of pubsub fanout operations in seconds.",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
		}, []string{"flow", "result"}),
		targets: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "pubsub_fanout_targets",
			Help:      "Number of downstream targets per pubsub fanout operation.",
			Buckets:   []float64{0, 1, 2, 4, 8, 16, 32, 64, 128},
		}, []string{"flow"}),
		successTargets: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "pubsub_fanout_success_targets",
			Help:      "Number of successful downstream forwards per pubsub fanout operation.",
			Buckets:   []float64{0, 1, 2, 4, 8, 16, 32, 64, 128},
		}, []string{"flow"}),
	}

	if err := reg.Register(m.total); err != nil {
		return nil, err
	}
	if err := reg.Register(m.duration); err != nil {
		return nil, err
	}
	if err := reg.Register(m.targets); err != nil {
		return nil, err
	}
	if err := reg.Register(m.successTargets); err != nil {
		return nil, err
	}

	return m, nil
}

// Observe records one pubsub fanout operation.
func (m *PubSubFanoutMetrics) Observe(flow string, targets int, successTargets int, err error, duration time.Duration) {
	if m == nil {
		return
	}

	flow = normalizeLabel(flow, "unknown")
	result := "success"
	if err != nil {
		result = "error"
	}

	m.total.WithLabelValues(flow, result).Inc()
	m.duration.WithLabelValues(flow, result).Observe(duration.Seconds())
	m.targets.WithLabelValues(flow).Observe(float64(targets))
	m.successTargets.WithLabelValues(flow).Observe(float64(successTargets))
}

func normalizeMetricNamespace(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return "snapclaw"
	}

	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_", " ", "_")
	return replacer.Replace(namespace)
}
