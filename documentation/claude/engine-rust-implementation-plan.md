# Rust Engine Service Implementation Plan

## Overview

This document provides a comprehensive implementation plan for the Rust engine service within the Cartridge reinforcement learning platform. The engine service provides a pluggable framework for different games using generic protobuf contracts, implementing deterministic simulation with a stable wire protocol that never changes when adding new games.

## Current State Analysis

The project structure shows:
- Basic directory structure exists with `services/engine-rust/` and `services/games/tictactoe/` 
- Documentation indicates a mature architecture design with clear separation of concerns
- No existing implementation found - starting from scratch
- Protobuf definitions need to be created
- Integration points with other services (Replay, Learner, Orchestrator) are well-defined

## Architecture Proposal

### High-Level Design

The engine service follows a "console and cartridge" model:

```
┌─────────────────────────────────────────┐
│           Engine Server (Console)       │
│  ┌─────────────────────────────────────┐ │
│  │         gRPC Service Layer          │ │
│  │    (Tonic + Stable Proto Contract)  │ │
│  └─────────────────────────────────────┘ │
│  ┌─────────────────────────────────────┐ │
│  │        ErasedGame Interface         │ │
│  │     (Bytes-only, No Generics)      │ │
│  └─────────────────────────────────────┘ │
│  ┌─────────────────────────────────────┐ │
│  │         Game Registry               │ │
│  │   (env_id -> Game Factory)         │ │
│  └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
                    │
                    │ Adapter Layer
                    ▼
┌─────────────────────────────────────────┐
│             Game Cartridges             │
│  ┌─────────────┐  ┌─────────────────┐   │
│  │  TicTacToe  │  │  Future Games   │   │
│  │   (Typed)   │  │    (Typed)      │   │
│  └─────────────┘  └─────────────────┘   │
└─────────────────────────────────────────┘
```

### Core Components

1. **Typed Game Trait**: Ergonomic interface for game developers with compile-time type safety
2. **Erased Game Interface**: Runtime interface that works only with bytes, no generics across gRPC boundary
3. **Adapter Layer**: Converts typed games to erased interface automatically
4. **Game Registry**: Static registry mapping env_id to game factory functions
5. **gRPC Server**: Tonic-based server implementing stable protobuf contract
6. **Buffer Management**: Reusable buffers for allocation-free hot paths

### Data Flow

```
Client Request → gRPC Layer → Registry Lookup → ErasedGame → Typed Game → Game Logic
                           ↓
Client Response ← Protobuf ← Bytes Encoding ← Adapter ← Typed Response ← Game Logic
```

### Technology Stack

- **Core Framework**: Rust with Tokio async runtime
- **gRPC**: Tonic for server implementation
- **Serialization**: Prost for protobuf, custom encoders for game data
- **Determinism**: ChaCha20Rng for reproducible randomness
- **Metrics**: Prometheus client for observability
- **Tracing**: OpenTelemetry integration
- **Testing**: Criterion for benchmarks, PropTest for fuzz testing

## Implementation Plan

### Phase 1: Core Infrastructure (Foundation)

#### 1.1 Project Structure Setup
- Create Cargo workspace with multiple crates:
  - `engine-core`: Core traits and types
  - `engine-server`: gRPC server implementation  
  - `engine-proto`: Generated protobuf code
  - `games-tictactoe`: Reference game implementation

#### 1.2 Protobuf Contract Definition
Create stable protobuf schema in `proto/engine/v1/engine.proto`:

```proto
syntax = "proto3";
package engine;

message EngineId { 
  string env_id = 1; 
  string build_id = 2; 
}

message Encoding { 
  string state = 1; 
  string action = 2; 
  string obs = 3; 
  uint32 schema_version = 4; 
}

message Capabilities {
  EngineId id = 1;
  Encoding enc = 2;
  uint32 max_horizon = 3;
  oneof action_space {
    uint32 discrete_n = 10;
    MultiDiscrete multi = 11;
    BoxSpec continuous = 12;
  }
  uint32 preferred_batch = 20;
}

service Engine {
  rpc GetCapabilities(EngineId) returns (Capabilities);
  rpc Reset(ResetRequest) returns (ResetResponse);
  rpc Step(StepRequest) returns (StepResponse);
  rpc BatchSimulate(BatchSimulateRequest) returns (stream SimResultChunk);
}
```

#### 1.3 Core Traits Implementation

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

    // Encoding/Decoding hooks
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

### Phase 2: Game Framework (Core Logic)

#### 2.1 Registry System
Implement static game registry for compile-time registration:

```rust
// engine-core/src/registry.rs
use std::collections::HashMap;
use once_cell::sync::Lazy;

type Factory = fn() -> Box<dyn ErasedGame>;
static REGISTRY: Lazy<HashMap<&'static str, Factory>> = Lazy::new(|| HashMap::new());

pub fn register(env_id: &'static str, factory: Factory) {
    REGISTRY.insert(env_id, factory);
}

pub fn create(env_id: &str) -> Option<Box<dyn ErasedGame>> {
    REGISTRY.get(env_id).map(|f| f())
}
```

#### 2.2 Adapter Implementation
Create blanket adapter converting typed games to erased interface:

```rust
// engine-core/src/adapter.rs
pub struct GameAdapter<T: Game> {
    game: T,
    rng: ChaCha20Rng,
}

impl<T: Game> ErasedGame for GameAdapter<T> {
    fn reset(&mut self, seed: u64, hint: &[u8], out_state: &mut Vec<u8>, out_obs: &mut Vec<u8>) {
        self.rng = ChaCha20Rng::seed_from_u64(seed);
        let (state, obs) = self.game.reset(&mut self.rng, hint);
        T::encode_state(&state, out_state);
        T::encode_obs(&obs, out_obs);
    }
    
    fn step(&mut self, state: &[u8], action: &[u8], out_state: &mut Vec<u8>, out_obs: &mut Vec<u8>) -> (f32, bool) {
        let mut state = T::decode_state(state);
        let action = T::decode_action(action);
        let (obs, reward, done) = self.game.step(&mut state, action);
        T::encode_state(&state, out_state);
        T::encode_obs(&obs, out_obs);
        (reward, done)
    }
}
```

### Phase 3: Reference Implementation (TicTacToe)

#### 3.1 TicTacToe Game Implementation
Create complete reference implementation demonstrating the framework:

```rust
// games-tictactoe/src/lib.rs
#[derive(Clone, Copy)]
pub struct State {
    board: [u8; 9],  // 0=empty, 1=X, 2=O
    current_player: u8,
    winner: u8,
}

#[derive(Clone, Copy)]
pub enum Action {
    Place(u8), // position 0-8
}

#[derive(Clone)]
pub struct Obs {
    board_view: [f32; 18], // one-hot encoding for X/O
    legal_moves: [f32; 9], // mask for valid moves
}

pub struct TicTacToe;

impl Game for TicTacToe {
    type State = State;
    type Action = Action;
    type Obs = Obs;
    
    fn engine_id(&self) -> EngineId {
        EngineId {
            env_id: "tictactoe".to_string(),
            build_id: env!("CARGO_PKG_VERSION").to_string(),
        }
    }
    
    fn capabilities(&self) -> Capabilities {
        Capabilities {
            id: Some(self.engine_id()),
            enc: Some(Encoding {
                state: "packed_u8:v1".to_string(),
                action: "discrete:v1".to_string(), 
                obs: "f32x27:v1".to_string(),
                schema_version: 1,
            }),
            max_horizon: 9,
            action_space: Some(ActionSpace::DiscreteN(9)),
            preferred_batch: 32,
        }
    }
    
    // Implementation of reset, step, encode/decode methods...
}
```

### Phase 4: gRPC Server (Network Layer)

#### 4.1 Tonic Server Implementation
```rust
// engine-server/src/service.rs
#[tonic::async_trait]
impl engine::engine_server::Engine for EngineSvc {
    async fn get_capabilities(&self, req: Request<EngineId>) -> Result<Response<Capabilities>, Status> {
        let id = req.into_inner();
        let game = registry::create(&id.env_id)
            .ok_or_else(|| Status::not_found("Unknown env_id"))?;
        Ok(Response::new(game.capabilities()))
    }

    async fn reset(&self, req: Request<ResetRequest>) -> Result<Response<ResetResponse>, Status> {
        let req = req.into_inner();
        let mut game = registry::create(&req.id.unwrap().env_id)
            .ok_or_else(|| Status::not_found("Unknown env_id"))?;
            
        let mut state_buf = Vec::new();
        let mut obs_buf = Vec::new();
        game.reset(req.seed, &req.hint, &mut state_buf, &mut obs_buf);
        
        Ok(Response::new(ResetResponse {
            state: state_buf,
            obs: obs_buf,
        }))
    }

    async fn batch_simulate(&self, req: Request<BatchSimulateRequest>) -> Result<Response<Self::BatchSimulateStream>, Status> {
        // Implement streaming batch simulation with backpressure
        todo!()
    }
}
```

#### 4.2 Buffer Pool Management
Implement allocation-free hot paths with buffer reuse:

```rust
// engine-server/src/buffers.rs
pub struct BufferPool {
    state_buffers: Arc<Mutex<Vec<Vec<u8>>>>,
    obs_buffers: Arc<Mutex<Vec<Vec<u8>>>>,
    action_buffers: Arc<Mutex<Vec<Vec<u8>>>>,
}

impl BufferPool {
    pub fn get_state_buffer(&self) -> Vec<u8> {
        self.state_buffers.lock().unwrap().pop().unwrap_or_else(Vec::new)
    }
    
    pub fn return_state_buffer(&self, mut buf: Vec<u8>) {
        buf.clear();
        self.state_buffers.lock().unwrap().push(buf);
    }
}
```

### Phase 5: Build System & Tooling

#### 5.1 Cargo Workspace Configuration
```toml
# Cargo.toml
[workspace]
members = [
    "engine-core", 
    "engine-server",
    "engine-proto",
    "games-tictactoe"
]

[workspace.dependencies]
tokio = { version = "1.0", features = ["full"] }
tonic = "0.10"
prost = "0.12"
rand_chacha = "0.3"
thiserror = "1.0"
anyhow = "1.0"
tracing = "0.1"
prometheus = "0.13"
```

#### 5.2 Build Scripts and Code Generation
```rust
// engine-proto/build.rs
fn main() {
    tonic_build::compile_protos("../proto/engine/v1/engine.proto")
        .unwrap_or_else(|e| panic!("Failed to compile protos {:?}", e));
}
```

#### 5.3 CI/CD Integration
- Configure cargo clippy with strict lints
- Set up rustfmt with project style
- Add criterion benchmarks to CI
- Performance regression detection

### Phase 6: Testing Strategy

#### 6.1 Unit Tests
- Determinism tests: Same seed produces identical results
- Round-trip encoding tests: encode → decode preserves data
- Game logic correctness tests
- Registry functionality tests

#### 6.2 Integration Tests  
- gRPC service tests with real clients
- Buffer pool stress tests
- Performance benchmarks
- Memory leak detection

#### 6.3 Property-Based Testing
```rust
// Use PropTest for fuzz testing
#[proptest]
fn roundtrip_state_encoding(state: State) {
    let mut buf = Vec::new();
    TicTacToe::encode_state(&state, &mut buf);
    let decoded = TicTacToe::decode_state(&buf);
    prop_assert_eq!(state, decoded);
}
```

#### 6.4 Performance Benchmarks
```rust
// Criterion benchmarks for hot paths
fn bench_step_performance(c: &mut Criterion) {
    c.bench_function("tictactoe_step", |b| {
        b.iter(|| {
            // Benchmark single step operation
        });
    });
}
```

### Phase 7: Observability & Production Readiness

#### 7.1 Metrics Integration
- Steps per second histogram
- Request latency percentiles  
- Error rates by game type
- Buffer pool utilization
- Memory usage tracking

#### 7.2 Tracing Integration
```rust
// OpenTelemetry spans for request tracing
#[tracing::instrument]
async fn reset(&self, req: Request<ResetRequest>) -> Result<Response<ResetResponse>, Status> {
    // Implementation with automatic tracing
}
```

#### 7.3 Structured Logging
- Correlation IDs across requests
- Game-specific context
- Error context preservation
- Performance debug logs

## Risk Mitigation

### Technical Risks
1. **Performance Bottlenecks**: Mitigate with extensive benchmarking and profiling
2. **Memory Leaks**: Address with careful buffer management and testing
3. **Determinism Bugs**: Prevent with comprehensive seed-based testing
4. **Proto Evolution**: Handle with versioning strategy and backwards compatibility

### Development Risks  
1. **Complexity Creep**: Start with minimal viable implementation, iterate
2. **Integration Issues**: Early integration testing with other services
3. **Documentation Drift**: Keep code and docs in sync with automation

## Dependencies and Prerequisites

### External Dependencies
- Rust toolchain (stable)
- Protocol Buffers compiler
- Docker for integration testing
- Redis/MinIO for local development

### Internal Dependencies
- Protobuf definitions must be created first
- Integration with replay-go service for testing
- Observability stack for production deployment

## Success Criteria

### MVP Success Criteria
- [ ] TicTacToe game fully implemented and functional
- [ ] gRPC server responds correctly to all service methods
- [ ] Deterministic behavior verified with seed testing
- [ ] Basic performance benchmarks establish baseline
- [ ] Integration tests pass with docker-compose stack

### Production Success Criteria  
- [ ] Multiple games registered and working
- [ ] Sub-millisecond step latencies for simple games
- [ ] Zero-allocation hot paths achieved
- [ ] Comprehensive observability integration
- [ ] Load testing validates performance targets
- [ ] Documentation complete for game developers

## Timeline Estimates

- **Phase 1 (Foundation)**: 3-4 days
- **Phase 2 (Framework)**: 2-3 days  
- **Phase 3 (TicTacToe)**: 2-3 days
- **Phase 4 (gRPC Server)**: 3-4 days
- **Phase 5 (Build System)**: 1-2 days
- **Phase 6 (Testing)**: 2-3 days
- **Phase 7 (Observability)**: 2-3 days

**Total Estimated Duration**: 15-22 days

## Next Steps

1. Begin with Phase 1: Create project structure and protobuf definitions
2. Implement core traits and registry system 
3. Build TicTacToe reference implementation
4. Create gRPC server with basic functionality
5. Add comprehensive testing and observability
6. Optimize performance and prepare for production

This implementation plan provides a complete roadmap for building a production-ready Rust engine service that fulfills all the architectural requirements while maintaining extensibility for future games.