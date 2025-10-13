package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/weights/internal/config"
	"github.com/cartridge/weights/internal/registry"
	"github.com/cartridge/weights/internal/service"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	cfg := config.Load()

	logger.Info().Str("endpoint", cfg.Server.Endpoint()).Msg("starting weights service skeleton")

	reg := registry.NewMemoryRegistry(cfg.Registry.HistoryDepth)
	_ = service.New(reg, logger)

	logger.Info().Msg("weights gRPC server wiring pending - standing by")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	<-ctx.Done()
	logger.Info().Msg("weights service shutdown complete")

	// Provide a small delay to flush logs when running in containers.
	time.Sleep(200 * time.Millisecond)
}
