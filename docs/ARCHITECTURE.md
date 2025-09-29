# Game Engine Extensibility
## 2) Stable wire contract (proto stays generic)

All games share one protobuf. Game-specific payloads ride as bytes with **declared encodings** and **schema versions** surfaced via capabilities.

```proto
syntax = "proto3";
package engine;

message EngineId { string env_id = 1; string build_id = 2; }
message Encoding { string state = 1; string action = 2; string obs = 3; uint32 schema_version = 4; }
message MultiDiscrete { repeated uint32 nvec = 1; }
message BoxSpec { repeated float low = 1; repeated float high = 2; repeated uint32 shape = 3; }

message Capabilities {
  EngineId id = 1;
  Encoding enc = 2;
  uint32 max_horizon = 3;
  oneof action_space {
    uint32 discrete_n = 10;
    MultiDiscrete multi = 11;
    BoxSpec continuous = 12;
  }
  uint32 preferred_batch = 20; // perf hint
}

message ResetRequest  { EngineId id = 1; uint64 seed = 2; bytes hint = 3; }
message ResetResponse { bytes state = 1; bytes obs = 2; }

message StepRequest   { EngineId id = 1; bytes state = 2; bytes action = 3; }
message StepResponse  { bytes next_state = 1; bytes obs = 2; float reward = 3; bool done = 4; uint64 info = 5; }

service Engine {
  rpc GetCapabilities(EngineId) returns (Capabilities);
  rpc Reset(ResetRequest) returns (ResetResponse);
  rpc Step(StepRequest) returns (StepResponse);
}
```

**Why this works:** The proto never changes when you add games. Encodings (e.g., `state=packed_u8:v1`) + `schema_version` tell trainers how to parse bytes.

---

## 3) Rust design: typed games + erased server

Two layers:

- **Typed `Game`** (per-cartridge): ergonomic, fast, deterministic.
    
- **Erased `ErasedGame`** (server-facing): works only with bytes and reusable buffers; no generics across the gRPC boundary.
    

```rust
// engine-core/src/typed.rs
pub trait Game: Send + Sync + 'static {
    type State;  // POD-like
    type Action; // small, Copy or compact
    type Obs;    // often contiguous f32s

    fn engine_id(&self) -> EngineId;
    fn capabilities(&self) -> Capabilities;

    fn reset(&mut self, rng: &mut ChaCha20Rng, hint: &[u8]) -> (Self::State, Self::Obs);
    fn step(&mut self, s: &mut Self::State, a: Self::Action) -> (Self::Obs, f32, bool);

    fn encode_state(s: &Self::State, out: &mut Vec<u8>);
    fn decode_state(buf: &[u8]) -> Self::State;
    fn encode_action(a: &Self::Action, out: &mut Vec<u8>);
    fn decode_action(buf: &[u8]) -> Self::Action;
    fn encode_obs(o: &Self::Obs, out: &mut Vec<u8>);
}

// engine-core/src/erased.rs
pub trait ErasedGame: Send + Sync + 'static {
    fn engine_id(&self) -> EngineId;
    fn capabilities(&self) -> Capabilities;
    fn reset(&mut self, seed: u64, hint: &[u8], out_state: &mut Vec<u8>, out_obs: &mut Vec<u8>);
    fn step(&mut self, state: &[u8], action: &[u8], out_state: &mut Vec<u8>, out_obs: &mut Vec<u8>) -> (f32, bool);
}
```

A blanket adapter converts any `Game` to `ErasedGame` by calling its encode/decode hooks, letting the server remain generic and allocation-aware.

---

## 4) Registry (cartridge slot)

Games register factories in a static registry at startup; the server resolves `env_id → game`.

```rust
// engine-core/src/registry.rs
use std::collections::HashMap;
use once_cell::sync::Lazy;

type Factory = fn() -> Box<dyn ErasedGame>;
static REGISTRY: Lazy<HashMap<&'static str, Factory>> = Lazy::new(|| HashMap::new());

pub fn register(env_id: &'static str, factory: Factory) {
    // optionally guard duplicates
    REGISTRY.insert(env_id, factory);
}
pub fn create(env_id: &str) -> Option<Box<dyn ErasedGame>> {
    REGISTRY.get(env_id).map(|f| f())
}
```

Each game crate exposes `pub fn register()`, called from `engine-server`’s `main`. (You can add a macro/inventory helper to auto-register.)

> **Optional later:** dynamic plugins via `libloading` for third-party games; start with compile-time registration for simplicity and safety.

---

## 5) Server glue (tonic) & buffer management

The server owns:

- **Buffer pools** (`Vec<u8>` or `bytes::BytesMut`) to minimize allocations for state/observation encoding

- **Registry management** for game lookup and instantiation

- **Error handling** with proper gRPC status codes


```rust
#[tonic::async_trait]
impl engine::engine_server::Engine for EngineSvc {
  async fn get_capabilities(&self, req: Request<EngineId>) -> Result<Response<Capabilities>, Status> {
    let id = req.into_inner();
    let g = registry::create(&id.env_id).ok_or_else(|| Status::not_found("env"))?;
    Ok(Response::new(g.capabilities()))
  }
  // reset/step handle individual game simulation with buffer reuse
}
```

---

## 6) Determinism & sessions

- **Determinism:** seed everything (`ChaCha20Rng`) and include `seed` (or seed base) in manifests/artifacts.
    
- **API contract is stateless.** Internally you may keep per-connection pools, arenas, and RNGs for perf—as long as state is always reconstructible from inputs.
    
- **Sessions (optional later):** Add `OpenSession/CloseSession` if an env benefits from server-held state; default remains stateless for elasticity.
    

---

## 7) Performance guidelines

- **State layout:** POD-like structs; avoid nested vecs; pack with `bytemuck`/manual LE packing.
    
- **Encodings:** e.g., `state:packed_u8:v1`, `action:discrete:v1`, `obs:f32xN:v1`.
    
- **Small collections:** use `smallvec` for tight upper bounds; only use `rayon` if you can keep RNG calls deterministic across threads.
    
- **Locks:** prefer message passing; if needed, use `parking_lot` selectively.
    
- **Hot path metrics:** `engine_steps_total`, `engine_step_latency_seconds` histogram; span per chunk with env/build ids.
    

---

## 8) Schema evolution

- **Capabilities** advertises `schema_version` and encoding names for `state/action/obs`.
    
- Keep old decoders around for ≥1 minor version when bumping a layout.
    
- In protobuf: add fields; never renumber; `reserved` on remove.
    
- Artifacts (Parquet) store `{env_id, build_id, schema_version}` in `manifest.json` so trainers pick the correct decoder.
    

---

## 9) Trainer parsing contract

Provide tiny parsing helpers (Go/Python) for each encoding:

- `state:packed_u8:v1` → fixed offsets (documented).
    
- `action:discrete:v1` → u32 LE.
    
- `obs:f32xN:v1` → contiguous LE f32 vector (N from capabilities).
    

On-wire stays compact; **Parquet+zstd** remains the analysis-friendly artifact format.

---

## 10) Add-a-game checklist (copy/paste)

1. **Scaffold** `games/<env_id>` crate with `Game` impl + `register("<env_id>", || Box::new(...))`.
    
2. Implement `reset/step` with `ChaCha20Rng` determinism.
    
3. Write **encode/decode** for `State/Action/Obs`; benchmark.
    
4. Fill **Capabilities** (action space, encodings, `max_horizon`, `preferred_batch`).
    
5. Tests:
    
    - Determinism (fixed seed → identical trajectories),
        
    - Round-trip encode/decode (fuzz with `proptest`),
        
    - Step correctness on hand-picked states.
        
6. Bench with `criterion`: report steps/sec and ns/step (CI smoke threshold).
    
7. Document optional `hint` bytes (e.g., map size/rules).
    
8. Add to CI matrix; ensure `GetCapabilities` reflects actual encodings.
    

---

## 11) Example: tiny gridworld (typed → erased)

```rust
#[derive(Clone, Copy)]
pub struct State { pub x: u8, pub y: u8, pub w: u8, pub h: u8 }
#[derive(Clone, Copy)]
pub enum Action { Up, Down, Left, Right }
#[derive(Clone)] pub struct Obs(pub [f32; 4]);

pub struct Gridworld;

impl Game for Gridworld {
  type State = State; type Action = Action; type Obs = Obs;
  fn engine_id(&self) -> EngineId { EngineId{ env_id:"gridworld".into(), build_id:BUILD_ID.into() } }
  fn capabilities(&self) -> Capabilities { /* discrete_n=4; encodings v1 */ }
  fn reset(&mut self, rng:&mut ChaCha20Rng, hint:&[u8]) -> (State, Obs) { /* ... */ }
  fn step(&mut self, s:&mut State, a:Action) -> (Obs, f32, bool) { /* ... */ }
  fn encode_state(s:&State, out:&mut Vec<u8>) { out.extend_from_slice(&[s.x,s.y,s.w,s.h]); }
  fn decode_state(b:&[u8]) -> State { State{ x:b[0], y:b[1], w:b[2], h:b[3] } }
  fn encode_action(a:&Action, out:&mut Vec<u8>) { out.push(match a { Action::Up=>0,Down=>1,Left=>2,Right=>3 }); }
  fn decode_action(b:&[u8]) -> Action { [Action::Up,Action::Down,Action::Left,Action::Right][b[0] as usize] }
  fn encode_obs(o:&Obs, out:&mut Vec<u8>) { out.extend_from_slice(bytemuck::bytes_of(&o.0)); }
}
```

---

## 12) When to consider runtime plugins

Only if you must load 3rd-party games without rebuilding:

- Pros: hot-plugging; separate release cadence.
    
- Cons: unsafe FFI, ABI stability, packaging complexity, security review.
    

Most teams stick with **compile-time registration** (fast, safe, simple).

---

**TL;DR:** One stable proto + bytes encodings; typed `Game` behind an erased adapter; static registry; deterministic seeds; batch + streaming; simple trainer parsers. Adding a new game is: implement trait → register → ship, no server/proto changes.

Definitions
Capabilities
*Think of _capabilities_ as the **self-description** of a game environment. They’re the metadata the engine publishes so that clients (trainers, dashboards, orchestration) know:

- **Who am I?** (`env_id`, `build_id`) → uniquely identify the game and version of its implementation.
- **How do I encode/decode state/actions/observations?** (`Encoding`) → e.g., `"state:packed_u8:v1"`, `"action:discrete:v1"`, `"obs:f32xN:v1"`.
- **What’s my action space?**
    - `discrete_n = 4` (actions 0..3)
    - or `multi_discrete` (vector of discrete slots)
    - or `BoxSpec` for continuous spaces.
- **How long can an episode run?** (`max_horizon`)
- **What batch size is efficient?** (`preferred_batch`)
So when a trainer or API consumer asks the engine “what can you do?” it doesn’t need to know _anything_ hardcoded — it just queries `GetCapabilities()` and adapts. This is the glue that makes your proto _generic_ and future-proof.




So we have an actor, learner, replay, and model registry system 
Actor (Rust)
Learner (Python)
Replay (Go)
Weights (Go)
Web (Go)
Orchestrator (Go)

