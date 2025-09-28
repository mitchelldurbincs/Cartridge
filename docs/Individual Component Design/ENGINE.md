## 1) Mental model: Console & Cartridges

- **Console (`engine-rust` server):** Owns networking (gRPC/tonic), batching, streaming backpressure, metrics/tracing, buffer reuse, and registry lookups. It is game-agnostic.
    
- **Cartridge (a game crate):** Implements a small `Game` trait; encodes/decodes its own `State/Action/Obs`; enforces determinism; exposes capabilities.


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