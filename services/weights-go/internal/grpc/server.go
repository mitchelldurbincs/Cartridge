package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	weightspb "github.com/cartridge/weights/internal/pb"
	"github.com/cartridge/weights/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service defines the subset of the weights service used by the transport layer.
type Service interface {
	Publish(ctx context.Context, input service.PublishInput) (service.VersionSnapshot, error)
	GetCurrent(ctx context.Context, runID string) (service.VersionSnapshot, error)
	Stream(runID string, opts service.WatchOptions) (<-chan service.VersionSnapshot, func())
}

// Server implements the generated gRPC interface.
type Server struct {
	weightspb.UnimplementedWeightsServiceServer
	svc Service
}

// New constructs a gRPC server backed by the provided service implementation.
func New(svc Service) *Server {
	return &Server{svc: svc}
}

// PublishWeights promotes a new version for the run.
func (s *Server) PublishWeights(ctx context.Context, req *weightspb.PublishWeightsRequest) (*weightspb.PublishWeightsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request body required")
	}
	snapshot, err := s.svc.Publish(ctx, toPublishInput(req))
	if err != nil {
		if errors.Is(err, service.ErrInvalidPublishInput) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to publish weights: %v", err)
	}
	return &weightspb.PublishWeightsResponse{Version: fromSnapshot(snapshot)}, nil
}

// StreamWeights subscribes the caller to future updates for the run.
func (s *Server) StreamWeights(req *weightspb.WatchRunRequest, stream weightspb.WeightsService_StreamWeightsServer) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "request body required")
	}
	if req.GetRunId() == "" {
		return status.Error(codes.InvalidArgument, "run_id is required")
	}

	updates, cancel := s.svc.Stream(req.GetRunId(), service.WatchOptions{ReplayLatest: req.GetReplayLatest()})
	defer cancel()

	for {
		select {
		case <-stream.Context().Done():
			return status.FromContextError(stream.Context().Err()).Err()
		case snapshot, ok := <-updates:
			if !ok {
				return nil
			}
			if err := stream.Send(fromSnapshot(snapshot)); err != nil {
				if errors.Is(err, context.Canceled) {
					return status.Error(codes.Canceled, "client cancelled stream")
				}
				return status.Errorf(codes.Unavailable, "failed to send update: %v", err)
			}
		}
	}
}

// GetCurrent returns the latest promoted version.
func (s *Server) GetCurrent(ctx context.Context, req *weightspb.GetCurrentRequest) (*weightspb.WeightsBlob, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request body required")
	}
	if req.GetRunId() == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}

	snapshot, err := s.svc.GetCurrent(ctx, req.GetRunId())
	if err != nil {
		if errors.Is(err, service.ErrRunNotFound) {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("run %s not found", req.GetRunId()))
		}
		return nil, status.Errorf(codes.Internal, "failed to fetch weights: %v", err)
	}
	return fromSnapshot(snapshot), nil
}

func toPublishInput(req *weightspb.PublishWeightsRequest) service.PublishInput {
	ts := req.GetPublishedAt()
	var publishedAtTime time.Time
	if ts != nil {
		publishedAtTime = ts.AsTime()
	}
	metadata := make(map[string]string, len(req.GetMetadata()))
	for k, v := range req.GetMetadata() {
		metadata[k] = v
	}
	return service.PublishInput{
		RunID:       req.GetRunId(),
		Step:        req.GetStep(),
		Checksum:    req.GetChecksum(),
		ArtifactURI: req.GetArtifactUri(),
		InlineBytes: append([]byte(nil), req.GetInlinePayload()...),
		Metadata:    metadata,
		PublishedAt: publishedAtTime,
	}
}

func fromSnapshot(snapshot service.VersionSnapshot) *weightspb.WeightsBlob {
	metadata := make(map[string]string, len(snapshot.Metadata))
	for k, v := range snapshot.Metadata {
		metadata[k] = v
	}
	var ts *timestamppb.Timestamp
	if !snapshot.PublishedAt.IsZero() {
		ts = timestamppb.New(snapshot.PublishedAt)
	}
	return &weightspb.WeightsBlob{
		RunId:         snapshot.RunID,
		Step:          snapshot.Step,
		Checksum:      snapshot.Checksum,
		ArtifactUri:   snapshot.ArtifactURI,
		InlinePayload: append([]byte(nil), snapshot.InlineBytes...),
		Metadata:      metadata,
		PublishedAt:   ts,
	}
}
