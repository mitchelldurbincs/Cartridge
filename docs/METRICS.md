# Metrics Inventory

This document summarizes the Prometheus metrics that Cartridge emits today, grouped by
service. Each table lists the metric name, type, label set, what the metric captures,
and the code path that records it.

## Learner Service (`services/learner-py`)

The learner exposes its Prometheus registry on port `9001` and tracks replay
interactions, training loop progress, and checkpointing outcomes.

| Metric | Type | Labels | Description | Emitted from |
| --- | --- | --- | --- | --- |
| `learner_sample_attempts_total` | Counter | _none_ | Counts every attempt to pull a batch from replay over gRPC. | Replay client before invoking `Sample` in `replay_client.py`.
| `learner_sample_results_total` | Counter | `result` (`success`, `error`, `ok`) | Breaks down replay sampling outcomes: successes and transport errors from the prefetch loop, plus end-to-end batch fetches in the training loop. | Replay client success/error paths and learner core `_fetch_batch`.
| `learner_sample_latency_seconds` | Histogram | _none_ | Measures latency to retrieve a batch from replay. | Training loop `track_sample_latency` context manager in `core.py`.
| `learner_sgd_steps_total` | Counter | _none_ | Total number of optimizer steps applied by the learner. | `_record_update` in `core.py`.
| `learner_policy_loss` | Gauge | _none_ | Last observed policy loss from the optimizer. | `_record_update` in `core.py`.
| `learner_value_loss` | Gauge | _none_ | Last observed value loss from the optimizer. | `_record_update` in `core.py`.
| `learner_entropy` | Gauge | _none_ | Current policy entropy reported by the algorithm. | `_record_update` in `core.py`.
| `learner_replay_queue_depth` | Gauge | _none_ | Prefetch queue depth for locally buffered replay batches. | Replay client initialization, shutdown, dequeue, and background loop.
| `learner_heartbeat_success` | Gauge | _none_ | Indicates whether the most recent heartbeat to the orchestrator succeeded (`1`) or failed (`0`). | Control client heartbeat lifecycle in `control.py`.
| `learner_checkpoint_duration_seconds` | Histogram | _none_ | Duration distribution for checkpoint save operations. | Checkpointing branch in `_maybe_checkpoint`.
| `learner_weights_publish_total` | Counter | _none_ | Total number of weight bundles successfully published after checkpoints. | `_maybe_checkpoint` after publishing to the weight service.

## Orchestrator Service (`services/orchestrator-go`)

The orchestrator instruments HTTP request handling, run lifecycle transitions, and
health monitoring. All collectors are registered with the shared Prometheus registry.

| Metric | Type | Labels | Description | Emitted from |
| --- | --- | --- | --- | --- |
| `orchestrator_heartbeat_latency_seconds` | Histogram | _none_ | Time between consecutive heartbeats for a run, used to flag delays in learner reporting. | Recorded when processing learner heartbeats in `service/orchestrator.go`.
| `orchestrator_http_request_duration_seconds` | Histogram | `method`, `route`, `status` | Latency for HTTP API handlers, segmented by verb, mux route, and status code. | Wrapped around each request via `observeRequest` in `http/server.go`.
| `orchestrator_run_state_transitions_total` | Counter | `from`, `to` | Counts run state transitions observed by the orchestrator state machine. | Incremented when recording run transitions in `service/orchestrator.go`.
| `orchestrator_health_events_total` | Counter | `type`, `severity` | Tracks emitted health monitoring events such as stale or unresponsive heartbeats. | Health monitor callbacks `markStale`/`markUnresponsive` in `health/monitor.go`.

## Engine Service (`services/engine-rust`)

The engine exposes Prometheus metrics for registry activity, RPC lifecycles, cache
behaviour, and buffer pool health via the shared `metrics` crate.

| Metric | Type | Labels | Description | Emitted from |
| --- | --- | --- | --- | --- |
| `engine_registry_registrations_total` | Counter | `env_id` | Counts game registrations into the global registry. | `register_game` in `engine-core/src/registry.rs`. |
| `engine_registry_games` | Gauge | _none_ | Current number of games registered with the engine. | Updated in `engine-core/src/registry.rs` and `engine-server/src/registry_init.rs`. |
| `engine_rpc_requests_total` | Counter | `method` | Tracks incoming RPC requests by method. | Beginning of each gRPC handler in `engine-server/src/service.rs`. |
| `engine_rpc_success_total` | Counter | `method` | Successful RPC responses by method. | Success paths in `engine-server/src/service.rs`. |
| `engine_rpc_failures_total` | Counter | `method`, `error` | Categorizes RPC failures by method and error condition. | Error paths in `engine-server/src/service.rs`. |
| `engine_rpc_latency_seconds` | Histogram | `method` | End-to-end latency for each RPC method. | Recorded just before returning from handlers in `engine-server/src/service.rs`. |
| `engine_game_cache_hits_total` | Counter | `method` | Cache hits when reusing game instances per RPC method. | Cache lookup in `reset`/`step` handlers in `engine-server/src/service.rs`. |
| `engine_game_cache_misses_total` | Counter | `method` | Cache misses when a game instance must be created. | Cache miss handling in `reset`/`step` handlers in `engine-server/src/service.rs`. |
| `engine_game_cache_entries` | Gauge | _none_ | Snapshot of cached game instances. | After cache access in `reset` and `step` within `engine-server/src/service.rs`. |
| `engine_buffer_pool_borrows_total` | Counter | `buffer` (`state`, `obs`, `action`) | Counts buffer borrows from the pool by type. | `get_*_buffer` methods in `engine-server/src/buffers.rs`. |
| `engine_buffer_pool_returns_total` | Counter | `buffer` (`state`, `obs`, `action`) | Counts buffer returns to the pool by type. | `return_*_buffer` methods in `engine-server/src/buffers.rs`. |
| `engine_buffer_pool_available` | Gauge | `buffer` (`state`, `obs`, `action`) | Available buffer count per pool. | Recorded whenever buffer availability changes in `engine-server/src/buffers.rs`. |

## Actor Service (`services/actor-rust`)

The actor runs inference loops against the engine, buffers experience locally, and
streams batches to replay. Metrics cover episode lifecycles and replay flush health.

| Metric | Type | Labels | Description | Emitted from |
| --- | --- | --- | --- | --- |
| `actor_episode_results_total` | Counter | `result`, `env_id` | Counts episodes by outcome (`success` or `error`). | Success/error paths in `Actor::run`. |
| `actor_episode_duration_seconds` | Histogram | `result`, `env_id` | Measures end-to-end duration of each episode with outcome segmentation. | Recorded after each `run_episode` invocation in `Actor::run`. |
| `actor_episode_last_steps` | Gauge | `env_id` | Number of steps in the most recently completed episode. | Success path in `Actor::run`. |
| `actor_episode_last_return` | Gauge | `env_id` | Total accumulated reward from the most recent successful episode. | Success path in `Actor::run`. |
| `actor_transitions_buffered` | Gauge | `env_id` | Current number of transitions awaiting flush to replay. | Updated when buffering transitions and after flushes in `Actor::run_episode`/`flush_buffer`. |
| `actor_transitions_flushed_total` | Counter | `env_id` | Counts individual transitions successfully delivered to replay. | Success path in `Actor::flush_buffer`. |
| `actor_flush_results_total` | Counter | `result`, `env_id` | Tracks replay flush attempts by result (`success` or `error`). | Success/error branches in `Actor::flush_buffer`. |


## Replay Service (`services/replay-go`)

The replay service emits counters and histograms for write, sample, and housekeeping
RPCs. Metrics are registered on startup and exported via the dedicated metrics HTTP
server (port `9090` by default).

| Metric | Type | Labels | Description | Emitted from |
| --- | --- | --- | --- | --- |
| `replay_store_requests_total` | Counter | `method` (`single`, `batch`), `result` (`success`, `error`, `invalid_argument`) | Counts StoreTransition and StoreBatch RPCs by method and outcome. | `RecordStore` from `StoreTransition` and `StoreBatch` in `internal/service/replay.go`. |
| `replay_store_duration_seconds` | Histogram | `method` (`single`, `batch`) | Latency distribution for storing transitions individually or in batches. | `RecordStore` in `internal/metrics/metrics.go` called by store handlers. |
| `replay_store_transitions_total` | Counter | `method` (`single`, `batch`) | Number of transitions persisted via StoreTransition/StoreBatch. | `RecordStore` in `internal/metrics/metrics.go`. |
| `replay_sample_requests_total` | Counter | `prioritized` (`true`, `false`), `result` (`success`, `error`, `invalid_argument`) | Counts Sample RPCs split by prioritized sampling flag and outcome. | `RecordSample` from `Sample` in `internal/service/replay.go`. |
| `replay_sample_duration_seconds` | Histogram | `prioritized` (`true`, `false`) | Latency distribution for sampling transitions. | `RecordSample` in `internal/metrics/metrics.go`. |
| `replay_sample_transitions_total` | Counter | `prioritized` (`true`, `false`) | Total transitions returned by Sample RPCs. | `RecordSample` in `internal/metrics/metrics.go`. |
| `replay_priority_updates_total` | Counter | `result` (`success`, `error`, `invalid_argument`) | Counts UpdatePriorities RPCs by outcome. | `RecordPriorityUpdate` from `UpdatePriorities` in `internal/service/replay.go`. |
| `replay_priority_transitions_total` | Counter | _none_ | Number of transitions whose priorities were updated successfully. | `RecordPriorityUpdate` in `internal/metrics/metrics.go`. |
| `replay_clear_requests_total` | Counter | `result` (`success`, `error`) | Counts Clear RPC invocations by outcome. | `RecordClear` from `Clear` in `internal/service/replay.go`. |
| `replay_clear_transitions_total` | Counter | `result` (`success`, `error`) | Number of transitions removed during Clear RPCs. | `RecordClear` in `internal/metrics/metrics.go`. |


## Adding New Metrics

When adding instrumentation to a service, prefer updating the existing collector class
or module for that service so that registration, exporter lifecycle, and testing stay
centralized. Use consistent naming (`<service>_<signal>`) and document the metric in this
file to keep operations visibility up to date.
