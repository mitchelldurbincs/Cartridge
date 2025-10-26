package observability

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics exposes Prometheus-compatible counters, histograms, and gauges.
type Metrics struct {
	publishTotal     *prometheus.CounterVec
	publishLatency   prometheus.Histogram
	streamSubscribers prometheus.Gauge
	gatherer         prometheus.Gatherer
}

// NewMetrics constructs a metrics collector instance, registering all metrics
// with the provided registerer. If reg is nil, the Prometheus default registerer
// is used, which includes standard Go process metrics (goroutines, memory, GC).
func NewMetrics() *Metrics {
	return NewMetricsWithRegisterer(nil)
}

// NewMetricsWithRegisterer allows explicit control over the registerer, useful for testing.
func NewMetricsWithRegisterer(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	gatherer := prometheus.DefaultGatherer
	if g, ok := reg.(prometheus.Gatherer); ok {
		gatherer = g
	}

	publishTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "weights_publish_total",
		Help: "Total number of publish attempts by outcome.",
	}, []string{"status"})

	publishLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "weights_publish_latency_seconds",
		Help: "Publish operation latency distribution.",
		// Buckets optimized for typical weight publishing times (10ms to 30s)
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	})

	streamSubscribers := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "weights_stream_subscribers",
		Help: "Current number of gRPC stream subscribers.",
	})

	reg.MustRegister(publishTotal, publishLatency, streamSubscribers)

	return &Metrics{
		publishTotal:      publishTotal,
		publishLatency:    publishLatency,
		streamSubscribers: streamSubscribers,
		gatherer:          gatherer,
	}
}

// PublishComplete records the outcome and duration of a publish attempt.
func (m *Metrics) PublishComplete(duration time.Duration, err error) {
	if m == nil {
		return
	}

	status := "success"
	if err != nil {
		status = "failure"
	}

	m.publishTotal.WithLabelValues(status).Inc()
	m.publishLatency.Observe(duration.Seconds())
}

// StreamSubscribed increments the active subscriber gauge.
func (m *Metrics) StreamSubscribed() {
	if m == nil {
		return
	}
	m.streamSubscribers.Inc()
}

// StreamCancelled decrements the active subscriber gauge.
func (m *Metrics) StreamCancelled() {
	if m == nil {
		return
	}
	m.streamSubscribers.Dec()
}

// Handler exposes the registered metrics using the official Prometheus HTTP handler.
// This includes histogram buckets, proper escaping, and standard Go process metrics
// (goroutines, memory, GC stats) automatically when using the default registerer.
func (m *Metrics) Handler() http.Handler {
	gatherer := prometheus.DefaultGatherer
	if m != nil && m.gatherer != nil {
		gatherer = m.gatherer
	}
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}
