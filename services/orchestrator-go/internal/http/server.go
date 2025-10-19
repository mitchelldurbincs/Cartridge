package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/cartridge/orchestrator/internal/metrics"
	"github.com/cartridge/orchestrator/internal/middleware"
	"github.com/cartridge/orchestrator/internal/service"
	"github.com/cartridge/orchestrator/internal/storage"
	"github.com/cartridge/orchestrator/internal/types"
)

const maxHeartbeatBody = 32 * 1024

// Server wires HTTP handlers to the orchestrator service.
type Server struct {
	orch    *service.Orchestrator
	logger  *zerolog.Logger
	metrics *metrics.Collector
}

// NewServer constructs a Server instance.
func NewServer(orch *service.Orchestrator, logger *zerolog.Logger, collector *metrics.Collector) *Server {
	return &Server{orch: orch, logger: logger, metrics: collector}
}

// Routes builds the HTTP router for the orchestrator service.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.CorrelationID)
	r.Use(middleware.RequestLogger(s.logger))
	if s.metrics != nil {
		r.Handle("/metrics", s.metrics.Handler())
	} else {
		r.Handle("/metrics", http.NotFoundHandler())
	}
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/runs", s.handleCreateRun)
		r.Get("/runs/{runID}", s.handleGetRun)
		r.Post("/runs/{runID}/heartbeat", s.handleHeartbeat)
		r.Post("/runs/{runID}/commands", s.handleCreateCommand)
		r.Get("/runs/{runID}/commands/next", s.handleNextCommand)
		r.Post("/runs/{runID}/commands/{commandID}/ack", s.handleAckCommand)
	})
	return r
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	record := s.observeRequest(r)
	status := http.StatusInternalServerError
	defer func() { record(status) }()

	var payload service.CreateRunInput
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		status = http.StatusBadRequest
		s.writeError(w, status, "invalid JSON payload")
		return
	}
	if payload.ID == "" {
		payload.ID = generateID()
	}
	run, err := s.orch.CreateRun(r.Context(), payload)
	if err != nil {
		status = s.respondError(w, err)
		return
	}
	status = http.StatusCreated
	s.writeJSON(w, status, run)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	record := s.observeRequest(r)
	status := http.StatusInternalServerError
	defer func() { record(status) }()

	runID := chi.URLParam(r, "runID")
	run, err := s.orch.GetRun(r.Context(), runID)
	if err != nil {
		status = s.respondError(w, err)
		return
	}
	status = http.StatusOK
	s.writeJSON(w, status, run)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	record := s.observeRequest(r)
	status := http.StatusInternalServerError
	defer func() { record(status) }()

	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		status = http.StatusUnsupportedMediaType
		s.writeError(w, status, "content type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxHeartbeatBody)
	defer r.Body.Close()
	var payload types.HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		status = http.StatusBadRequest
		s.writeError(w, status, "invalid heartbeat payload")
		return
	}
	runID := chi.URLParam(r, "runID")
	run, err := s.orch.HandleHeartbeat(r.Context(), runID, payload)
	if err != nil {
		status = s.respondError(w, err)
		return
	}
	status = http.StatusOK
	s.writeJSON(w, status, run)
}

func (s *Server) handleCreateCommand(w http.ResponseWriter, r *http.Request) {
	record := s.observeRequest(r)
	status := http.StatusInternalServerError
	defer func() { record(status) }()

	runID := chi.URLParam(r, "runID")
	r.Body = http.MaxBytesReader(w, r.Body, maxHeartbeatBody)
	defer r.Body.Close()
	var payload struct {
		ID       string             `json:"id"`
		Type     types.CommandType  `json:"type"`
		IssuedAt time.Time          `json:"issued_at"`
		Actor    types.CommandActor `json:"actor"`
		Payload  json.RawMessage    `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid command payload")
		return
	}
	if payload.ID == "" {
		payload.ID = generateID()
	}
	if payload.IssuedAt.IsZero() {
		payload.IssuedAt = time.Now().UTC()
	}
	command := types.RunCommand{
		ID:        payload.ID,
		RunID:     runID,
		Type:      payload.Type,
		Payload:   payload.Payload,
		Actor:     payload.Actor,
		IssuedAt:  payload.IssuedAt,
		CreatedAt: time.Now().UTC(),
	}
	command, err := s.orch.CreateCommand(r.Context(), command)
	if err != nil {
		status = s.respondError(w, err)
		return
	}
	status = http.StatusAccepted
	s.writeJSON(w, status, command)
}

func (s *Server) handleNextCommand(w http.ResponseWriter, r *http.Request) {
	record := s.observeRequest(r)
	status := http.StatusInternalServerError
	defer func() { record(status) }()

	runID := chi.URLParam(r, "runID")
	cmd, err := s.orch.NextCommand(r.Context(), runID)
	if err != nil {
		status = s.respondError(w, err)
		return
	}
	status = http.StatusOK
	s.writeJSON(w, status, cmd)
}

func (s *Server) handleAckCommand(w http.ResponseWriter, r *http.Request) {
	record := s.observeRequest(r)
	status := http.StatusInternalServerError
	defer func() { record(status) }()

	runID := chi.URLParam(r, "runID")
	commandID := chi.URLParam(r, "commandID")
	defer r.Body.Close()
	cmd, err := s.orch.AckCommand(r.Context(), runID, commandID)
	if err != nil {
		status = s.respondError(w, err)
		return
	}
	status = http.StatusOK
	s.writeJSON(w, status, cmd)
}

func (s *Server) respondError(w http.ResponseWriter, err error) int {
	status := http.StatusUnprocessableEntity
	message := err.Error()
	switch {
	case errors.Is(err, storage.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, storage.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, storage.ErrNoCommands):
		status = http.StatusNoContent
		s.writeJSON(w, status, map[string]string{"message": "no pending commands"})
		return status
	default:
		status = http.StatusUnprocessableEntity
	}
	s.writeError(w, status, message)
	return status
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Error().Err(err).Msg("failed to encode response")
	}
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (s *Server) observeRequest(r *http.Request) func(int) {
	if s.metrics == nil {
		return func(int) {}
	}
	start := time.Now()
	route := r.URL.Path
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if pattern := rc.RoutePattern(); pattern != "" {
			route = pattern
		}
	}
	method := r.Method
	return func(status int) {
		s.metrics.ObserveAPIRequest(method, route, status, time.Since(start))
	}
}
