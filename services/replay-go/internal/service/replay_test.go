package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cartridge/replay/internal/storage"
	replayv1 "github.com/cartridge/replay/pkg/proto/replay/v1"
)

type mockBackend struct {
	storeErr          error
	storeBatchIDs     []string
	storeBatchErr     error
	sampleTransitions []*storage.Transition
	sampleWeights     []float32
	sampleErr         error
	stats             *storage.Stats
	statsErr          error
	updateErr         error
	clearCount        uint64
	clearErr          error
}

func (m *mockBackend) Store(ctx context.Context, transition *storage.Transition) error {
	return m.storeErr
}

func (m *mockBackend) StoreBatch(ctx context.Context, transitions []*storage.Transition) ([]string, error) {
	return m.storeBatchIDs, m.storeBatchErr
}

func (m *mockBackend) Sample(ctx context.Context, config *storage.SampleConfig) ([]*storage.Transition, []float32, error) {
	return m.sampleTransitions, m.sampleWeights, m.sampleErr
}

func (m *mockBackend) GetStats(ctx context.Context, envID string) (*storage.Stats, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.stats, nil
}

func (m *mockBackend) UpdatePriorities(ctx context.Context, transitionIDs []string, priorities []float32) error {
	return m.updateErr
}

func (m *mockBackend) Clear(ctx context.Context, envID string, beforeTimestamp *time.Time, keepLastN uint32) (uint64, error) {
	return m.clearCount, m.clearErr
}

func (m *mockBackend) Close() error {
	return nil
}

func TestReplayService_SampleEmptyReplay(t *testing.T) {
	backend := &mockBackend{
		sampleErr: storage.ErrEmptyReplay,
	}

	service := NewReplayService(backend)

	resp, err := service.Sample(context.Background(), &replayv1.SampleRequest{Config: &replayv1.SampleConfig{BatchSize: 4}})
	require.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestReplayService_SampleSuccessIncludesStats(t *testing.T) {
	sampleTimestamp := time.Unix(1700000000, 0)
	backend := &mockBackend{
		sampleTransitions: []*storage.Transition{
			{
				ID:         "transition-1",
				EnvID:      "env-1",
				EpisodeID:  "episode-42",
				StepNumber: 3,
				Reward:     1.5,
				Done:       true,
				Timestamp:  sampleTimestamp,
			},
		},
		sampleWeights: []float32{0.7},
		stats: &storage.Stats{
			TotalTransitions: 9,
			TransitionsByEnv: map[string]uint64{
				"env-1": 5,
			},
		},
	}

	service := NewReplayService(backend)

	resp, err := service.Sample(context.Background(), &replayv1.SampleRequest{
		Config: &replayv1.SampleConfig{
			EnvId:     "env-1",
			BatchSize: 1,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Transitions, 1)
	require.Equal(t, "transition-1", resp.Transitions[0].Id)
	require.Equal(t, uint32(3), resp.Transitions[0].StepNumber)
	require.Equal(t, uint64(sampleTimestamp.Unix()), resp.Transitions[0].Timestamp)
	require.Equal(t, uint32(5), resp.TotalAvailable)
	require.Equal(t, backend.sampleWeights, resp.Weights)
}

func TestReplayService_UpdatePrioritiesLengthMismatch(t *testing.T) {
	service := NewReplayService(&mockBackend{})

	resp, err := service.UpdatePriorities(context.Background(), &replayv1.UpdatePrioritiesRequest{
		TransitionIds: []string{"a"},
		NewPriorities: []float32{},
	})

	require.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())
}

func TestReplayService_StoreTransitionRequiresTransition(t *testing.T) {
	service := NewReplayService(&mockBackend{})

	resp, err := service.StoreTransition(context.Background(), &replayv1.StoreTransitionRequest{})
	require.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())
}
