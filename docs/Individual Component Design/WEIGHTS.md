# Weights Service (Go)

## Overview

The weights service is the Go component that bridges trained policy artifacts from the learner to the fleet of actors, completing the reinforcement-learning feedback loop alongside the existing Rust, Go, and Python services.

It turns the current learner-to-actor weight hand-off into a dedicated gRPC distribution path so the latest checkpoints can be streamed instead of polled, while still leaning on MinIO/GCS for blob storage and Redis for lightweight version metadata where needed.

## Responsibilities & scope

- **Accept weight publications** emitted after each learner checkpoint, validating the (step, checksum, uri) manifest that the Python learner already produces when saving to object storage.

- **Persist version metadata** and expose an authoritative "current weights" view that resolves to MinIO/GCS object paths, while keeping Redis as an optional cache for simple consumers.

- **Stream updates to actors** over a stable gRPC API so they can react to new weights immediately instead of polling bespoke channels.

- **Provide a compatibility bridge** for today's Redis-backed publisher so existing configuration continues to work while actors migrate to the new client library.

## Architecture

### High-level flow

```
Learner checkpoint
    │  (step, checksum, uri)
    ▼
PublishWeights gRPC
    ▼
Version registry + manifest store ──► optional Redis mirror
    ▼
Subscription fan-out (StreamWeights, GetCurrent)
    ▼
Actors load from MinIO/GCS using provided URI & checksum
```

### Components

- **Publish API** – A gRPC surface offering PublishWeights so learners can push new versions immediately after checkpoints, matching the planned evolution called out in the learner design.

- **Artifact validator** – Confirms the learner-supplied checksum and metadata before promoting a version, aligning with the existing manifest format produced during checkpoint saves.

- **Version registry** – Maintains the latest (and optionally recent) weight descriptors per run, persisting pointer data and exposing lightweight reads via Redis for backwards compatibility.

- **Distribution fan-out** – Streams WeightsBlob updates to connected actors or other subscribers, mirroring the gRPC row defined in the platform data flow.

- **Compatibility bridge** – Continues publishing to the legacy Redis channel until every consumer has moved to the gRPC client, using the same config knobs (backend, endpoint, channel) the learner already exposes.

## External contracts

### gRPC surface

- **PublishWeights(PublishWeightsRequest) → PublishWeightsResponse**
  Request fields capture run_id, step, checksum, artifact_uri, and optional inline payloads, matching what the learner already emits in WeightPayload and configuration.

- **StreamWeights(WatchRunRequest) → stream WeightsBlob**
  Long-lived server-streaming RPC that pushes the newest manifest whenever the registry advances, so actors can hot-reload policies without polling.

- **GetCurrent(GetCurrentRequest) → WeightsBlob**
  Lightweight unary call for components that only need the latest version at start-up.

### Compatibility endpoints

Expose an internal Redis publisher that mirrors WeightsBlob announcements onto the configured channel when the backend is set to redis, ensuring the learner's existing configuration keeps working during rollout.

## Data lifecycle & storage

- The learner saves checkpoint shards plus a JSON manifest to object storage and computes a checksum.

- Immediately afterward it calls PublishWeights with the (step, checksum, uri) triple, incrementing its weights_published_total metric on success.

- The weights service validates the payload, records it in the registry, mirrors metadata to Redis when enabled, and persists audit logs.

- Connected actors receive the streamed update, verify the checksum, and download the referenced object from MinIO/GCS as already defined in the system data flow.

## Observability & operations

- Mirror the learner's metric naming so weights_published_total continues as a cross-service signal, adding histograms for publish latency and subscriber fan-out time.

- Emit structured logs around publication, validation failures, and subscriber counts, matching the structured logging that already exists in the learner publisher.

- Instrument gRPC handlers with tracing spans so Tempo/Jaeger visualisations can stitch the checkpoint-to-actor path together, consistent with the platform's tracing strategy.

## Deployment & configuration

- Source lives in services/weights-go/ with a corresponding Kubernetes manifest (weights.yaml) under deployments/k8s/base, as outlined in the repository's file-structure plan.

- The learner keeps pointing at the weights service via its existing weights.backend/endpoint/channel config; swapping the backend from redis to grpc can be a simple configuration change once the Go service is in place.

## Migration plan

- Deploy the weights service in "shadow" mode where it consumes PublishWeights calls and mirrors announcements to Redis so current actors remain unaffected.

- Roll out new actor clients that subscribe via StreamWeights, falling back to Redis if the stream is unavailable during the transition.

- Flip learner configuration to the grpc backend and retire the Redis compatibility bridge once all actors use the new API.

## Future work

- Support delta/segment streaming for very large models once the learner produces sharded artifacts in addition to the single-blob manifest.

- Add run-scoped retention policies (e.g., keep N historical versions) to simplify rollbacks and evaluations.

- Explore multi-run tenanting so a single service instance can safely broker weights for many concurrent experiments without interference.

## Implementation status

Initial scaffolding for the Go service lives under `services/weights-go/` and includes:

- **Proto contract** – `proto/weights/weights.proto` defines `PublishWeights`, `StreamWeights`, and `GetCurrent` RPCs plus the `WeightsBlob` manifest, mirroring the learner's manifest fields.
- **Registry + fan-out** – `internal/registry` provides an in-memory store with bounded history and per-run subscriptions that back both unary and streaming reads.
- **Core service logic** – `internal/service` validates publish inputs, records versions, and exposes streaming helpers that the forthcoming gRPC handlers will wrap.
- **Configuration** – `internal/config` reads environment variables for listener settings, registry backend selection, Redis mirroring, and compatibility toggles.
- **Executable skeleton** – `cmd/server/main.go` wires config, logging, and the in-memory registry while we finish integrating the gRPC surface.

Remaining work before productionizing includes generating gRPC stubs from the proto, wiring the server, persisting metadata beyond memory (e.g., Redis/Postgres), and implementing the Redis compatibility bridge described above.
