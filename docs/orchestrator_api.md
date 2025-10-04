# Orchestrator HTTP API Contract

This document captures the contract between the Orchestrator service and external clients.  It supplements the high-level design
notes in `DESIGN_DOC.md` with per-endpoint schemas, lifecycle semantics, and validation rules so other teams can build against a
stable interface.

## Versioning and Conventions

* **Base URL:** `/api/v1`
* **Content type:** All requests and responses use `application/json` unless otherwise noted.
* **Authentication:** Requests must include a bearer token understood by the gateway (out of scope for this doc).
* **Idempotency:** Mutating endpoints accept an optional `Idempotency-Key` header.  When provided, repeated requests with the same
  key and payload must return the original response.

---

## Run State Model

Runs progress through a finite set of states that dictate what actions are allowed.  Terminal states are `completed`, `failed`, and
`terminated`.

| State       | Description                                      | Permitted Transitions                                                                 |
|-------------|--------------------------------------------------|----------------------------------------------------------------------------------------|
| `pending`   | Run created, waiting for resources               | `running`, `cancelled` (internal), `failed`                                           |
| `running`   | Trainers and engines are actively executing      | `paused`, `completed`, `failed`, `terminated`                                        |
| `paused`    | Execution suspended, resources held where safe   | `running`, `terminated`                                                               |
| `completed` | Successful finish with all artifacts persisted   | —                                                                                    |
| `failed`    | Aborted due to error                             | —                                                                                    |
| `terminated`| Manually stopped by an operator                  | —                                                                                    |

### State Mutation by Endpoint

* `POST /runs` — `pending`
* `POST /runs/{id}/resume` — `paused → running`
* `POST /runs/{id}/pause` — `running → paused`
* `POST /runs/{id}/terminate` — `pending|running|paused → terminated`
* Scheduler reports `running`, `completed`, `failed` transitions via internal heartbeats, exposed through the Runs resource.

All state transitions are recorded with actor, timestamp, and reason (where applicable).

---

## `/experiments`

### `GET /experiments`

* **Purpose:** List experiment templates.
* **Status codes:** `200`
* **Response schema:**

```json
{
  "experiments": [
    {
      "id": "exp_frostbite",
      "name": "Frostbite PPO Baseline",
      "created_at": "2024-05-06T15:30:00Z",
      "owner": "ml-research",
      "game": "generals",
      "algo": "PPO"
    }
  ],
  "next_page_token": ""
}
```

### `POST /experiments`

* **Purpose:** Create an immutable experiment template.
* **Status codes:** `201`, `400` (validation), `409` (duplicate `id`), `422` (invalid overrides).
* **Request schema:**

```json
{
  "id": "exp_frostbite",
  "name": "Frostbite PPO Baseline",
  "description": "PPO baseline for frostbite map",
  "game": {
    "env_id": "generals",
    "map": "frostbite"
  },
  "algo": "PPO",
  "trainer": {
    "lr": 0.0003,
    "gamma": 0.99,
    "gae_lambda": 0.95,
    "clip_range": 0.2,
    "entropy_coef": 0.01,
    "max_grad_norm": 0.5
  },
  "simulation": {
    "horizon": 1024,
    "rollout_length": 256,
    "num_envs": 32
  },
  "resources": {
    "gpus": 1,
    "cpu": 8,
    "memory_gb": 32
  },
  "retention": {
    "checkpoint_interval_steps": 200000,
    "max_checkpoints": 5
  },
  "allowed_overrides": [
    "trainer.lr",
    "trainer.entropy_coef",
    "simulation.rollout_length",
    "evaluation.interval"
  ]
}
```

* **Response schema:**

```json
{
  "id": "exp_frostbite",
  "created_at": "2024-05-06T15:30:00Z"
}
```

---

## `/runs`

### `GET /runs`

* **Purpose:** List runs with filters such as experiment id, state, or owner.
* **Query parameters:** `experiment_id`, `state`, `owner`, `page_size`, `page_token`.
* **Status codes:** `200`
* **Response schema:**

```json
{
  "runs": [
    {
      "id": "run_123",
      "experiment_id": "exp_frostbite",
      "state": "running",
      "created_at": "2024-05-09T10:12:00Z",
      "started_at": "2024-05-09T10:20:00Z",
      "steps_completed": 450000,
      "latest_metric_at": "2024-05-09T11:00:00Z"
    }
  ],
  "next_page_token": null
}
```

### `POST /runs`

* **Purpose:** Start a new run from an experiment template.
* **Status codes:** `201`, `202` (queued), `400`, `404` (unknown experiment), `409` (duplicate client token), `422` (invalid overrides).
* **Headers:** Optional `Idempotency-Key` for safe retries.
* **Request schema:**

```json
{
  "experiment_id": "exp_frostbite",
  "display_name": "ppo-frostbite-May09",
  "owner": "ml-research",
  "overrides": {
    "trainer": {
      "lr": 0.00025
    },
    "simulation": {
      "rollout_length": 192
    }
  },
  "schedule": {
    "priority": "high",
    "start_after": "2024-05-09T10:00:00Z"
  }
}
```

* **Response schema:**

```json
{
  "id": "run_123",
  "state": "pending",
  "created_at": "2024-05-09T10:12:00Z"
}
```

* **State mutation:** New runs are placed in `pending` until the scheduler launches workers.

---

## `/runs/{id}` lifecycle controls

### `POST /runs/{id}/pause`

* **Purpose:** Pause an active run.
* **Status codes:** `202`, `400` (run not pausable), `404`.
* **Request schema:**

```json
{
  "reason": "Investigating reward regressions"
}
```

* **Response schema:**

```json
{
  "id": "run_123",
  "previous_state": "running",
  "state": "paused",
  "requested_at": "2024-05-09T11:05:00Z"
}
```

* **State mutation:** `running → paused` if current state is `running`.

### `POST /runs/{id}/resume`

* **Purpose:** Resume a paused run.
* **Status codes:** `202`, `400`, `404`.
* **Request schema:**

```json
{
  "reason": "Regression resolved"
}
```

* **Response schema:**

```json
{
  "id": "run_123",
  "previous_state": "paused",
  "state": "running",
  "requested_at": "2024-05-09T11:25:00Z"
}
```

* **State mutation:** `paused → running`.

### `POST /runs/{id}/terminate`

* **Purpose:** Force terminate a run regardless of state.
* **Status codes:** `202`, `400` (already terminal), `404`.
* **Request schema:**

```json
{
  "reason": "Budget exhausted",
  "preserve_artifacts": true
}
```

* **Response schema:**

```json
{
  "id": "run_123",
  "previous_state": "running",
  "state": "terminated",
  "requested_at": "2024-05-09T12:00:00Z"
}
```

* **State mutation:** `pending|running|paused → terminated`.

### `POST /runs/{id}/tune`

* **Purpose:** Apply bounded runtime overrides to a running or paused run.
* **Status codes:** `202`, `400` (invalid state or no-op), `404`, `422` (validation failure).
* **Request schema:**

```json
{
  "request_id": "tune-5f2f",
  "overrides": {
    "trainer": {
      "lr": 0.0002,
      "entropy_coef": 0.008
    },
    "evaluation": {
      "interval": 25000
    }
  },
  "comment": "Lower LR after plateau"
}
```

* **Response schema:**

```json
{
  "id": "run_123",
  "state": "running",
  "applied_overrides": {
    "trainer": {
      "lr": 0.0002,
      "entropy_coef": 0.008
    },
    "evaluation": {
      "interval": 25000
    }
  },
  "applied_at": "2024-05-09T11:45:00Z"
}
```

* **State mutation:** Run remains in current state (`running` or `paused`), but a new configuration revision is recorded.
* **Idempotency:** `request_id` or `Idempotency-Key` required; duplicate requests with same ID must be no-ops.

---

## Metrics

### `GET /runs/{id}/metrics`

* **Purpose:** Fetch time-series metrics for dashboards (reward, loss, throughput, resource usage).
* **Status codes:** `200`, `404`.
* **Query parameters:** `names[]=reward_mean&names[]=loss_value`, `start_time`, `end_time`, `resolution`.
* **Response schema:**

```json
{
  "run_id": "run_123",
  "series": [
    {
      "name": "reward_mean",
      "unit": "points",
      "points": [
        { "timestamp": "2024-05-09T10:21:00Z", "value": 12.3 },
        { "timestamp": "2024-05-09T10:26:00Z", "value": 15.7 }
      ]
    },
    {
      "name": "samples_per_sec",
      "unit": "steps/s",
      "points": [
        { "timestamp": "2024-05-09T10:21:00Z", "value": 8900 }
      ]
    }
  ]
}
```

* **Pagination:** Clients may request downsampled windows using `resolution` (e.g., `30s`, `5m`).

---

## Artifacts

### `GET /runs/{id}/artifacts`

* **Purpose:** Enumerate checkpoints, trajectory shards, logs, and other run outputs.
* **Status codes:** `200`, `404`.
* **Response schema:**

```json
{
  "run_id": "run_123",
  "artifacts": [
    {
      "id": "ckpt_0004",
      "kind": "checkpoint",
      "path": "s3://cartridge/checkpoints/run_123/ckpt_0004.tar",
      "created_at": "2024-05-09T10:55:00Z",
      "size_bytes": 734003200,
      "metadata": {
        "steps": 400000
      }
    },
    {
      "id": "traj_shard_17",
      "kind": "trajectory",
      "path": "s3://cartridge/trajectories/run_123/shard_17.parquet",
      "created_at": "2024-05-09T10:58:00Z",
      "size_bytes": 268435456
    }
  ]
}
```

### `POST /runs/{id}/artifacts/signed-url`

* **Purpose:** Generate a short-lived signed URL for downloading an artifact.
* **Status codes:** `200`, `400` (unknown artifact), `404` (run not found).
* **Request schema:**

```json
{
  "artifact_id": "ckpt_0004",
  "expires_in_seconds": 900
}
```

* **Response schema:**

```json
{
  "url": "https://signed.example.com/abc123",
  "expires_at": "2024-05-09T11:10:00Z"
}
```

---

## Evaluation Jobs

### `POST /evaluations`

* **Purpose:** Schedule an evaluation job comparing checkpoints or policies.
* **Status codes:** `201`, `400`, `404` (unknown run or artifact), `422` (invalid configuration).
* **Request schema:**

```json
{
  "baseline": {
    "run_id": "run_123",
    "artifact_id": "ckpt_0004"
  },
  "challenger": {
    "run_id": "run_456",
    "artifact_id": "ckpt_0007"
  },
  "match_settings": {
    "map_pool": ["frostbite", "marsh"],
    "num_matches": 200,
    "seeds": [1, 2, 3, 4],
    "max_game_length": 1800
  },
  "owner": "ml-research",
  "metadata": {
    "campaign": "spring_playtest"
  }
}
```

* **Response schema:**

```json
{
  "id": "eval_789",
  "state": "pending",
  "created_at": "2024-05-09T12:30:00Z"
}
```

### `GET /evaluations/{id}`

* **Purpose:** Retrieve job status, including aggregated metrics.
* **Status codes:** `200`, `404`.
* **Response schema:**

```json
{
  "id": "eval_789",
  "state": "running",
  "matches_completed": 120,
  "metrics": {
    "win_rate": 0.58,
    "elo_delta": 23.4
  },
  "artifacts": [
    {
      "kind": "score_report",
      "path": "s3://cartridge/evals/eval_789/report.json"
    }
  ]
}
```

---

## Validation Rules

The orchestrator enforces strict validation to guarantee reproducibility and guardrail safety.

1. **Whitelisted runtime overrides:** Only keys declared in the experiment `allowed_overrides` array may be included in `POST /runs` or
   `POST /runs/{id}/tune`.  All other keys result in `422`.
2. **Bounds:**
   * `trainer.lr` — `(0, 1e-1]`
   * `trainer.entropy_coef` — `[0, 0.05]`
   * `simulation.rollout_length` — integers in `[32, 1024]` and divisible by 32.
   * `evaluation.interval` — integer steps in `[5000, 200000]`.
3. **Immutability:** Experiment templates cannot be mutated after creation.  To adjust defaults, create a new experiment ID.
4. **Idempotency:**
   * All POST endpoints accept `Idempotency-Key`.  Servers must return `409` if the payload differs from the original request for the same key.
   * `POST /runs/{id}/tune` additionally requires `request_id` inside the payload; repeated requests must be no-ops.
5. **State preconditions:**
   * `pause` only allowed when run is `running`.
   * `resume` only allowed when run is `paused`.
   * `terminate` forbidden once run is `completed` or `failed`.
   * `tune` allowed when run is `running` or `paused`.
6. **Artifact access:** Signed URL requests require the artifact to belong to the run and to be in a finalized state.
7. **Evaluation inputs:** Both baseline and challenger must reference either a run-level default checkpoint (`latest`) or an explicit
   artifact ID.  Mixed references cause `400`.
8. **Metrics rate limits:** Clients must not request windows wider than 7 days at a 1-second resolution; the server returns `400` when the
   implied sample count exceeds 100k points.

---

## Error Envelope

Errors use a consistent structure:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "trainer.lr must be <= 0.1",
    "details": {
      "field": "trainer.lr"
    }
  }
}
```

## Future Considerations

* Add `PATCH /experiments/{id}` for administrative notes (non-executable metadata).
* Expose run event logs via `GET /runs/{id}/events` for richer auditing.
* Provide OpenAPI specification auto-generated from protobuf definitions.
