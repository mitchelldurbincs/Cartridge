package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	webconfig "github.com/cartridge/web/internal/config"
	webhttp "github.com/cartridge/web/internal/http"
	"github.com/cartridge/web/internal/logging"
	"github.com/cartridge/web/internal/orchestrator"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startedAt := time.Now()

	baseLogger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	cfg, err := webconfig.Load("configs/web-go", "./configs/web-go", "./")
	if err != nil {
		baseLogger.Fatal().Err(err).Msg("failed to load config")
	}

	logLevel := logging.ParseLevel(cfg.Observability.LogLevel)
	logger := baseLogger

	client := orchestrator.NewInMemoryClient(startedAt)
	router := webhttp.NewRouter(cfg, client, logger, startedAt)

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		if logging.ShouldLog(logLevel, zerolog.InfoLevel) {
			logger.Info().Str("service", "web-go").Str("addr", cfg.Server.Address()).Msg("starting http server")
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("http server exited")
		}
	}()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if logging.ShouldLog(logLevel, zerolog.InfoLevel) {
		logger.Info().Str("service", "web-go").Msg("shutting down http server")
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		if logging.ShouldLog(logLevel, zerolog.ErrorLevel) {
			logger.Error().Str("service", "web-go").Err(err).Msg("graceful shutdown failed")
		}
		if err := srv.Close(); err != nil {
			if logging.ShouldLog(logLevel, zerolog.ErrorLevel) {
				logger.Error().Str("service", "web-go").Err(err).Msg("forced close failed")
			}
		}
	}
	if logging.ShouldLog(logLevel, zerolog.InfoLevel) {
		logger.Info().Str("service", "web-go").Msg("shutdown complete")
	}
}
