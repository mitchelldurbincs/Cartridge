package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/cartridge/web/internal/logging"
	"github.com/cartridge/web/internal/orchestrator"
)

type Handler struct {
	client    orchestrator.Client
	logger    *zerolog.Logger
	startedAt time.Time
	level     zerolog.Level
	service   string
}

func (h Handler) health(w http.ResponseWriter, r *http.Request) {
	payload := map[string]any{
		"status":         "ok",
		"service":        "web-go",
		"started_at":     h.startedAt.Format(time.RFC3339),
		"uptime_seconds": time.Since(h.startedAt).Seconds(),
	}
	h.writeJSON(w, http.StatusOK, payload)
}

func (h Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.client.ListRuns(r.Context())
	if err != nil {
		if h.logger != nil && logging.ShouldLog(h.level, zerolog.ErrorLevel) {
			h.logger.Error().Str("service", h.service).Err(err).Msg("failed to list runs")
		}
		h.writeError(w, http.StatusBadGateway, "failed to load runs")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (h Handler) getRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if runID == "" {
		h.writeError(w, http.StatusBadRequest, "missing run id")
		return
	}

	run, err := h.client.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, orchestrator.ErrRunNotFound) {
			h.writeError(w, http.StatusNotFound, "run not found")
			return
		}
		if h.logger != nil && logging.ShouldLog(h.level, zerolog.ErrorLevel) {
			h.logger.Error().Str("service", h.service).Err(err).Str("run_id", runID).Msg("failed to fetch run")
		}
		h.writeError(w, http.StatusBadGateway, "failed to load run")
		return
	}

	h.writeJSON(w, http.StatusOK, run)
}

func (h Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		if h.logger != nil {
			h.logger.Error().Str("service", h.service).Err(err).Msg("failed to encode response")
		}
	}
}

func (h Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}
