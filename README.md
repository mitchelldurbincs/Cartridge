# Cartridge

A distributed reinforcement learning platform for training AI agents to play games at scale.

## Overview

Cartridge is a production-grade microservices architecture designed for high-throughput reinforcement learning. It provides a pluggable game engine framework, distributed experience replay, PyTorch-based learning, and comprehensive observability—all orchestrated to train AI agents efficiently.

**Key Features:**
- **Pluggable Game Engine**: Add new games without changing the core infrastructure using a stable protobuf contract
- **Distributed Architecture**: Microservices designed for scalability and fault tolerance
- **Production-Ready RL**: PyTorch-based learner with checkpointing, distributed experience replay, and model distribution
- **Comprehensive Observability**: Integrated metrics (Prometheus), logging (Loki), and tracing (Tempo/Jaeger)
- **Reproducible Training**: Deterministic game simulation with seeded RNGs and versioned schemas

## Architecture

Cartridge consists of seven core microservices:

```
┌─────────────────────────────────────────────────────────────┐
│                      Orchestrator (Go)                      │
│           Coordinates experiments & system state            │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
┌───────▼────────┐   ┌───────▼────────┐   ┌───────▼────────┐
│  Actor (Rust)  │   │ Learner (Python)│   │  Web (Go)      │
│ Plays games    │   │ Trains models   │   │ Dashboard UI   │
│ Generates exp  │   │ PyTorch-based   │   │ Monitoring     │
└───────┬────────┘   └───────┬────────┘   └────────────────┘
        │                    │
        │ gRPC               │ gRPC
        ▼                    ▼
┌───────────────┐   ┌────────────────┐   ┌─────────────────┐
│ Engine (Rust) │   │  Replay (Go)   │   │  Weights (Go)   │
│ Game sims     │   │ Experience buf │   │ Model serving   │
│ Deterministic │   │ Sampling       │   │ Distribution    │
└───────────────┘   └────────────────┘   └─────────────────┘
```

**Data Flow:**
- Actor generates game experiences → Replay buffer
- Learner samples from Replay → trains models → saves checkpoints
- Weights service distributes models → Actors
- All services emit metrics/logs/traces → Observability stack

## Quick Start

### Prerequisites

Ensure the following are installed:

- **Docker & Docker Compose** - [Install](https://docs.docker.com/get-docker/)
- **Protocol Buffers Compiler** (protoc v25.1+) - `apt-get install protobuf-compiler` or [download](https://github.com/protocolbuffers/protobuf/releases)
- **Rust toolchain** - [rustup](https://rustup.rs/)
- **Go 1.21+** - [Install](https://go.dev/doc/install)
- **Python 3.10+** with Poetry - [Install Poetry](https://python-poetry.org/docs/#installation)

Verify protoc installation:
```bash
protoc --version  # Should show v25.1 or higher
```

### Local Development Setup

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd Cartridge
   ```

2. **Configure environment**
   ```bash
   cp .env.example .env
   # Edit .env with your settings
   ```

3. **Start all services**
   ```bash
   docker-compose -f deployments/local/docker-compose.yml up
   ```

4. **Access the dashboard**
   - Open http://localhost:8080 for the Web UI
   - Grafana: http://localhost:3000 (metrics and dashboards)
   - Prometheus: http://localhost:9090 (metrics storage)

## Usage

### Running an Experiment

Once the services are running, you can start a training experiment:

```bash
# Submit an experiment via the orchestrator API
curl -X POST http://localhost:8080/api/experiments \
  -H "Content-Type: application/json" \
  -d '{
    "game": "tictactoe",
    "algorithm": "ppo",
    "num_actors": 4,
    "training_steps": 100000
  }'
```

Monitor progress through:
- Web dashboard at http://localhost:8080
- Grafana dashboards for detailed metrics
- Logs via Loki integration

### Adding a New Game

Cartridge makes it easy to add new games without modifying core infrastructure:

1. **Create game crate** in `services/engine-rust/games-<your_game>`
   ```rust
   pub struct YourGame {
       // game state
   }

   impl Game for YourGame {
       type State = YourState;
       type Action = YourAction;
       type Observation = YourObservation;

       fn reset(&mut self, seed: u64) -> Self::Observation { /* ... */ }
       fn step(&mut self, action: Self::Action) -> StepResult<Self::Observation> { /* ... */ }
       // Implement encode/decode for your types
   }
   ```

2. **Register in engine server** - Add your game factory
3. **Test determinism** - Verify seeded replays are identical
4. **Benchmark** - Use criterion to measure performance

See `services/engine-rust/games-tictactoe` for a complete example.

## Project Structure

```
Cartridge/
├── services/           # Microservices
│   ├── actor-rust/     # Game actor (generates experience)
│   ├── engine-rust/    # Game simulation engine
│   │   └── games-*/    # Game implementations
│   ├── learner-py/     # PyTorch-based RL training
│   ├── orchestrator-go/# Experiment coordination
│   ├── replay-go/      # Experience replay buffer
│   ├── web-go/         # Dashboard web server
│   └── weights-go/     # Model distribution service
├── proto/              # Protobuf definitions
│   ├── engine/         # Game engine contract
│   ├── replay/         # Replay buffer API
│   └── weights/        # Model serving API
├── deployments/        # Deployment configurations
│   ├── local/          # Docker Compose for local dev
│   └── k8s/            # Kubernetes manifests
├── observability/      # Monitoring stack configs
│   ├── grafana/        # Dashboards and datasources
│   ├── prometheus/     # Metrics collection
│   └── loki/           # Log aggregation
├── docs/               # Architecture documentation
├── configs/            # Runtime configurations
└── schemas/            # Data schemas (Parquet, SQL)
```

## Development

### Service-Specific Development

**Rust Services** (Actor, Engine):
```bash
cd services/engine-rust
cargo build          # Build
cargo test           # Test
cargo clippy         # Lint
cargo fmt            # Format
cargo run -p engine-server  # Run
```

**Go Services** (Orchestrator, Replay, Web, Weights):
```bash
cd services/orchestrator-go
go build ./cmd/server    # Build
go test ./...            # Test
go run ./cmd/server      # Run
```

**Python Service** (Learner):
```bash
cd services/learner-py
poetry install                              # Install dependencies
poetry run pytest                           # Test
poetry run ruff check .                     # Lint
poetry run learner --config configs/dev.yaml  # Run
```

### Code Quality

- **Rust**: `cargo clippy` for linting, `rustfmt` for formatting
- **Go**: `golangci-lint` for linting, `gosec` for security
- **Python**: `ruff` for linting, `black` for formatting, `mypy` for type checking

### Testing

Each service maintains colocated tests:
- Rust: `#[cfg(test)]` modules and integration tests
- Go: `*_test.go` files using standard testing package
- Python: `tests/test_*.py` using pytest

Run full test suites before committing:
```bash
# Rust
cargo test --workspace

# Go (from each service directory)
go test ./...

# Python
poetry run pytest
```

## Technology Stack

| Component | Technologies |
|-----------|-------------|
| **Languages** | Rust, Go, Python |
| **Frameworks** | Tonic (Rust gRPC), Chi (Go web), PyTorch (ML) |
| **Protocols** | gRPC, Protocol Buffers |
| **Storage** | MinIO/GCS (objects), PostgreSQL (metadata), Redis (caching) |
| **Observability** | Prometheus (metrics), Loki (logs), Tempo/Jaeger (traces) |
| **Infrastructure** | Docker Compose (local), Kubernetes + Tilt (production) |

## Documentation

- [CLAUDE.md](./CLAUDE.md) - Project overview and guidelines for AI assistants
- [AGENTS.md](./AGENTS.md) - Repository guidelines and development practices
- [ENGINE_OPTIMIZATIONS.md](./ENGINE_OPTIMIZATIONS.md) - Engine performance optimizations
- [docs/](./docs/) - Detailed architecture and component design documentation

## Key Design Principles

### Game Engine Extensibility
All games share a stable protobuf contract—adding new games doesn't require server or protocol changes. Games declare their state/action/observation encodings and schema versions via Capabilities.

### Deterministic Simulation
Game engines use seeded ChaCha20Rng for reproducible training. All artifacts store seeds and schema versions for complete reproducibility.

### Message Passing Architecture
Services communicate via gRPC with structured protobuf messages. Prefer message passing over shared state for scalability and fault tolerance.

### Observability First
All services emit metrics, structured logs with correlation IDs, and distributed traces for comprehensive system visibility.

## Contributing

1. Follow the coding style guidelines in [AGENTS.md](./AGENTS.md)
2. Run tests and linters before submitting PRs
3. Keep commits small and focused with clear commit messages
4. Update documentation for user-facing changes
5. Reference issue IDs in commit messages and PRs

## License

[Add your license information here]

---

Built with Rust, Go, Python, and powered by PyTorch for reinforcement learning at scale.
