package health

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/orchestrator/internal/events"
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
	// This would require adding a method to the service to list runs
	// that need health checking. For now, we'll outline the logic:

	now := time.Now()
	staleThreshold := now.Add(-m.config.HeartbeatStaleAfter)
	unresponsiveThreshold := now.Add(-m.config.HeartbeatUnresponsive)

	// TODO: Add ListRunsForHealthCheck to service layer
	// runs, err := m.orch.ListRunsForHealthCheck(ctx, types.RunStateRunning)

	m.logger.Debug().
		Time("stale_threshold", staleThreshold).
		Time("unresponsive_threshold", unresponsiveThreshold).
		Msg("Checking run health")

	// Example logic for what this would do:
	// for _, run := range runs {
	//     if run.LastHeartbeatAt != nil {
	//         if run.LastHeartbeatAt.Before(unresponsiveThreshold) && run.HealthStatus != types.RunHealthUnresponsive {
	//             m.markUnresponsive(ctx, run)
	//         } else if run.LastHeartbeatAt.Before(staleThreshold) && run.HealthStatus == types.RunHealthHealthy {
	//             m.markStale(ctx, run)
	//         }
	//     }
	// }
}

func (m *Monitor) markStale(ctx context.Context, run types.Run) {
	m.logger.Warn().
		Str("run_id", run.ID).
		Time("last_heartbeat", *run.LastHeartbeatAt).
		Msg("Marking run as stale")

	// Update run health status
	run.HealthStatus = types.RunHealthHeartbeatStale
	// Would need UpdateRunHealth method in service

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
	m.logger.Error().
		Str("run_id", run.ID).
		Time("last_heartbeat", *run.LastHeartbeatAt).
		Msg("Marking run as unresponsive")

	// Update run health status
	run.HealthStatus = types.RunHealthUnresponsive
	// Would need UpdateRunHealth method in service

	// Publish unresponsive event (triggers PagerDuty)
	event := events.RunStatusEvent{
		RunID:         run.ID,
		State:         string(run.State),
		RuntimeStatus: string(run.RuntimeStatus),
		HealthStatus:  string(run.HealthStatus),
		Step:          run.CurrentStep,
		LastError:     "Run unresponsive - no heartbeat for over 2 minutes",
	}

	if err := m.publisher.PublishRunStatus(ctx, event); err != nil {
		m.logger.Error().Err(err).Str("run_id", run.ID).Msg("Failed to publish unresponsive event")
	}
}