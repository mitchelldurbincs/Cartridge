package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/cartridge/replay/internal/service"
	"github.com/cartridge/replay/internal/storage"
	replayv1 "github.com/cartridge/replay/pkg/proto/replay/v1"
)

func main() {
	var (
		port    = flag.Int("port", 8080, "gRPC server port")
		maxSize = flag.Uint64("max-size", 100000, "Maximum number of transitions to store")
	)
	flag.Parse()

	log.Printf("Starting Replay service on port %d", *port)

	// Create storage backend
	backend := storage.NewMemoryBackend(*maxSize)
	defer func() {
		if err := backend.Close(); err != nil {
			log.Printf("Error closing backend: %v", err)
		}
	}()

	// Create gRPC service
	replayService := service.NewReplayService(backend)

	// Create gRPC server
	server := grpc.NewServer(
		grpc.UnaryInterceptor(loggingInterceptor),
	)

	// Register service
	replayv1.RegisterReplayServer(server, replayService)

	// Enable reflection for development
	reflection.Register(server)

	// Create listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Replay service listening on %s", lis.Addr())
		if err := server.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down gracefully...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutdown timeout exceeded, forcing stop")
		server.Stop()
	case <-stopped:
		log.Println("Server stopped gracefully")
	}
}

// loggingInterceptor logs gRPC requests
func loggingInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	// Call the handler
	resp, err := handler(ctx, req)

	// Log the request
	duration := time.Since(start)
	status := "OK"
	if err != nil {
		status = "ERROR"
	}

	log.Printf("[%s] %s - %v (%s)", status, info.FullMethod, duration, req)

	return resp, err
}