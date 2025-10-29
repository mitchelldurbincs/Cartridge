package health

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/orchestrator/internal/events"
	"github.com/cartridge/orchestrator/internal/metrics"
	"github.com/cartridge/orchestrator/internal/service"
	"github.com/cartridge/orchestrator/internal/types"
)

// Config holds health monitoring configuration
type Config struct {
	CheckInterval         time.Duration
	HeartbeatStaleAfter   time.Duration
	HeartbeatUnresponsive time.Duration
}

// Monitor runs background health checks
type Monitor struct {
	orch      *service.Orchestrator
	publisher events.Publisher
	config    Config
	logger    zerolog.Logger
	metrics   *metrics.Collector
}

// NewMonitor creates a new health monitor
func NewMonitor(orch *service.Orchestrator, publisher events.Publisher, config Config, logger zerolog.Logger) *Monitor {
	return &Monitor{
		orch:      orch,
		publisher: publisher,
		config:    config,
		logger:    logger,
	}
}

// WithMetrics configures the monitor to emit Prometheus metrics.
func (m *Monitor) WithMetrics(collector *metrics.Collector) {
	m.metrics = collector
}

// Start begins the health monitoring loop
func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	m.logger.Info().
		Dur("check_interval", m.config.CheckInterval).
		Dur("stale_after", m.config.HeartbeatStaleAfter).
		Dur("unresponsive_after", m.config.HeartbeatUnresponsive).
		Msg("Starting health monitor")

	for {
		select {
		case <-ctx.Done():
			m.logger.Info().Msg("Health monitor stopped")
			return
		case <-ticker.C:
			m.checkStaleHeartbeats(ctx)
		}
	}
}

func (m *Monitor) checkStaleHeartbeats(ctx context.Context) {
	now := time.Now()
	staleThreshold := now.Add(-m.config.HeartbeatStaleAfter)
	unresponsiveThreshold := now.Add(-m.config.HeartbeatUnresponsive)

	runs, err := m.orch.ListRunsForHealthCheck(ctx, types.RunStateRunning)
	if err != nil {
		m.logger.Error().Err(err).Msg("Failed to list runs for health check")
		return
	}

	m.logger.Debug().
		Time("stale_threshold", staleThreshold).
		Time("unresponsive_threshold", unresponsiveThreshold).
		Msg("Checking run health")

	for _, run := range runs {
		if run.LastHeartbeatAt == nil {
			continue
		}

		if run.LastHeartbeatAt.Before(unresponsiveThreshold) && run.HealthStatus != types.RunHealthUnresponsive {
			m.markUnresponsive(ctx, run)
			continue
		}

		if run.LastHeartbeatAt.Before(staleThreshold) && run.HealthStatus == types.RunHealthHealthy {
			m.markStale(ctx, run)
		}
	}
}

func (m *Monitor) markStale(ctx context.Context, run types.Run) {
	logEvent := m.logger.Warn().Str("run_id", run.ID)
	if run.LastHeartbeatAt != nil {
		logEvent = logEvent.Time("last_heartbeat", *run.LastHeartbeatAt)
	}
	logEvent.Msg("Marking run as stale")

	updatedRun, err := m.orch.UpdateRunHealth(ctx, run.ID, types.RunHealthHeartbeatStale, "Heartbeat stale")
	if err != nil {
		m.logger.Error().Err(err).Str("run_id", run.ID).Msg("Failed to update run health to stale")
		return
	}

	run = updatedRun

	if m.metrics != nil {
		m.metrics.RecordHealthEvent("heartbeat_stale", "warning")
	}

	// Publish stale event
	event := events.RunStatusEvent{
		RunID:         run.ID,
		State:         string(run.State),
		RuntimeStatus: string(run.RuntimeStatus),
		HealthStatus:  string(run.HealthStatus),
		Step:          run.CurrentStep,
		LastError:     "Heartbeat stale",
	}

	if err := m.publisher.PublishRunStatus(ctx, event); err != nil {
		m.logger.Error().Err(err).Str("run_id", run.ID).Msg("Failed to publish stale event")
	}
}

func (m *Monitor) markUnresponsive(ctx context.Context, run types.Run) {
	logEvent := m.logger.Error().Str("run_id", run.ID)
	if run.LastHeartbeatAt != nil {
		logEvent = logEvent.Time("last_heartbeat", *run.LastHeartbeatAt)
	}
	logEvent.Msg("Marking run as unresponsive")

	reason := fmt.Sprintf("Run unresponsive - no heartbeat for over %s", m.config.HeartbeatUnresponsive)
	updatedRun, err := m.orch.UpdateRunHealth(ctx, run.ID, types.RunHealthUnresponsive, reason)
	if err != nil {
		m.logger.Error().Err(err).Str("run_id", run.ID).Msg("Failed to update run health to unresponsive")
		return
	}

	run = updatedRun

	if m.metrics != nil {
		m.metrics.RecordHealthEvent("heartbeat_unresponsive", "critical")
	}

	// Publish unresponsive event (triggers PagerDuty)
	event := events.RunStatusEvent{
		RunID:         run.ID,
		State:         string(run.State),
		RuntimeStatus: string(run.RuntimeStatus),
		HealthStatus:  string(run.HealthStatus),
		Step:          run.CurrentStep,
		LastError:     reason,
	}

	if err := m.publisher.PublishRunStatus(ctx, event); err != nil {
		m.logger.Error().Err(err).Str("run_id", run.ID).Msg("Failed to publish unresponsive event")
	}
}
