package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/cartridge/orchestrator/internal/events"
	"github.com/cartridge/orchestrator/internal/storage"
	"github.com/cartridge/orchestrator/internal/types"
)

// CreateRunInput captures the payload required to create a run.
type CreateRunInput struct {
	ID             string          `json:"id"`
	ExperimentID   string          `json:"experiment_id"`
	VersionID      string          `json:"version_id"`
	LaunchManifest json.RawMessage `json:"launch_manifest"`
	Overrides      json.RawMessage `json:"overrides,omitempty"`
	Priority       int             `json:"priority"`
	CreatedBy      string          `json:"created_by"`
}

// Orchestrator implements the orchestrator workflows on top of storage.
type Orchestrator struct {
	store  storage.RunStore
	events events.Publisher
	logger *zerolog.Logger
	now    func() time.Time
}

// NewOrchestrator constructs an Orchestrator instance.
func NewOrchestrator(store storage.RunStore, publisher events.Publisher, logger *zerolog.Logger) *Orchestrator {
	return &Orchestrator{
		store:  store,
		events: publisher,
		logger: logger,
		now:    time.Now,
	}
}

// WithNow allows tests to override the time source.
func (o *Orchestrator) WithNow(now func() time.Time) {
	o.now = now
}

// CreateRun persists a new run and an initial transition entry.
func (o *Orchestrator) CreateRun(ctx context.Context, input CreateRunInput) (types.Run, error) {
	if input.ID == "" || input.ExperimentID == "" || input.VersionID == "" {
		return types.Run{}, errors.New("id, experiment_id, and version_id are required")
	}
	now := o.now()
	run := types.Run{
		ID:               input.ID,
		ExperimentID:     input.ExperimentID,
		VersionID:        input.VersionID,
		State:            types.RunStateQueued,
		LaunchManifest:   input.LaunchManifest,
		Overrides:        input.Overrides,
		Priority:         input.Priority,
		RuntimeStatus:    types.RuntimeStatusRunning,
		HealthStatus:     types.RunHealthHealthy,
		CurrentStep:      0,
		SamplesPerSecond: 0,
		Loss:             0,
		CreatedBy:        input.CreatedBy,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := o.store.CreateRun(ctx, run); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			o.logger.Warn().Str("run_id", input.ID).Msg("run already exists")
			return o.store.GetRun(ctx, input.ID)
		}
		return types.Run{}, err
	}
	transition := storage.RunTransition{
		RunID:     run.ID,
		FromState: "",
		ToState:   run.State,
		ChangedBy: input.CreatedBy,
		Reason:    "created",
		CreatedAt: now,
	}
	if err := o.store.AppendTransition(ctx, transition); err != nil {
		o.logger.Error().Err(err).Str("run_id", run.ID).Msg("failed to record transition")
	}
	return run, nil
}

// GetRun returns run metadata.
func (o *Orchestrator) GetRun(ctx context.Context, runID string) (types.Run, error) {
	return o.store.GetRun(ctx, runID)
}

// HandleHeartbeat processes a learner heartbeat and updates run state.
func (o *Orchestrator) HandleHeartbeat(ctx context.Context, runID string, payload types.HeartbeatPayload) (types.Run, error) {
	run, err := o.store.GetRun(ctx, runID)
	if err != nil {
		return types.Run{}, err
	}
	if err := payload.Validate(runID, run.CurrentStep, run.CheckpointVersion); err != nil {
		return types.Run{}, err
	}
	now := o.now()
	run = run.MergeHeartbeat(payload, now)
	run.HealthStatus = types.RunHealthHealthy
	run.UpdatedAt = now
	if err := o.store.UpdateRun(ctx, run); err != nil {
		return types.Run{}, err
	}
	event := events.RunStatusEvent{
		RunID:            run.ID,
		State:            string(run.State),
		RuntimeStatus:    string(run.RuntimeStatus),
		HealthStatus:     string(run.HealthStatus),
		Step:             run.CurrentStep,
		SamplesPerSecond: run.SamplesPerSecond,
		Loss:             run.Loss,
	}
	if err := o.events.PublishRunStatus(ctx, event); err != nil {
		o.logger.Error().Err(err).Str("run_id", run.ID).Msg("failed to publish run status event")
	}
	return run, nil
}

// CreateCommand validates and persists a control command.
func (o *Orchestrator) CreateCommand(ctx context.Context, command types.RunCommand) (types.RunCommand, error) {
	if _, err := o.store.GetRun(ctx, command.RunID); err != nil {
		return types.RunCommand{}, err
	}
	if err := command.Validate(); err != nil {
		return types.RunCommand{}, err
	}
	if err := o.store.AppendCommand(ctx, command); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			return o.store.GetCommand(ctx, command.RunID, command.ID)
		}
		return types.RunCommand{}, err
	}
	if err := o.events.PublishCommandEvent(ctx, events.CommandEvent{
		RunID:     command.RunID,
		CommandID: command.ID,
		Type:      string(command.Type),
		Event:     "queued",
	}); err != nil {
		o.logger.Error().Err(err).Str("run_id", command.RunID).Str("command_id", command.ID).Msg("failed to publish command event")
	}
	return command, nil
}

// NextCommand returns the oldest undelivered command and marks it delivered.
func (o *Orchestrator) NextCommand(ctx context.Context, runID string) (types.RunCommand, error) {
	cmd, err := o.store.NextPendingCommand(ctx, runID)
	if err != nil {
		return types.RunCommand{}, err
	}
	now := o.now()
	cmd.DeliveredAt = &now
	if err := o.store.SaveCommand(ctx, cmd); err != nil {
		return types.RunCommand{}, err
	}
	if err := o.events.PublishCommandEvent(ctx, events.CommandEvent{
		RunID:     cmd.RunID,
		CommandID: cmd.ID,
		Type:      string(cmd.Type),
		Event:     "delivered",
	}); err != nil {
		o.logger.Error().Err(err).Str("run_id", cmd.RunID).Str("command_id", cmd.ID).Msg("failed to publish delivery event")
	}
	return cmd, nil
}

// AckCommand marks a command as acknowledged by the learner.
func (o *Orchestrator) AckCommand(ctx context.Context, runID, commandID string) (types.RunCommand, error) {
	cmd, err := o.store.GetCommand(ctx, runID, commandID)
	if err != nil {
		return types.RunCommand{}, err
	}
	now := o.now()
	cmd.AcknowledgedAt = &now
	if err := o.store.SaveCommand(ctx, cmd); err != nil {
		return types.RunCommand{}, err
	}
	if err := o.events.PublishCommandEvent(ctx, events.CommandEvent{
		RunID:     cmd.RunID,
		CommandID: cmd.ID,
		Type:      string(cmd.Type),
		Event:     "acknowledged",
	}); err != nil {
		o.logger.Error().Err(err).Str("run_id", cmd.RunID).Str("command_id", cmd.ID).Msg("failed to publish ack event")
	}
	return cmd, nil
}
