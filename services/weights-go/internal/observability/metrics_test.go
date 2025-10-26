package observability

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsPublishCompleteSuccess(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer(reg)

	m.PublishComplete(100*time.Millisecond, nil)
	m.PublishComplete(200*time.Millisecond, nil)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	var foundCounter, foundHistogram bool
	for _, mf := range metrics {
		if mf.GetName() == "weights_publish_total" {
			foundCounter = true
			for _, m := range mf.GetMetric() {
				for _, label := range m.GetLabel() {
					if label.GetName() == "status" && label.GetValue() == "success" {
						if got := m.GetCounter().GetValue(); got != 2 {
							t.Errorf("expected success counter=2, got %f", got)
						}
					}
				}
			}
		}
		if mf.GetName() == "weights_publish_latency_seconds" {
			foundHistogram = true
			if got := mf.GetMetric()[0].GetHistogram().GetSampleCount(); got != 2 {
				t.Errorf("expected histogram count=2, got %d", got)
			}
		}
	}

	if !foundCounter {
		t.Error("expected to find weights_publish_total counter")
	}
	if !foundHistogram {
		t.Error("expected to find weights_publish_latency_seconds histogram")
	}
}

func TestMetricsPublishCompleteFailure(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer(reg)

	testErr := errors.New("publish failed")
	m.PublishComplete(50*time.Millisecond, testErr)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	for _, mf := range metrics {
		if mf.GetName() == "weights_publish_total" {
			for _, m := range mf.GetMetric() {
				for _, label := range m.GetLabel() {
					if label.GetName() == "status" && label.GetValue() == "failure" {
						if got := m.GetCounter().GetValue(); got != 1 {
							t.Errorf("expected failure counter=1, got %f", got)
						}
						return
					}
				}
			}
		}
	}

	t.Error("expected to find weights_publish_total{status=\"failure\"} counter")
}

func TestMetricsStreamSubscribers(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer(reg)

	m.StreamSubscribed()
	m.StreamSubscribed()
	m.StreamCancelled()

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	for _, mf := range metrics {
		if mf.GetName() == "weights_stream_subscribers" {
			got := mf.GetMetric()[0].GetGauge().GetValue()
			if got != 1 {
				t.Errorf("expected gauge value=1, got %f", got)
			}
			return
		}
	}

	t.Error("expected to find weights_stream_subscribers gauge")
}

func TestMetricsHandler(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer(reg)

	m.PublishComplete(100*time.Millisecond, nil)
	m.StreamSubscribed()

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	m.Handler().ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	output := string(body)

	// Verify we have the expected metrics in the output
	expectedMetrics := []string{
		"weights_publish_total",
		"weights_publish_latency_seconds",
		"weights_stream_subscribers",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected metric %q in output, but it was not found", metric)
		}
	}

	// Verify we have histogram buckets (not just sum/count)
	if !strings.Contains(output, "weights_publish_latency_seconds_bucket") {
		t.Error("expected histogram buckets in output, but found none")
	}

	// Verify TYPE declarations
	if !strings.Contains(output, "TYPE weights_publish_total counter") {
		t.Error("expected TYPE declaration for counter")
	}
	if !strings.Contains(output, "TYPE weights_publish_latency_seconds histogram") {
		t.Error("expected TYPE declaration for histogram")
	}
	if !strings.Contains(output, "TYPE weights_stream_subscribers gauge") {
		t.Error("expected TYPE declaration for gauge")
	}
}

func TestMetricsNilReceiverSafety(t *testing.T) {
	var m *Metrics

	// These should not panic
	m.PublishComplete(100*time.Millisecond, nil)
	m.StreamSubscribed()
	m.StreamCancelled()

	// Handler should return a valid handler even with nil receiver
	handler := m.Handler()
	if handler == nil {
		t.Error("expected non-nil handler even with nil metrics")
	}
}
