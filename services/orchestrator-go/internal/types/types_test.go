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

func floatPtr(v float64) *float64 { return &v }

func TestMergeHeartbeatUpdatesAllFields(t *testing.T) {
	oldTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)

	run := Run{
		ID:                "run-1",
		CurrentStep:       5,
		CheckpointVersion: 2,
		RuntimeStatus:     RuntimeStatusRunning,
		HealthStatus:      RunHealthHeartbeatStale,
		UpdatedAt:         oldTime,
		LastHeartbeatAt:   &oldTime,
		SamplesPerSecond:  50.0,
		Loss:              0.5,
	}

	payload := HeartbeatPayload{
		RunID:             "run-1",
		Status:            RuntimeStatusPaused,
		Step:              10,
		SamplesPerSecond:  100.0,
		Loss:              0.3,
		CheckpointVersion: 3,
	}

	updated := run.MergeHeartbeat(payload, newTime)

	// Verify all heartbeat fields are updated
	if updated.CurrentStep != 10 {
		t.Errorf("expected step 10, got %d", updated.CurrentStep)
	}
	if updated.CheckpointVersion != 3 {
		t.Errorf("expected checkpoint 3, got %d", updated.CheckpointVersion)
	}
	if updated.RuntimeStatus != RuntimeStatusPaused {
		t.Errorf("expected status paused, got %s", updated.RuntimeStatus)
	}
	if updated.SamplesPerSecond != 100.0 {
		t.Errorf("expected samples/sec 100.0, got %f", updated.SamplesPerSecond)
	}
	if updated.Loss != 0.3 {
		t.Errorf("expected loss 0.3, got %f", updated.Loss)
	}

	// Verify UpdatedAt is updated to receivedAt time
	if !updated.UpdatedAt.Equal(newTime) {
		t.Errorf("expected UpdatedAt to be %v, got %v", newTime, updated.UpdatedAt)
	}

	// Verify LastHeartbeatAt is updated to receivedAt time
	if updated.LastHeartbeatAt == nil || !updated.LastHeartbeatAt.Equal(newTime) {
		t.Errorf("expected LastHeartbeatAt to be %v, got %v", newTime, updated.LastHeartbeatAt)
	}

	// Verify HealthStatus is set to healthy
	if updated.HealthStatus != RunHealthHealthy {
		t.Errorf("expected health status healthy, got %s", updated.HealthStatus)
	}
}
