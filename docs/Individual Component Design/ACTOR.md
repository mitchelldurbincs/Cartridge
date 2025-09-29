# Actor Service (Rust)

## Overview
The actor connects to the engine over gRPC, rolls out full episodes with a policy, and ships transition batches to the replay service. It keeps minimal shared state (RNG-backed policy, episode counter, and a transition buffer) behind mutexes so the async runtime can interleave flush timers with episode execution.【F:services/actor-rust/src/actor.rs†L1-L111】

## Execution flow
1. **Startup**: `main.rs` loads `Config` from CLI/env, validates it, initializes tracing, and builds an `Actor`. Configuration covers engine/replay endpoints, identifiers, batching, and timeouts.【F:services/actor-rust/src/main.rs†L1-L88】【F:services/actor-rust/src/config.rs†L1-L73】
2. **Capability discovery**: `Actor::new` dials both services, calls `GetCapabilities`, and constructs a `RandomPolicy` tailored to the advertised action space.【F:services/actor-rust/src/actor.rs†L17-L74】【F:services/actor-rust/src/policy.rs†L1-L164】
3. **Main loop**: `Actor::run` alternates between ticking a flush interval and launching new episodes until `max_episodes` is hit or `shutdown` is requested.【F:services/actor-rust/src/actor.rs†L76-L151】
4. **Episode rollout**: For each episode the actor calls `Reset`, iterates `Step` with policy-selected actions, and appends fully annotated transitions (state/action/obs, reward, done, timestamps) to the buffer.【F:services/actor-rust/src/actor.rs†L153-L235】
5. **Buffer management**: When the buffer reaches `batch_size` (or the flush timer fires) the actor sends a `StoreBatch` request and clears the local queue.【F:services/actor-rust/src/actor.rs†L198-L222】【F:services/actor-rust/src/actor.rs†L237-L260】

## Policies
`policy.rs` defines a trait so future inference-backed strategies can slot in. The default `RandomPolicy` inspects the engine capability payload to either sample from a discrete space, iterate multi-discrete dimensions, or draw uniform floating point vectors for continuous boxes. It reuses a `SmallRng` and per-action scratch buffers to minimize allocations.【F:services/actor-rust/src/policy.rs†L1-L164】

## Resilience & observability
- Timeouts on `Reset`/`Step` prevent hung episodes; errors are logged and the actor continues with the next attempt.【F:services/actor-rust/src/actor.rs†L110-L196】
- Partial batches are flushed on a fixed cadence so replay receives steady traffic even if episodes are long.【F:services/actor-rust/src/actor.rs†L96-L149】
- Structured tracing logs cover connections, capability discovery, episode milestones, and batch uploads.【F:services/actor-rust/src/actor.rs†L17-L230】

## Testing hooks
The actor module includes async unit tests that stand up an in-process replay server and verify that `flush_buffer` drains queued transitions and forwards them over gRPC.【F:services/actor-rust/src/actor.rs†L262-L370】
