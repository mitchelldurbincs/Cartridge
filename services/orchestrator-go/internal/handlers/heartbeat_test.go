package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cartridge/orchestrator/internal/events"
	"github.com/cartridge/orchestrator/internal/storage"
	"github.com/cartridge/orchestrator/internal/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func TestEnhancedHeartbeatHandler_HandleHeartbeat(t *testing.T) {
	runID := uuid.New()

	// Create a test run
	testRun := types.Run{
		ID:                runID.String(),
		ExperimentID:      "test-experiment",
		VersionID:         "v1",
		State:             types.RunStateRunning,
		RuntimeStatus:     types.RuntimeStatusRunning,
		HealthStatus:      types.RunHealthHealthy,
		CurrentStep:       999,
		SamplesPerSecond:  0,
		Loss:              0,
		CheckpointVersion: 5,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Setup in-memory storage
	store := storage.NewMemoryStore()
	ctx := context.Background()
	store.CreateRun(ctx, testRun)

	// Setup handler
	publisher := events.NoopPublisher{}
	logger := zerolog.New(io.Discard).Level(zerolog.Disabled)
	handler := NewEnhancedHeartbeatHandler(store, publisher, *logger)

	// Create request
	requestBody := LearnerHeartbeatRequest{
		Step:           1000,
		PolicyLoss:     0.1,
		ValueLoss:      0.15,
		CheckpointStep: &[]int64{10}[0],
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/runs/"+runID.String()+"/heartbeat", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	// Add URL param
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", runID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Execute
	rr := httptest.NewRecorder()
	handler.HandleHeartbeat(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		return
	}

	// Verify the run was updated
	updatedRun, err := store.GetRun(ctx, runID.String())
	if err != nil {
		t.Fatalf("Failed to get updated run: %v", err)
	}

	if updatedRun.CurrentStep != 1000 {
		t.Errorf("Expected step 1000, got %d", updatedRun.CurrentStep)
	}

	expectedLoss := (0.1 + 0.15) / 2 // Combined loss
	if updatedRun.Loss != expectedLoss {
		t.Errorf("Expected loss %f, got %f", expectedLoss, updatedRun.Loss)
	}

	if updatedRun.CheckpointVersion != 10 {
		t.Errorf("Expected checkpoint version 10, got %d", updatedRun.CheckpointVersion)
	}
}
