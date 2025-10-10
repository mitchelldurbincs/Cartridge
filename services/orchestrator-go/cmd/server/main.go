package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/orchestrator/internal/events"
	httpServer "github.com/cartridge/orchestrator/internal/http"
	"github.com/cartridge/orchestrator/internal/service"
	"github.com/cartridge/orchestrator/internal/storage"
)

func main() {
	var addr string
	flag.StringVar(&addr, "addr", ":8080", "HTTP listen address")
	flag.Parse()

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	store := storage.NewMemoryStore()
	publisher := events.NoopPublisher{}
	orch := service.NewOrchestrator(store, publisher, logger)

	// Auto-create local-run for development
	ctx := context.Background()
	if _, err := orch.GetRun(ctx, "local-run"); err != nil {
		input := service.CreateRunInput{
			ID:             "local-run",
			ExperimentID:   "local-experiment",
			VersionID:      "v1",
			LaunchManifest: []byte("{}"),
			Priority:       1,
			CreatedBy:      "local-setup",
		}
		if _, err := orch.CreateRun(ctx, input); err != nil {
			logger.Error().Err(err).Msg("failed to create local-run")
		} else {
			logger.Info().Msg("created local-run for development")
		}
	}

	h := httpServer.NewServer(orch, logger)
	srv := &http.Server{
		Addr:              addr,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		logger.Info().Str("addr", addr).Msg("orchestrator HTTP server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("http server failed")
		}
		close(done)
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	logger.Info().Msg("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("graceful shutdown failed")
	}
	<-done
	logger.Info().Msg("orchestrator stopped")
}
