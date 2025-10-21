package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	h := newHandler(t, mockClient{})

	rec := callHandler(t, h.instrument("healthz", h.health), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", payload["status"])
	}
	if _, ok := payload["uptime_seconds"]; !ok {
		t.Fatalf("expected uptime_seconds field")
	}
}
