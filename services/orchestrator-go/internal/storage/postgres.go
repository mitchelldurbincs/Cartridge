package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/cartridge/orchestrator/internal/types"
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

// Helper function to check for PostgreSQL unique constraint violations
func isUniqueViolation(err error) bool {
	// This would check the PostgreSQL error code for unique constraint violations
	// Implementation depends on the specific PostgreSQL driver being used
	return false // Simplified for now
}