# Actor Service

The Actor service runs game episodes by connecting to the Engine service and collecting experience transitions for the Replay service. It acts as the "experience generator" in the Cartridge RL pipeline.

## Features

- **Engine Integration**: Connects to engine service for game simulation
- **Random Policy**: Selects random valid actions (foundation for future ML policies)
- **Experience Collection**: Converts episodes into transitions for training
- **Batch Processing**: Efficiently batches transitions before sending to replay
- **Multi-Environment Support**: Can run any game environment supported by the engine
- **Configurable**: Extensive configuration options for different deployment scenarios

## Architecture

```
┌─────────┐    gRPC     ┌────────┐    gRPC     ┌─────────┐
│  Actor  │────────────▶│ Engine │             │ Replay  │
│         │             │        │             │         │
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
# Run with defaults (tictactoe, localhost services)
go run cmd/actor/main.go

# Run with custom environment
go run cmd/actor/main.go --env-id cartpole --max-episodes 100

# Run with custom endpoints
go run cmd/actor/main.go \
  --engine-addr engine.example.com:50051 \
  --replay-addr replay.example.com:8080
```

### Configuration Options

| Flag | Default | Description |
|------|---------|-------------|
| `--engine-addr` | `localhost:50051` | Engine service address |
| `--replay-addr` | `localhost:8080` | Replay service address |
| `--actor-id` | `actor-1` | Unique actor identifier |
| `--env-id` | `tictactoe` | Environment to run |
| `--max-episodes` | `-1` (unlimited) | Maximum episodes to run |
| `--episode-timeout` | `30s` | Timeout per episode |
| `--batch-size` | `32` | Batch size for replay buffer |
| `--flush-interval` | `5s` | Interval to flush partial batches |
| `--log-level` | `info` | Log level |

### Environment Variables

All flags can be set via environment variables with `ACTOR_` prefix:

```bash
export ACTOR_ENGINE_ADDR=engine.prod.com:50051
export ACTOR_REPLAY_ADDR=replay.prod.com:8080
export ACTOR_ENV_ID=chess
export ACTOR_MAX_EPISODES=1000
go run cmd/actor/main.go
```

## Supported Games

The actor can run any game environment registered with the engine service:

- **TicTacToe**: 9 discrete actions (positions 0-8)
- **Future games**: Any game implementing the engine `Game` trait

## Action Selection

Currently implements random action selection:

- **Discrete**: Randomly selects from 0 to N-1
- **Multi-Discrete**: Randomly selects for each dimension
- **Continuous**: Randomly samples within bounds

**Future**: The policy interface is designed to support ML-based policies that can be swapped in later.

## Data Flow

### Transition Format

Each game step produces a transition with:

```go
type Transition struct {
    ID              string  // Unique transition ID
    EnvID           string  // Environment (e.g., "tictactoe")
    EpisodeID       string  // Episode identifier
    StepNumber      uint32  // Step within episode
    State           []byte  // Current state (engine format)
    Action          []byte  // Action taken (engine format)
    NextState       []byte  // Resulting state
    Observation     []byte  // Current observation
    NextObservation []byte  // Next observation
    Reward          float32 // Reward received
    Done            bool    // Episode termination flag
    Priority        float32 // Priority for replay (default 1.0)
    Timestamp       uint64  // Unix timestamp
}
```

### Example TicTacToe Data

- **State**: 11 bytes (9 board positions + current player + winner)
- **Action**: 1 byte (position 0-8)
- **Observation**: 116 bytes (29 f32 values: board view + legal moves + player info)

## Building and Deployment

### Local Development

```bash
# Install dependencies
go mod tidy

# Generate protobuf code (requires buf)
buf generate

# Build
go build -o bin/actor cmd/actor/main.go

# Run
./bin/actor --env-id tictactoe
```

### Docker

```bash
# Build image
docker build -t cartridge/actor .

# Run container
docker run -it --rm \
  -e ACTOR_ENGINE_ADDR=engine:50051 \
  -e ACTOR_REPLAY_ADDR=replay:8080 \
  cartridge/actor
```

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

### Future: Weights Service

When implementing ML policies, the actor will also connect to:
- Weights service for model updates
- Periodic policy refreshing during training

## Monitoring and Logging

The actor logs:
- Episode completion with step count and final reward
- Batch flushing to replay service
- Connection status and errors
- Periodic progress updates (every 10 episodes)

For production, integrate with your observability stack (Prometheus, Grafana, etc.).

## Development

### Adding New Policies

Implement the `Policy` interface:

```go
type Policy interface {
    SelectAction(observation []byte) ([]byte, error)
}
```

### Testing

```bash
# Run tests
go test ./...

# Run with race detection
go test -race ./...

# Integration tests (requires engine and replay services)
go test -tags=integration ./...
```

## Troubleshooting

### Common Issues

1. **Connection refused**: Ensure engine/replay services are running
2. **Unknown env_id**: Ensure game is registered with engine
3. **Invalid actions**: Check action encoding matches engine expectations
4. **Memory usage**: Adjust batch size for your memory constraints

### Debug Mode

```bash
go run cmd/actor/main.go --log-level debug
```

This provides detailed logging of:
- gRPC calls and responses
- Action selection details
- Transition creation
- Buffer management