package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/cartridge/web/internal/config"
	"github.com/cartridge/web/internal/logging"
	"github.com/cartridge/web/internal/orchestrator"
)

// NewRouter wires routes and middleware for the HTTP server.
func NewRouter(cfg config.Config, client orchestrator.Client, logger *zerolog.Logger, started time.Time) *chi.Mux {
	r := chi.NewRouter()

	r.Use(requestIDMiddleware)
	r.Use(realIPMiddleware)
	r.Use(recovererMiddleware)
	r.Use(timeoutMiddleware(cfg.Orchestrator.RequestTimeout + 2*time.Second))

	handlers := Handler{
		client:    client,
		logger:    logger,
		startedAt: started,
		level:     logging.ParseLevel(cfg.Observability.LogLevel),
		service:   "web-go",
	}

	r.Get("/healthz", handlers.instrument("healthz", handlers.health))
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/runs", handlers.instrument("runs.list", handlers.listRuns))
		r.Get("/runs/{id}", handlers.instrument("runs.detail", handlers.getRun))
	})

	r.Method(http.MethodGet, "/metrics", promhttp.Handler())

	return r
}
