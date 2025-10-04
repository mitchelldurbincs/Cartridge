package events

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

// NATSPublisher implements Publisher using NATS
type NATSPublisher struct {
	conn    *nats.Conn
	subject string
	logger  zerolog.Logger
}

// NewNATSPublisher creates a new NATS-backed publisher
func NewNATSPublisher(natsURL, subject string, logger zerolog.Logger) (*NATSPublisher, error) {
	conn, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}

	return &NATSPublisher{
		conn:    conn,
		subject: subject,
		logger:  logger,
	}, nil
}

// Close closes the NATS connection
func (n *NATSPublisher) Close() {
	if n.conn != nil {
		n.conn.Close()
	}
}

// PublishRunStatus publishes run status events to NATS
func (n *NATSPublisher) PublishRunStatus(ctx context.Context, event RunStatusEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Publish to main subject
	if err := n.conn.Publish(n.subject, data); err != nil {
		n.logger.Error().Err(err).Str("subject", n.subject).Msg("Failed to publish run status")
		return err
	}

	// Publish to specific routing keys for alerting
	routingKey := ""
	switch event.HealthStatus {
	case "heartbeat_stale":
		routingKey = n.subject + ".heartbeat_stale"
	case "unresponsive":
		routingKey = n.subject + ".unresponsive"
	}

	if event.State == "errored" || event.State == "failed" {
		routingKey = n.subject + ".error"
	}

	if routingKey != "" {
		if err := n.conn.Publish(routingKey, data); err != nil {
			n.logger.Error().Err(err).Str("routing_key", routingKey).Msg("Failed to publish to routing key")
		}
	}

	n.logger.Debug().
		Str("run_id", event.RunID).
		Str("state", event.State).
		Str("subject", n.subject).
		Msg("Published run status event")

	return nil
}

// PublishCommandEvent publishes command events to NATS
func (n *NATSPublisher) PublishCommandEvent(ctx context.Context, event CommandEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	subject := n.subject + ".commands"
	if err := n.conn.Publish(subject, data); err != nil {
		n.logger.Error().Err(err).Str("subject", subject).Msg("Failed to publish command event")
		return err
	}

	n.logger.Debug().
		Str("run_id", event.RunID).
		Str("command_id", event.CommandID).
		Str("event", event.Event).
		Str("subject", subject).
		Msg("Published command event")

	return nil
}