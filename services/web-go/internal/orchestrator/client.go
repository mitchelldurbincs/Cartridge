package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrRunNotFound is returned when a run cannot be located.
var ErrRunNotFound = errors.New("run not found")

// TODO: replace with a gRPC/REST backed client once the orchestrator API is available.
// Client represents the orchestrator facade the web API relies on.
type Client interface {
	ListRuns(ctx context.Context) ([]Run, error)
	GetRun(ctx context.Context, id string) (Run, error)
}

// Run holds the key bits of state the dashboard expects to render.
type Run struct {
	ID             string             `json:"id"`
	Status         string             `json:"status"`
	Owner          string             `json:"owner"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	LastHeartbeat  time.Time          `json:"last_heartbeat"`
	Labels         []string           `json:"labels,omitempty"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
	HeartbeatDelay time.Duration      `json:"heartbeat_delay"`
}

// InMemoryClient provides a temporary in-process implementation.
type InMemoryClient struct {
	mu   sync.RWMutex
	runs map[string]Run
}

// NewInMemoryClient seeds the client with static sample data.
func NewInMemoryClient(now time.Time) *InMemoryClient {
	runs := map[string]Run{
		"run-001": {
			ID:             "run-001",
			Status:         "running",
			Owner:          "team-alpha",
			CreatedAt:      now.Add(-10 * time.Minute),
			UpdatedAt:      now.Add(-30 * time.Second),
			LastHeartbeat:  now.Add(-5 * time.Second),
			Labels:         []string{"dev", "priority:high"},
			Metrics:        map[string]float64{"frames_processed": 12034, "throughput_fps": 512.4},
			HeartbeatDelay: 5 * time.Second,
		},
		"run-002": {
			ID:             "run-002",
			Status:         "completed",
			Owner:          "team-beta",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now.Add(-1 * time.Hour),
			LastHeartbeat:  now.Add(-65 * time.Minute),
			Labels:         []string{"prod"},
			Metrics:        map[string]float64{"frames_processed": 220034, "throughput_fps": 498.1},
			HeartbeatDelay: 65 * time.Minute,
		},
	}

	return &InMemoryClient{runs: runs}
}

// ListRuns returns the current set of runs.
func (c *InMemoryClient) ListRuns(ctx context.Context) ([]Run, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	runs := make([]Run, 0, len(c.runs))
	for _, run := range c.runs {
		runs = append(runs, run)
	}
	return runs, nil
}

// GetRun finds a specific run by identifier.
func (c *InMemoryClient) GetRun(ctx context.Context, id string) (Run, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	run, ok := c.runs[id]
	if !ok {
		return Run{}, ErrRunNotFound
	}
	return run, nil
}

// Seed allows tests to add or replace runs dynamically.
func (c *InMemoryClient) Seed(run Run) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.runs == nil {
		c.runs = make(map[string]Run)
	}
	c.runs[run.ID] = run
}
