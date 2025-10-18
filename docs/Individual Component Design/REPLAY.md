# Replay Service (Go)

## Overview
- Implements the `replay.v1.Replay` gRPC API and delegates persistence to a storage backend provided at construction (`NewReplayService`).【F:services/replay-go/internal/service/replay.go†L10-L47】
- Ships with a single in-memory backend that keeps transitions in a bounded FIFO structure backed by a ring buffer, providing best-effort uniform sampling and lightweight stats.【F:services/replay-go/internal/storage/memory.go†L12-L111】
- Priorities, timestamp filtering, and per-environment eviction are stubbed in the API surface but currently handled as pass-through operations; advanced sampling modes are not yet implemented.

## Components
- **Service layer (`internal/service`)** – Exposes the gRPC methods: `StoreTransition`, `StoreBatch`, `Sample`, `GetStats`, `UpdatePriorities`, and `Clear`. Each handler converts between protobuf and storage types and maps backend failures to gRPC status codes.【F:services/replay-go/internal/service/replay.go†L19-L151】
- **Storage contract (`internal/storage/interface.go`)** – Defines the `Transition` struct, sampling configuration, stats payload, and the `Backend` interface consumed by the service. Backends must implement store, batch store, sample, stats, priority update, clear, and close semantics.【F:services/replay-go/internal/storage/interface.go†L1-L61】
- **Memory backend (`internal/storage/memory.go`)** – Maintains transitions in maps keyed by ID plus helper indexes (episode list, env list, timestamp-sorted slice) and enforces a global `maxSize`. New transitions are assigned UUIDs, missing timestamps default to `time.Now`, and priorities default to `1.0`. Eviction removes the oldest entries once capacity is exceeded.【F:services/replay-go/internal/storage/memory.go†L12-L115】【F:services/replay-go/internal/storage/memory.go†L197-L238】

## Request Behaviour
- **StoreTransition / StoreBatch** – Convert incoming protobuf transitions to storage objects, fill default ID/timestamp/priority when absent, insert into the backend, and bubble any errors back in the response payload. Batch requests return IDs for stored transitions and include partial progress if the backend fails mid-way.【F:services/replay-go/internal/service/replay.go†L49-L99】
- **Sample** – Accepts a `SampleConfig` (batch size, optional env filter, prioritized flag, priority alpha). The memory backend collects candidate transitions matching env/timestamp filters, then:
  - performs uniform sampling when `Prioritized` is false (random subset or full list depending on availability),
  - attempts proportional sampling when `Prioritized` is true (raising an error if no candidates exist). Returned weights are either importance weights (prioritized) or `1.0` for uniform.【F:services/replay-go/internal/service/replay.go†L101-L143】【F:services/replay-go/internal/storage/memory.go†L117-L211】【F:services/replay-go/internal/storage/memory.go†L239-L396】
  - The service also queries `GetStats` to approximate `total_available` for the client.
- **GetStats** – Reports total transitions, episodes, per-environment counts, approximate storage bytes, and oldest/newest timestamps based on the indexed data.【F:services/replay-go/internal/service/replay.go†L145-L170】
- **UpdatePriorities** – Validates matching list lengths, then mutates stored priorities in-place. The current backend simply overwrites the `Priority` field without additional bookkeeping; callers should not expect downstream sampling changes beyond the power-law weighting described above.【F:services/replay-go/internal/service/replay.go†L172-L189】【F:services/replay-go/internal/storage/memory.go†L213-L237】
- **Clear** – Deletes transitions filtered by environment and/or timestamp; when `keep_last_n` is set, the backend retains the most recent items matching the filter and removes the remaining oldest entries. The service returns the number cleared and the remaining count derived from `GetStats`. A request with no filters effectively resets the buffer once older-than windows are satisfied.【F:services/replay-go/internal/service/replay.go†L191-L235】【F:services/replay-go/internal/storage/memory.go†L239-L288】

## Storage Details (MemoryBackend)
- **Indexes**: `transitions` (`map[id]*Transition`), `episodes` (`map[episode][]id`), `envIndex` (`map[env][]id`), and `timeIndex` (`[]id` sorted by timestamp) to support filtering and eviction.【F:services/replay-go/internal/storage/memory.go†L16-L115】
- **Capacity management**: `evictIfNeeded` removes the oldest transitions (by `timeIndex`) when the total exceeds `maxSize`. Evictions update all indexes to keep them consistent.【F:services/replay-go/internal/storage/memory.go†L197-L232】
- **Sampling helpers**: `getCandidates` filters by env and timestamp; `uniformSample` performs Fisher–Yates shuffles; prioritized sampling computes scaled priorities with alpha, normalizes them, and samples without replacement, falling back to uniform when probabilities collapse.【F:services/replay-go/internal/storage/memory.go†L286-L396】
- **Stats**: computed on demand from current maps; storage bytes are estimated by summing payload lengths with a constant overhead to approximate metadata cost.【F:services/replay-go/internal/storage/memory.go†L139-L188】

## Observability & Testing
- No metrics or logging are emitted from the service layer; operators rely on stats RPCs and external instrumentation for visibility.
- Unit tests validate storage behaviour (store, sample, update priorities, clear, eviction) in `internal/storage/memory_test.go`; integration tests exercise the gRPC surface end-to-end in `integration_test.go`.【F:services/replay-go/internal/storage/memory_test.go†L1-L290】【F:services/replay-go/integration_test.go†L1-L205】

## Known Gaps / Future Work
- Prioritized sampling does not currently maintain auxiliary structures required for efficient large-scale replay; it recalculates probabilities on each request.
- Timestamp windows rely on per-transition timestamps but no wall-clock synchronisation; production variants should align with actor clock skew tolerances.
- Additional backends (e.g., Redis, disk-backed) can plug into the `Backend` interface; this document will need revisiting when persistence strategies evolve.
