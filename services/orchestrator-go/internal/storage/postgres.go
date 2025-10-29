//go:build postgres

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/cartridge/orchestrator/internal/types"
	_ "github.com/lib/pq"
)

// PostgresStore implements RunStore backed by PostgreSQL
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a new PostgreSQL-backed store
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (p *PostgresStore) CreateRun(ctx context.Context, run types.Run) error {
	query := `
		INSERT INTO runs (id, experiment_id, version_id, state, status_message, priority,
						 launch_manifest, overrides, runtime_status, health_status,
						 current_step, samples_per_sec, loss, checkpoint_version,
						 created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	_, err := p.db.ExecContext(ctx, query,
		run.ID, run.ExperimentID, run.VersionID, run.State, run.StatusMessage,
		run.Priority, run.LaunchManifest, run.Overrides, run.RuntimeStatus,
		run.HealthStatus, run.CurrentStep, run.SamplesPerSecond, run.Loss,
		run.CheckpointVersion, run.CreatedBy, run.CreatedAt, run.UpdatedAt)

	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("failed to create run: %w", err)
	}

	return nil
}

func (p *PostgresStore) GetRun(ctx context.Context, id string) (types.Run, error) {
	query := `
		SELECT id, experiment_id, version_id, state, status_message, priority,
			   launch_manifest, overrides, last_heartbeat_at, runtime_status,
			   health_status, current_step, samples_per_sec, loss, checkpoint_version,
			   started_at, ended_at, created_by, created_at, updated_at
		FROM runs WHERE id = $1`

	var run types.Run
	var launchManifest, overrides []byte

	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&run.ID, &run.ExperimentID, &run.VersionID, &run.State, &run.StatusMessage,
		&run.Priority, &launchManifest, &overrides, &run.LastHeartbeatAt,
		&run.RuntimeStatus, &run.HealthStatus, &run.CurrentStep,
		&run.SamplesPerSecond, &run.Loss, &run.CheckpointVersion,
		&run.StartedAt, &run.EndedAt, &run.CreatedBy, &run.CreatedAt, &run.UpdatedAt)

	if err == sql.ErrNoRows {
		return types.Run{}, ErrNotFound
	}
	if err != nil {
		return types.Run{}, fmt.Errorf("failed to get run: %w", err)
	}

	run.LaunchManifest = json.RawMessage(launchManifest)
	run.Overrides = json.RawMessage(overrides)

	return run, nil
}

func (p *PostgresStore) UpdateRun(ctx context.Context, run types.Run) error {
	query := `
		UPDATE runs SET
			state = $2, status_message = $3, last_heartbeat_at = $4,
			runtime_status = $5, health_status = $6, current_step = $7,
			samples_per_sec = $8, loss = $9, checkpoint_version = $10,
			started_at = $11, ended_at = $12, updated_at = $13
		WHERE id = $1`

	result, err := p.db.ExecContext(ctx, query,
		run.ID, run.State, run.StatusMessage, run.LastHeartbeatAt,
		run.RuntimeStatus, run.HealthStatus, run.CurrentStep,
		run.SamplesPerSecond, run.Loss, run.CheckpointVersion,
		run.StartedAt, run.EndedAt, run.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (p *PostgresStore) ListRunsByState(ctx context.Context, state types.RunState) ([]types.Run, error) {
	query := `
		SELECT id, experiment_id, version_id, state, status_message, priority,
			   launch_manifest, overrides, last_heartbeat_at, runtime_status,
			   health_status, current_step, samples_per_sec, loss, checkpoint_version,
			   started_at, ended_at, created_by, created_at, updated_at
		FROM runs WHERE state = $1
		ORDER BY created_at DESC`

	rows, err := p.db.QueryContext(ctx, query, state)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs by state: %w", err)
	}
	defer rows.Close()

	var runs []types.Run
	for rows.Next() {
		var run types.Run
		var launchManifest, overrides []byte

		err := rows.Scan(
			&run.ID, &run.ExperimentID, &run.VersionID, &run.State, &run.StatusMessage,
			&run.Priority, &launchManifest, &overrides, &run.LastHeartbeatAt,
			&run.RuntimeStatus, &run.HealthStatus, &run.CurrentStep,
			&run.SamplesPerSecond, &run.Loss, &run.CheckpointVersion,
			&run.StartedAt, &run.EndedAt, &run.CreatedBy, &run.CreatedAt, &run.UpdatedAt)

		if err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}

		run.LaunchManifest = json.RawMessage(launchManifest)
		run.Overrides = json.RawMessage(overrides)
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating runs: %w", err)
	}

	return runs, nil
}

func (p *PostgresStore) AppendTransition(ctx context.Context, transition RunTransition) error {
	query := `
		INSERT INTO run_transitions (run_id, from_state, to_state, changed_by, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := p.db.ExecContext(ctx, query,
		transition.RunID, transition.FromState, transition.ToState,
		transition.ChangedBy, transition.Reason, transition.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to append transition: %w", err)
	}

	return nil
}

func (p *PostgresStore) AppendCommand(ctx context.Context, command types.RunCommand) error {
	query := `
		INSERT INTO run_commands (id, run_id, type, payload, actor_type, actor_id,
								 issued_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := p.db.ExecContext(ctx, query,
		command.ID, command.RunID, command.Type, command.Payload,
		command.Actor.Type, command.Actor.ID, command.IssuedAt, command.CreatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("failed to append command: %w", err)
	}

	return nil
}

func (p *PostgresStore) GetCommand(ctx context.Context, runID, commandID string) (types.RunCommand, error) {
	query := `
		SELECT id, run_id, type, payload, actor_type, actor_id, issued_at,
			   delivered_at, acknowledged_at, created_at
		FROM run_commands WHERE run_id = $1 AND id = $2`

	var cmd types.RunCommand
	var actorType, actorID string
	var payload []byte

	err := p.db.QueryRowContext(ctx, query, runID, commandID).Scan(
		&cmd.ID, &cmd.RunID, &cmd.Type, &payload, &actorType, &actorID,
		&cmd.IssuedAt, &cmd.DeliveredAt, &cmd.AcknowledgedAt, &cmd.CreatedAt)

	if err == sql.ErrNoRows {
		return types.RunCommand{}, ErrNotFound
	}
	if err != nil {
		return types.RunCommand{}, fmt.Errorf("failed to get command: %w", err)
	}

	cmd.Payload = json.RawMessage(payload)
	cmd.Actor = types.CommandActor{
		Type: types.CommandActorType(actorType),
		ID:   actorID,
	}

	return cmd, nil
}

func (p *PostgresStore) NextPendingCommand(ctx context.Context, runID string) (types.RunCommand, error) {
	query := `
		SELECT id, run_id, type, payload, actor_type, actor_id, issued_at,
			   delivered_at, acknowledged_at, created_at
		FROM run_commands
		WHERE run_id = $1 AND delivered_at IS NULL
		ORDER BY issued_at ASC
		LIMIT 1`

	var cmd types.RunCommand
	var actorType, actorID string
	var payload []byte

	err := p.db.QueryRowContext(ctx, query, runID).Scan(
		&cmd.ID, &cmd.RunID, &cmd.Type, &payload, &actorType, &actorID,
		&cmd.IssuedAt, &cmd.DeliveredAt, &cmd.AcknowledgedAt, &cmd.CreatedAt)

	if err == sql.ErrNoRows {
		return types.RunCommand{}, ErrNoCommands
	}
	if err != nil {
		return types.RunCommand{}, fmt.Errorf("failed to get next pending command: %w", err)
	}

	cmd.Payload = json.RawMessage(payload)
	cmd.Actor = types.CommandActor{
		Type: types.CommandActorType(actorType),
		ID:   actorID,
	}

	return cmd, nil
}

func (p *PostgresStore) SaveCommand(ctx context.Context, command types.RunCommand) error {
	query := `
		UPDATE run_commands SET
			delivered_at = $3, acknowledged_at = $4
		WHERE run_id = $1 AND id = $2`

	result, err := p.db.ExecContext(ctx, query,
		command.RunID, command.ID, command.DeliveredAt, command.AcknowledgedAt)

	if err != nil {
		return fmt.Errorf("failed to save command: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// Helper function to check for PostgreSQL unique constraint violations
func isUniqueViolation(err error) bool {
	// This would check the PostgreSQL error code for unique constraint violations
	// Implementation depends on the specific PostgreSQL driver being used
	return false // Simplified for now
}
