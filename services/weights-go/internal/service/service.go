package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

// ErrInvalidPublishInput is returned when Publish is invoked with bad data.
var ErrInvalidPublishInput = errors.New("weights service: invalid publish input")

// ErrRunNotFound indicates that no weights were recorded for the run.
var ErrRunNotFound = errors.New("weights service: run not found")

// Registry is the storage backend abstraction used by the service.
type Registry interface {
	Upsert(ctx context.Context, input PublishInput) (VersionSnapshot, error)
	Current(ctx context.Context, runID string) (VersionSnapshot, error)
	Subscribe(runID string, opts WatchOptions) (<-chan VersionSnapshot, func())
}

// Service coordinates registry persistence and fan-out.
type Service struct {
	registry Registry
	logger   *zerolog.Logger
}

// New constructs a Service.
func New(reg Registry, logger *zerolog.Logger) *Service {
	return &Service{registry: reg, logger: logger}
}

// Publish validates and persists a new weights version.
func (s *Service) Publish(ctx context.Context, input PublishInput) (VersionSnapshot, error) {
	if err := validatePublish(input); err != nil {
		s.logger.Warn().Err(err).Str("run_id", input.RunID).Msg("rejecting weight publish")
		return VersionSnapshot{}, err
	}
	if input.PublishedAt.IsZero() {
		input.PublishedAt = time.Now().UTC()
	}

	snapshot, err := s.registry.Upsert(ctx, input)
	if err != nil {
		s.logger.Error().Err(err).Str("run_id", input.RunID).Msg("failed to upsert weights")
		return VersionSnapshot{}, err
	}

	s.logger.Info().
		Str("run_id", snapshot.RunID).
		Int64("step", snapshot.Step).
		Str("checksum", snapshot.Checksum).
		Msg("weights published")

	return snapshot, nil
}

// GetCurrent returns the latest weights for the run.
func (s *Service) GetCurrent(ctx context.Context, runID string) (VersionSnapshot, error) {
	snapshot, err := s.registry.Current(ctx, runID)
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			s.logger.WithLevel(zerolog.DebugLevel).Str("run_id", runID).Msg("no weights found for run")
		} else {
			s.logger.Error().Err(err).Str("run_id", runID).Msg("failed to load weights")
		}
		return VersionSnapshot{}, err
	}
	return snapshot, nil
}

// Stream registers for future updates and returns a cancellation callback.
func (s *Service) Stream(runID string, opts WatchOptions) (<-chan VersionSnapshot, func()) {
	ch, cancel := s.registry.Subscribe(runID, opts)
	s.logger.WithLevel(zerolog.DebugLevel).Str("run_id", runID).Msg("weights stream subscribed")

	wrapped := make(chan VersionSnapshot)
	go func() {
		defer close(wrapped)
		for snapshot := range ch {
			select {
			case wrapped <- snapshot:
			default:
				s.logger.Warn().Str("run_id", runID).Msg("dropping weights update due to slow consumer")
			}
		}
	}()

	return wrapped, func() {
		cancel()
		s.logger.WithLevel(zerolog.DebugLevel).Str("run_id", runID).Msg("weights stream cancelled")
	}
}

func validatePublish(input PublishInput) error {
	if input.RunID == "" {
		return fmt.Errorf("%w: run id is required", ErrInvalidPublishInput)
	}
	if input.Step < 0 {
		return fmt.Errorf("%w: step must be >= 0", ErrInvalidPublishInput)
	}
	if input.Checksum == "" {
		return fmt.Errorf("%w: checksum is required", ErrInvalidPublishInput)
	}
	if input.ArtifactURI == "" {
		return fmt.Errorf("%w: artifact uri is required", ErrInvalidPublishInput)
	}
	return nil
}
