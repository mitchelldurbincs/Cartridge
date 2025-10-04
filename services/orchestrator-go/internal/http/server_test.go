package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/orchestrator/internal/events"
	"github.com/cartridge/orchestrator/internal/service"
	"github.com/cartridge/orchestrator/internal/storage"
)

func TestCreateRunAndHeartbeat(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := zerolog.New(io.Discard)
	orch := service.NewOrchestrator(store, events.NoopPublisher{}, logger)
	server := NewServer(orch, logger)

	runPayload := map[string]any{
		"id":              "run-1",
		"experiment_id":   "exp-1",
		"version_id":      "ver-1",
		"launch_manifest": map[string]any{"foo": "bar"},
		"created_by":      "tester",
	}
	body, _ := json.Marshal(runPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewReader(body))
	res := httptest.NewRecorder()
	server.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.Code)
	}

	heartbeat := map[string]any{
		"run_id":             "run-1",
		"status":             "running",
		"step":               5,
		"samples_per_sec":    123.0,
		"loss":               0.3,
		"checkpoint_version": 1,
	}
	hbBody, _ := json.Marshal(heartbeat)
	hbReq := httptest.NewRequest(http.MethodPost, "/api/v1/runs/run-1/heartbeat", bytes.NewReader(hbBody))
	hbReq.Header.Set("Content-Type", "application/json")
	hbRes := httptest.NewRecorder()
	server.Routes().ServeHTTP(hbRes, hbReq)
	if hbRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", hbRes.Code)
	}
}

func TestCommandLifecycle(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := zerolog.New(io.Discard)
	orch := service.NewOrchestrator(store, events.NoopPublisher{}, logger)
	server := NewServer(orch, logger)

	runPayload := map[string]any{
		"id":              "run-2",
		"experiment_id":   "exp-1",
		"version_id":      "ver-1",
		"launch_manifest": map[string]any{"foo": "bar"},
		"created_by":      "tester",
	}
	body, _ := json.Marshal(runPayload)
	server.Routes().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewReader(body)))

	cmdPayload := map[string]any{
		"id":        "cmd-1",
		"type":      "pause",
		"issued_at": time.Now().UTC(),
		"actor":     map[string]any{"type": "operator", "id": "tester"},
		"payload":   map[string]any{},
	}
	cmdBody, _ := json.Marshal(cmdPayload)
	cmdReq := httptest.NewRequest(http.MethodPost, "/api/v1/runs/run-2/commands", bytes.NewReader(cmdBody))
	cmdRes := httptest.NewRecorder()
	server.Routes().ServeHTTP(cmdRes, cmdReq)
	if cmdRes.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", cmdRes.Code)
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-2/commands/next", nil)
	nextRes := httptest.NewRecorder()
	server.Routes().ServeHTTP(nextRes, nextReq)
	if nextRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", nextRes.Code)
	}

	ackReq := httptest.NewRequest(http.MethodPost, "/api/v1/runs/run-2/commands/cmd-1/ack", nil)
	ackRes := httptest.NewRecorder()
	server.Routes().ServeHTTP(ackRes, ackReq)
	if ackRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ackRes.Code)
	}
}
