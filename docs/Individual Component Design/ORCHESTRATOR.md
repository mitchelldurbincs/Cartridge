# Orchestrator Service (Go)

## Overview
- Lightweight REST service that tracks run metadata, accepts learner heartbeats, and manages a FIFO queue of control commands. Built with `chi` for routing and `zerolog` for structured logs.【F:services/orchestrator-go/internal/http/server.go†L19-L80】【F:services/orchestrator-go/cmd/server/main.go†L13-L87】
- Core logic lives in `internal/service.Orchestrator`, which persists state through a pluggable `storage.RunStore` (in-memory by default) and fans out status/command events through an `events.Publisher` interface (a no-op publisher is wired in development).【F:services/orchestrator-go/internal/service/orchestrator.go†L12-L146】【F:services/orchestrator-go/internal/storage/storage.go†L27-L134】【F:services/orchestrator-go/internal/events/events.go†L1-L32】
- Designed for local development: the server seeds a `local-run` record on startup when using the memory store so other components can connect immediately.【F:services/orchestrator-go/cmd/server/main.go†L34-L66】

## HTTP Surface
All routes live under `/api/v1` and speak JSON.

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/runs` | Create a run record. Generates an ID if omitted. Returns the persisted `types.Run`.【F:services/orchestrator-go/internal/http/server.go†L32-L53】 |
| `GET` | `/runs/{runID}` | Fetch run metadata (current state, runtime/health status, stats).【F:services/orchestrator-go/internal/http/server.go†L55-L61】 |
| `POST` | `/runs/{runID}/heartbeat` | Update runtime metrics from a learner heartbeat. Payload must match `types.HeartbeatPayload` (see below). Returns the updated run snapshot.【F:services/orchestrator-go/internal/http/server.go†L63-L84】【F:services/orchestrator-go/internal/types/types.go†L79-L118】 |
| `POST` | `/runs/{runID}/commands` | Queue a control command (pause/resume/terminate/tune). Generates IDs and timestamps if omitted; validates payloads via `types.RunCommand.Validate`. Returns the stored command record.【F:services/orchestrator-go/internal/http/server.go†L86-L119】【F:services/orchestrator-go/internal/types/types.go†L120-L193】 |
| `GET` | `/runs/{runID}/commands/next` | Pop the oldest undelivered command for a run and mark it delivered. Returns `204`-equivalent payload `{message:"no pending commands"}` when empty.【F:services/orchestrator-go/internal/http/server.go†L121-L132】【F:services/orchestrator-go/internal/storage/storage.go†L136-L167】 |
| `POST` | `/runs/{runID}/commands/{commandID}/ack` | Mark a delivered command as acknowledged (sets `AcknowledgedAt`). Returns the updated command.【F:services/orchestrator-go/internal/http/server.go†L134-L145】 |

### Heartbeat Payload
`types.HeartbeatPayload` requires:

- `run_id`, `status` (`running|paused|terminating|errored`), `step`, `samples_per_sec`, `loss`, and `checkpoint_version`. Optional `queued_commands` and `notes` provide learner diagnostics.【F:services/orchestrator-go/internal/types/types.go†L79-L118】
- `Validate` rejects mismatched IDs, status enums outside the supported list, negative values, and regressions in `step` or `checkpoint_version`. Violations propagate as `422 Unprocessable Entity` responses.【F:services/orchestrator-go/internal/types/types.go†L100-L118】【F:services/orchestrator-go/internal/http/server.go†L76-L84】
- Successful heartbeats merge values into the stored run (`MergeHeartbeat`), set health to `healthy`, and trigger a `RunStatusEvent` via the configured publisher.【F:services/orchestrator-go/internal/types/types.go†L195-L203】【F:services/orchestrator-go/internal/service/orchestrator.go†L66-L117】

### Command Validation
- `types.RunCommand.Validate` enforces schema rules per command type (tune payload bounds, empty payloads for pause/resume, terminate reason, actor metadata, issued-at presence). Errors surface as `422` responses.【F:services/orchestrator-go/internal/types/types.go†L120-L193】【F:services/orchestrator-go/internal/http/server.go†L100-L119】
- Commands are stored once (`storage.ErrConflict` short-circuits duplicates) and mirrored through the publisher as `"queued"`, `"delivered"`, and `"acknowledged"` events when lifecycle methods succeed.【F:services/orchestrator-go/internal/service/orchestrator.go†L119-L187】

## Storage Backends
- `internal/storage.MemoryStore` powers local/dev runs: a mutex-protected map of runs plus per-run command maps. Commands are sorted by `IssuedAt` when delivering `NextPendingCommand`. Transitions are captured in-memory for auditing but not exposed via HTTP yet.【F:services/orchestrator-go/internal/storage/storage.go†L39-L167】
- `internal/storage/postgres.go` (gated behind the `postgres` build tag) sketches a PostgreSQL-backed `RunStore` for Create/Get/Update operations. Command/transition helpers still rely on the memory implementation until the SQL version is completed.【F:services/orchestrator-go/internal/storage/postgres.go†L1-L114】

## Event Fan-out
- The orchestrator emits `events.RunStatusEvent` after each heartbeat and `events.CommandEvent` as commands move through the queue. The default `events.NoopPublisher` drops events; a NATS-backed publisher is available behind the `nats` build tag for future integration.【F:services/orchestrator-go/internal/service/orchestrator.go†L94-L146】【F:services/orchestrator-go/internal/events/events.go†L1-L32】【F:services/orchestrator-go/internal/events/nats.go†L5-L101】

## Logging & Configuration
- `cmd/server/main.go` exposes an `-addr` flag (default `:8080`) and configures conservative read/write timeouts. Logging uses `zerolog` to stdout with timestamps. No metrics or health endpoints are shipped yet.【F:services/orchestrator-go/cmd/server/main.go†L13-L87】

## Known Gaps
- Heartbeat cadence and escalation policy are not enforced in code (no timers or health downgrades besides “set healthy on heartbeat”). Missing heartbeats leave the last recorded health untouched until another component updates it.
- Command auditing (hash chains, operator attribution) and REST endpoints for transitions/history are not implemented.
- Postgres storage lacks full coverage (transitions, commands) and `isUniqueViolation` still returns `false`, so conflict handling relies on memory store semantics.
- There is no authentication or authorization layer; endpoints trust callers.

These gaps are intentional in the current development snapshot and will need addressing before production deployment.
