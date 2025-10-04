package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RunState enumerates the canonical lifecycle states persisted in the registry.
type RunState string

const (
	RunStateQueued       RunState = "queued"
	RunStateProvisioning RunState = "provisioning"
	RunStateRunning      RunState = "running"
	RunStatePaused       RunState = "paused"
	RunStateTerminating  RunState = "terminating"
	RunStateCompleted    RunState = "completed"
	RunStateFailed       RunState = "failed"
	RunStateErrored      RunState = "errored"
	RunStateTerminated   RunState = "terminated"
)

// RuntimeStatus mirrors learner-reported state coming from heartbeats.
type RuntimeStatus string

const (
	RuntimeStatusRunning     RuntimeStatus = "running"
	RuntimeStatusPaused      RuntimeStatus = "paused"
	RuntimeStatusTerminating RuntimeStatus = "terminating"
	RuntimeStatusErrored     RuntimeStatus = "errored"
)

// RunHealth reflects orchestrator derived health.
type RunHealth string

const (
	RunHealthHealthy        RunHealth = "healthy"
	RunHealthHeartbeatStale RunHealth = "heartbeat_stale"
	RunHealthUnresponsive   RunHealth = "unresponsive"
)

// CommandType captures the control commands the orchestrator can deliver.
type CommandType string

const (
	CommandTypeTune      CommandType = "tune"
	CommandTypePause     CommandType = "pause"
	CommandTypeResume    CommandType = "resume"
	CommandTypeTerminate CommandType = "terminate"
)

// CommandActorType differentiates between human and automated initiators.
type CommandActorType string

const (
	CommandActorOperator CommandActorType = "operator"
	CommandActorSystem   CommandActorType = "system"
)

// CommandActor metadata.
type CommandActor struct {
	Type CommandActorType `json:"type"`
	ID   string           `json:"id"`
}

// TunePayload mirrors the documented schema for tune commands.
type TunePayload struct {
	LearningRate *float64 `json:"learning_rate,omitempty"`
	EntropyCoef  *float64 `json:"entropy_coef,omitempty"`
	ClipEpsilon  *float64 `json:"clip_epsilon,omitempty"`
	Notes        string   `json:"notes,omitempty"`
}

// TerminatePayload captures terminate command specific fields.
type TerminatePayload struct {
	Reason          string `json:"reason"`
	FinalCheckpoint bool   `json:"final_checkpoint,omitempty"`
}

// RunCommand is the canonical representation stored in the registry.
type RunCommand struct {
	ID             string          `json:"id"`
	RunID          string          `json:"run_id"`
	Type           CommandType     `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	Actor          CommandActor    `json:"actor"`
	IssuedAt       time.Time       `json:"issued_at"`
	DeliveredAt    *time.Time      `json:"delivered_at,omitempty"`
	AcknowledgedAt *time.Time      `json:"acknowledged_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// Run captures canonical run metadata.
type Run struct {
	ID                string          `json:"id"`
	ExperimentID      string          `json:"experiment_id"`
	VersionID         string          `json:"version_id"`
	State             RunState        `json:"state"`
	StatusMessage     string          `json:"status_message,omitempty"`
	Priority          int             `json:"priority"`
	LaunchManifest    json.RawMessage `json:"launch_manifest"`
	Overrides         json.RawMessage `json:"overrides,omitempty"`
	LastHeartbeatAt   *time.Time      `json:"last_heartbeat_at,omitempty"`
	RuntimeStatus     RuntimeStatus   `json:"runtime_status"`
	HealthStatus      RunHealth       `json:"health_status"`
	CurrentStep       int64           `json:"current_step"`
	SamplesPerSecond  float64         `json:"samples_per_sec"`
	Loss              float64         `json:"loss"`
	CheckpointVersion int64           `json:"checkpoint_version"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	EndedAt           *time.Time      `json:"ended_at,omitempty"`
	CreatedBy         string          `json:"created_by"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// HeartbeatPayload is the payload accepted by the heartbeat endpoint.
type HeartbeatPayload struct {
	RunID             string        `json:"run_id"`
	Status            RuntimeStatus `json:"status"`
	Step              int64         `json:"step"`
	SamplesPerSecond  float64       `json:"samples_per_sec"`
	Loss              float64       `json:"loss"`
	CheckpointVersion int64         `json:"checkpoint_version"`
	QueuedCommands    []string      `json:"queued_commands,omitempty"`
	Notes             string        `json:"notes,omitempty"`
}

// Validate ensures the payload respects schema invariants.
func (h HeartbeatPayload) Validate(expectedRunID string, currentStep, currentCheckpoint int64) error {
	if h.RunID == "" {
		return errors.New("run_id is required")
	}
	if expectedRunID != "" && h.RunID != expectedRunID {
		return fmt.Errorf("run_id mismatch: expected %s got %s", expectedRunID, h.RunID)
	}
	switch h.Status {
	case RuntimeStatusRunning, RuntimeStatusPaused, RuntimeStatusTerminating, RuntimeStatusErrored:
	default:
		return fmt.Errorf("invalid status %q", h.Status)
	}
	if h.Step < 0 {
		return errors.New("step must be non-negative")
	}
	if h.CheckpointVersion < 0 {
		return errors.New("checkpoint_version must be non-negative")
	}
	if currentStep > 0 && h.Step < currentStep {
		return fmt.Errorf("step regression: %d < %d", h.Step, currentStep)
	}
	if currentCheckpoint > 0 && h.CheckpointVersion < currentCheckpoint {
		return fmt.Errorf("checkpoint regression: %d < %d", h.CheckpointVersion, currentCheckpoint)
	}
	return nil
}

// Validate performs type-specific checks for run commands.
func (c RunCommand) Validate() error {
	switch c.Type {
	case CommandTypeTune:
		var payload TunePayload
		if err := json.Unmarshal(c.Payload, &payload); err != nil {
			return fmt.Errorf("invalid tune payload: %w", err)
		}
		if payload.LearningRate == nil && payload.EntropyCoef == nil && payload.ClipEpsilon == nil {
			return errors.New("tune payload requires at least one tunable field")
		}
		if payload.LearningRate != nil {
			if *payload.LearningRate <= 0 || *payload.LearningRate > 1 {
				return errors.New("learning_rate must be in (0,1]")
			}
		}
		if payload.EntropyCoef != nil {
			if *payload.EntropyCoef < 0 || *payload.EntropyCoef > 0.1 {
				return errors.New("entropy_coef must be within [0,0.1]")
			}
		}
		if payload.ClipEpsilon != nil {
			if *payload.ClipEpsilon < 0.05 || *payload.ClipEpsilon > 0.3 {
				return errors.New("clip_epsilon must be within [0.05,0.3]")
			}
		}
	case CommandTypePause, CommandTypeResume:
		if len(c.Payload) > 0 && string(c.Payload) != "{}" {
			return errors.New("pause/resume payload must be empty")
		}
	case CommandTypeTerminate:
		var payload TerminatePayload
		if err := json.Unmarshal(c.Payload, &payload); err != nil {
			return fmt.Errorf("invalid terminate payload: %w", err)
		}
		if payload.Reason == "" {
			return errors.New("terminate payload requires reason")
		}
	default:
		return fmt.Errorf("unsupported command type %q", c.Type)
	}
	switch c.Actor.Type {
	case CommandActorOperator, CommandActorSystem:
	default:
		return fmt.Errorf("invalid actor type %q", c.Actor.Type)
	}
	if c.Actor.ID == "" {
		return errors.New("actor.id is required")
	}
	if c.IssuedAt.IsZero() {
		return errors.New("issued_at is required")
	}
	return nil
}

// MergeHeartbeat applies the heartbeat values to a run and returns the updated copy.
func (r Run) MergeHeartbeat(h HeartbeatPayload, receivedAt time.Time) Run {
	r.LastHeartbeatAt = &receivedAt
	r.RuntimeStatus = h.Status
	r.CurrentStep = h.Step
	r.SamplesPerSecond = h.SamplesPerSecond
	r.Loss = h.Loss
	r.CheckpointVersion = h.CheckpointVersion
	return r
}
