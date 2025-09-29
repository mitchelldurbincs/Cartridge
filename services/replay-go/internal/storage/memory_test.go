package storage

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryBackend_Store(t *testing.T) {
	backend := NewMemoryBackend(1000)
	defer backend.Close()

	ctx := context.Background()

	transition := &Transition{
		EnvID:     "tictactoe",
		EpisodeID: "episode-1",
		State:     []byte{1, 2, 3},
		Action:    []byte{4},
		Reward:    1.5,
		Done:      false,
		Priority:  1.0,
	}

	err := backend.Store(ctx, transition)
	require.NoError(t, err)
	assert.NotEmpty(t, transition.ID)
	assert.False(t, transition.Timestamp.IsZero())

	// Verify storage
	stats, err := backend.GetStats(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, uint64(1), stats.TotalTransitions)
	assert.Equal(t, uint64(1), stats.TransitionsByEnv["tictactoe"])
}

func TestMemoryBackend_StoreBatch(t *testing.T) {
	backend := NewMemoryBackend(1000)
	defer backend.Close()

	ctx := context.Background()

	transitions := []*Transition{
		{EnvID: "tictactoe", EpisodeID: "episode-1", State: []byte{1}, Action: []byte{1}, Reward: 1.0},
		{EnvID: "tictactoe", EpisodeID: "episode-1", State: []byte{2}, Action: []byte{2}, Reward: 2.0},
		{EnvID: "gridworld", EpisodeID: "episode-2", State: []byte{3}, Action: []byte{3}, Reward: 3.0},
	}

	ids, err := backend.StoreBatch(ctx, transitions)
	require.NoError(t, err)
	assert.Len(t, ids, 3)

	stats, err := backend.GetStats(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, uint64(3), stats.TotalTransitions)
	assert.Equal(t, uint64(2), stats.TransitionsByEnv["tictactoe"])
	assert.Equal(t, uint64(1), stats.TransitionsByEnv["gridworld"])
}

func TestMemoryBackend_Sample(t *testing.T) {
	backend := NewMemoryBackend(1000)
	defer backend.Close()

	backend.rng = rand.New(rand.NewSource(42))
	ctx := context.Background()

	// Store test data
	transitions := []*Transition{
		{EnvID: "tictactoe", State: []byte{1}, Action: []byte{1}, Reward: 1.0, Priority: 1.0},
		{EnvID: "tictactoe", State: []byte{2}, Action: []byte{2}, Reward: 2.0, Priority: 2.0},
		{EnvID: "gridworld", State: []byte{3}, Action: []byte{3}, Reward: 3.0, Priority: 1.0},
	}

	_, err := backend.StoreBatch(ctx, transitions)
	require.NoError(t, err)

	// Test uniform sampling
	config := &SampleConfig{
		BatchSize:   2,
		Prioritized: false,
	}

	sampled, weights, err := backend.Sample(ctx, config)
	require.NoError(t, err)
	assert.Len(t, sampled, 2)
	assert.Len(t, weights, 2)
	assert.Equal(t, float32(1.0), weights[0]) // Uniform weights

	// Test environment filtering
	config.EnvID = "tictactoe"
	sampled, _, err = backend.Sample(ctx, config)
	require.NoError(t, err)
	assert.Len(t, sampled, 2)
	for _, transition := range sampled {
		assert.Equal(t, "tictactoe", transition.EnvID)
	}

	// Test prioritized sampling
	config.Prioritized = true
	config.PriorityAlpha = 1.0
	config.EnvID = "" // Reset filter

	sampled, weights, err = backend.Sample(ctx, config)
	require.NoError(t, err)
	assert.Len(t, sampled, 2)
	assert.Len(t, weights, 2)
}

func TestMemoryBackend_PrioritizedSampleWeightsNonIntegerAlpha(t *testing.T) {
	backend := NewMemoryBackend(1000)
	defer backend.Close()

	backend.rng = rand.New(rand.NewSource(1))
	ctx := context.Background()

	transitions := []*Transition{
		{ID: "low", Priority: 0.2},
		{ID: "medium", Priority: 0.8},
		{ID: "high", Priority: 1.7},
	}

	_, err := backend.StoreBatch(ctx, transitions)
	require.NoError(t, err)

	config := &SampleConfig{
		BatchSize:     uint32(len(transitions)),
		Prioritized:   true,
		PriorityAlpha: 0.6,
	}

	sampled, weights, err := backend.Sample(ctx, config)
	require.NoError(t, err)
	require.Len(t, sampled, len(transitions))
	require.Len(t, weights, len(transitions))

	probabilities := computePrioritizedProbabilities(transitions, config.PriorityAlpha)
	expectedWeights := make(map[string]float32, len(transitions))
	for i, transition := range transitions {
		expectedWeights[transition.ID] = importanceWeight(probabilities[i], len(transitions))
	}

	for i, transition := range sampled {
		expected, ok := expectedWeights[transition.ID]
		require.True(t, ok)
		assert.InDelta(t, expected, weights[i], 1e-6)
	}
}

func TestMemoryBackend_PrioritizedSampleDistribution(t *testing.T) {
	backend := NewMemoryBackend(1000)
	defer backend.Close()

	backend.rng = rand.New(rand.NewSource(123))
	ctx := context.Background()

	transitions := []*Transition{
		{ID: "low", Priority: 0.1},
		{ID: "medium", Priority: 1.0},
		{ID: "high", Priority: 2.4},
	}

	_, err := backend.StoreBatch(ctx, transitions)
	require.NoError(t, err)

	config := &SampleConfig{
		BatchSize:     1,
		Prioritized:   true,
		PriorityAlpha: 0.6,
	}

	iterations := 2000
	counts := map[string]int{}

	for i := 0; i < iterations; i++ {
		sampled, _, err := backend.Sample(ctx, config)
		require.NoError(t, err)
		require.Len(t, sampled, 1)
		counts[sampled[0].ID]++
	}

	probabilities := computePrioritizedProbabilities(transitions, config.PriorityAlpha)
	tolerance := float64(iterations) * 0.05

	for i, transition := range transitions {
		expected := float64(iterations) * probabilities[i]
		actual := float64(counts[transition.ID])
		assert.InDeltaf(t, expected, actual, tolerance, "unexpected sampling frequency for %s", transition.ID)
	}
}

func TestMemoryBackend_UpdatePriorities(t *testing.T) {
	backend := NewMemoryBackend(1000)
	defer backend.Close()

	ctx := context.Background()

	transition := &Transition{
		EnvID:    "tictactoe",
		State:    []byte{1},
		Action:   []byte{1},
		Reward:   1.0,
		Priority: 1.0,
	}

	err := backend.Store(ctx, transition)
	require.NoError(t, err)

	// Update priority
	err = backend.UpdatePriorities(ctx, []string{transition.ID}, []float32{5.0})
	require.NoError(t, err)

	// Verify update
	backend.mu.RLock()
	stored := backend.transitions[transition.ID]
	backend.mu.RUnlock()

	assert.Equal(t, float32(5.0), stored.Priority)
}

func TestMemoryBackend_Clear(t *testing.T) {
	backend := NewMemoryBackend(1000)
	defer backend.Close()

	ctx := context.Background()

	now := time.Now()

	// Store transitions with different timestamps
	transitions := []*Transition{
		{EnvID: "tictactoe", State: []byte{1}, Timestamp: now.Add(-1 * time.Hour)},
		{EnvID: "tictactoe", State: []byte{2}, Timestamp: now.Add(-30 * time.Minute)},
		{EnvID: "gridworld", State: []byte{3}, Timestamp: now.Add(-10 * time.Minute)},
	}

	_, err := backend.StoreBatch(ctx, transitions)
	require.NoError(t, err)

	// Clear old transitions
	cutoff := now.Add(-45 * time.Minute)
	clearedCount, err := backend.Clear(ctx, "", &cutoff, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), clearedCount) // Should clear the oldest one

	stats, err := backend.GetStats(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, uint64(2), stats.TotalTransitions)
}

func TestMemoryBackend_MaxSize(t *testing.T) {
	backend := NewMemoryBackend(2) // Max 2 transitions
	defer backend.Close()

	ctx := context.Background()

	// Store 3 transitions
	transitions := []*Transition{
		{EnvID: "test", State: []byte{1}, Timestamp: time.Now()},
		{EnvID: "test", State: []byte{2}, Timestamp: time.Now().Add(1 * time.Minute)},
		{EnvID: "test", State: []byte{3}, Timestamp: time.Now().Add(2 * time.Minute)},
	}

	for _, transition := range transitions {
		err := backend.Store(ctx, transition)
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	stats, err := backend.GetStats(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, uint64(2), stats.TotalTransitions) // Should evict oldest
}

func TestMemoryBackend_TimeFiltering(t *testing.T) {
	backend := NewMemoryBackend(1000)
	defer backend.Close()

	ctx := context.Background()

	now := time.Now()

	transitions := []*Transition{
		{EnvID: "test", State: []byte{1}, Timestamp: now.Add(-2 * time.Hour)},
		{EnvID: "test", State: []byte{2}, Timestamp: now.Add(-1 * time.Hour)},
		{EnvID: "test", State: []byte{3}, Timestamp: now},
	}

	_, err := backend.StoreBatch(ctx, transitions)
	require.NoError(t, err)

	// Sample with time filtering
	minTime := now.Add(-90 * time.Minute)
	maxTime := now.Add(-30 * time.Minute)

	config := &SampleConfig{
		BatchSize:    10,
		MinTimestamp: &minTime,
		MaxTimestamp: &maxTime,
	}

	sampled, _, err := backend.Sample(ctx, config)
	require.NoError(t, err)
	assert.Len(t, sampled, 1) // Only middle transition should match
	assert.Equal(t, []byte{2}, sampled[0].State)
}
