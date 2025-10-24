package metrics

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestCollectorRecordsMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollector(registry)

	collector.RecordStore("StoreBatch", "success", 250*time.Millisecond, 7)
	collector.RecordSample(true, "success", 150*time.Millisecond, 11)
	collector.RecordPriorityUpdate("success", 5)
	collector.RecordPriorityUpdate("error", 3)
	collector.RecordClear("failure", 9)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	storeRequests := findMetric(t, families, "replay_store_requests_total")
	if got := storeRequests.Metrics[0].Value; got != 1 {
		t.Fatalf("store requests counter = %v, want 1", got)
	}

	storeDuration := findMetric(t, families, "replay_store_duration_seconds")
	histogram := storeDuration.Metrics[0]
	if histogram.Count != 1 {
		t.Fatalf("store duration count = %v, want 1", histogram.Count)
	}
	if diff := math.Abs(histogram.Sum - 0.25); diff > 1e-9 {
		t.Fatalf("store duration sum = %v, want 0.25", histogram.Sum)
	}

	storeTransitions := findMetric(t, families, "replay_store_transitions_total")
	if got := storeTransitions.Metrics[0].Value; got != 7 {
		t.Fatalf("store transitions counter = %v, want 7", got)
	}

	sampleRequests := findMetric(t, families, "replay_sample_requests_total")
	sampleMetric := sampleRequests.Metrics[0]
	if got := sampleMetric.Value; got != 1 {
		t.Fatalf("sample requests counter = %v, want 1", got)
	}
	if sampleMetric.Labels["prioritized"] != "true" || sampleMetric.Labels["result"] != "success" {
		t.Fatalf("unexpected sample request labels: %v", sampleMetric.Labels)
	}

	sampleDuration := findMetric(t, families, "replay_sample_duration_seconds")
	histogram = sampleDuration.Metrics[0]
	if histogram.Count != 1 {
		t.Fatalf("sample duration count = %v, want 1", histogram.Count)
	}
	if diff := math.Abs(histogram.Sum - 0.15); diff > 1e-9 {
		t.Fatalf("sample duration sum = %v, want 0.15", histogram.Sum)
	}

	sampleTransitions := findMetric(t, families, "replay_sample_transitions_total")
	if got := sampleTransitions.Metrics[0].Value; got != 11 {
		t.Fatalf("sample transitions counter = %v, want 11", got)
	}

	priorityUpdates := findMetric(t, families, "replay_priority_updates_total")
	if len(priorityUpdates.Metrics) != 2 {
		t.Fatalf("priority updates metrics = %d, want 2", len(priorityUpdates.Metrics))
	}
	for _, metric := range priorityUpdates.Metrics {
		switch metric.Labels["result"] {
		case "success":
			if metric.Value != 1 {
				t.Fatalf("priority success counter = %v, want 1", metric.Value)
			}
		case "error":
			if metric.Value != 1 {
				t.Fatalf("priority error counter = %v, want 1", metric.Value)
			}
		default:
			t.Fatalf("unexpected priority update label: %v", metric.Labels)
		}
	}

	priorityTransitions := findMetric(t, families, "replay_priority_transitions_total")
	if got := priorityTransitions.Metrics[0].Value; got != 5 {
		t.Fatalf("priority transitions counter = %v, want 5", got)
	}

	clearRequests := findMetric(t, families, "replay_clear_requests_total")
	clearMetric := clearRequests.Metrics[0]
	if clearMetric.Value != 1 {
		t.Fatalf("clear requests counter = %v, want 1", clearMetric.Value)
	}
	if clearMetric.Labels["result"] != "failure" {
		t.Fatalf("unexpected clear labels: %v", clearMetric.Labels)
	}

	clearTransitions := findMetric(t, families, "replay_clear_transitions_total")
	if got := clearTransitions.Metrics[0].Value; got != 9 {
		t.Fatalf("clear transitions counter = %v, want 9", got)
	}
}

func TestCollectorHandlerUsesCustomGatherer(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := NewCollector(registry)

	collector.RecordClear("success", 3)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	collector.Handler().ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("unexpected status code: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "replay_clear_transitions_total") {
		t.Fatalf("metrics response missing clear transitions counter: %s", body)
	}
}

func TestBoolLabel(t *testing.T) {
	if got := boolLabel(true); got != "true" {
		t.Fatalf("boolLabel(true) = %q, want 'true'", got)
	}
	if got := boolLabel(false); got != "false" {
		t.Fatalf("boolLabel(false) = %q, want 'false'", got)
	}
}

func findMetric(t *testing.T, families []*prometheus.MetricFamily, name string) *prometheus.MetricFamily {
	t.Helper()
	for _, fam := range families {
		if fam.Name == name {
			return fam
		}
	}
	t.Fatalf("metric %q not found", name)
	return nil
}
