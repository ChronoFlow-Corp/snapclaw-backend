package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type OperationObservation struct {
	Component   string
	Action      string
	Flow        string
	Result      string
	ErrorKind   string
	ErrorSource string
	Duration    time.Duration
}

type OperationMetrics struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewOperationMetrics(reg prometheus.Registerer, namespace string) (*OperationMetrics, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	namespace = normalizeMetricNamespace(namespace)

	m := &OperationMetrics{
		total: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "operation_total",
			Help:      "Total number of observed operations.",
		}, []string{"component", "action", "flow", "result", "error_kind", "error_source"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "operation_duration_seconds",
			Help:      "Operation duration in seconds.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
		}, []string{"component", "action", "flow", "result"}),
	}

	if err := reg.Register(m.total); err != nil {
		return nil, err
	}
	if err := reg.Register(m.duration); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *OperationMetrics) Observe(obs OperationObservation) {
	if m == nil {
		return
	}

	result := NormalizeResult(obs.Result)
	kind := ""
	source := ""
	if result == ResultError || result == ResultValidationError || result == ResultNotFound || result == ResultDenied {
		kind = NormalizeErrorKind(obs.ErrorKind)
		source = NormalizeErrorSource(obs.ErrorSource)
	}

	component := NormalizeComponent(obs.Component)
	action := NormalizeAction(obs.Action)
	flow := NormalizeFlow(obs.Flow)

	m.total.WithLabelValues(component, action, flow, result, kind, source).Inc()
	m.duration.WithLabelValues(component, action, flow, result).Observe(obs.Duration.Seconds())
}
