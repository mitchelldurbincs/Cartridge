package service

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/cartridge/orchestrator/internal/events"
	"github.com/cartridge/orchestrator/internal/metrics"
	"github.com/cartridge/orchestrator/internal/storage"
	"github.com/cartridge/orchestrator/internal/types"
)

type stubRunStore struct {
	createRunFn          func(context.Context, types.Run) error
	getRunFn             func(context.Context, string) (types.Run, error)
	updateRunFn          func(context.Context, types.Run) error
	listRunsFn           func(context.Context) ([]types.Run, error)
	appendTransitionFn   func(context.Context, storage.RunTransition) error
	appendCommandFn      func(context.Context, types.RunCommand) error
	getCommandFn         func(context.Context, string, string) (types.RunCommand, error)
	nextPendingCommandFn func(context.Context, string) (types.RunCommand, error)
	saveCommandFn        func(context.Context, types.RunCommand) error
}

func (s *stubRunStore) CreateRun(ctx context.Context, run types.Run) error {
	if s.createRunFn != nil {
		return s.createRunFn(ctx, run)
	}
	return nil
}

func (s *stubRunStore) GetRun(ctx context.Context, id string) (types.Run, error) {
	if s.getRunFn != nil {
		return s.getRunFn(ctx, id)
	}
	return types.Run{}, nil
}

func (s *stubRunStore) UpdateRun(ctx context.Context, run types.Run) error {
	if s.updateRunFn != nil {
		return s.updateRunFn(ctx, run)
	}
	return nil
}

func (s *stubRunStore) ListRuns(ctx context.Context) ([]types.Run, error) {
	if s.listRunsFn != nil {
		return s.listRunsFn(ctx)
	}
	return nil, nil
}

func (s *stubRunStore) AppendTransition(ctx context.Context, transition storage.RunTransition) error {
	if s.appendTransitionFn != nil {
		return s.appendTransitionFn(ctx, transition)
	}
	return nil
}

func (s *stubRunStore) AppendCommand(ctx context.Context, command types.RunCommand) error {
	if s.appendCommandFn != nil {
		return s.appendCommandFn(ctx, command)
	}
	return nil
}

func (s *stubRunStore) GetCommand(ctx context.Context, runID, commandID string) (types.RunCommand, error) {
	if s.getCommandFn != nil {
		return s.getCommandFn(ctx, runID, commandID)
	}
	return types.RunCommand{}, nil
}

func (s *stubRunStore) NextPendingCommand(ctx context.Context, runID string) (types.RunCommand, error) {
	if s.nextPendingCommandFn != nil {
		return s.nextPendingCommandFn(ctx, runID)
	}
	return types.RunCommand{}, storage.ErrNoCommands
}

func (s *stubRunStore) SaveCommand(ctx context.Context, command types.RunCommand) error {
	if s.saveCommandFn != nil {
		return s.saveCommandFn(ctx, command)
	}
	return nil
}

type stubPublisher struct {
	runStatusEvents []events.RunStatusEvent
	commandEvents   []events.CommandEvent
	runStatusErr    error
	commandEventErr error
}

func (s *stubPublisher) PublishRunStatus(ctx context.Context, payload events.RunStatusEvent) error {
	if s.runStatusErr != nil {
		return s.runStatusErr
	}
	s.runStatusEvents = append(s.runStatusEvents, payload)
	return nil
}

func (s *stubPublisher) PublishCommandEvent(ctx context.Context, payload events.CommandEvent) error {
	if s.commandEventErr != nil {
		return s.commandEventErr
	}
	s.commandEvents = append(s.commandEvents, payload)
	return nil
}

func newTestLogger() *zerolog.Logger {
	return zerolog.New(io.Discard)
}

func TestCreateRunPersistsRunAndRecordsMetrics(t *testing.T) {
	fixed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var createdRun types.Run
	var recordedTransition storage.RunTransition
	reg := prometheus.NewRegistry()
	collector := metrics.NewCollector(reg)

	store := &stubRunStore{
		createRunFn: func(ctx context.Context, run types.Run) error {
			createdRun = run
			return nil
		},
		appendTransitionFn: func(ctx context.Context, transition storage.RunTransition) error {
			recordedTransition = transition
			return nil
		},
	}

	orch := NewOrchestrator(store, &stubPublisher{}, newTestLogger())
	orch.WithMetrics(collector)
	orch.WithNow(func() time.Time { return fixed })

	manifest := json.RawMessage(`{"image":"trainer:latest"}`)
	overrides := json.RawMessage(`{"lr":0.1}`)
	input := CreateRunInput{
		ID:             "run-123",
		ExperimentID:   "exp-456",
		VersionID:      "ver-1",
		LaunchManifest: manifest,
		Overrides:      overrides,
		Priority:       10,
		CreatedBy:      "tester",
	}

	run, err := orch.CreateRun(context.Background(), input)
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}

	if run.ID != input.ID {
		t.Fatalf("expected run ID %q, got %q", input.ID, run.ID)
	}
	if run.CreatedAt != fixed || run.UpdatedAt != fixed {
		t.Fatalf("expected timestamps to be %v, got created %v updated %v", fixed, run.CreatedAt, run.UpdatedAt)
	}

	if recordedTransition.RunID != input.ID || recordedTransition.ToState != types.RunStateQueued {
		t.Fatalf("unexpected transition recorded: %#v", recordedTransition)
	}

	if createdRun.State != types.RunStateQueued {
		t.Fatalf("stored run should be queued, got %s", createdRun.State)
	}

	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	var transitionCount float64
	for _, mf := range metricFamilies {
		if mf.Name != "orchestrator_run_state_transitions_total" {
			continue
		}
		for _, metric := range mf.Metrics {
			transitionCount += metric.Value
		}
	}
	if transitionCount != 1 {
		t.Fatalf("expected run state transition count 1, got %f", transitionCount)
	}
}

func TestCreateRunReturnsExistingOnConflict(t *testing.T) {
	existing := types.Run{ID: "run-1"}
	appendCalled := false
	store := &stubRunStore{
		createRunFn: func(ctx context.Context, run types.Run) error {
			return storage.ErrConflict
		},
		getRunFn: func(ctx context.Context, id string) (types.Run, error) {
			return existing, nil
		},
		appendTransitionFn: func(ctx context.Context, transition storage.RunTransition) error {
			appendCalled = true
			return nil
		},
	}

	orch := NewOrchestrator(store, &stubPublisher{}, newTestLogger())
	_, err := orch.CreateRun(context.Background(), CreateRunInput{
		ID:           existing.ID,
		ExperimentID: "exp",
		VersionID:    "ver",
		CreatedBy:    "tester",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if appendCalled {
		t.Fatalf("expected no transition to be appended on conflict")
	}
}

func TestHandleHeartbeatUpdatesRunAndPublishesEvent(t *testing.T) {
	prev := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)
	now := prev.Add(10 * time.Second)
	storedRun := types.Run{
		ID:                "run-1",
		CurrentStep:       5,
		CheckpointVersion: 2,
		LastHeartbeatAt:   &prev,
		RuntimeStatus:     types.RuntimeStatusRunning,
		HealthStatus:      types.RunHealthHealthy,
	}
	var updatedRun types.Run
	publisher := &stubPublisher{}
	reg := prometheus.NewRegistry()
	collector := metrics.NewCollector(reg)

	store := &stubRunStore{
		getRunFn: func(ctx context.Context, id string) (types.Run, error) {
			return storedRun, nil
		},
		updateRunFn: func(ctx context.Context, run types.Run) error {
			updatedRun = run
			return nil
		},
	}

	orch := NewOrchestrator(store, publisher, newTestLogger())
	orch.WithMetrics(collector)
	orch.WithNow(func() time.Time { return now })

	payload := types.HeartbeatPayload{
		RunID:             storedRun.ID,
		Status:            types.RuntimeStatusRunning,
		Step:              6,
		SamplesPerSecond:  123.4,
		Loss:              0.75,
		CheckpointVersion: 3,
	}

	run, err := orch.HandleHeartbeat(context.Background(), storedRun.ID, payload)
	if err != nil {
		t.Fatalf("HandleHeartbeat returned error: %v", err)
	}

	if updatedRun.LastHeartbeatAt == nil || !updatedRun.LastHeartbeatAt.Equal(now) {
		t.Fatalf("expected last heartbeat at %v, got %v", now, updatedRun.LastHeartbeatAt)
	}
	if updatedRun.CurrentStep != payload.Step || updatedRun.CheckpointVersion != payload.CheckpointVersion {
		t.Fatalf("run was not updated with heartbeat payload: %#v", updatedRun)
	}
	if run.HealthStatus != types.RunHealthHealthy {
		t.Fatalf("expected run health to be healthy, got %s", run.HealthStatus)
	}
	if len(publisher.runStatusEvents) != 1 {
		t.Fatalf("expected run status event to be published")
	}
	event := publisher.runStatusEvents[0]
	if event.RunID != storedRun.ID || event.Step != payload.Step {
		t.Fatalf("unexpected event payload: %#v", event)
	}

	metricFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	var heartbeatCount uint64
	for _, mf := range metricFamilies {
		if mf.Name != "orchestrator_heartbeat_latency_seconds" {
			continue
		}
		for _, metric := range mf.Metrics {
			heartbeatCount += metric.Count
		}
	}
	if heartbeatCount == 0 {
		t.Fatalf("expected heartbeat latency metric to record an observation")
	}
}

func TestCreateCommandPublishesQueuedEvent(t *testing.T) {
	fixed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var appended types.RunCommand
	store := &stubRunStore{
		getRunFn: func(ctx context.Context, id string) (types.Run, error) {
			return types.Run{ID: id}, nil
		},
		appendCommandFn: func(ctx context.Context, command types.RunCommand) error {
			appended = command
			return nil
		},
	}
	publisher := &stubPublisher{}
	orch := NewOrchestrator(store, publisher, newTestLogger())

	command := types.RunCommand{
		ID:        "cmd-1",
		RunID:     "run-1",
		Type:      types.CommandTypePause,
		Payload:   json.RawMessage(`{}`),
		Actor:     types.CommandActor{Type: types.CommandActorOperator, ID: "op"},
		IssuedAt:  fixed,
		CreatedAt: fixed,
	}

	_, err := orch.CreateCommand(context.Background(), command)
	if err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	if appended.ID != command.ID {
		t.Fatalf("expected command to be appended")
	}
	if len(publisher.commandEvents) != 1 {
		t.Fatalf("expected command event to be published")
	}
	event := publisher.commandEvents[0]
	if event.Event != "queued" || event.CommandID != command.ID {
		t.Fatalf("unexpected command event: %#v", event)
	}
}

func TestCreateCommandReturnsExistingOnConflict(t *testing.T) {
	existing := types.RunCommand{ID: "cmd-1", RunID: "run-1"}
	store := &stubRunStore{
		getRunFn: func(ctx context.Context, id string) (types.Run, error) {
			return types.Run{ID: id}, nil
		},
		appendCommandFn: func(ctx context.Context, command types.RunCommand) error {
			return storage.ErrConflict
		},
		getCommandFn: func(ctx context.Context, runID, commandID string) (types.RunCommand, error) {
			return existing, nil
		},
	}
	publisher := &stubPublisher{}
	orch := NewOrchestrator(store, publisher, newTestLogger())

	command := types.RunCommand{
		ID:        existing.ID,
		RunID:     existing.RunID,
		Type:      types.CommandTypePause,
		Payload:   json.RawMessage(`{}`),
		Actor:     types.CommandActor{Type: types.CommandActorOperator, ID: "op"},
		IssuedAt:  time.Now(),
		CreatedAt: time.Now(),
	}

	result, err := orch.CreateCommand(context.Background(), command)
	if err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	if result.ID != existing.ID {
		t.Fatalf("expected existing command to be returned")
	}
	if len(publisher.commandEvents) != 0 {
		t.Fatalf("expected no command event on conflict")
	}
}

func TestNextCommandMarksDeliveryAndPublishesEvent(t *testing.T) {
	issuedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now := issuedAt.Add(time.Minute)
	pending := types.RunCommand{ID: "cmd-1", RunID: "run-1", Type: types.CommandTypePause, IssuedAt: issuedAt, CreatedAt: issuedAt}
	var saved types.RunCommand
	store := &stubRunStore{
		nextPendingCommandFn: func(ctx context.Context, runID string) (types.RunCommand, error) {
			return pending, nil
		},
		saveCommandFn: func(ctx context.Context, command types.RunCommand) error {
			saved = command
			return nil
		},
	}
	publisher := &stubPublisher{}
	orch := NewOrchestrator(store, publisher, newTestLogger())
	orch.WithNow(func() time.Time { return now })

	cmd, err := orch.NextCommand(context.Background(), pending.RunID)
	if err != nil {
		t.Fatalf("NextCommand returned error: %v", err)
	}
	if saved.DeliveredAt == nil || !saved.DeliveredAt.Equal(now) {
		t.Fatalf("expected command to be saved with delivered timestamp %v, got %v", now, saved.DeliveredAt)
	}
	if cmd.DeliveredAt == nil || !cmd.DeliveredAt.Equal(now) {
		t.Fatalf("returned command missing delivered timestamp")
	}
	if len(publisher.commandEvents) != 1 || publisher.commandEvents[0].Event != "delivered" {
		t.Fatalf("expected delivery event to be published")
	}
}

func TestAckCommandMarksAcknowledgedAndPublishesEvent(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	existing := types.RunCommand{ID: "cmd-1", RunID: "run-1", Type: types.CommandTypePause, IssuedAt: now, CreatedAt: now}
	var saved types.RunCommand
	store := &stubRunStore{
		getCommandFn: func(ctx context.Context, runID, commandID string) (types.RunCommand, error) {
			return existing, nil
		},
		saveCommandFn: func(ctx context.Context, command types.RunCommand) error {
			saved = command
			return nil
		},
	}
	publisher := &stubPublisher{}
	orch := NewOrchestrator(store, publisher, newTestLogger())
	orch.WithNow(func() time.Time { return now })

	cmd, err := orch.AckCommand(context.Background(), existing.RunID, existing.ID)
	if err != nil {
		t.Fatalf("AckCommand returned error: %v", err)
	}
	if saved.AcknowledgedAt == nil || !saved.AcknowledgedAt.Equal(now) {
		t.Fatalf("expected command to be saved with acknowledged timestamp")
	}
	if cmd.AcknowledgedAt == nil || !cmd.AcknowledgedAt.Equal(now) {
		t.Fatalf("returned command missing acknowledged timestamp")
	}
	if len(publisher.commandEvents) != 1 || publisher.commandEvents[0].Event != "acknowledged" {
		t.Fatalf("expected acknowledged event to be published")
	}
}

func TestListRunsForHealthCheckFiltersStates(t *testing.T) {
	running := types.Run{ID: "run-running", State: types.RunStateRunning}
	completed := types.Run{ID: "run-completed", State: types.RunStateCompleted}
	store := &stubRunStore{
		listRunsFn: func(ctx context.Context) ([]types.Run, error) {
			return []types.Run{running, completed}, nil
		},
	}

	orch := NewOrchestrator(store, &stubPublisher{}, newTestLogger())
	runs, err := orch.ListRunsForHealthCheck(context.Background(), types.RunStateRunning)
	if err != nil {
		t.Fatalf("ListRunsForHealthCheck returned error: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != running.ID {
		t.Fatalf("expected only running run to be returned, got %#v", runs)
	}

	allRuns, err := orch.ListRunsForHealthCheck(context.Background())
	if err != nil {
		t.Fatalf("ListRunsForHealthCheck with no filter returned error: %v", err)
	}
	if len(allRuns) != 2 {
		t.Fatalf("expected all runs to be returned when no filter provided")
	}
}

func TestUpdateRunHealthPersistsChanges(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stored := types.Run{ID: "run-1", HealthStatus: types.RunHealthHealthy, StatusMessage: "ok"}
	var saved types.Run
	store := &stubRunStore{
		getRunFn: func(ctx context.Context, id string) (types.Run, error) {
			return stored, nil
		},
		updateRunFn: func(ctx context.Context, run types.Run) error {
			saved = run
			return nil
		},
	}

	orch := NewOrchestrator(store, &stubPublisher{}, newTestLogger())
	orch.WithNow(func() time.Time { return now })

	updated, err := orch.UpdateRunHealth(context.Background(), stored.ID, types.RunHealthUnresponsive, "no heartbeat")
	if err != nil {
		t.Fatalf("UpdateRunHealth returned error: %v", err)
	}
	if saved.HealthStatus != types.RunHealthUnresponsive {
		t.Fatalf("expected run health to be updated, got %s", saved.HealthStatus)
	}
	if saved.StatusMessage != "no heartbeat" {
		t.Fatalf("expected status message to be updated, got %q", saved.StatusMessage)
	}
	if saved.UpdatedAt != now {
		t.Fatalf("expected updated timestamp to be set to now")
	}
	if updated.HealthStatus != types.RunHealthUnresponsive {
		t.Fatalf("expected updated run to reflect new health status")
	}
}
