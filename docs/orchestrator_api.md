# Orchestrator HTTP API Contract - Currently Implemented

This document captures the **currently implemented** contract between the Orchestrator service and external clients.

**Note:** This reflects the actual implementation in `services/orchestrator-go/internal/http/server.go`. The service currently provides basic run management, heartbeat handling, and command delivery functionality.

## Versioning and Conventions

* **Base URL:** `/api/v1`
* **Content type:** All requests and responses use `application/json` unless otherwise noted.
* **Authentication:** Not currently implemented
* **Idempotency:** Not currently implemented

---

### `POST /runs`

* **Purpose:** Create a new run.
* **Status codes:** `201` (created), `400` (invalid JSON), `422` (validation error).
* **Request schema:** JSON payload with run configuration (see service implementation for details).
* **Response:** Returns the created run object.

### `GET /runs/{runID}`

* **Purpose:** Get details of a specific run.
* **Status codes:** `200` (success), `404` (run not found).
* **Response:** Returns the run object with current state and metadata.

---

## Heartbeat Management

### `POST /runs/{runID}/heartbeat`

* **Purpose:** Accept heartbeat updates from learners.
* **Status codes:** `200` (success), `400` (invalid payload), `415` (wrong content type), `422` (validation error).
* **Headers:** `Content-Type: application/json` required.
* **Request size limit:** 32 KiB maximum.
* **Request schema:**

```json
{
  "run_id": "run_123",
  "status": "running",
  "step": 450000,
  "samples_per_sec": 8900.5,
  "loss": 0.234,
  "checkpoint_version": 12,
  "queued_commands": ["cmd_abc123"],
  "notes": "Optional diagnostic information"
}
```

**Field descriptions:**
- `run_id` (required): Must match the path parameter
- `status` (required): Enum: `running`, `paused`, `terminating`, `errored`
- `step` (required): Global optimizer step processed (non-decreasing)
- `samples_per_sec` (required): Rolling average of learner ingest rate
- `loss` (required): Last full-batch loss scalar for monitoring
- `checkpoint_version` (required): Highest checkpoint successfully uploaded (monotonically increasing)
- `queued_commands` (optional): IDs of control commands still buffered on the learner
- `notes` (optional): Free-form diagnostic text for temporary anomalies

* **Response:** Returns updated run object.

---

## Command Management

### `POST /runs/{runID}/commands`

* **Purpose:** Issue control commands to a run.
* **Status codes:** `202` (accepted), `400` (invalid payload), `404` (run not found), `422` (validation error).
* **Request size limit:** 32 KiB maximum.
* **Request schema:**

```json
{
  "id": "cmd_abc123",
  "type": "tune",
  "issued_at": "2024-05-09T11:45:00Z",
  "actor": {
    "type": "operator",
    "id": "user@example.com"
  },
  "payload": {
    "learning_rate": 0.0002,
    "entropy_coef": 0.008,
    "clip_epsilon": 0.2,
    "notes": "Adjusting parameters after plateau"
  }
}
```

**Command types:**
- `tune`: Runtime parameter adjustments
- `pause`: Pause run execution
- `resume`: Resume paused run
- `terminate`: Terminate run

**Actor types:**
- `operator`: Human operator
- `system`: Automated system action

* **Response:** Returns the created command object.

### `GET /runs/{runID}/commands/next`

* **Purpose:** Long-poll for the next pending command for a run.
* **Status codes:** `200` (command available), `204` (no commands), `404` (run not found).
* **Response:** Returns the next pending command, or 204 if no commands are available.

### `POST /runs/{runID}/commands/{commandID}/ack`

* **Purpose:** Acknowledge that a command has been processed by the learner.
* **Status codes:** `200` (acknowledged), `404` (command/run not found).
* **Response:** Returns the acknowledged command object with updated timestamps.

---

## Error Handling

Errors are handled with standard HTTP status codes:

- `400 Bad Request` - Invalid JSON payload or malformed request
- `404 Not Found` - Run or command not found
- `409 Conflict` - Conflicts (e.g., from storage layer)
- `415 Unsupported Media Type` - Wrong content type (must be `application/json`)
- `422 Unprocessable Entity` - Validation errors

Error responses include a simple message:
```json
{
  "error": "Validation failed: run_id is required"
}
```

No content responses (204) are returned for successful operations with no data:
```json
{
  "message": "no pending commands"
}
```

---

## Future Considerations

The current implementation provides the core functionality for run management and learner communication. Future versions may add:

* Experiment templates and management
* Metrics collection and querying
* Artifact management and signed URLs
* Evaluation job scheduling
* Enhanced validation and bounds checking
* Idempotency support
* Authentication and authorization

For the complete planned API, refer to the design documents in `docs/DESIGN_DOC.md`.
