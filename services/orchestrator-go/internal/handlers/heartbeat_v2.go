package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cartridge/orchestrator/internal/events"
	"github.com/cartridge/orchestrator/internal/storage"
	"github.com/cartridge/orchestrator/internal/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Enhanced heartbeat handler with better integration
type EnhancedHeartbeatHandler struct {
	runStore  storage.RunStore
	publisher events.Publisher
	logger    zerolog.Logger
}

// Compatible with learner service expectations
type LearnerHeartbeatRequest struct {
	Step           int64   `json:"step"`
	PolicyLoss     float64 `json:"policy_loss"`
	ValueLoss      float64 `json:"value_loss"`
	CheckpointStep *int64  `json:"checkpoint_step"`
	// Additional fields for orchestrator
	Status         *types.RuntimeStatus `json:"status,omitempty"`
	SamplesPerSec  *float64             `json:"samples_per_sec,omitempty"`
	QueuedCommands []string             `json:"queued_commands,omitempty"`
	Notes          *string              `json:"notes,omitempty"`
}

func NewEnhancedHeartbeatHandler(runStore storage.RunStore, publisher events.Publisher, logger zerolog.Logger) *EnhancedHeartbeatHandler {
	return &EnhancedHeartbeatHandler{
		runStore:  runStore,
		publisher: publisher,
		logger:    logger,
	}
}

func (h *EnhancedHeartbeatHandler) HandleHeartbeat(w http.ResponseWriter, r *http.Request) {
	// Add request correlation ID
	correlationID := r.Header.Get("X-Correlation-ID")
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	logger := h.logger.With().Str("correlation_id", correlationID).Logger()

	runIDStr := chi.URLParam(r, "id")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		logger.Error().Str("run_id", runIDStr).Msg("Invalid run ID")
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	var req LearnerHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error().Err(err).Msg("Failed to decode heartbeat request")
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Get the current run
	ctx := r.Context()
	currentRun, err := h.runStore.GetRun(ctx, runID.String())
	if err != nil {
		if err == storage.ErrNotFound {
			logger.Error().Str("run_id", runIDStr).Msg("Run not found")
			http.Error(w, "Run not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Str("run_id", runIDStr).Msg("Failed to get run")
		http.Error(w, "Failed to get run", http.StatusInternalServerError)
		return
	}

	// Convert learner format to internal heartbeat format
	status := types.RuntimeStatusRunning
	if req.Status != nil {
		status = *req.Status
	}

	samplesPerSec := 0.0
	if req.SamplesPerSec != nil {
		samplesPerSec = *req.SamplesPerSec
	}

	checkpointVersion := int64(0)
	if req.CheckpointStep != nil {
		checkpointVersion = *req.CheckpointStep
	}

	heartbeat := types.HeartbeatPayload{
		RunID:             runID.String(),
		Status:            status,
		Step:              req.Step,
		SamplesPerSecond:  samplesPerSec,
		Loss:              (req.PolicyLoss + req.ValueLoss) / 2, // Combined loss
		CheckpointVersion: checkpointVersion,
		QueuedCommands:    req.QueuedCommands,
		Notes:             "",
	}
	if req.Notes != nil {
		heartbeat.Notes = *req.Notes
	}

	// Validate heartbeat
	if err := heartbeat.Validate(runID.String(), currentRun.CurrentStep, currentRun.CheckpointVersion); err != nil {
		logger.Error().Err(err).Str("run_id", runIDStr).Msg("Invalid heartbeat")
		http.Error(w, fmt.Sprintf("Invalid heartbeat: %v", err), http.StatusBadRequest)
		return
	}

	// Update the run with heartbeat data
	updatedRun := currentRun.MergeHeartbeat(heartbeat, time.Now())
	if err := h.runStore.UpdateRun(ctx, updatedRun); err != nil {
		logger.Error().Err(err).Str("run_id", runIDStr).Msg("Failed to update run")
		http.Error(w, "Failed to update run", http.StatusInternalServerError)
		return
	}

	// Publish event with enhanced data
	event := events.RunStatusEvent{
		RunID:            runID.String(),
		State:            string(updatedRun.State),
		RuntimeStatus:    string(updatedRun.RuntimeStatus),
		HealthStatus:     string(updatedRun.HealthStatus),
		Step:             updatedRun.CurrentStep,
		SamplesPerSecond: updatedRun.SamplesPerSecond,
		Loss:             updatedRun.Loss,
	}

	if err := h.publisher.PublishRunStatus(ctx, event); err != nil {
		logger.Error().Err(err).Msg("Failed to publish heartbeat event")
	}

	logger.Info().
		Str("run_id", runIDStr).
		Int64("step", req.Step).
		Float64("policy_loss", req.PolicyLoss).
		Float64("value_loss", req.ValueLoss).
		Msg("Heartbeat processed successfully")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Correlation-ID", correlationID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accepted":       true,
		"correlation_id": correlationID,
	})
}