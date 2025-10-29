package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHeartbeatValidateRegression(t *testing.T) {
	h := HeartbeatPayload{
		RunID:             "run-1",
		Status:            RuntimeStatusRunning,
		Step:              9,
		SamplesPerSecond:  100.0,
		Loss:              0.5,
		CheckpointVersion: 2,
	}
	if err := h.Validate("run-1", 10, 1); err == nil {
		t.Fatalf("expected regression error, got nil")
	}
}

func TestRunCommandValidateTunePayload(t *testing.T) {
	cmd := RunCommand{
		ID:       "cmd-1",
		RunID:    "run-1",
		Type:     CommandTypeTune,
		Actor:    CommandActor{Type: CommandActorOperator, ID: "user@example.com"},
		IssuedAt: time.Now(),
	}
	payload := TunePayload{LearningRate: floatPtr(0.5)}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	cmd.Payload = data
	if err := cmd.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestRunCommandValidateTuneMissingPayload(t *testing.T) {
	cmd := RunCommand{
		ID:       "cmd-1",
		RunID:    "run-1",
		Type:     CommandTypeTune,
		Actor:    CommandActor{Type: CommandActorSystem, ID: "orchestrator"},
		IssuedAt: time.Now(),
		Payload:  json.RawMessage("{}"),
	}
	if err := cmd.Validate(); err == nil {
		t.Fatalf("expected error for empty tune payload")
	}
}

func TestRunMergeHeartbeatUpdatesDerivedFields(t *testing.T) {
	receivedAt := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)
	run := Run{State: RunStateQueued, UpdatedAt: time.Time{}}
	payload := HeartbeatPayload{
		RunID:             "run-1",
		Status:            RuntimeStatusRunning,
		Step:              10,
		SamplesPerSecond:  50.5,
		Loss:              0.42,
		CheckpointVersion: 3,
	}

	updated := run.MergeHeartbeat(payload, receivedAt)

	if updated.UpdatedAt != receivedAt {
		t.Fatalf("expected UpdatedAt to be %v, got %v", receivedAt, updated.UpdatedAt)
	}
	if updated.State != RunStateRunning {
		t.Fatalf("expected state to transition to running, got %s", updated.State)
	}
	if updated.HealthStatus != RunHealthHealthy {
		t.Fatalf("expected health to be marked healthy, got %s", updated.HealthStatus)
	}
	if updated.LastHeartbeatAt == nil || !updated.LastHeartbeatAt.Equal(receivedAt) {
		t.Fatalf("expected last heartbeat to be %v, got %v", receivedAt, updated.LastHeartbeatAt)
	}
}

func TestRunMergeHeartbeatPreservesTerminalState(t *testing.T) {
	receivedAt := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)
	run := Run{State: RunStateCompleted}
	payload := HeartbeatPayload{
		RunID:             "run-1",
		Status:            RuntimeStatusRunning,
		Step:              10,
		SamplesPerSecond:  50.5,
		Loss:              0.42,
		CheckpointVersion: 3,
	}

	updated := run.MergeHeartbeat(payload, receivedAt)

	if updated.State != RunStateCompleted {
		t.Fatalf("expected terminal state to be preserved, got %s", updated.State)
	}
}

func floatPtr(v float64) *float64 { return &v }
