package metrics

import (
	"time"

	"github.com/rs/zerolog"
)

// Metrics collector for orchestrator operations
type Collector struct {
	logger zerolog.Logger
}

func NewCollector(logger zerolog.Logger) *Collector {
	return &Collector{
		logger: logger,
	}
}

// Track heartbeat metrics
func (c *Collector) HeartbeatReceived(runID string, step int64, latency time.Duration) {
	c.logger.Info().
		Str("metric", "heartbeat_received").
		Str("run_id", runID).
		Int64("step", step).
		Dur("latency", latency).
		Msg("Heartbeat metric")
}

// Track API request metrics
func (c *Collector) APIRequest(method, endpoint string, statusCode int, duration time.Duration) {
	c.logger.Info().
		Str("metric", "api_request").
		Str("method", method).
		Str("endpoint", endpoint).
		Int("status_code", statusCode).
		Dur("duration", duration).
		Msg("API request metric")
}

// Track run state transitions
func (c *Collector) RunStateTransition(runID string, fromState, toState string) {
	c.logger.Info().
		Str("metric", "run_state_transition").
		Str("run_id", runID).
		Str("from_state", fromState).
		Str("to_state", toState).
		Msg("Run state transition metric")
}

// Track health monitoring events
func (c *Collector) HealthEvent(runID string, eventType string, severity string) {
	c.logger.Warn().
		Str("metric", "health_event").
		Str("run_id", runID).
		Str("event_type", eventType).
		Str("severity", severity).
		Msg("Health monitoring event")
}