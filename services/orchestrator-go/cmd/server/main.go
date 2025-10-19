package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/orchestrator/internal/config"
	"github.com/cartridge/orchestrator/internal/events"
	httpServer "github.com/cartridge/orchestrator/internal/http"
	"github.com/cartridge/orchestrator/internal/metrics"
	"github.com/cartridge/orchestrator/internal/service"
	"github.com/cartridge/orchestrator/internal/storage"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load orchestrator config")
	}

	var (
		addr            string
		readTimeout     time.Duration
		writeTimeout    time.Duration
		shutdownTimeout time.Duration
	)

	defaultAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	flag.StringVar(&addr, "addr", defaultAddr, "HTTP listen address")
	flag.DurationVar(&readTimeout, "read-timeout", cfg.Server.ReadTimeout, "HTTP read timeout")
	flag.DurationVar(&writeTimeout, "write-timeout", cfg.Server.WriteTimeout, "HTTP write timeout")
	flag.DurationVar(&shutdownTimeout, "shutdown-timeout", cfg.Server.ShutdownTimeout, "Graceful shutdown timeout")
	flag.Parse()

	store := storage.NewMemoryStore()
	publisher := events.NoopPublisher{}
	collector := metrics.NewCollector(nil)
	orch := service.NewOrchestrator(store, publisher, logger)
	orch.WithMetrics(collector)

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

	h := httpServer.NewServer(orch, logger, collector)
	srv := &http.Server{
		Addr:              addr,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
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

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("graceful shutdown failed")
	}
	<-done
	logger.Info().Msg("orchestrator stopped")
}
