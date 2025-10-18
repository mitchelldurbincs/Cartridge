package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	rediscompat "github.com/cartridge/weights/internal/compat/redis"
	"github.com/cartridge/weights/internal/config"
	grpcserver "github.com/cartridge/weights/internal/grpc"
	"github.com/cartridge/weights/internal/observability"
	weightspb "github.com/cartridge/weights/internal/pb"
	"github.com/cartridge/weights/internal/redisclient"
	"github.com/cartridge/weights/internal/registry"
	"github.com/cartridge/weights/internal/service"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	cfg := config.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	metrics := observability.NewMetrics()
	if addr := cfg.Observability.MetricsAddress; addr != "" {
		observability.RunMetricsServer(ctx, addr, metrics.Handler(), logger)
	}

	tracer := observability.NoopTracer()
	if cfg.Observability.TracingEnabled {
		tracer = observability.NewLoggerTracer(logger)
	}

	reg, err := buildRegistry(ctx, cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to configure registry backend")
	}

	opts := []service.Option{service.WithTracer(tracer), service.WithMetrics(metrics)}

	if cfg.Compatibility.MirrorToRedis {
		publisher, err := buildCompatibilityPublisher(ctx, cfg, logger)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to configure redis compatibility publisher")
		} else {
			opts = append(opts, service.WithPublisher(publisher))
		}
	}

	svc := service.New(reg, logger, opts...)

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

func buildRegistry(ctx context.Context, cfg config.Config) (service.Registry, error) {
	switch cfg.Registry.Backend {
	case "", "memory":
		return registry.NewMemoryRegistry(cfg.Registry.HistoryDepth), nil
	case "redis":
		address := cfg.Registry.PersistenceDSN
		if address == "" {
			address = cfg.Redis.Address
		}
		client, err := redisclient.New(redisclient.Options{
			Address:  address,
			Password: cfg.Redis.Password,
			Database: cfg.Redis.Database,
			Timeout:  cfg.Redis.Timeout,
		})
		if err != nil {
			return nil, err
		}
		if _, err := client.Do(ctx, "PING"); err != nil {
			return nil, err
		}
		return registry.NewRedisRegistry(client, cfg.Registry.HistoryDepth)
	default:
		return nil, fmt.Errorf("unsupported registry backend %q", cfg.Registry.Backend)
	}
}

func buildCompatibilityPublisher(ctx context.Context, cfg config.Config, logger *zerolog.Logger) (service.Publisher, error) {
	client, err := redisclient.New(redisclient.Options{
		Address:  cfg.Redis.Address,
		Password: cfg.Redis.Password,
		Database: cfg.Redis.Database,
		Timeout:  cfg.Redis.Timeout,
	})
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(ctx, "PING"); err != nil {
		return nil, err
	}
	return rediscompat.NewPublisher(client, cfg.Redis.Channel, logger)
}
