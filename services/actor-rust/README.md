# Actor Service (Rust)

The Actor service runs game episodes by connecting to the Engine service and collecting experience transitions for the Replay service. This is the Rust implementation of the actor, providing high-performance episode generation for reinforcement learning training.

## Features

- **High Performance**: Rust implementation for maximum throughput
- **Engine Integration**: Connects to engine service for game simulation
- **Random Policy**: Selects random valid actions (foundation for future ML policies)
- **Experience Collection**: Converts episodes into transitions for training
- **Batch Processing**: Efficiently batches transitions before sending to replay
- **Multi-Environment Support**: Can run any game environment supported by the engine
- **Configurable**: Extensive configuration options via CLI and environment variables

## Architecture

```
┌─────────┐    gRPC     ┌────────┐    gRPC     ┌─────────┐
│  Actor  │────────────▶│ Engine │             │ Replay  │
│ (Rust)  │             │        │             │         │
│         │◀────────────│        │             │         │
└─────────┘   Reset/Step└────────┘             └─────────┘
     │                                              ▲
     │                                              │
     └─────────── StoreBatch ──────────────────────┘
```

The actor:
1. Calls `Reset` on the engine to start an episode
2. Repeatedly calls `Step` with random actions until episode ends
3. Collects transitions `(state, action, reward, next_state, done)`
4. Batches transitions and sends to replay via `StoreBatch`

## Usage

### Basic Usage

```bash
# Build the actor
cargo build --release

# Run with defaults (tictactoe, localhost services)
./target/release/actor

# Run with custom environment
./target/release/actor --env-id cartpole --max-episodes 100

# Run with custom endpoints
./target/release/actor \
  --engine-addr http://engine.example.com:50051 \
  --replay-addr http://replay.example.com:8080
```

### Configuration Options

| Flag | Default | Description |
|------|---------|-------------|
| `--engine-addr` | `http://localhost:50051` | Engine service address |
| `--replay-addr` | `http://localhost:8080` | Replay service address |
| `--actor-id` | `actor-rust-1` | Unique actor identifier |
| `--env-id` | `tictactoe` | Environment to run |
| `--max-episodes` | `-1` (unlimited) | Maximum episodes to run |
| `--episode-timeout-secs` | `30` | Timeout per episode |
| `--batch-size` | `32` | Batch size for replay buffer |
| `--flush-interval-secs` | `5` | Interval to flush partial batches |
| `--log-level` | `info` | Log level |

### Environment Variables

All flags can be set via environment variables with `ACTOR_` prefix:

```bash
export ACTOR_ENGINE_ADDR=http://engine.prod.com:50051
export ACTOR_REPLAY_ADDR=http://replay.prod.com:8080
export ACTOR_ENV_ID=chess
export ACTOR_MAX_EPISODES=1000
cargo run
```

## Building and Development

### Prerequisites

- Rust 1.70 or later
- Protocol Buffers compiler (`protoc`)

### Build

```bash
# Install dependencies and build
cargo build

# Build optimized release version
cargo build --release

# Run tests
cargo test

# Run with logging
RUST_LOG=debug cargo run
```

### Docker

```bash
# Build image
docker build -t cartridge/actor-rust .

# Run container
docker run -it --rm \
  -e ACTOR_ENGINE_ADDR=http://engine:50051 \
  -e ACTOR_REPLAY_ADDR=http://replay:8080 \
  cartridge/actor-rust
```

## Performance

The Rust actor is designed for high-throughput episode generation:

- **Zero-copy buffers**: Efficient handling of state/action/observation data
- **Concurrent execution**: Async I/O for network operations
- **Minimal allocations**: Reuses buffers where possible
- **Deterministic RNG**: ChaCha20Rng for reproducible randomness

Expected performance improvements over Go actor:
- 2-3x higher episode throughput
- Lower memory usage per episode
- Better CPU utilization

## Integration with Other Services

### Engine Service

The actor requires:
- Engine service running and accessible
- Game environment registered (via `env-id`)
- Engine protobuf contract in `proto/engine/v1/`

### Replay Service

The actor requires:
- Replay service running and accessible
- Replay protobuf contract in `proto/replay/v1/`

### Future: ML Policies

The policy interface is designed to support ML-based policies:

```rust
trait Policy: Send + Sync {
    fn select_action(&mut self, observation: &[u8]) -> Result<Vec<u8>>;
}
```

This will enable:
- Neural network policy inference
- Model loading from weights service
- Batch inference for efficiency

## Monitoring and Logging

The actor logs:
- Episode completion with step count and final reward
- Batch flushing to replay service
- Connection status and errors
- Periodic progress updates (every 10 episodes)

Use `RUST_LOG=debug` for detailed logging during development.

## Troubleshooting

### Common Issues

1. **Connection refused**: Ensure engine/replay services are running
2. **Unknown env_id**: Ensure game is registered with engine
3. **Build errors**: Ensure protoc is installed and proto files exist
4. **gRPC errors**: Check network connectivity and service health

### Debug Mode

```bash
RUST_LOG=debug cargo run -- --log-level debug
```

This provides detailed logging of:
- gRPC calls and responses
- Action selection details
- Transition creation
- Buffer management