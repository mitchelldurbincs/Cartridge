# Replay Service (Go)

## Overview
The Go replay service exposes the `replay.v1` gRPC API and forwards each request to a pluggable storage backend. The default `MemoryBackend` keeps per-environment indices, ordered timestamps, and episode groupings so it can serve uniform or prioritized samples while enforcing a global capacity limit.【F:services/replay-go/internal/service/replay.go†L1-L211】【F:services/replay-go/internal/storage/memory.go†L1-L240】

## Components
- **gRPC surface (`internal/service`)**: Implements every protobuf method (StoreTransition, StoreBatch, Sample, GetStats, UpdatePriorities, Clear). Each handler validates inputs, converts between proto structs and storage structs, and maps backend errors onto gRPC status codes.【F:services/replay-go/internal/service/replay.go†L1-L211】
- **Storage interface**: Defines the `Transition` schema, sampling configuration, statistics payload, and the methods that any backend must implement. This keeps the service layer agnostic to persistence details.【F:services/replay-go/internal/storage/interface.go†L1-L61】
- **In-memory backend**: Tracks transitions in maps plus auxiliary indices (per-episode, per-env, timestamp ordering) to support eviction, sampling, stats, and priority updates. It generates IDs, fills timestamps, and evicts oldest data when `maxSize` is exceeded.【F:services/replay-go/internal/storage/memory.go†L1-L240】【F:services/replay-go/internal/storage/memory.go†L240-L384】

## Request handling highlights
- **StoreTransition / StoreBatch**: Convert proto messages into storage transitions, auto-populating IDs and timestamps if they are missing, then update the indices and enforce capacity.【F:services/replay-go/internal/service/replay.go†L17-L74】【F:services/replay-go/internal/storage/memory.go†L25-L115】
- **Sample**: Accepts uniform or prioritized sampling with optional timestamp windows. The backend builds a candidate set, applies weighting (including alpha-scaling for priorities), and returns transitions plus importance weights.【F:services/replay-go/internal/service/replay.go†L76-L133】【F:services/replay-go/internal/storage/memory.go†L117-L240】【F:services/replay-go/internal/storage/memory.go†L320-L432】
- **GetStats & Clear**: Provide observability and housekeeping using the backend's aggregated counters and time index. `Clear` can drop old data before a timestamp while respecting a "keep last N" guardrail.【F:services/replay-go/internal/service/replay.go†L135-L211】【F:services/replay-go/internal/storage/memory.go†L177-L320】

## Testing hooks
`memory_test.go` exercises the in-memory backend for storing, sampling, and eviction behaviour, while `integration_test.go` runs higher-level gRPC scenarios against the compiled server binary.【F:services/replay-go/internal/storage/memory_test.go†L1-L280】【F:services/replay-go/integration_test.go†L1-L205】
