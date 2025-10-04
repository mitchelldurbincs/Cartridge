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
