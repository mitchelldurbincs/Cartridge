# Engine Service (Rust)

## 1. Mental model: console + cartridges
- **Console (`engine-server`)**: A tonic-based gRPC process that keeps a cache of hot `ErasedGame` instances and hands out reusable buffers so every `Reset`/`Step` call avoids reallocation. It is responsible for translating protobuf requests into the byte-oriented engine core API and for enforcing that clients call `Reset` before `Step`.【F:services/engine-rust/engine-server/src/service.rs†L18-L124】【F:services/engine-rust/engine-server/src/buffers.rs†L1-L86】
- **Cartridges (game crates)**: Each game implements the strongly typed `Game` trait and registers itself with the global registry. The server asks the registry for a boxed `ErasedGame`, which wraps the typed implementation and performs encode/decode work on demand.【F:services/engine-rust/engine-core/src/typed.rs†L8-L125】【F:services/engine-rust/engine-core/src/registry.rs†L1-L87】【F:services/engine-rust/engine-core/src/adapter.rs†L1-L118】

## 2. Crate layout and responsibilities
- `engine-core`: Owns the typed `Game` trait, error types, and the `GameAdapter` that bridges typed games to the erased engine-facing API. It also exposes the registry helpers that the server uses at runtime.【F:services/engine-rust/engine-core/src/typed.rs†L8-L158】【F:services/engine-rust/engine-core/src/adapter.rs†L1-L118】【F:services/engine-rust/engine-core/src/registry.rs†L1-L147】
- `engine-proto`: Generated tonic client/server bindings for `proto/engine/v1`. The server crate consumes this crate so the wire format stays versioned independently of gameplay code.
- `engine-server`: Hosts the gRPC implementation, buffer pooling, and the cache of initialized games so multiple `Step` calls share mutable state without rebuilding cartridges.【F:services/engine-rust/engine-server/src/service.rs†L18-L216】
- `games-*` crates (e.g. `games-tictactoe`): Implement concrete `Game` traits and call `register_game!` in their `lib.rs` to make the environment discoverable.【F:services/engine-rust/games-tictactoe/src/lib.rs†L1-L82】

## 3. Typed games → erased server boundary
1. Games implement `Game` with typed `State`, `Action`, and `Obs` plus encode/decode hooks that describe how to serialize those types into reusable byte buffers.【F:services/engine-rust/engine-core/src/typed.rs†L45-L125】
2. `GameAdapter` owns a `ChaCha20Rng` and wraps any typed game behind the object-safe `ErasedGame` trait, translating between byte buffers and typed values for every `reset`/`step` call.【F:services/engine-rust/engine-core/src/adapter.rs†L20-L118】
3. The registry stores factories that yield boxed adapters; the server looks up an `(env_id, build_id)` pair and caches the resulting `ErasedGame` so repeated requests reuse state and RNG streams.【F:services/engine-rust/engine-core/src/registry.rs†L1-L119】【F:services/engine-rust/engine-server/src/service.rs†L30-L119】

## 4. Hot-path execution in the server
- **Reset flow**: The server leases state/observation buffers from the pool, calls `ErasedGame::reset(seed, hint, …)` to fill them, and returns the filled buffers to the client before releasing them back to the pool.【F:services/engine-rust/engine-server/src/service.rs†L78-L119】【F:services/engine-rust/engine-server/src/buffers.rs†L33-L86】
- **Step flow**: Clients provide the previous state/action bytes. The server reuses buffers to hold the next state and observation, invokes the cached game, and returns reward/done flags along with the updated bytes.【F:services/engine-rust/engine-server/src/service.rs†L120-L170】
- **Buffer pool**: A shared `BufferPool` owns separate vectors for state, observation, and action data; each is cleared and recycled on return to minimize allocations across concurrent RPCs.【F:services/engine-rust/engine-server/src/buffers.rs†L1-L86】

## 5. Game cartridge example: Tic-Tac-Toe
`games-tictactoe` showcases the pattern: `TicTacToe::new()` implements `Game`, encodes its 11-byte state and 116-byte observation, and registers itself so the engine can serve `env_id = "tictactoe"`. This crate is built into the Docker image, so the server can satisfy requests immediately after startup.【F:services/engine-rust/games-tictactoe/src/lib.rs†L1-L82】

## 6. Testing hooks
- `engine-core` includes unit tests around trait ergonomics and adapter conversions so new games can rely on encode/decode helpers.【F:services/engine-rust/engine-core/src/typed.rs†L160-L246】【F:services/engine-rust/engine-core/src/adapter.rs†L120-L226】
- `engine-server` tests the tonic service end-to-end by registering mock games and asserting reset/step buffer sizes and error handling paths.【F:services/engine-rust/engine-server/src/service.rs†L172-L284】
