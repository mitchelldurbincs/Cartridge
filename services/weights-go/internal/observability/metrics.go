package observability

import (
	"fmt"
	"math"
	"net/http"
	"sync/atomic"
	"time"
)

// Metrics exposes Prometheus-compatible counters and gauges.
type Metrics struct {
	publishSuccess      atomic.Uint64
	publishFailure      atomic.Uint64
	publishLatencySum   atomic.Uint64 // stores math.Float64bits(sum)
	publishLatencyCount atomic.Uint64
	streamSubscribers   atomic.Int64
}

// NewMetrics constructs a metrics collector instance.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// PublishComplete records the outcome and duration of a publish attempt.
func (m *Metrics) PublishComplete(duration time.Duration, err error) {
	if err != nil {
		m.publishFailure.Add(1)
	} else {
		m.publishSuccess.Add(1)
	}
	m.addLatency(duration.Seconds())
}

func (m *Metrics) addLatency(value float64) {
	for {
		currentBits := m.publishLatencySum.Load()
		current := math.Float64frombits(currentBits)
		next := math.Float64bits(current + value)
		if m.publishLatencySum.CompareAndSwap(currentBits, next) {
			break
		}
	}
	m.publishLatencyCount.Add(1)
}

// StreamSubscribed increments the active subscriber gauge.
func (m *Metrics) StreamSubscribed() {
	m.streamSubscribers.Add(1)
}

// StreamCancelled decrements the active subscriber gauge without allowing negatives.
func (m *Metrics) StreamCancelled() {
	for {
		current := m.streamSubscribers.Load()
		if current <= 0 {
			return
		}
		if m.streamSubscribers.CompareAndSwap(current, current-1) {
			return
		}
	}
}

// Handler exposes metrics in Prometheus text exposition format.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		success := m.publishSuccess.Load()
		failure := m.publishFailure.Load()
		sum := math.Float64frombits(m.publishLatencySum.Load())
		count := m.publishLatencyCount.Load()
		subscribers := m.streamSubscribers.Load()

		fmt.Fprintf(w, "# HELP weights_publish_total Total number of publish attempts by outcome.\n")
		fmt.Fprintf(w, "# TYPE weights_publish_total counter\n")
		fmt.Fprintf(w, "weights_publish_total{status=\"success\"} %d\n", success)
		fmt.Fprintf(w, "weights_publish_total{status=\"failure\"} %d\n", failure)

		fmt.Fprintf(w, "# HELP weights_publish_latency_seconds Publish latency histogram buckets (sum/count only).\n")
		fmt.Fprintf(w, "# TYPE weights_publish_latency_seconds histogram\n")
		fmt.Fprintf(w, "weights_publish_latency_seconds_sum %f\n", sum)
		fmt.Fprintf(w, "weights_publish_latency_seconds_count %d\n", count)

		fmt.Fprintf(w, "# HELP weights_stream_subscribers Current number of gRPC stream subscribers.\n")
		fmt.Fprintf(w, "# TYPE weights_stream_subscribers gauge\n")
		fmt.Fprintf(w, "weights_stream_subscribers %d\n", subscribers)
	})
}

// Snapshot returns the current counter values. Intended for testing.
type MetricsSnapshot struct {
	PublishSuccess      uint64
	PublishFailure      uint64
	PublishLatencySum   float64
	PublishLatencyCount uint64
	StreamSubscribers   int64
}

// Snapshot extracts metric values without mutating state.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		PublishSuccess:      m.publishSuccess.Load(),
		PublishFailure:      m.publishFailure.Load(),
		PublishLatencySum:   math.Float64frombits(m.publishLatencySum.Load()),
		PublishLatencyCount: m.publishLatencyCount.Load(),
		StreamSubscribers:   m.streamSubscribers.Load(),
	}
}
