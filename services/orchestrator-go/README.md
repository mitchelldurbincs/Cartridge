# Orchestrator Service (Go)

The orchestrator coordinates the lifecycle of training runs. It exposes a REST API backed by an in-memory store in this iteration and mirrors the design described in [`docs/Individual Component Design/ORCHESTRATOR.md`](../../docs/Individual%20Component%20Design/ORCHESTRATOR.md).

## Features
- Run creation and inspection endpoints.
- Learner heartbeat ingestion with monotonic counter validation and health status updates.
- Control command queue supporting tune, pause, resume, and terminate envelopes with validation.
- Command delivery and acknowledgement semantics with event hook stubs.
- No-op event publisher and in-memory persistence to keep the binary self-contained for development.

## Running the service
```bash
cd services/orchestrator-go
go run ./cmd/server -addr :8080
```

## API surface (MVP)
- `POST /api/v1/runs` – create a new run record.
- `GET /api/v1/runs/{id}` – fetch canonical run metadata.
- `POST /api/v1/runs/{id}/heartbeat` – ingest learner heartbeat payloads.
- `POST /api/v1/runs/{id}/commands` – enqueue a control command.
- `GET /api/v1/runs/{id}/commands/next` – fetch the next pending control command (marks delivered).
- `POST /api/v1/runs/{id}/commands/{command_id}/ack` – acknowledge a delivered command.

All responses use JSON. Heartbeat requests must use `Content-Type: application/json` and are limited to 32KiB.

## Testing
```bash
cd services/orchestrator-go
go test ./...
```
