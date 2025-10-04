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

	"github.com/cartridge/orchestrator/internal/service"
	"github.com/cartridge/orchestrator/internal/storage"
	"github.com/cartridge/orchestrator/internal/types"
)

const maxHeartbeatBody = 32 * 1024

// Server wires HTTP handlers to the orchestrator service.
type Server struct {
	orch   *service.Orchestrator
	logger *zerolog.Logger
}

// NewServer constructs a Server instance.
func NewServer(orch *service.Orchestrator, logger *zerolog.Logger) *Server {
	return &Server{orch: orch, logger: logger}
}

// Routes builds the HTTP router for the orchestrator service.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
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
	var payload service.CreateRunInput
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if payload.ID == "" {
		payload.ID = generateID()
	}
	run, err := s.orch.CreateRun(r.Context(), payload)
	if err != nil {
		s.respondError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, run)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	run, err := s.orch.GetRun(r.Context(), runID)
	if err != nil {
		s.respondError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		s.writeError(w, http.StatusUnsupportedMediaType, "content type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxHeartbeatBody)
	defer r.Body.Close()
	var payload types.HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid heartbeat payload")
		return
	}
	runID := chi.URLParam(r, "runID")
	run, err := s.orch.HandleHeartbeat(r.Context(), runID, payload)
	if err != nil {
		s.respondError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleCreateCommand(w http.ResponseWriter, r *http.Request) {
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
		s.respondError(w, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, command)
}

func (s *Server) handleNextCommand(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	cmd, err := s.orch.NextCommand(r.Context(), runID)
	if err != nil {
		s.respondError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, cmd)
}

func (s *Server) handleAckCommand(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	commandID := chi.URLParam(r, "commandID")
	defer r.Body.Close()
	cmd, err := s.orch.AckCommand(r.Context(), runID, commandID)
	if err != nil {
		s.respondError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, cmd)
}

func (s *Server) respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		s.writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, storage.ErrConflict):
		s.writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, storage.ErrNoCommands):
		s.writeJSON(w, http.StatusNoContent, map[string]string{"message": "no pending commands"})
	default:
		s.writeError(w, http.StatusUnprocessableEntity, err.Error())
	}
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
