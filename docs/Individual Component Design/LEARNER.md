# Learner Service (Python)

## Overview
- Coordinates policy optimisation using PyTorch PPO, sourcing experience from replay, persisting checkpoints locally, publishing new weights through Redis, and emitting heartbeats to the orchestrator.【F:services/learner-py/learner/main.py†L35-L114】【F:services/learner-py/learner/core.py†L35-L202】
- Built as a single-tenant process: every learner instance owns one configuration bundle (run id, algorithm hyperparameters, endpoints) parsed via Pydantic with CLI overrides for quick tuning.【F:services/learner-py/learner/config.py†L14-L167】
- Runs entirely asynchronously on top of `asyncio`, with background prefetch of replay batches and cooperative shutdown that closes long-lived sockets cleanly.【F:services/learner-py/learner/replay_client.py†L36-L173】【F:services/learner-py/learner/main.py†L109-L120】

## Responsibilities & Scope
- **Sampling**: maintain a resilient gRPC connection to replay, prefetching batches into an `asyncio.Queue` with exponential backoff and failure thresholds.【F:services/learner-py/learner/replay_client.py†L36-L169】
- **Optimisation**: execute PPO updates against the sampled batches, including GAE calculation, gradient clipping, loss logging, and device placement control.【F:services/learner-py/learner/algo/ppo.py†L18-L177】
- **Checkpointing & weights**: write `.safetensors` checkpoints plus manifests to a configured filesystem path, keep the most recent N, and publish weight metadata to Redis subscribers as part of the checkpoint cadence.【F:services/learner-py/learner/checkpoints.py†L40-L200】【F:services/learner-py/learner/weights.py†L23-L107】
- **Control plane**: send structured heartbeat payloads to the orchestrator; command ingestion (pause/tune) is not yet implemented and pending future work.【F:services/learner-py/learner/control.py†L27-L174】
- **Observability**: expose Prometheus metrics on port 9001, structured JSON logs via `structlog`, and queue depth telemetry for replay health checks.【F:services/learner-py/learner/metrics.py†L12-L45】【F:services/learner-py/learner/replay_client.py†L105-L115】【F:services/learner-py/learner/utils/logging.py†L11-L46】

Out of scope for the current implementation: multi-run scheduling, distributed data-parallel training, adaptive hyperparameter tuning, and alternative weight distribution backends.

## Component Architecture
```
┌──────────────────────────────────────────────────────────────┐
│                        learner/main.py                       │
│                                                              │
│  parse_args + load_config  ──►  configure_logging            │
│             │                                    │           │
│             ▼                                    ▼           │
│  ReplayClient ──▶ asyncio prefetch queue ──▶ LearnerCore ──▶ MetricsRegistry │
│             │                                    │    │      │
│             │                                    │    └────► WeightPublisher (Redis) │
│             │                                    │
│             │                                    └───► CheckpointManager (fs) │
│             │                                                    │           │
│             └───────────────────────────────────────────────► ControlClient │
└──────────────────────────────────────────────────────────────┘
```

- **ReplayClient**: wraps the `replay.v1` stub, maintaining a bounded queue (`prefetch_depth`) of decoded `TransitionBatch` objects. Failures are retried with Tenacity; too many consecutive errors stop the learner to surface incidents quickly.【F:services/learner-py/learner/replay_client.py†L36-L169】
- **LearnerCore**: pulls from the queue, runs PPO updates, updates metrics, triggers checkpoints/weight publishes, and pushes heartbeat telemetry through a callback supplied by `main.py`.【F:services/learner-py/learner/core.py†L63-L202】
- **CheckpointManager** / **WeightPublisher**: persist artifacts to disk under `CheckpointConfig.bucket` and publish checkpoint metadata (`step`, `checksum`, URI) to Redis subscribers; old checkpoints are trimmed asynchronously to honour `keep_last`.【F:services/learner-py/learner/checkpoints.py†L43-L171】【F:services/learner-py/learner/weights.py†L23-L107】
- **ControlClient**: maintains a single shared `aiohttp` session and posts heartbeats (`/runs/{run_id}/heartbeat`) with loop statistics and checkpoint counters.【F:services/learner-py/learner/control.py†L27-L140】
- **MetricsRegistry**: launches a Prometheus HTTP exporter in a background task when the training loop starts, exposing counters/gauges/histograms for sampling, SGD steps, and checkpoint/write operations.【F:services/learner-py/learner/metrics.py†L12-L38】

## Startup & Lifecycle
1. **Argument parsing & overrides** – `parse_args` requires a config path and captures optional `key=value` overrides (dot-path syntax).【F:services/learner-py/learner/config.py†L96-L108】
2. **Configuration loading** – `load_config` reads YAML/JSON, applies overrides, and validates against `LearnerConfig`, enforcing PPO-only algorithms and a `minibatch_size ≤ rollout_size` constraint.【F:services/learner-py/learner/config.py†L14-L167】
3. **Logging & seeding** – `configure_logging` enables structured JSON output and honours `LOG_LEVEL`; `_seed_everything` seeds `random`, NumPy, CPU and (optional) CUDA RNGs for reproducibility.【F:services/learner-py/learner/utils/logging.py†L11-L46】【F:services/learner-py/learner/main.py†L27-L52】
4. **Component wiring** – instantiate metrics, weight publisher, checkpoint manager, control client, and replay client. A heartbeat callback is bound to the learner to report progress after each update.【F:services/learner-py/learner/main.py†L54-L95】
5. **Training loop** – `LearnerCore.run()` owns the async context of the replay client, performs PPO updates until `shutdown` is requested, emits metrics/logs, checkpoints, weight publishes, and heartbeats.【F:services/learner-py/learner/core.py†L63-L188】
6. **Shutdown** – on cancellation or error, `main.py` stops the learner, closes the control client session, drains replay prefetch queues, and closes Redis connections.【F:services/learner-py/learner/main.py†L99-L120】【F:services/learner-py/learner/replay_client.py†L74-L104】【F:services/learner-py/learner/weights.py†L99-L107】

## Replay Integration
- Sample requests are executed via the async gRPC stub (`ReplayStub.Sample`) with retries capped at three attempts per call and a global limit of ten consecutive failures before aborting the learner.【F:services/learner-py/learner/replay_client.py†L116-L169】
- Responses are converted to tensors in `sample_response_to_batch`, supporting both float32 and discrete (uint8) action encodings and defaulting log-prob/value metadata to zero when missing (for legacy actors).【F:services/learner-py/learner/replay.py†L98-L195】
- Queue depth / capacity is surfaced for diagnostics and included in heartbeat notes so operators can spot starvation quickly.【F:services/learner-py/learner/replay_client.py†L105-L115】【F:services/learner-py/learner/main.py†L63-L85】

## Training Loop & Algorithm
- The algorithm registry currently exposes a single implementation: PPO backed by `ActorCriticNetwork`, stochastic categorical policies, Adam optimisation, advantage normalisation, and gradient clipping.【F:services/learner-py/learner/algo/__init__.py†L1-L17】【F:services/learner-py/learner/algo/ppo.py†L18-L177】
- Advantages/returns are provided either by the actors or computed on the fly using the shared `compute_gae` helper; bootstrapping tolerates batches that omit the final value prediction.【F:services/learner-py/learner/algo/ppo.py†L145-L172】【F:services/learner-py/learner/utils/math.py†L18-L76】
- `LearnerCore` records loss scalars to Prometheus, computes throughput (samples/sec), and logs progress every ten steps or thirty seconds to avoid log spam.【F:services/learner-py/learner/core.py†L63-L155】

## Checkpoints & Weight Publishing
- Checkpoints are stored beneath `<bucket>/step_<N>/`, containing `weights.safetensors`, `optimizer.pt`, and a JSON manifest with metadata and SHA256 checksums; older checkpoints beyond `keep_last` are removed in FIFO order.【F:services/learner-py/learner/checkpoints.py†L43-L198】
- Weight publishes piggyback on checkpoint completion: the learner sends `{step, checksum, uri}` JSON messages to a Redis channel. Redis is the only backend wired today; other transports will need new handlers in `WeightPublisher`’s switch.【F:services/learner-py/learner/weights.py†L23-L98】

## Control Plane Integration
- Heartbeats carry the latest optimisation step, aggregate loss, samples/sec, checkpoint version, outstanding command ids (placeholder for future command handling), and queue depth context strings. Timeouts or HTTP failures bubble up so the orchestrator can treat them as degraded runs.【F:services/learner-py/learner/control.py†L74-L174】
- Command ingestion (pause/resume/tune) and orchestrator-driven overrides are not implemented yet; `LearnerCore.update_pending_commands` exists as a stub for future integration.【F:services/learner-py/learner/core.py†L226-L229】

## Observability
- **Metrics (Prometheus)**: `learner_samples_total{status}`, `learner_sample_latency_seconds`, `learner_sgd_steps_total`, `learner_policy_loss`, `learner_value_loss`, `learner_entropy`, `learner_checkpoint_duration_seconds`, `learner_weights_publish_total`. The exporter binds to TCP port `9001` by default.【F:services/learner-py/learner/metrics.py†L12-L38】
- **Logging**: JSON-formatted, structured via `structlog`, seeded with ISO timestamps and enriched with run-level metadata; log level is configurable through `LOG_LEVEL`.【F:services/learner-py/learner/utils/logging.py†L11-L46】【F:services/learner-py/learner/main.py†L39-L52】
- **Telemetry surfaces**: heartbeats expose replay queue utilisation, checkpoint cadence, and outstanding commands for dashboards; warnings fire for slow batch fetches or missing actor metadata.【F:services/learner-py/learner/core.py†L135-L147】【F:services/learner-py/learner/replay.py†L154-L167】

## Configuration Surface
- `ReplayConfig`: endpoint, TLS toggle, prefetch depth, batch size—controls prefetch queue size and gRPC target.【F:services/learner-py/learner/config.py†L14-L20】
- `TrainingConfig`: rollout size, learning rate, deterministic seed, device string, observation/action dimensions (provided by the orchestrator or experiment spec).【F:services/learner-py/learner/config.py†L36-L44】
- `AlgorithmConfig`: PPO hyperparameters, validated to match the shipped implementation (alternate algorithms rejected).【F:services/learner-py/learner/config.py†L23-L34】【F:services/learner-py/learner/config.py†L79-L92】
- `CheckpointConfig`: filesystem bucket (path), interval in steps, retention count.【F:services/learner-py/learner/config.py†L47-L52】
- `WeightPublisherConfig`: backend identifier, Redis endpoint, channel name.【F:services/learner-py/learner/config.py†L55-L60】
- `ControlConfig`: orchestrator HTTP endpoint, run id, heartbeat cadence.【F:services/learner-py/learner/config.py†L63-L66】
- Command-line overrides mutate nested keys via dot notation (`training.learning_rate=1e-4`) before validation.【F:services/learner-py/learner/config.py†L111-L153】

## Failure Modes & Mitigations
- **Replay outages**: Tenacity retries individual sample calls (backoff and stop-after-attempt safeguards). After ten consecutive failures the learner surfaces a fatal error to trigger operator intervention.【F:services/learner-py/learner/replay_client.py†L116-L169】
- **Heartbeat failures**: timeouts and HTTP errors bubble back to the caller; connection resets trigger a session reset and the next loop iteration retries with a fresh `aiohttp` session.【F:services/learner-py/learner/control.py†L74-L158】
- **Checkpoint / weight failures**: exceptions during disk writes or Redis publish propagate, causing the learner loop to log errors and unwind so the orchestrator can react (e.g., restart).【F:services/learner-py/learner/core.py†L164-L197】【F:services/learner-py/learner/weights.py†L43-L76】
- **Non-finite losses**: PPO rejects NaN/Inf losses and raises `ValueError`, leaving the run in a failed state rather than silently propagating corrupted weights.【F:services/learner-py/learner/algo/ppo.py†L95-L106】

## Testing Strategy
- Unit tests cover configuration validation, PPO math (including GAE helper), and replay conversion edge cases (`test_config.py`, `test_ppo.py`, `test_utils_math.py`, `test_replay_integration.py`). These run under `pytest` and exercise both success paths and key validation failures.
- Integration and performance automation is planned but not yet wired; existing scaffolding (`test_integration_validation.py`) focuses on import-time checks until full end-to-end harnesses are available.

## Future Extensions
- Add orchestrator command handling (pause/resume/tune) in `ControlClient` and `LearnerCore.update_pending_commands`.
- Support additional weight backends (gRPC publisher, direct file watching) and remote checkpoint sinks (MinIO/GCS) once the storage strategy stabilises.
- Introduce distributed or multi-GPU learners and richer replay prioritisation once the single-process MVP is hardened.
