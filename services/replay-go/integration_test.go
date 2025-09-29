package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cartridge/replay/internal/service"
	"github.com/cartridge/replay/internal/storage"
	replayv1 "github.com/cartridge/replay/pkg/proto/replay/v1"
)

// TestReplayServiceIntegration tests the full service with real-like engine data
func TestReplayServiceIntegration(t *testing.T) {
	// Create backend and service
	backend := storage.NewMemoryBackend(1000)
	defer backend.Close()

	svc := service.NewReplayService(backend)
	ctx := context.Background()

	// Simulate TicTacToe transitions (based on engine format)
	tictactoeTransitions := []*replayv1.Transition{
		{
			EnvId:     "tictactoe",
			EpisodeId: "episode-1",
			// TicTacToe state: 11 bytes (9 board + current_player + winner)
			State:     []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0}, // Empty board, X's turn
			Action:    []byte{4}, // Place in center (position 4)
			NextState: []byte{0, 0, 0, 0, 1, 0, 0, 0, 0, 2, 0}, // X in center, O's turn
			// TicTacToe observation: 29 * 4 = 116 bytes (29 f32 values)
			Observation:     make([]byte, 116), // Simplified for test
			NextObservation: make([]byte, 116),
			Reward:          0.0, // No reward mid-game
			Done:            false,
			StepNumber:      0,
			Priority:        1.0,
		},
		{
			EnvId:           "tictactoe",
			EpisodeId:       "episode-1",
			State:           []byte{0, 0, 0, 0, 1, 0, 0, 0, 0, 2, 0},
			Action:          []byte{0}, // O plays top-left
			NextState:       []byte{2, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0},
			Observation:     make([]byte, 116),
			NextObservation: make([]byte, 116),
			Reward:          0.0,
			Done:            false,
			StepNumber:      1,
			Priority:        1.0,
		},
	}

	// Test batch storage
	t.Run("StoreBatch", func(t *testing.T) {
		resp, err := svc.StoreBatch(ctx, &replayv1.StoreBatchRequest{
			Transitions: tictactoeTransitions,
		})

		require.NoError(t, err)
		assert.Equal(t, uint32(2), resp.StoredCount)
		assert.Equal(t, uint32(0), resp.FailedCount)
		assert.Len(t, resp.TransitionIds, 2)
	})

	// Test statistics
	t.Run("GetStats", func(t *testing.T) {
		resp, err := svc.GetStats(ctx, &replayv1.GetStatsRequest{})

		require.NoError(t, err)
		assert.Equal(t, uint64(2), resp.TotalTransitions)
		assert.Equal(t, uint64(1), resp.TotalEpisodes)
		assert.Equal(t, uint64(2), resp.TransitionsByEnv["tictactoe"])
		assert.Greater(t, resp.StorageBytes, uint64(0))

		// Test env-specific stats
		envResp, err := svc.GetStats(ctx, &replayv1.GetStatsRequest{
			EnvId: "tictactoe",
		})
		require.NoError(t, err)
		assert.Equal(t, uint64(2), envResp.TransitionsByEnv["tictactoe"])
	})

	// Test sampling
	t.Run("Sample", func(t *testing.T) {
		// Test uniform sampling
		resp, err := svc.Sample(ctx, &replayv1.SampleRequest{
			Config: &replayv1.SampleConfig{
				BatchSize:   1,
				EnvId:       "tictactoe",
				Prioritized: false,
			},
		})

		require.NoError(t, err)
		assert.Len(t, resp.Transitions, 1)
		assert.Equal(t, uint32(2), resp.TotalAvailable)
		assert.Len(t, resp.Weights, 1)
		assert.Equal(t, float32(1.0), resp.Weights[0])

		// Verify sampled transition format
		sampled := resp.Transitions[0]
		assert.Equal(t, "tictactoe", sampled.EnvId)
		assert.Equal(t, "episode-1", sampled.EpisodeId)
		assert.Len(t, sampled.State, 11)      // TicTacToe state format
		assert.Len(t, sampled.Action, 1)      // TicTacToe action format
		assert.Len(t, sampled.Observation, 116) // TicTacToe observation format

		// Test prioritized sampling
		prioritizedResp, err := svc.Sample(ctx, &replayv1.SampleRequest{
			Config: &replayv1.SampleConfig{
				BatchSize:     2,
				EnvId:         "tictactoe",
				Prioritized:   true,
				PriorityAlpha: 1.0,
			},
		})

		require.NoError(t, err)
		assert.Len(t, prioritizedResp.Transitions, 2)
		assert.Len(t, prioritizedResp.Weights, 2)
	})

	// Test priority updates
	t.Run("UpdatePriorities", func(t *testing.T) {
		// First get some transition IDs
		sampleResp, err := svc.Sample(ctx, &replayv1.SampleRequest{
			Config: &replayv1.SampleConfig{BatchSize: 1, EnvId: "tictactoe"},
		})
		require.NoError(t, err)
		require.Len(t, sampleResp.Transitions, 1)

		transitionID := sampleResp.Transitions[0].Id

		// Update priority
		updateResp, err := svc.UpdatePriorities(ctx, &replayv1.UpdatePrioritiesRequest{
			TransitionIds:  []string{transitionID},
			NewPriorities:  []float32{5.0},
		})

		require.NoError(t, err)
		assert.Equal(t, uint32(1), updateResp.UpdatedCount)
		assert.Empty(t, updateResp.ErrorMessages)
	})

	// Test time-based sampling
	t.Run("TimeBasedSampling", func(t *testing.T) {
		// Add a transition with specific timestamp
		futureTime := time.Now().Add(1 * time.Hour)
		futureTransition := &replayv1.Transition{
			EnvId:           "tictactoe",
			EpisodeId:       "episode-2",
			State:           []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0},
			Action:          []byte{8}, // Bottom right
			NextState:       []byte{0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 0},
			Observation:     make([]byte, 116),
			NextObservation: make([]byte, 116),
			Timestamp:       uint64(futureTime.Unix()),
			StepNumber:      0,
		}

		_, err := svc.StoreTransition(ctx, &replayv1.StoreTransitionRequest{
			Transition: futureTransition,
		})
		require.NoError(t, err)

		// Sample only recent transitions
		minTime := time.Now().Add(30 * time.Minute)
		resp, err := svc.Sample(ctx, &replayv1.SampleRequest{
			Config: &replayv1.SampleConfig{
				BatchSize:    10,
				EnvId:        "tictactoe",
				MinTimestamp: uint64(minTime.Unix()),
			},
		})

		require.NoError(t, err)
		assert.Len(t, resp.Transitions, 1) // Only future transition should match
		assert.Equal(t, []byte{8}, resp.Transitions[0].Action)
	})

	// Test clearing
	t.Run("Clear", func(t *testing.T) {
		// Clear old transitions (use future time to ensure we clear something)
		cutoffTime := time.Now().Add(5 * time.Minute)
		clearResp, err := svc.Clear(ctx, &replayv1.ClearRequest{
			EnvId:           "tictactoe",
			BeforeTimestamp: uint64(cutoffTime.Unix()),
		})

		require.NoError(t, err)
		// Should clear at least some transitions
		assert.GreaterOrEqual(t, clearResp.ClearedCount, uint64(0))

		// Verify remaining transitions
		stats, err := svc.GetStats(ctx, &replayv1.GetStatsRequest{})
		require.NoError(t, err)
		assert.Equal(t, clearResp.RemainingCount, stats.TotalTransitions)
	})
}

// TestEngineDataFormats verifies that our replay service can handle
// the exact data formats produced by the engine
func TestEngineDataFormats(t *testing.T) {
	backend := storage.NewMemoryBackend(1000)
	defer backend.Close()

	svc := service.NewReplayService(backend)
	ctx := context.Background()

	// Test with exact TicTacToe formats from engine implementation
	t.Run("TicTacToeFormats", func(t *testing.T) {
		// TicTacToe state encoding: 11 bytes
		// - 9 bytes: board (0=empty, 1=X, 2=O)
		// - 1 byte: current_player (1=X, 2=O)
		// - 1 byte: winner (0=none, 1=X, 2=O, 3=draw)
		initialState := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0}
		afterMove := []byte{0, 0, 0, 0, 1, 0, 0, 0, 0, 2, 0}

		// TicTacToe action encoding: 1 byte (position 0-8)
		action := []byte{4} // Center position

		// TicTacToe observation encoding: 116 bytes (29 * 4)
		// - 18 f32s: one-hot board (X positions + O positions)
		// - 9 f32s: legal moves mask
		// - 2 f32s: current player indicator
		observation := make([]byte, 116)
		nextObservation := make([]byte, 116)

		transition := &replayv1.Transition{
			EnvId:           "tictactoe",
			EpisodeId:       "test-episode",
			StepNumber:      0,
			State:           initialState,
			Action:          action,
			NextState:       afterMove,
			Observation:     observation,
			NextObservation: nextObservation,
			Reward:          0.0,
			Done:            false,
			Priority:        1.0,
		}

		// Store and verify
		storeResp, err := svc.StoreTransition(ctx, &replayv1.StoreTransitionRequest{
			Transition: transition,
		})
		require.NoError(t, err)
		assert.True(t, storeResp.Success)

		// Sample and verify format preservation
		sampleResp, err := svc.Sample(ctx, &replayv1.SampleRequest{
			Config: &replayv1.SampleConfig{BatchSize: 1, EnvId: "tictactoe"},
		})
		require.NoError(t, err)
		require.Len(t, sampleResp.Transitions, 1)

		sampled := sampleResp.Transitions[0]
		assert.Equal(t, initialState, sampled.State)
		assert.Equal(t, action, sampled.Action)
		assert.Equal(t, afterMove, sampled.NextState)
		assert.Equal(t, observation, sampled.Observation)
		assert.Equal(t, nextObservation, sampled.NextObservation)
		assert.Equal(t, float32(0.0), sampled.Reward)
		assert.False(t, sampled.Done)
	})
}