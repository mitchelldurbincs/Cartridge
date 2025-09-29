# Replay Service

The Replay service provides experience replay buffer functionality for the Cartridge reinforcement learning platform. It stores and manages transitions from game episodes, supporting both uniform and prioritized sampling for training.

## Features

- **Experience Storage**: Store single transitions or batches of experience data
- **Flexible Sampling**: Uniform and prioritized replay sampling with configurable parameters
- **Multi-Environment Support**: Handle transitions from multiple game environments
- **Time-Based Filtering**: Sample transitions within specific time windows
- **Buffer Management**: Automatic eviction of old transitions with configurable limits
- **Statistics**: Real-time buffer statistics and metrics

## Architecture

The service is built with:
- **gRPC API**: Defined in `proto/replay/v1/replay.proto`
- **Pluggable Storage**: Interface-based storage with in-memory implementation
- **Go Implementation**: Efficient concurrent processing with proper resource management

## API Overview

### Core Methods

- `StoreTransition`: Store a single experience transition
- `StoreBatch`: Store multiple transitions efficiently
- `Sample`: Sample transitions for training (uniform or prioritized)
- `GetStats`: Get buffer statistics and metrics
- `UpdatePriorities`: Update priorities for prioritized replay
- `Clear`: Remove old or filtered transitions

### Data Format

The service handles transitions with the following structure:

```protobuf
message Transition {
    string id = 1;                    // Unique identifier
    string env_id = 2;                // Environment (e.g., "tictactoe")
    string episode_id = 3;            // Episode identifier
    uint32 step_number = 4;           // Step within episode

    bytes state = 5;                  // Current state (engine format)
    bytes action = 6;                 // Action taken (engine format)
    bytes next_state = 7;             // Resulting state
    bytes observation = 8;            // Current observation
    bytes next_observation = 9;       // Next observation

    float reward = 10;                // Reward received
    bool done = 11;                   // Episode termination flag
    float priority = 12;              // Priority for sampling
    uint64 timestamp = 13;            // Storage timestamp
    map<string, string> metadata = 14; // Additional metadata
}
```

## Usage

### Starting the Server

```bash
# Build the server
go build -o bin/replay-server ./cmd/server

# Run with default settings (port 8080, max 100k transitions)
./bin/replay-server

# Run with custom settings
./bin/replay-server -port 8081 -max-size 500000
```

### Example: Storing Engine Data

```go
// Store a TicTacToe transition
transition := &replayv1.Transition{
    EnvId:     "tictactoe",
    EpisodeId: "episode-123",
    StepNumber: 0,

    // Engine-encoded data (exact format from engine service)
    State:     []byte{0,0,0,0,0,0,0,0,0,1,0}, // 11 bytes: TicTacToe state
    Action:    []byte{4},                      // 1 byte: center position
    NextState: []byte{0,0,0,0,1,0,0,0,0,2,0}, // Updated board

    Observation:     make([]byte, 116),        // 116 bytes: f32x29 encoding
    NextObservation: make([]byte, 116),

    Reward: 0.0,
    Done:   false,
    Priority: 1.0,
}

response, err := replayClient.StoreTransition(ctx, &replayv1.StoreTransitionRequest{
    Transition: transition,
})
```

### Example: Sampling for Training

```go
// Sample batch for training
sampleResponse, err := replayClient.Sample(ctx, &replayv1.SampleRequest{
    Config: &replayv1.SampleConfig{
        BatchSize:     32,
        EnvId:         "tictactoe",      // Filter by environment
        Prioritized:   true,             // Use priority sampling
        PriorityAlpha: 0.6,              // Priority exponent
    },
})

transitions := sampleResponse.Transitions  // Sampled experience
weights := sampleResponse.Weights         // Importance sampling weights
```

## Testing

```bash
# Run all tests
go test ./...

# Run storage backend tests
go test -v ./internal/storage

# Run integration tests with engine data formats
go test -v ./integration_test.go

# Run with race detection
go test -race ./...
```

## Integration with Engine

The Replay service is designed to work seamlessly with the Cartridge engine:

1. **Data Format Compatibility**: Handles exact byte formats produced by the engine
2. **Environment Awareness**: Supports multiple game environments simultaneously
3. **Efficient Storage**: Optimized for the engine's state/action/observation encodings
4. **Scalable Sampling**: Supports the batching requirements of RL training

### TicTacToe Example

The service correctly handles TicTacToe data formats:
- **State**: 11 bytes (9 board positions + current player + winner)
- **Action**: 1 byte (position 0-8)
- **Observation**: 116 bytes (29 f32 values: board view + legal moves + player)

## Configuration

Environment variables:
- `REPLAY_PORT`: Server port (default: 8080)
- `REPLAY_MAX_SIZE`: Maximum transitions to store (default: 100000)
- `REPLAY_LOG_LEVEL`: Logging level (default: info)

## Production Deployment

For production use:
1. Replace `MemoryBackend` with persistent storage (PostgreSQL, Redis, etc.)
2. Add authentication and authorization
3. Implement distributed storage for scale
4. Add monitoring and alerting
5. Configure proper resource limits

The modular design makes it easy to swap storage backends without changing the API.