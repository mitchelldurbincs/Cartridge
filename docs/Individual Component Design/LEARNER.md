# Learner Service (Python)

## Overview
The learner service consumes experience from the replay buffer, performs policy/value updates with PyTorch, and exports fresh weights and checkpoints on a fixed cadence. It is written in modern, type-annotated Python (3.11) and organised into clearly testable modules so we can iterate on algorithms without touching infrastructure plumbing. The process is intentionally single-tenanted per run: each learner instance owns a single experiment configuration, streams batches from replay, produces metrics, and emits artifacts to object storage for the dashboard and orchestrator.

## Responsibilities & scope
- **Policy optimisation**: implement PPO first, but keep an interface that supports swapping algorithms (e.g. IMPALA, DQN) later.
- **Replay integration**: maintain a gRPC streaming client that keeps a rolling window of sampled transitions ready for SGD.
- **Checkpoint lifecycle**: materialise model/optimizer RNG state to MinIO/GCS and surface manifests so actors and evaluators can fetch versions.
- **Weights distribution**: push the latest weights to the weight-publisher service (or directly expose a `/weights/current` endpoint in MVP) so actors can refresh policies.
- **Run control**: consume tune/pause/resume commands from the orchestrator, apply bounded hyperparameter adjustments, and report heartbeats.
- **Observability**: emit Prometheus metrics, structured logs, and OTLP traces for sampling latency, step time, loss scalars, and checkpoint timings.

Out of scope for the first milestone: multi-run scheduling, distributed data-parallel training, and advanced prioritised replay beyond what `replay-go` already offers.

## Process architecture
```
┌──────────────────────────────────────────────────────────┐
│                 learner (per experiment run)             │
│                                                          │
│  ┌───────────┐    ┌─────────────┐    ┌────────────────┐  │
│  │Config/CLI │──► │ReplayClient │──► │BatchPrefetcher │──┼──┐
│  └───────────┘    └─────────────┘    └────────────────┘  │  │
│                                 │                         │  │ samples
│                                 ▼                         │  │ (async)
│                        ┌────────────────┐                 │  │
│                        │LearnerCore     │◄──┐             │  │
│                        │(algo loop)     │   │ gradients    │  │
│                        └────────────────┘   │             │  │
│                                 │            │             │  │
│        metrics/checkpoints      ▼            │             │  │
│  ┌──────────────┐    ┌────────────────┐      │             │  │
│  │Metrics/Trace │◄── │CheckpointMgr   │◄─────┘             │  │
│  └──────────────┘    └────────────────┘                    │  │
│                                   │ weights               │  │
│                                   ▼                       │  │
│                            ┌────────────┐                 │  │
│                            │WeightSink  │─────────────────┘  │
│                            └────────────┘                    │
└──────────────────────────────────────────────────────────┘
```

- **Config/CLI**: parses experiment config (from orchestrator or file) using Pydantic, validates hyperparameter bounds, and seeds RNGs.
- **ReplayClient**: wraps the `replay.v1` gRPC stub with retry/backoff, translating `SampleResponse` messages into typed tensors.
- **BatchPrefetcher**: runs in an asyncio Task, keeping N batches buffered (e.g., a bounded `asyncio.Queue`) so the training loop never stalls on network latency.
- **LearnerCore**: houses the algorithm implementation (initially PPO) with pluggable model/backbone registries.
- **CheckpointMgr**: serialises state dicts to `.safetensors` + JSON manifests, uploads to object storage, and records versions in Postgres via orchestrator API if needed.
- **WeightSink**: publishes the latest policy weights (and optional value net) to the weight distribution channel.

## Startup and control flow
1. **Initialisation**
   - Parse CLI flags / env vars for config URI, run ID, output bucket, replay endpoint, and weights endpoint.
   - Fetch experiment config from orchestrator REST (`/runs/{id}`) or local file. Validate using schema to enforce safe ranges.
   - Seed Python, NumPy, and torch RNGs using the orchestrator-provided seed base so runs are reproducible.
   - Build the model architecture from config (policy + value nets), instantiate optimizer (Adam), and load initial weights (fresh init or resume from latest checkpoint).

2. **Connections**
   - Dial `replay-go` via gRPC, using TLS in prod and plaintext locally. Warm up the sample stream with `SampleRequest` matching the engine capabilities (action/state sizes).
   - Register with the weight publisher (if present) and publish the starting weights version (e.g., `step=0`).
   - Kick off periodic heartbeats to orchestrator (`/runs/{id}/heartbeat`) with learner status (step count, last loss, checkpoint version).

3. **Training loop**
   - `BatchPrefetcher` continuously calls `Replay.Sample` with `batch_size = rollout_len * num_envs`, applying prioritised sampling parameters when enabled.
   - `LearnerCore` dequeues a batch, parses byte payloads into tensors using engine encoding metadata (float32 observations, discrete actions), and constructs advantages/targets.
   - Compute PPO losses (policy, value, entropy) across multiple minibatches/epochs per rollout, backpropagate, and step the optimizer.
   - Update running metrics (reward estimates, KL divergence) and emit them through Prometheus Gauges/Summaries.
   - Every K SGD updates (or wall-clock interval), trigger `CheckpointMgr.save(step, metrics)` which:
        1. Exports `policy.safetensors`, `value.safetensors`, optimizer state, RNG seed snapshot.
        2. Compresses shards with `zstd` if configured.
        3. Uploads to MinIO/GCS with retries and verifies checksum.
        4. Writes a `MANIFEST.json` containing URIs, versions, and metadata for the UI.
   - After each optimizer step or on a configurable throttle, push the latest weights to `WeightSink` so actors can refresh (e.g., send `WeightsBlob{version, payload_uri}` via gRPC or Redis pub-sub).

4. **Run control**
   - Respond to orchestrator tune commands: apply validated changes (e.g., adjust learning rate, entropy coef) without restarting the process. Changes are logged and recorded in metrics.
   - Support `pause`: drain outstanding optimizer steps, stop sampling, and wait until `resume` arrives (keeping heartbeats alive). `terminate` gracefully flushes metrics and exits after final checkpoint.

## Module layout
```
services/learner-py/
├── pyproject.toml        # poetry project metadata, dependencies (torch, grpcio, pydantic, prometheus-client, aiohttp)
├── learner/
│   ├── __init__.py
│   ├── config.py          # dataclasses/pydantic models, CLI parsing
│   ├── main.py            # entrypoint, wiring
│   ├── replay_client.py   # async gRPC client & prefetch queue
│   ├── datamodel.py       # tensor conversion helpers, capability parsing
│   ├── algo/
│   │   ├── __init__.py
│   │   ├── registry.py    # maps algo names → factory (PPO default)
│   │   ├── ppo.py         # PPO implementation (actor-critic)
│   │   └── networks.py    # policy/value modules, weight init utilities
│   ├── weights.py         # publishing to weight service / redis
│   ├── checkpoints.py     # save/load logic, manifest management
│   ├── control.py         # orchestrator client, heartbeat + tune listener
│   ├── metrics.py         # prometheus exporters, tracing setup
│   └── utils/
│       ├── logging.py
│       └── math.py        # GAE, normalization helpers
└── tests/
    ├── test_config.py
    ├── test_replay_client.py
    ├── test_ppo.py
    ├── test_checkpoints.py
    └── fixtures.py        # fake replay server, temp MinIO stub
```

## Replay integration details
- Use the generated Python stubs from `proto/replay/v1/replay.proto` (built via `poetry run make proto`).
- Sampling strategy:
  - Maintain `prefetch_depth` (e.g., 4 batches). Each prefetch Task issues a `Sample` request and pushes results to the queue.
  - If prioritized sampling is enabled, track TD-error estimates from learner updates and call `UpdatePriorities` asynchronously.
- Deserialisation:
  - Observations are decoded based on engine encoding metadata (supplied via run config from orchestrator). Provide helpers to convert `bytes` → `torch.Tensor` without extra copies when possible (use `torch.frombuffer`).
  - Maintain alignment with actor encodings: discrete actions stored as little-endian integers, continuous as packed `f32`.
- Backpressure: if the training loop falls behind, the queue blocks and naturally limits outstanding gRPC calls. We also expose metrics (`replay_sample_latency_seconds`, `prefetch_queue_size`).
- Failure handling: implement exponential backoff with jitter for gRPC errors; after repeated failures, escalate via orchestrator heartbeat status.

## Weight distribution
- MVP: write weights to MinIO (`runs/<run_id>/weights/latest.pt`) and publish version metadata to Redis (`weights:<exp_id>`). Actors poll Redis to detect updates and fetch from MinIO.
- Later: integrate with dedicated weight service (Go) via gRPC `PublishWeights` that streams binary payloads to connected actors, enabling push-based refresh.
- Always sign weights with `(run_id, step, sha256)` in manifest so actors can verify integrity before loading.

## Checkpoint strategy
- Store every N environment steps or M minutes (configurable). Keep last `keep_n` checkpoints and a rolling `best` (based on eval reward if available).
- Use `safetensors` for deterministic serialization and to avoid Python pickle security risks. Combine with `.json` manifest describing tensor shapes, dtype, and config snapshot.
- Support resume: on startup, check object store for `latest` symlink (JSON file pointing to most recent checkpoint), download shards concurrently, and load state dict.
- Include replay cursor metadata (last sampled transition IDs) to detect training gaps after restarts.

## Observability
- **Metrics** (Prometheus):
  - `learner_samples_total{status}`
  - `learner_sample_latency_seconds`
  - `learner_sgd_steps_total`
  - `learner_policy_loss`, `learner_value_loss`, `learner_entropy`
  - `learner_checkpoint_duration_seconds`
  - `learner_weights_publish_total`
- **Tracing**: wrap sampling and optimizer steps in `tracing` spans via `opentelemetry-sdk`, exporting to Tempo/Jaeger.
- **Logging**: structured JSON logs via `structlog`, enriched with run/experiment IDs, step numbers, and config hashes.

## Configuration surface
- CLI flags for endpoints (`--replay`, `--weights`, `--orchestrator`), object store bucket, run ID, log level.
- Config file (YAML/JSON) or orchestrator payload specifying algorithm hyperparameters, batching (rollout length, minibatch size, epochs), gradient clipping, learning rate schedule, entropy coefficient, value loss coef.
- Safety rails: enforce min/max values (from docs) and fail fast on invalid combos (e.g., minibatch size > rollout size).

## Failure modes & mitigations
- **Replay unavailable**: prefetcher backs off; learner reports degraded status. After threshold, pause optimizer and wait.
- **Object storage outage**: checkpoint uploads retried with exponential backoff; if still failing, mark run as `checkpoint_stalled` via orchestrator while continuing training (configurable).
- **Weights publish failure**: retain local copy and retry; actors continue using last known version.
- **OOM / NaN**: gradient nan detection triggers automatic LR reduction and optional rollback to previous checkpoint.
- **Process crash**: systemd/Kubernetes restarts container; resume from latest checkpoint manifest.

## Testing strategy
- **Unit tests**: deterministic RNG tests for GAE, PPO loss calculations (compare against analytical expectations), config validation.
- **Component tests**: spin up an in-process fake replay server serving scripted transitions, assert that sampling + training loop progress and priorities are updated.
- **Integration tests**: docker-compose target launching replay-go + learner with MinIO/miniredis, running a short PPO session to verify checkpoints and metrics endpoints.
- **Benchmarking**: optional profiling harness using `pytest-benchmark` to measure steps/sec with synthetic data.

## Rollout plan
1. Scaffold `services/learner-py` with poetry, generated protos, and stub modules.
2. Implement config + wiring + replay client; verify sampling against replay-go using existing integration tests as reference.
3. Land PPO core with CPU-only support; confirm checkpoints and metrics.
4. Hook into orchestrator control plane (heartbeats, tune commands).
5. Add weight publishing integration for actors.
6. Harden with observability, failure injection tests, and GPU support.

## Future extensions
- Multi-GPU / distributed (DDP) training.
- Learner ensembles or population-based training coordination.
- On-policy actors (learner also driving rollouts directly through engine) for algorithms like A2C.
- Rich evaluation scheduler tied to aggregator/leaderboard service.
