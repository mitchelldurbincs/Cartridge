# Weights Service Redis Integration Implementation Plan

## Background

The Go weights service already validates learner publications, stores the latest version in a pluggable registry, and fans updates out to subscribers via `StreamWeights` and `GetCurrent`. The service currently ships with an in-memory registry and configuration toggles for Redis mirroring, but the Redis compatibility bridge described in the component design has not yet been implemented. Redis continues to be the discovery surface for existing learners and actors, so mirroring the authoritative registry to Redis is required during the migration from Redis-centric workflows to the new gRPC API.

## Goals

1. Provide a Redis-backed compatibility layer that mirrors version metadata emitted through `PublishWeights`, maintaining backwards compatibility for existing learners/actors while gRPC clients are adopted.
2. Reuse the service's registry abstraction so that Redis mirroring remains optional and can be disabled once all consumers move to gRPC.
3. Maintain operational safety: no loss of published weight metadata, resilience to transient Redis failures, and clear observability around the mirroring path.

## Assumptions and Dependencies

- Proto surface and core service logic are already in place (`proto/weights/weights.proto`, `internal/service`).
- Runtime configuration exposes Redis connection details and feature toggles (`internal/config`).
- The long-term architecture still expects MinIO/GCS to store artifacts, with Redis holding only lightweight manifests.
- Redis cluster is provisioned externally (e.g., Memorystore) and reachable from the weights service deployment.

## Deliverables

1. **Redis Publisher Package** (`internal/redis/publisher.go`)
   - Connection management (with pooling and health checking) driven by `config.Redis`.
   - Serialization of `service.VersionSnapshot` into a stable payload (JSON manifest mirroring existing learner fields).
   - `Publish(ctx, snapshot)` method with retry/backoff for transient errors.
   - Structured logging and counters for publish attempts/successes/failures.

2. **Registry Mirroring Hook**
   - Extend `service.Service` to accept an optional Redis publisher dependency.
   - Mirror successful `Publish` calls to Redis when `config.Compatibility.MirrorToRedis` is `true`.
   - Ensure ordering guarantees: registry write must complete before mirroring; Redis errors should be surfaced via logs/metrics but must not roll back the canonical registry.

3. **Redis-backed Registry Implementation (Optional backend)**
   - Implement `internal/registry/redis.go` to persist snapshots in Redis hashes (e.g., `weights:<run_id>`).
   - Provide bounded history via sorted sets or list trimming to satisfy rollback requirements.
   - Wire registry factory in `cmd/server/main.go` to select between memory and Redis backends based on `WEIGHTS_REGISTRY_BACKEND`.

4. **Configuration & Wiring**
   - Update `cmd/server/main.go` to construct Redis clients (for publisher and optional registry) when enabled.
   - Validate configuration at startup (e.g., refuse to enable mirroring without Redis enabled, warn when Redis DSN missing).
   - Add graceful shutdown logic to close Redis connections.

5. **Observability & Resilience**
   - Counters: `weights_redis_publish_total{status=success|failure}`.
   - Histograms: `weights_redis_publish_duration_seconds`.
   - Structured logs for connection lifecycle and publish outcomes.
   - Retry policy (exponential backoff capped) with circuit breaking or fast-fail after configured threshold.
   - Optional dead-letter channel (log + metric) when Redis errors persist beyond retry budget.

6. **Testing & Validation**
   - Unit tests for Redis publisher serialization and error handling (use `miniredis` or gomock).
   - Service-level tests ensuring mirroring occurs when enabled and skipped otherwise.
   - Integration test harness launching Redis (e.g., via Docker or `miniredis`) to verify end-to-end publish → Redis flow.
   - Load tests or benchmarks to validate throughput/latency targets against Redis (`weights_published_total` parity with learner expectations).

7. **Operational Readiness**
   - Documentation updates describing configuration flags, expected Redis schema, and migration guidance.
   - Deployment manifests (Kubernetes) updated with Redis connection env vars and secrets.
   - Runbook entries: detecting stale mirrors, clearing corrupted manifests, failover strategies.

## Iterative Delivery Plan

1. **Phase A – Foundations**
   - Introduce Redis publisher package with exhaustive unit tests.
   - Integrate optional publisher into `service.Service` guarded by configuration.
   - Expose metrics/logging for publish outcomes.

2. **Phase B – Redis Registry Backend**
   - Implement Redis registry storage adhering to `service.Registry` interface.
   - Provide migration script or fallback to populate Redis from in-memory state during rollout.
   - Extend configuration wiring and add integration tests.

3. **Phase C – Hardening & Observability**
   - Add retries/backoff, circuit breaking, and shutdown handling.
   - Expand metrics/tracing coverage to satisfy SLO monitoring.
   - Update operations documentation and Kubernetes manifests.

4. **Phase D – Rollout & Validation**
   - Deploy in shadow mode (mirror only) to validate Redis payloads against existing learner expectations.
   - Enable Redis registry backend in staging, verify actor compatibility, and compare metrics against baseline.
   - Promote to production once parity achieved and error budgets maintained.

## Acceptance Criteria

- Redis mirroring can be toggled on/off without restarts beyond configuration reloads.
- Successful `PublishWeights` calls result in Redis entries matching the canonical registry.
- Transient Redis outages degrade gracefully (publish succeeds, mirror retries/logs).
- Metrics and logs provide enough signal for on-call debugging during rollout.
- Documentation and manifests reflect new configuration knobs and operational steps.
