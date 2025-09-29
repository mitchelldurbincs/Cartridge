package storage

import (
	"context"
	"time"
)

// Transition represents a single experience transition
type Transition struct {
	ID              string            `json:"id"`
	EnvID           string            `json:"env_id"`
	EpisodeID       string            `json:"episode_id"`
	StepNumber      uint32            `json:"step_number"`
	State           []byte            `json:"state"`
	Action          []byte            `json:"action"`
	NextState       []byte            `json:"next_state"`
	Observation     []byte            `json:"observation"`
	NextObservation []byte            `json:"next_observation"`
	Reward          float32           `json:"reward"`
	Done            bool              `json:"done"`
	Priority        float32           `json:"priority"`
	Timestamp       time.Time         `json:"timestamp"`
	Metadata        map[string]string `json:"metadata"`
}

// SampleConfig defines parameters for sampling transitions
type SampleConfig struct {
	BatchSize     uint32
	EnvID         string
	Prioritized   bool
	PriorityAlpha float32
	MinTimestamp  *time.Time
	MaxTimestamp  *time.Time
}

// Stats represents replay buffer statistics
type Stats struct {
	TotalTransitions   uint64
	TotalEpisodes      uint64
	TransitionsByEnv   map[string]uint64
	OldestTimestamp    *time.Time
	NewestTimestamp    *time.Time
	StorageBytes       uint64
}

// Backend defines the interface for replay buffer storage implementations
type Backend interface {
	// Store a single transition
	Store(ctx context.Context, transition *Transition) error

	// Store multiple transitions in a batch
	StoreBatch(ctx context.Context, transitions []*Transition) ([]string, error)

	// Sample transitions according to the given configuration
	Sample(ctx context.Context, config *SampleConfig) ([]*Transition, []float32, error)

	// Get buffer statistics
	GetStats(ctx context.Context, envID string) (*Stats, error)

	// Update priorities for prioritized replay
	UpdatePriorities(ctx context.Context, transitionIDs []string, priorities []float32) error

	// Clear transitions based on criteria
	Clear(ctx context.Context, envID string, beforeTimestamp *time.Time, keepLastN uint32) (uint64, error)

	// Close the backend and cleanup resources
	Close() error
}