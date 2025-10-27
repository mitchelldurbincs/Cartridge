# Troubleshooting: Learner Heartbeat 422 Errors

## Problem

The learner service fails with a 422 Unprocessable Entity error when sending heartbeats to the orchestrator:

```
learner-1  | {"error": "422, message='Unprocessable Entity', url='http://orchestrator:8080/api/v1/runs/local-run/heartbeat'", ...}
aiohttp.client_exceptions.ClientResponseError: 422, message='Unprocessable Entity', url='http://orchestrator:8080/api/v1/runs/local-run/heartbeat'
```

## Root Cause

This error occurs due to **step or checkpoint regression validation** in the orchestrator. The heartbeat validation enforces that:

1. The step number must not decrease from the last known value
2. The checkpoint version must not decrease from the last known value

This commonly happens when:
- The learner service restarts but the orchestrator database still has the old run state
- The learner starts from step 0 but the orchestrator expects it to continue from the last known step
- Development environments where services restart frequently

## Validation Details

The orchestrator validates heartbeats using these rules (see `services/orchestrator-go/internal/types/types.go:132`):

- `run_id` must match
- `status` must be one of: "running", "paused", "terminating", "errored"
- `step` must be non-negative
- `checkpoint_version` must be non-negative
- `step` must not decrease (regression check)
- `checkpoint_version` must not decrease (regression check)

## Solution 1: Reset Run Progress (Recommended for Development)

Use the new `/reset` endpoint to clear a run's progress:

```bash
curl -X POST http://localhost:8080/api/v1/runs/local-run/reset
```

This resets:
- `current_step` to 0
- `checkpoint_version` to 0
- `loss` to 0
- `samples_per_sec` to 0
- `last_heartbeat_at` to null
- `runtime_status` to "running"
- `health_status` to "healthy"

## Solution 2: Clear Database State

For a fresh start in development:

```bash
# Stop all services
docker compose -f deployments/local/docker-compose.yml down -v

# This removes volumes including the PostgreSQL database

# Start services again
docker compose -f deployments/local/docker-compose.yml up
```

## Solution 3: Resume from Checkpoint (Production)

In production, the learner should resume from the last checkpoint:

1. Load the latest checkpoint on startup
2. Continue training from the saved step
3. Send heartbeats with monotonically increasing steps

## Enhanced Diagnostics

The orchestrator now includes detailed logging for heartbeat failures:

```json
{
  "level": "error",
  "error": "step regression: 0 < 100",
  "run_id": "local-run",
  "current_step": 100,
  "current_checkpoint": 10,
  "payload_step": 0,
  "payload_checkpoint": 0,
  "payload_status": "running",
  "message": "heartbeat validation failed"
}
```

Check orchestrator logs to see exactly which validation failed:

```bash
docker compose -f deployments/local/docker-compose.yml logs orchestrator | grep "heartbeat validation"
```

## Prevention

For development workflows:

1. Use the `/reset` endpoint before restarting the learner
2. Implement checkpoint resumption in the learner service
3. Consider adding a `--fresh-start` flag to the learner that automatically calls the reset endpoint

For production:

1. Always resume from checkpoints
2. Implement proper run lifecycle management
3. Use different run IDs for different training sessions

## API Reference

### Reset Run Progress

**Endpoint:** `POST /api/v1/runs/{runID}/reset`

**Description:** Resets a run's progress to allow restarting from step 0.

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/runs/my-run-id/reset
```

**Response:** Returns the updated run object with reset progress fields.
