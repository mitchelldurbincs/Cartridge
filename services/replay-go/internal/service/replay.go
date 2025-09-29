package service

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cartridge/replay/internal/storage"
	replayv1 "github.com/cartridge/replay/pkg/proto/replay/v1"
)

// ReplayService implements the Replay gRPC service
type ReplayService struct {
	replayv1.UnimplementedReplayServer
	backend storage.Backend
}

// NewReplayService creates a new ReplayService
func NewReplayService(backend storage.Backend) *ReplayService {
	return &ReplayService{
		backend: backend,
	}
}

// StoreTransition stores a single transition
func (s *ReplayService) StoreTransition(ctx context.Context, req *replayv1.StoreTransitionRequest) (*replayv1.StoreTransitionResponse, error) {
	if req.Transition == nil {
		return nil, status.Error(codes.InvalidArgument, "transition is required")
	}

	// Convert proto transition to storage transition
	transition := protoToStorageTransition(req.Transition)

	// Store the transition
	if err := s.backend.Store(ctx, transition); err != nil {
		return &replayv1.StoreTransitionResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &replayv1.StoreTransitionResponse{
		TransitionId: transition.ID,
		Success:      true,
	}, nil
}

// StoreBatch stores multiple transitions in a batch
func (s *ReplayService) StoreBatch(ctx context.Context, req *replayv1.StoreBatchRequest) (*replayv1.StoreBatchResponse, error) {
	if len(req.Transitions) == 0 {
		return &replayv1.StoreBatchResponse{
			StoredCount: 0,
			FailedCount: 0,
		}, nil
	}

	// Convert proto transitions to storage transitions
	transitions := make([]*storage.Transition, len(req.Transitions))
	for i, protoTransition := range req.Transitions {
		transitions[i] = protoToStorageTransition(protoTransition)
	}

	// Store the batch
	ids, err := s.backend.StoreBatch(ctx, transitions)
	if err != nil {
		return &replayv1.StoreBatchResponse{
			StoredCount:    uint32(len(ids)),
			FailedCount:    uint32(len(req.Transitions) - len(ids)),
			ErrorMessages:  []string{err.Error()},
			TransitionIds:  ids,
		}, nil
	}

	return &replayv1.StoreBatchResponse{
		TransitionIds: ids,
		StoredCount:   uint32(len(ids)),
		FailedCount:   0,
	}, nil
}

// Sample samples transitions for training
func (s *ReplayService) Sample(ctx context.Context, req *replayv1.SampleRequest) (*replayv1.SampleResponse, error) {
	if req.Config == nil {
		return nil, status.Error(codes.InvalidArgument, "sample config is required")
	}

	// Convert proto config to storage config
	config := protoToStorageConfig(req.Config)

	// Sample transitions
	transitions, weights, err := s.backend.Sample(ctx, config)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert storage transitions to proto transitions
	protoTransitions := make([]*replayv1.Transition, len(transitions))
	for i, transition := range transitions {
		protoTransitions[i] = storageToProtoTransition(transition)
	}

	// Get total available count (approximation)
	stats, _ := s.backend.GetStats(ctx, config.EnvID)
	totalAvailable := uint32(0)
	if stats != nil {
		if config.EnvID != "" {
			if count, exists := stats.TransitionsByEnv[config.EnvID]; exists {
				totalAvailable = uint32(count)
			}
		} else {
			totalAvailable = uint32(stats.TotalTransitions)
		}
	}

	return &replayv1.SampleResponse{
		Transitions:    protoTransitions,
		TotalAvailable: totalAvailable,
		Weights:        weights,
	}, nil
}

// GetStats returns replay buffer statistics
func (s *ReplayService) GetStats(ctx context.Context, req *replayv1.GetStatsRequest) (*replayv1.StatsResponse, error) {
	stats, err := s.backend.GetStats(ctx, req.EnvId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	response := &replayv1.StatsResponse{
		TotalTransitions:  stats.TotalTransitions,
		TotalEpisodes:     stats.TotalEpisodes,
		TransitionsByEnv:  stats.TransitionsByEnv,
		StorageBytes:      stats.StorageBytes,
	}

	if stats.OldestTimestamp != nil {
		response.OldestTimestamp = uint64(stats.OldestTimestamp.Unix())
	}
	if stats.NewestTimestamp != nil {
		response.NewestTimestamp = uint64(stats.NewestTimestamp.Unix())
	}

	return response, nil
}

// UpdatePriorities updates transition priorities for prioritized replay
func (s *ReplayService) UpdatePriorities(ctx context.Context, req *replayv1.UpdatePrioritiesRequest) (*replayv1.UpdatePrioritiesResponse, error) {
	if len(req.TransitionIds) != len(req.NewPriorities) {
		return nil, status.Error(codes.InvalidArgument, "transition IDs and priorities must have same length")
	}

	err := s.backend.UpdatePriorities(ctx, req.TransitionIds, req.NewPriorities)
	if err != nil {
		return &replayv1.UpdatePrioritiesResponse{
			UpdatedCount:  0,
			ErrorMessages: []string{err.Error()},
		}, nil
	}

	return &replayv1.UpdatePrioritiesResponse{
		UpdatedCount: uint32(len(req.TransitionIds)),
	}, nil
}

// Clear clears transitions based on criteria
func (s *ReplayService) Clear(ctx context.Context, req *replayv1.ClearRequest) (*replayv1.ClearResponse, error) {
	var beforeTimestamp *time.Time
	if req.BeforeTimestamp > 0 {
		ts := time.Unix(int64(req.BeforeTimestamp), 0)
		beforeTimestamp = &ts
	}

	clearedCount, err := s.backend.Clear(ctx, req.EnvId, beforeTimestamp, req.KeepLastN)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get remaining count
	stats, _ := s.backend.GetStats(ctx, req.EnvId)
	remainingCount := uint64(0)
	if stats != nil {
		if req.EnvId != "" {
			if count, exists := stats.TransitionsByEnv[req.EnvId]; exists {
				remainingCount = count
			}
		} else {
			remainingCount = stats.TotalTransitions
		}
	}

	return &replayv1.ClearResponse{
		ClearedCount:   clearedCount,
		RemainingCount: remainingCount,
	}, nil
}

// Conversion functions

func protoToStorageTransition(proto *replayv1.Transition) *storage.Transition {
	transition := &storage.Transition{
		ID:              proto.Id,
		EnvID:           proto.EnvId,
		EpisodeID:       proto.EpisodeId,
		StepNumber:      proto.StepNumber,
		State:           proto.State,
		Action:          proto.Action,
		NextState:       proto.NextState,
		Observation:     proto.Observation,
		NextObservation: proto.NextObservation,
		Reward:          proto.Reward,
		Done:            proto.Done,
		Priority:        proto.Priority,
		Metadata:        proto.Metadata,
	}

	if proto.Timestamp > 0 {
		transition.Timestamp = time.Unix(int64(proto.Timestamp), 0)
	}

	return transition
}

func storageToProtoTransition(storage *storage.Transition) *replayv1.Transition {
	return &replayv1.Transition{
		Id:              storage.ID,
		EnvId:           storage.EnvID,
		EpisodeId:       storage.EpisodeID,
		StepNumber:      storage.StepNumber,
		State:           storage.State,
		Action:          storage.Action,
		NextState:       storage.NextState,
		Observation:     storage.Observation,
		NextObservation: storage.NextObservation,
		Reward:          storage.Reward,
		Done:            storage.Done,
		Priority:        storage.Priority,
		Timestamp:       uint64(storage.Timestamp.Unix()),
		Metadata:        storage.Metadata,
	}
}

func protoToStorageConfig(proto *replayv1.SampleConfig) *storage.SampleConfig {
	config := &storage.SampleConfig{
		BatchSize:     proto.BatchSize,
		EnvID:         proto.EnvId,
		Prioritized:   proto.Prioritized,
		PriorityAlpha: proto.PriorityAlpha,
	}

	if proto.MinTimestamp > 0 {
		ts := time.Unix(int64(proto.MinTimestamp), 0)
		config.MinTimestamp = &ts
	}
	if proto.MaxTimestamp > 0 {
		ts := time.Unix(int64(proto.MaxTimestamp), 0)
		config.MaxTimestamp = &ts
	}

	return config
}