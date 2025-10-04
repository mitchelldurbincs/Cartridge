# Run Registry Persistence Schema

## Overview
The run registry persists user-facing experiment templates, concrete run executions, generated artifacts, and live tuning events so the API service can power REST endpoints like `POST /experiments`, `POST /runs`, `POST /runs/{id}/pause`, and `POST /runs/{id}/tune`. The schema below favors:

* **Immutable experiments** – templates are versioned, never mutated in place.
* **Auditable runs** – every state change and live-tuning mutation is tracked.
* **Fast dashboard queries** – common lookups (by experiment, state, recency) have dedicated indexes.

All tables use `uuid` primary keys generated server-side, timestamps in UTC (`timestamptz`), and optimistic row locking via `updated_at`.

---

## Table specifications

### `experiments`
| Column | Type | Constraints / Notes |
| --- | --- | --- |
| `id` | `uuid` | Primary key. |
| `slug` | `text` | Human readable identifier, unique. |
| `display_name` | `text` | Required. |
| `description` | `text` | Optional markdown/HTML summary. |
| `config` | `jsonb` | Frozen experiment manifest. Immutable after creation. |
| `overrides_whitelist` | `jsonb` | Array of dot-path strings allowed in `POST /runs/{id}/tune`. |
| `created_by` | `uuid` | FK → `users.id` (or service account). Nullable until auth lands. |
| `created_at` | `timestamptz` | Default `now()`. |
| `archived_at` | `timestamptz` | Nullable; experiment hidden from defaults when set. |
| `config_hash` | `bytea` | SHA256 of `config` for deduping. Unique partial index on non-null values. |

**Indexes**
* `UNIQUE (slug)` for REST-friendly lookups.
* `UNIQUE (config_hash) WHERE config_hash IS NOT NULL` to prevent duplicate templates.
* `INDEX ON (created_at DESC)` for dashboard ordering.

### `experiment_versions`
Immutable history of experiment edits. REST contract exposes latest via `GET /experiments/{id}`; previous versions support auditing.

| Column | Type | Constraints / Notes |
| --- | --- | --- |
| `id` | `uuid` | Primary key. |
| `experiment_id` | `uuid` | FK → `experiments.id` ON DELETE CASCADE. |
| `version` | `integer` | Monotonic, starts at 1. UNIQUE with `experiment_id`. |
| `config` | `jsonb` | Snapshot of manifest at this version. |
| `created_by` | `uuid` | FK → `users.id`. |
| `created_at` | `timestamptz` | Default `now()`. |
| `change_note` | `text` | Optional summary for audit log. |

**Indexes**
* `UNIQUE (experiment_id, version)`.
* `INDEX ON (experiment_id, created_at DESC)` for timeline queries.

### `runs`
Concrete instantiations of experiments. Drives `POST /runs`, `POST /runs/{id}/pause|resume|terminate`, and `GET /runs/:id`.

| Column | Type | Constraints / Notes |
| --- | --- | --- |
| `id` | `uuid` | Primary key. |
| `experiment_id` | `uuid` | FK → `experiments.id` ON DELETE RESTRICT. |
| `version_id` | `uuid` | FK → `experiment_versions.id` capturing the manifest used at launch. |
| `state` | `run_state` enum | Values: `queued`, `provisioning`, `running`, `paused`, `completed`, `failed`, `terminated`. Mirrors REST contract for pause/resume/terminate and dashboard filters. |
| `status_message` | `text` | Latest user-facing status detail. |
| `priority` | `integer` | Scheduler priority (higher = sooner). Default 0. |
| `launch_manifest` | `jsonb` | Resolved run manifest (experiment config + overrides + scheduler metadata). Immutable. |
| `overrides` | `jsonb` | User-provided overrides allowed by whitelist. |
| `heartbeat_at` | `timestamptz` | Updated by `/runs/{id}/heartbeat` endpoint. |
| `started_at` | `timestamptz` | Set when state enters `running`. |
| `ended_at` | `timestamptz` | Set when state in terminal set `{completed, failed, terminated}`. |
| `created_by` | `uuid` | FK → `users.id`. |
| `created_at` | `timestamptz` | Default `now()`. |
| `updated_at` | `timestamptz` | Default `now()`, maintained via trigger. |

**Indexes**
* `INDEX ON (experiment_id, created_at DESC)` for experiment detail page.
* `INDEX ON (state)` to filter active runs quickly.
* Partial index `INDEX ON (heartbeat_at) WHERE state = 'running'` for stale-run watchdog.

### `run_state_transitions`
Audit log for state changes, feeding WebSocket events.

| Column | Type | Notes |
| --- | --- | --- |
| `id` | `uuid` | Primary key. |
| `run_id` | `uuid` | FK → `runs.id` ON DELETE CASCADE. |
| `from_state` | `run_state` | Nullable for initial transition. |
| `to_state` | `run_state` | Required. |
| `changed_by` | `uuid` | FK → `users.id` or system principal. |
| `reason` | `text` | Optional message (e.g., failure cause). |
| `created_at` | `timestamptz` | Default `now()`. |

**Indexes**
* `INDEX ON (run_id, created_at)` for time-ordered queries.

### `artifacts`
Represents blobs accessible via `GET /runs/:id/artifacts`.

| Column | Type | Constraints / Notes |
| --- | --- | --- |
| `id` | `uuid` | Primary key. |
| `run_id` | `uuid` | FK → `runs.id` ON DELETE CASCADE. |
| `step` | `bigint` | Training step associated with artifact. Nullable for non-step outputs. |
| `artifact_type` | `artifact_kind` enum | Values: `checkpoint`, `policy`, `replay`, `evaluation`, `log_bundle`, `custom`. |
| `uri` | `text` | Required object storage URI. |
| `size_bytes` | `bigint` | Optional. |
| `checksum` | `text` | Optional integrity hash. |
| `metadata` | `jsonb` | Additional typed fields (shard IDs, score summaries). |
| `created_at` | `timestamptz` | Default `now()`. |

**Indexes**
* `INDEX ON (run_id, step DESC)` for checkpoint listings.
* Partial index `INDEX ON (artifact_type) WHERE artifact_type IN ('checkpoint', 'policy')` to accelerate UI filters.
* Unique constraint `(run_id, artifact_type, step)` where step not null to avoid duplicates per step.

### `tune_events`
Tracks `/runs/{id}/tune` API calls and eventual application outcome.

| Column | Type | Constraints / Notes |
| --- | --- | --- |
| `id` | `uuid` | Primary key. |
| `run_id` | `uuid` | FK → `runs.id` ON DELETE CASCADE. |
| `path` | `text` | Dot-delimited override path (validated against whitelist). |
| `requested_value` | `jsonb` | Raw value from REST payload. |
| `applied_value` | `jsonb` | Value after clamping/coercion. Nullable when rejected. |
| `state` | `tune_state` enum | Values: `pending`, `applied`, `rejected`, `superseded`. |
| `reason` | `text` | Explanation for rejection or supersession. |
| `requested_by` | `uuid` | FK → `users.id`. |
| `applied_at` | `timestamptz` | Set when state transitions to `applied`. |
| `created_at` | `timestamptz` | Default `now()`. |

**Indexes**
* `INDEX ON (run_id, created_at DESC)` for run timeline.
* Partial `INDEX ON (state) WHERE state = 'pending'` to find outstanding requests.

### `run_metrics_rollup` (optional helper)
Materialized view or table to store aggregates (latest step, reward mean) for dashboard cards. Populated asynchronously from metrics ingestion. Not part of MVP but referenced by UI.

---

## Relationships summary
* `experiments.id` ←→ `runs.experiment_id` (one-to-many).
* `experiments.id` ←→ `experiment_versions.experiment_id` (one-to-many).
* `experiment_versions.id` ←→ `runs.version_id` to snapshot launch manifest.
* `runs.id` ←→ `artifacts.run_id` (one-to-many).
* `runs.id` ←→ `run_state_transitions.run_id` (one-to-many).
* `runs.id` ←→ `tune_events.run_id` (one-to-many).
* `users.id` referenced from creator / actor columns for auditability.

These foreign keys are set to `ON DELETE CASCADE` for child records that should disappear with a run (artifacts, transitions, tune events) and `ON DELETE RESTRICT` where historical integrity matters (runs referencing experiments).

---

## Enum definitions

```sql
CREATE TYPE run_state AS ENUM (
  'queued',
  'provisioning',
  'running',
  'paused',
  'completed',
  'failed',
  'terminated'
);

CREATE TYPE artifact_kind AS ENUM (
  'checkpoint',
  'policy',
  'replay',
  'evaluation',
  'log_bundle',
  'custom'
);

CREATE TYPE tune_state AS ENUM (
  'pending',
  'applied',
  'rejected',
  'superseded'
);
```

Enum values line up with REST responses and WebSocket events so clients can map states 1:1.

---

## Migration sketch
Below is an initial PostgreSQL migration (`0001_run_registry.sql`) establishing core tables and indexes. Subsequent migrations can add optional tables (metrics, alerts) as services mature.

```sql
-- 0001_run_registry.sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- for gen_random_uuid()

-- Enums
CREATE TYPE run_state AS ENUM ('queued','provisioning','running','paused','completed','failed','terminated');
CREATE TYPE artifact_kind AS ENUM ('checkpoint','policy','replay','evaluation','log_bundle','custom');
CREATE TYPE tune_state AS ENUM ('pending','applied','rejected','superseded');

-- Experiments
CREATE TABLE experiments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug text NOT NULL UNIQUE,
  display_name text NOT NULL,
  description text,
  config jsonb NOT NULL,
  overrides_whitelist jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz,
  config_hash bytea,
  CONSTRAINT experiments_config_hash_unique UNIQUE (config_hash)
    DEFERRABLE INITIALLY IMMEDIATE
);
CREATE UNIQUE INDEX experiments_config_hash_unique_partial
  ON experiments (config_hash)
  WHERE config_hash IS NOT NULL;
CREATE INDEX experiments_created_at_idx ON experiments (created_at DESC);

CREATE TABLE experiment_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  experiment_id uuid NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
  version integer NOT NULL,
  config jsonb NOT NULL,
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  change_note text,
  UNIQUE (experiment_id, version)
);
CREATE INDEX experiment_versions_recent_idx
  ON experiment_versions (experiment_id, created_at DESC);

-- Runs
CREATE TABLE runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  experiment_id uuid NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
  version_id uuid NOT NULL REFERENCES experiment_versions(id) ON DELETE RESTRICT,
  state run_state NOT NULL DEFAULT 'queued',
  status_message text,
  priority integer NOT NULL DEFAULT 0,
  launch_manifest jsonb NOT NULL,
  overrides jsonb NOT NULL DEFAULT '{}'::jsonb,
  heartbeat_at timestamptz,
  started_at timestamptz,
  ended_at timestamptz,
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX runs_experiment_created_idx ON runs (experiment_id, created_at DESC);
CREATE INDEX runs_state_idx ON runs (state);
CREATE INDEX runs_active_heartbeat_idx ON runs (heartbeat_at)
  WHERE state = 'running';

CREATE TABLE run_state_transitions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  from_state run_state,
  to_state run_state NOT NULL,
  changed_by uuid,
  reason text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX run_state_transitions_run_idx
  ON run_state_transitions (run_id, created_at);

-- Artifacts
CREATE TABLE artifacts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  step bigint,
  artifact_type artifact_kind NOT NULL,
  uri text NOT NULL,
  size_bytes bigint,
  checksum text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT artifacts_unique_step UNIQUE (run_id, artifact_type, step)
    DEFERRABLE INITIALLY IMMEDIATE
);
CREATE INDEX artifacts_run_step_idx ON artifacts (run_id, step DESC);
CREATE INDEX artifacts_type_idx ON artifacts (artifact_type)
  WHERE artifact_type IN ('checkpoint','policy');

-- Tune events
CREATE TABLE tune_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  path text NOT NULL,
  requested_value jsonb NOT NULL,
  applied_value jsonb,
  state tune_state NOT NULL DEFAULT 'pending',
  reason text,
  requested_by uuid,
  applied_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tune_events_run_idx ON tune_events (run_id, created_at DESC);
CREATE INDEX tune_events_pending_idx ON tune_events (state)
  WHERE state = 'pending';

-- Trigger to maintain updated_at
CREATE OR REPLACE FUNCTION touch_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER runs_touch_updated_at
BEFORE UPDATE ON runs
FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
```

### ORM model reference (SQLModel / SQLAlchemy style)
If the team prefers Python models, the migration above maps cleanly to SQLModel:

```python
class RunState(enum.Enum):
    queued = "queued"
    provisioning = "provisioning"
    running = "running"
    paused = "paused"
    completed = "completed"
    failed = "failed"
    terminated = "terminated"

class Experiment(SQLModel, table=True):
    id: UUID = Field(default_factory=uuid4, primary_key=True)
    slug: str = Field(index=True, sa_column_kwargs={"unique": True})
    display_name: str
    description: str | None = None
    config: dict
    overrides_whitelist: list[str] = Field(default_factory=list)
    created_by: UUID | None = Field(foreign_key="users.id")
    created_at: datetime = Field(default_factory=datetime.utcnow, sa_column=sa.Column(sa.DateTime(timezone=True)))
    archived_at: datetime | None = Field(default=None, sa_column=sa.Column(sa.DateTime(timezone=True)))
    config_hash: bytes | None = Field(default=None, sa_column=sa.Column(sa.LargeBinary, unique=True))
    versions: list["ExperimentVersion"] = Relationship(back_populates="experiment")

class Run(SQLModel, table=True):
    id: UUID = Field(default_factory=uuid4, primary_key=True)
    experiment_id: UUID = Field(foreign_key="experiments.id")
    version_id: UUID = Field(foreign_key="experiment_versions.id")
    state: RunState = Field(default=RunState.queued)
    status_message: str | None = None
    priority: int = 0
    launch_manifest: dict
    overrides: dict = Field(default_factory=dict)
    heartbeat_at: datetime | None = Field(default=None, sa_column=sa.Column(sa.DateTime(timezone=True)))
    started_at: datetime | None = Field(default=None, sa_column=sa.Column(sa.DateTime(timezone=True)))
    ended_at: datetime | None = Field(default=None, sa_column=sa.Column(sa.DateTime(timezone=True)))
    created_by: UUID | None = Field(foreign_key="users.id")
    created_at: datetime = Field(default_factory=datetime.utcnow, sa_column=sa.Column(sa.DateTime(timezone=True)))
    updated_at: datetime = Field(default_factory=datetime.utcnow, sa_column=sa.Column(sa.DateTime(timezone=True)))
    artifacts: list["Artifact"] = Relationship(back_populates="run")
    tune_events: list["TuneEvent"] = Relationship(back_populates="run")
```

This snippet mirrors the SQL migration, ensuring the REST layer can hydrate response payloads without impedance mismatch.
