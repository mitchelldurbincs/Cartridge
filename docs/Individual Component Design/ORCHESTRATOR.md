# Orchestrator Service (Go)

## Overview
The orchestrator coordinates the lifecycle of every experiment run. It assigns run IDs, stores canonical configuration and
status in Postgres, brokers runtime control messages, and fans out state changes to downstream consumers like the web dashboard
and alerting stack. The service is built in Go on top of `chi` for HTTP routing and `zerolog` for structured logging, keeping
request/response handlers lightweight while critical workflows (scheduling, auditing, status propagation) live in dedicated
packages.

## Responsibilities & scope
- **Run lifecycle management**: track each run from creation through termination, persist configuration and status, and expose a
  consistent REST surface that other components can poll or subscribe to.
- **Control plane**: receive heartbeats from learners, issue bounded runtime tune/pause/resume/terminate commands, and ensure
  reliable delivery semantics with acknowledgements and retry policy.
- **State propagation**: write run state transitions to Postgres, publish them to the dashboard event stream, and trigger
  notifications when service-level objectives (SLOs) are breached.
- **Auditability**: maintain an append-only log of all operator and automated control actions for compliance and incident
  response.

Out of scope: GPU/host placement decisions (delegated to the scheduler), per-run cost attribution, and complex multi-tenant
queueing.

## Control plane

### Heartbeat endpoint
Learners POST heartbeats to `/runs/{run_id}/heartbeat`. Payloads are JSON with the following schema:

| Field | Type | Units | Required | Description |
| --- | --- | --- | --- | --- |
| `run_id` | string | — | ✅ | Must match the path parameter; double-checked server-side. |
| `status` | string | — | ✅ | Enum: `running`, `paused`, `terminating`, `errored`. Mirrors learner local state. |
| `step` | integer | steps | ✅ | Global optimizer step processed. Non-decreasing. |
| `samples_per_sec` | number | samples/second | ✅ | Rolling average of learner ingest rate. |
| `loss` | number | unitless | ✅ | Last full-batch loss scalar for monitoring. |
| `checkpoint_version` | integer | monotonically increasing version | ✅ | Highest checkpoint successfully uploaded. |
| `queued_commands` | array of strings | — | optional | IDs of control commands still buffered on the learner. |
| `notes` | string | — | optional | Free-form diagnostic text for temporary anomalies. |

The orchestrator enforces:
- Content-Type `application/json` and request size ≤ 32 KiB.
- Monotonic `step` and `checkpoint_version`; regressions reject with `409 Conflict`.
- Missing required fields ⇒ `422 Unprocessable Entity` with validation details.

### Frequency & timeout policy
- Learners **must send heartbeats every 15 seconds**. Shorter cadences are accepted but throttled with 429s if below 5 seconds to
  avoid excessive load.
- If no valid heartbeat is received within **45 seconds**, the run is marked `heartbeat_stale` and a warning is published to the
  dashboard and alerting topic.
- After **3 consecutive missed intervals (≈135 seconds)**, the orchestrator escalates to `unresponsive`, triggers a PagerDuty
  incident, and emits a terminate recommendation. Operators can override to allow additional time.

## Runtime control commands

Control commands are persisted and delivered via the `/runs/{run_id}/commands` endpoint. Clients POST JSON envelopes with the
following shape:

```json
{
  "id": "uuid-v4",
  "type": "tune" | "pause" | "resume" | "terminate",
  "issued_at": "RFC3339 timestamp",
  "actor": {
    "type": "operator" | "system",
    "id": "user@example.com" | "orchestrator/auto-slo"
  },
  "payload": { /* type-specific */ }
}
```

### Type-specific payloads
- **tune**
  - Payload fields match the learner’s `TuneCommand` schema:
    - `learning_rate` (number, optional, 0 < value ≤ 1).
    - `entropy_coef` (number, optional, 0 ≤ value ≤ 0.1).
    - `clip_epsilon` (number, optional, 0.05 ≤ value ≤ 0.3).
    - `notes` (string, optional) describing rationale.
  - At least one tunable must be supplied; otherwise validation fails.
- **pause** / **resume**
  - Empty payload. Orchestrator ensures state transitions are valid (`running` → `pause` only when active, `pause` → `resume`
    when previously paused).
- **terminate**
  - Payload requires `reason` (string, ≤ 256 chars) and optional `final_checkpoint` (boolean) to request a last checkpoint before
    exit.

### Validation & delivery
- Commands must reference existing runs; unknown IDs ⇒ `404`.
- Duplicate command IDs are idempotently ignored and return `200` with existing record.
- All accepted commands are written to the `run_commands` table with a `delivered_at` nullable column.
- The learner control client long-polls `/runs/{run_id}/commands/next`; when it ACKs a command, the orchestrator stamps
  `delivered_at` and `acknowledged_at` timestamps and mirrors them in audit logs.
- Validation errors respond with `422` and machine-readable error codes so learners can log actionable messages.

### Auditing
- Every command persists the envelope plus request metadata (source IP, user-agent, mTLS client cert fingerprint) into an
  append-only `control_audit` table.
- Audit entries carry a cryptographic hash chain (`prev_hash`, `entry_hash`) to detect tampering.
- An hourly job exports deltas to object storage for long-term retention and feeds a read-only dashboard for compliance review.

## Status propagation
- **Postgres** is the source of truth: heartbeat-derived fields (`last_heartbeat_at`, `step`, `status`) and command state are
  stored within the `runs` table.
- **Web dashboard** subscribes to a `run-status` NATS subject published by the orchestrator. Each state change fan-outs a compact
  JSON document (`run_id`, `status`, `step`, `samples_per_sec`, `last_error`) so UI tables update in near real time.
- **Alerting** hooks are implemented via the same event stream with routing keys:
  - `heartbeat_stale` triggers a Slack warning in `#ml-ops`.
  - `unresponsive` escalates to PagerDuty.
  - `terminate` and `errored` events emit structured payloads consumed by the incident pipeline.
- Downstream consumers are expected to treat the stream as level-triggered: they must coalesce repeated states and always fall
  back to the persisted values in Postgres on reconnect.

## Interactions with other services
- Learner clients trust the orchestrator to validate tune payloads and surface resulting state via both heartbeats and the
  command acknowledgement trail.
- Dashboard queries the REST API for historical runs but relies on the status stream for live updates, ensuring consistent views
  with orchestrator-owned truth.
- Alerting systems ingest only orchestrator-signed events, keeping downstream behaviours consistent even if individual services
  report conflicting signals.
