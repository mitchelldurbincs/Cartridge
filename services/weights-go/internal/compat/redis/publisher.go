package rediscompat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/weights/internal/redisclient"
	"github.com/cartridge/weights/internal/service"
)

// Publisher mirrors published weights to Redis for backward compatibility.
type Publisher struct {
	client  *redisclient.Client
	channel string
	logger  *zerolog.Logger
}

// NewPublisher creates a Redis-backed compatibility publisher.
func NewPublisher(client *redisclient.Client, channel string, logger *zerolog.Logger) (*Publisher, error) {
	if client == nil {
		return nil, fmt.Errorf("rediscompat: client is required")
	}
	if channel == "" {
		return nil, fmt.Errorf("rediscompat: channel is required")
	}
	return &Publisher{client: client, channel: channel, logger: logger}, nil
}

type payload struct {
	RunID       string            `json:"run_id"`
	Step        int64             `json:"step"`
	Checksum    string            `json:"checksum"`
	ArtifactURI string            `json:"artifact_uri"`
	InlineBytes []byte            `json:"inline_payload,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	PublishedAt time.Time         `json:"published_at"`
}

// Publish serialises the snapshot and publishes it to the configured Redis channel.
func (p *Publisher) Publish(ctx context.Context, snapshot service.VersionSnapshot) error {
	message, err := json.Marshal(payload{
		RunID:       snapshot.RunID,
		Step:        snapshot.Step,
		Checksum:    snapshot.Checksum,
		ArtifactURI: snapshot.ArtifactURI,
		InlineBytes: snapshot.InlineBytes,
		Metadata:    snapshot.Metadata,
		PublishedAt: snapshot.PublishedAt,
	})
	if err != nil {
		return fmt.Errorf("rediscompat: failed to serialise snapshot: %w", err)
	}

	if _, err := p.client.Do(ctx, "PUBLISH", p.channel, string(message)); err != nil {
		if p.logger != nil {
			p.logger.Error().Err(err).Str("channel", p.channel).Msg("failed to publish weights to redis")
		}
		return err
	}
	if p.logger != nil {
		p.logger.WithLevel(zerolog.DebugLevel).Str("channel", p.channel).Str("run_id", snapshot.RunID).Msg("weights mirrored to redis")
	}
	return nil
}
