package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cartridge/orchestrator/internal/db"
	"github.com/cartridge/orchestrator/internal/events"
	"github.com/cartridge/orchestrator/pkg/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Enhanced heartbeat handler with better integration
type EnhancedHeartbeatHandler struct {
	runRepo   *db.RunRepository
	publisher *events.Publisher
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

func NewEnhancedHeartbeatHandler(runRepo *db.RunRepository, publisher *events.Publisher, logger zerolog.Logger) *EnhancedHeartbeatHandler {
	return &EnhancedHeartbeatHandler{
		runRepo:   runRepo,
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

	// Convert learner format to internal format
	heartbeatData := &db.HeartbeatData{
		Status:            req.Status,
		Step:              req.Step,
		SamplesPerSec:     req.SamplesPerSec,
		Loss:              (req.PolicyLoss + req.ValueLoss) / 2, // Combined loss
		CheckpointVersion: req.CheckpointStep,
		QueuedCommands:    req.QueuedCommands,
		Notes:             req.Notes,
	}

	// Apply defaults if not provided
	if heartbeatData.Status == nil {
		defaultStatus := types.RuntimeStatusRunning
		heartbeatData.Status = &defaultStatus
	}

	if heartbeatData.SamplesPerSec == nil {
		defaultSPS := 0.0
		heartbeatData.SamplesPerSec = &defaultSPS
	}

	// Existing validation and update logic...
	if err := h.runRepo.UpdateHeartbeat(runID, heartbeatData); err != nil {
		logger.Error().Err(err).Str("run_id", runIDStr).Msg("Failed to update heartbeat")
		http.Error(w, "Failed to update heartbeat", http.StatusInternalServerError)
		return
	}

	// Publish event with enhanced data
	event := &events.RunStatusEvent{
		RunID:         runID,
		Status:        string(*heartbeatData.Status),
		Step:          &req.Step,
		SamplesPerSec: heartbeatData.SamplesPerSec,
		Timestamp:     time.Now(),
	}

	if err := h.publisher.PublishRunStatus(event); err != nil {
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