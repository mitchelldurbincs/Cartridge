# Weights Service (Go)

## Overview
- Provides a dedicated gRPC surface for publishing and consuming learner checkpoint metadata (`PublishWeights`, `StreamWeights`, `GetCurrent`).【F:proto/weights/weights.proto†L5-L36】【F:services/weights-go/internal/grpc/server.go†L31-L97】
- Persists the latest manifest per run in an in-memory registry (with bounded history) and fans updates out to streaming subscribers.【F:services/weights-go/internal/registry/registry.go†L22-L106】
- Ships as a standalone binary (`cmd/server`) that wires configuration, logging, the memory registry, and the gRPC server; mediation with Redis or external persistence is not yet implemented.【F:services/weights-go/cmd/server/main.go†L15-L95】

## Current Responsibilities & Implementation
- **Accept weight publications** – `PublishWeights` validates the learner’s (run_id, step, checksum, artifact_uri) payload, applies optional metadata/inline bytes, and records the version in the registry. Invalid inputs produce gRPC `InvalidArgument` errors sourced from shared validation logic.【F:services/weights-go/internal/service/service.go†L28-L72】【F:services/weights-go/internal/grpc/server.go†L45-L71】
- **Persist version metadata** – `internal/registry.MemoryRegistry` stores the latest snapshot plus a configurable history depth per run; persistence is in-process only (no Redis/Postgres backends yet).【F:services/weights-go/internal/registry/registry.go†L40-L71】
- **Serve current state** – `GetCurrent` returns the latest snapshot or `NotFound` when no weights exist for the run.【F:services/weights-go/internal/service/service.go†L74-L92】【F:services/weights-go/internal/grpc/server.go†L99-L121】
- **Stream updates** – `StreamWeights` registers watchers on the registry; the server brokers the channel into gRPC streams, supporting an optional “replay latest” option for late subscribers.【F:services/weights-go/internal/service/service.go†L94-L129】【F:services/weights-go/internal/registry/registry.go†L73-L104】【F:services/weights-go/internal/grpc/server.go†L73-L97】
- **Configuration** – `internal/config` reads environment variables for gRPC host/port, shutdown timeout, registry backend selection (currently only `memory`), history depth, and placeholders for Redis compatibility toggles. Flags exist but no Redis publisher implementation has landed yet, so all compatibility switches are effectively no-ops.【F:services/weights-go/internal/config/config.go†L8-L83】
- **Operational envelope** – The server exposes standard gRPC health endpoints (`grpc_health_v1`) and handles graceful shutdown on SIGINT/SIGTERM, with logs emitted via `zerolog`. Metrics/tracing hooks are not wired in yet.【F:services/weights-go/cmd/server/main.go†L57-L95】

## External Contracts
- **Proto definitions** – Manifests are described by `WeightsBlob` (run_id, step, checksum, artifact_uri, optional inline payload, metadata map, published_at). Requests mirror this shape so the service is transport-only; blob retrieval still happens directly from object storage referenced by `artifact_uri`.【F:proto/weights/weights.proto†L22-L35】
- **Go client stubs** – Generated code lives in `internal/pb` (`weights.pb.go`), enabling other Go services to integrate immediately.【F:services/weights-go/internal/pb/weights.pb.go†L1-L94】

## Observability & Testing
- Unit tests cover the in-memory registry’s upsert/current semantics and subscription delivery.【F:services/weights-go/internal/registry/registry_test.go†L1-L68】
- No Prometheus metrics, structured event logs, or tracing spans have been wired beyond the basic logging in the service layer; these remain future work.

## Known Gaps / Future Work
- **Persistence & durability** – Only in-memory registry is implemented; support for Redis/Postgres backends is planned (config scaffolding exists but is unused).
- **Redis compatibility bridge** – Configuration knobs (`WEIGHTS_REDIS_*`) are present but the bridge that mirrors publishes to Redis has not been implemented.
- **Inline payload handling** – Inline byte blobs are stored in memory; large payload support (chunking/deltas) remains a future enhancement.
- **Metrics & tracing** – Need to add Prometheus counters/histograms and instrumentation to follow the learner ➔ weights ➔ actor path.
- **Retention policies** – History depth is bounded but no API exists to list historical versions or enforce retention per run.

These items align with the roadmap described in the design narrative (redis mirroring, persistence, richer observability) and should be tracked before promoting the service beyond development use.
