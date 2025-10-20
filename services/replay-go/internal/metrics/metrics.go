package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collector wraps the Prometheus collectors exposed by the replay service.
type Collector struct {
	storeRequests       *prometheus.CounterVec
	storeDuration       *prometheus.HistogramVec
	storeTransitions    *prometheus.CounterVec
	sampleRequests      *prometheus.CounterVec
	sampleDuration      *prometheus.HistogramVec
	sampleTransitions   *prometheus.CounterVec
	priorityUpdates     *prometheus.CounterVec
	priorityTransitions prometheus.Counter
	clearRequests       *prometheus.CounterVec
	clearTransitions    *prometheus.CounterVec
	gatherer            prometheus.Gatherer
}

// NewCollector constructs the replay metrics collector, registering the
// counters and histograms with the supplied registerer. If reg is nil, the
// Prometheus default registerer is used.
func NewCollector(reg prometheus.Registerer) *Collector {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	gatherer := prometheus.DefaultGatherer
	if g, ok := reg.(prometheus.Gatherer); ok {
		gatherer = g
	}

	storeRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_store_requests_total",
		Help: "Count of StoreTransition and StoreBatch RPC invocations.",
	}, []string{"method", "result"})

	storeDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "replay_store_duration_seconds",
		Help:    "Latency for storing transitions individually or in batches.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})

	storeTransitions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_store_transitions_total",
		Help: "Total transitions persisted via store RPCs, partitioned by method.",
	}, []string{"method"})

	sampleRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_sample_requests_total",
		Help: "Count of Sample RPC invocations segmented by prioritized flag and result.",
	}, []string{"prioritized", "result"})

	sampleDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "replay_sample_duration_seconds",
		Help:    "Latency for sampling transitions from replay.",
		Buckets: prometheus.DefBuckets,
	}, []string{"prioritized"})

	sampleTransitions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_sample_transitions_total",
		Help: "Total transitions returned to callers from Sample RPCs.",
	}, []string{"prioritized"})

	priorityUpdates := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_priority_updates_total",
		Help: "Count of UpdatePriorities RPC invocations segmented by result.",
	}, []string{"result"})

	priorityTransitions := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "replay_priority_transitions_total",
		Help: "Number of transitions whose priorities were updated successfully.",
	})

	clearRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_clear_requests_total",
		Help: "Count of Clear RPC invocations segmented by result.",
	}, []string{"result"})

	clearTransitions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_clear_transitions_total",
		Help: "Number of transitions removed via Clear RPCs, segmented by result.",
	}, []string{"result"})

	reg.MustRegister(
		storeRequests,
		storeDuration,
		storeTransitions,
		sampleRequests,
		sampleDuration,
		sampleTransitions,
		priorityUpdates,
		priorityTransitions,
		clearRequests,
		clearTransitions,
	)

	return &Collector{
		storeRequests:       storeRequests,
		storeDuration:       storeDuration,
		storeTransitions:    storeTransitions,
		sampleRequests:      sampleRequests,
		sampleDuration:      sampleDuration,
		sampleTransitions:   sampleTransitions,
		priorityUpdates:     priorityUpdates,
		priorityTransitions: priorityTransitions,
		clearRequests:       clearRequests,
		clearTransitions:    clearTransitions,
		gatherer:            gatherer,
	}
}

// RecordStore captures the result, latency, and transition count for store RPCs.
func (c *Collector) RecordStore(method, result string, duration time.Duration, transitions int) {
	if c == nil {
		return
	}
	c.storeRequests.WithLabelValues(method, result).Inc()
	c.storeDuration.WithLabelValues(method).Observe(duration.Seconds())
	c.storeTransitions.WithLabelValues(method).Add(float64(transitions))
}

// RecordSample captures the result, latency, and transition count for sampling.
func (c *Collector) RecordSample(prioritized bool, result string, duration time.Duration, transitions int) {
	if c == nil {
		return
	}
	label := boolLabel(prioritized)
	c.sampleRequests.WithLabelValues(label, result).Inc()
	c.sampleDuration.WithLabelValues(label).Observe(duration.Seconds())
	c.sampleTransitions.WithLabelValues(label).Add(float64(transitions))
}

// RecordPriorityUpdate records the outcome of an UpdatePriorities call.
func (c *Collector) RecordPriorityUpdate(result string, updated int) {
	if c == nil {
		return
	}
	c.priorityUpdates.WithLabelValues(result).Inc()
	if result == "success" && updated > 0 {
		c.priorityTransitions.Add(float64(updated))
	}
}

// RecordClear records the outcome of a Clear call and how many transitions were removed.
func (c *Collector) RecordClear(result string, cleared uint64) {
	if c == nil {
		return
	}
	c.clearRequests.WithLabelValues(result).Inc()
	c.clearTransitions.WithLabelValues(result).Add(float64(cleared))
}

// Handler exposes the registered metrics using promhttp.
func (c *Collector) Handler() http.Handler {
	gatherer := prometheus.DefaultGatherer
	if c != nil && c.gatherer != nil {
		gatherer = c.gatherer
	}
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}

func boolLabel(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
