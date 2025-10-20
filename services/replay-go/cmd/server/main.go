package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/cartridge/replay/internal/metrics"
	"github.com/cartridge/replay/internal/service"
	"github.com/cartridge/replay/internal/storage"
	replayv1 "github.com/cartridge/replay/pkg/proto/replay/v1"
)

func main() {
	portDefault := 8080
	if envPort := os.Getenv("REPLAY_PORT"); envPort != "" {
		if parsed, err := strconv.Atoi(envPort); err == nil {
			portDefault = parsed
		} else {
			log.Printf("Invalid REPLAY_PORT %q, using default %d", envPort, portDefault)
		}
	}

	metricsPortDefault := 9090
	if envPort := os.Getenv("REPLAY_METRICS_PORT"); envPort != "" {
		if parsed, err := strconv.Atoi(envPort); err == nil {
			metricsPortDefault = parsed
		} else {
			log.Printf("Invalid REPLAY_METRICS_PORT %q, using default %d", envPort, metricsPortDefault)
		}
	}

	maxSizeDefault := uint64(100000)
	if envMax := os.Getenv("REPLAY_MAX_SIZE"); envMax != "" {
		if parsed, err := strconv.ParseUint(envMax, 10, 64); err == nil {
			maxSizeDefault = parsed
		} else {
			log.Printf("Invalid REPLAY_MAX_SIZE %q, using default %d", envMax, maxSizeDefault)
		}
	}

	var (
		port        = flag.Int("port", portDefault, "gRPC server port")
		maxSize     = flag.Uint64("max-size", maxSizeDefault, "Maximum number of transitions to store")
		metricsPort = flag.Int("metrics-port", metricsPortDefault, "HTTP port for Prometheus metrics")
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

	// Create metrics collector
	collector := metrics.NewCollector(nil)

	// Create gRPC service
	replayService := service.NewReplayService(backend)
	replayService.WithMetrics(collector)

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

	metricsAddr := fmt.Sprintf(":%d", *metricsPort)
	metricsSrv := &http.Server{
		Addr:              metricsAddr,
		Handler:           collector.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Replay service listening on %s", lis.Addr())
		if err := server.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	metricsDone := make(chan struct{})
	go func() {
		log.Printf("Replay metrics listening on %s", metricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
		close(metricsDone)
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

	if err := metricsSrv.Shutdown(ctx); err != nil {
		log.Printf("Metrics server shutdown error: %v", err)
	}
	<-metricsDone
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

	log.Printf("[%s] %s - %v %s", status, info.FullMethod, duration, summarizeRequest(req))

	return resp, err
}

func summarizeRequest(req interface{}) string {
	summary, ok := buildSummary(req)
	if !ok {
		return fmt.Sprintf("(type=%T)", req)
	}

	encoded, err := json.Marshal(summary)
	if err != nil {
		return fmt.Sprintf("(type=%T)", req)
	}

	return string(encoded)
}

func buildSummary(req interface{}) (interface{}, bool) {
	switch r := req.(type) {
	case *replayv1.StoreTransitionRequest:
		return &struct {
			Transition *transitionSummary `json:"transition"`
		}{Transition: summarizeTransition(r.GetTransition())}, true
	case *replayv1.StoreBatchRequest:
		return summarizeStoreBatchRequest(r), true
	case *replayv1.SampleRequest:
		if r.GetConfig() == nil {
			return &struct{}{}, true
		}
		cfg := r.GetConfig()
		return &struct {
			BatchSize     uint32  `json:"batch_size"`
			EnvID         string  `json:"env_id,omitempty"`
			Prioritized   bool    `json:"prioritized"`
			PriorityAlpha float32 `json:"priority_alpha,omitempty"`
			MinTimestamp  uint64  `json:"min_timestamp,omitempty"`
			MaxTimestamp  uint64  `json:"max_timestamp,omitempty"`
		}{
			BatchSize:     cfg.GetBatchSize(),
			EnvID:         cfg.GetEnvId(),
			Prioritized:   cfg.GetPrioritized(),
			PriorityAlpha: cfg.GetPriorityAlpha(),
			MinTimestamp:  cfg.GetMinTimestamp(),
			MaxTimestamp:  cfg.GetMaxTimestamp(),
		}, true
	case *replayv1.GetStatsRequest:
		return &struct {
			EnvID string `json:"env_id,omitempty"`
		}{EnvID: r.GetEnvId()}, true
	case *replayv1.UpdatePrioritiesRequest:
		return &struct {
			TransitionCount int      `json:"transition_count"`
			PriorityCount   int      `json:"priority_count"`
			SampledIDs      []string `json:"sampled_ids,omitempty"`
		}{
			TransitionCount: len(r.GetTransitionIds()),
			PriorityCount:   len(r.GetNewPriorities()),
			SampledIDs:      sampleStrings(r.GetTransitionIds(), 3),
		}, true
	case *replayv1.ClearRequest:
		return &struct {
			EnvID     string `json:"env_id,omitempty"`
			Before    uint64 `json:"before_timestamp,omitempty"`
			KeepLastN uint32 `json:"keep_last_n,omitempty"`
		}{
			EnvID:     r.GetEnvId(),
			Before:    r.GetBeforeTimestamp(),
			KeepLastN: r.GetKeepLastN(),
		}, true
	default:
		return nil, false
	}
}

func summarizeStoreBatchRequest(req *replayv1.StoreBatchRequest) interface{} {
	transitions := req.GetTransitions()
	if len(transitions) == 0 {
		return &struct {
			TransitionCount int `json:"transition_count"`
		}{TransitionCount: 0}
	}

	envCounts := make(map[string]int)
	for _, t := range transitions {
		env := t.GetEnvId()
		if env == "" {
			env = "(unknown)"
		}
		envCounts[env]++
	}

	return &struct {
		TransitionCount int                  `json:"transition_count"`
		EnvCounts       map[string]int       `json:"env_counts"`
		Sampled         []*transitionSummary `json:"sampled_transitions,omitempty"`
	}{
		TransitionCount: len(transitions),
		EnvCounts:       envCounts,
		Sampled:         sampleTransitions(transitions, 2),
	}
}

type transitionSummary struct {
	ID                string   `json:"id"`
	EnvID             string   `json:"env_id"`
	EpisodeID         string   `json:"episode_id"`
	StepNumber        uint32   `json:"step_number"`
	Reward            float32  `json:"reward"`
	Done              bool     `json:"done"`
	Priority          float32  `json:"priority"`
	Timestamp         uint64   `json:"timestamp"`
	StateLength       int      `json:"state_length"`
	ActionLength      int      `json:"action_length"`
	NextStateLength   int      `json:"next_state_length"`
	ObservationLength int      `json:"observation_length"`
	NextObsLength     int      `json:"next_observation_length"`
	MetadataKeys      []string `json:"metadata_keys,omitempty"`
}

func summarizeTransition(t *replayv1.Transition) *transitionSummary {
	if t == nil {
		return nil
	}

	return &transitionSummary{
		ID:                t.GetId(),
		EnvID:             t.GetEnvId(),
		EpisodeID:         t.GetEpisodeId(),
		StepNumber:        t.GetStepNumber(),
		Reward:            t.GetReward(),
		Done:              t.GetDone(),
		Priority:          t.GetPriority(),
		Timestamp:         t.GetTimestamp(),
		StateLength:       len(t.GetState()),
		ActionLength:      len(t.GetAction()),
		NextStateLength:   len(t.GetNextState()),
		ObservationLength: len(t.GetObservation()),
		NextObsLength:     len(t.GetNextObservation()),
		MetadataKeys:      sortedKeys(t.GetMetadata()),
	}
}

func sampleTransitions(transitions []*replayv1.Transition, limit int) []*transitionSummary {
	if limit <= 0 || len(transitions) == 0 {
		return nil
	}

	count := limit
	if len(transitions) < limit {
		count = len(transitions)
	}

	sampled := make([]*transitionSummary, 0, count)
	for i := 0; i < count; i++ {
		sampled = append(sampled, summarizeTransition(transitions[i]))
	}
	return sampled
}

func sampleStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) <= limit {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[:limit]...)
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
