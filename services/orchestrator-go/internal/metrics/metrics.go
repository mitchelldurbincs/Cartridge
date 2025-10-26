package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collector exposes Prometheus metrics for orchestrator operations.
type Collector struct {
	heartbeatLatency    prometheus.Observer
	apiRequestDuration  *prometheus.HistogramVec
	runStateTransitions *prometheus.CounterVec
	healthEvents        *prometheus.CounterVec
	gatherer            prometheus.Gatherer
}

// NewCollector constructs the metrics collector, registering counters and histograms
// with the provided registerer. If reg is nil, the Prometheus default registerer is
// used. If gatherer is nil, the collector attempts to extract a gatherer from reg
// by type assertion; if that fails, it falls back to prometheus.DefaultGatherer.
// Passing an explicit gatherer is necessary when using wrapped registerers (e.g.,
// prometheus.WrapRegistererWith) that do not implement prometheus.Gatherer.
func NewCollector(reg prometheus.Registerer, gatherer prometheus.Gatherer) *Collector {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
		if g, ok := reg.(prometheus.Gatherer); ok {
			gatherer = g
		}
	}

	heartbeatLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "orchestrator_heartbeat_latency_seconds",
		Help:    "Time elapsed between consecutive heartbeats per run.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120},
	})

	apiRequestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "orchestrator_http_request_duration_seconds",
		Help:    "Latency for orchestrator HTTP API requests.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route", "status"})

	runStateTransitions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "orchestrator_run_state_transitions_total",
		Help: "Count of run state transitions processed by the orchestrator.",
	}, []string{"from", "to"})

	healthEvents := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "orchestrator_health_events_total",
		Help: "Count of health monitoring events emitted by the orchestrator.",
	}, []string{"type", "severity"})

	reg.MustRegister(heartbeatLatency, apiRequestDuration, runStateTransitions, healthEvents)

	return &Collector{
		heartbeatLatency:    heartbeatLatency,
		apiRequestDuration:  apiRequestDuration,
		runStateTransitions: runStateTransitions,
		healthEvents:        healthEvents,
		gatherer:            gatherer,
	}
}

// ObserveHeartbeatLatency records the time elapsed between heartbeats.
func (c *Collector) ObserveHeartbeatLatency(latency time.Duration) {
	if c == nil {
		return
	}
	c.heartbeatLatency.Observe(latency.Seconds())
}

// ObserveAPIRequest records request duration per method, route, and status code.
func (c *Collector) ObserveAPIRequest(method, route string, statusCode int, duration time.Duration) {
	if c == nil {
		return
	}
	c.apiRequestDuration.WithLabelValues(method, route, strconv.Itoa(statusCode)).Observe(duration.Seconds())
}

// RecordRunStateTransition increments the run state transition counter.
func (c *Collector) RecordRunStateTransition(fromState, toState string) {
	if c == nil {
		return
	}
	c.runStateTransitions.WithLabelValues(fromState, toState).Inc()
}

// RecordHealthEvent increments the health monitoring counter.
func (c *Collector) RecordHealthEvent(eventType, severity string) {
	if c == nil {
		return
	}
	c.healthEvents.WithLabelValues(eventType, severity).Inc()
}

// Handler exposes the registered metrics using promhttp.
func (c *Collector) Handler() http.Handler {
	gatherer := prometheus.DefaultGatherer
	if c != nil && c.gatherer != nil {
		gatherer = c.gatherer
	}
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}
