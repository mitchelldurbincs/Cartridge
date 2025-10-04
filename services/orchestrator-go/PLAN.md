# Orchestrator Implementation Plan

This plan captures how we will turn the design captured in `docs/Individual Component Design/ORCHESTRATOR.md` and the persistence expectations in `docs/run_registry_schema.md` into a working Go service. The goal is to land an initial vertical slice that exercises the critical orchestrator responsibilities: run lifecycle book-keeping, heartbeat ingestion, control command delivery, and event fan-out stubs for the dashboard/alerting systems.

## 1. Service boundaries & module layout
- Create a dedicated Go module under `services/orchestrator-go`.
- Entry point in `cmd/server/main.go` wires configuration (port, DB DSN placeholders), structured logging via `zerolog`, and graceful shutdown plumbing.
- HTTP surface implemented with `chi`, exposing the MVP endpoints described in the design doc: run creation, run inspection, heartbeat ingestion, command submission, command consumption/ack.
- Internal packages:
  - `internal/types`: enums and structs mirroring the registry schema (`Run`, `RunState`, `RunHealth`, `RuntimeStatus`, `RunCommand`, `HeartbeatPayload`).
  - `internal/storage`: interface describing persistence operations expected from Postgres plus an in-memory implementation so the service remains runnable without DB provisioning. Methods mirror the read/write paths mentioned in the design doc (create run, get run, update heartbeat, append command, fetch next command, mark delivered/acknowledged, record transitions).
  - `internal/service`: business logic that orchestrates validation, monotonicity enforcement, health escalation timing, and event emission. Holds a `Scheduler` struct bundling storage + event sink.
  - `internal/http`: request decoding, validation, error mapping, and route registration.
  - `internal/events`: interface and noop publisher stub so downstream integrations can be added without modifying handlers.

## 2. Data model alignment
- Define enums exactly as captured in the docs: `run_state`, `runtime_status`, `run_health`, and the command types `pause`, `resume`, `terminate`, `tune`.
- `Run` struct fields should reflect the `runs` table plus derived metadata (`LastHeartbeatAt`, `CurrentStep`, etc.).
- `RunCommand` struct matches the `run_commands` table with timestamps for delivery and acknowledgement.
- `HeartbeatPayload` enforces the validation matrix from the design doc (required fields, monotonic `step`/`checkpoint_version`, request size guard handled upstream via `http.MaxBytesReader`).

## 3. HTTP workflows
- `POST /api/v1/runs`: accepts experiment/version identifiers and optional overrides; persists a new run with `queued` state and writes an initial transition record.
- `GET /api/v1/runs/{id}`: returns canonical run data, including runtime/health status fields.
- `POST /api/v1/runs/{id}/heartbeat`: validates payload (content type, monotonic counters, max body size), updates run metrics, recomputes `health_status`, stores heartbeats, and emits a `run-status` event via the publisher stub.
- `POST /api/v1/runs/{id}/commands`: accepts a control command envelope, validates type-specific payloads, persists command/audit data, and enqueues it for delivery.
- `GET /api/v1/runs/{id}/commands/next`: returns the oldest undelivered command (if any) and stamps `delivered_at`.
- `POST /api/v1/runs/{id}/commands/{cmd_id}/ack`: stamps `acknowledged_at` and updates state for audit.

## 4. Event propagation stub
- Provide a simple publisher interface with a concrete noop implementation that logs events with `zerolog`. The service layer emits run status updates and command lifecycle events so downstream systems can hook in later.

## 5. Testing strategy
- Unit tests for validation and storage monotonicity rules (heartbeat regression should fail, duplicate command ID should be idempotent, tune payload requiring at least one field, etc.).
- Handler tests covering happy path/validation errors using the in-memory storage backend.

## 6. Deliverables for this iteration
- Compilable orchestrator server with the modules above.
- Go unit tests exercising core workflows.
- Documentation updates: service README describing configuration + endpoints, plus plan (this file).

This slice gives other teams a runnable orchestrator that respects the documented contract while keeping components swappable (e.g., replacing the in-memory storage with a Postgres-backed implementation later).
