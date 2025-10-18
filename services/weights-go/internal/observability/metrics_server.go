package observability

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// RunMetricsServer starts an HTTP server exposing the provided metrics handler.
func RunMetricsServer(ctx context.Context, addr string, handler http.Handler, logger *zerolog.Logger) {
	if handler == nil || addr == "" {
		return
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			if logger != nil {
				logger.Warn().Err(err).Msg("failed to shutdown metrics server cleanly")
			}
		}
	}()

	go func() {
		if logger != nil {
			logger.Info().Str("addr", addr).Msg("metrics server starting")
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if logger != nil {
				logger.Error().Err(err).Str("addr", addr).Msg("metrics server failed")
			}
		}
	}()
}
