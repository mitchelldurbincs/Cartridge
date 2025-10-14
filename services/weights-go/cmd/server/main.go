package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/cartridge/weights/internal/config"
	grpcserver "github.com/cartridge/weights/internal/grpc"
	weightspb "github.com/cartridge/weights/internal/pb"
	"github.com/cartridge/weights/internal/registry"
	"github.com/cartridge/weights/internal/service"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	cfg := config.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	reg := registry.NewMemoryRegistry(cfg.Registry.HistoryDepth)
	svc := service.New(reg, &logger)

	lis, err := net.Listen("tcp", cfg.Server.Endpoint())
	if err != nil {
		logger.Fatal().Err(err).Str("endpoint", cfg.Server.Endpoint()).Msg("failed to start listener")
	}

	grpcSrv := grpc.NewServer()
	weightspb.RegisterWeightsServiceServer(grpcSrv, grpcserver.New(svc))

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("cartridge.weights.v1.WeightsService", healthpb.HealthCheckResponse_SERVING)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info().Str("endpoint", cfg.Server.Endpoint()).Msg("weights gRPC server starting")
		if err := grpcSrv.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info().Msg("shutdown signal received")
	case err, ok := <-serverErr:
		if ok && err != nil {
			logger.Fatal().Err(err).Msg("gRPC server terminated unexpectedly")
		}
	}

	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthSrv.SetServingStatus("cartridge.weights.v1.WeightsService", healthpb.HealthCheckResponse_NOT_SERVING)

	done := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info().Msg("gRPC server shutdown complete")
	case <-time.After(cfg.Server.ShutdownTimeout):
		logger.Warn().Dur("timeout", cfg.Server.ShutdownTimeout).Msg("forcing gRPC shutdown")
		grpcSrv.Stop()
	}

	// Provide a small delay to flush logs when running in containers.
	time.Sleep(200 * time.Millisecond)
}
