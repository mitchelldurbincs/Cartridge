# Cartridge

Cartridge is a modular reinforcement learning platform that combines a high-performance Rust game engine with Go- and Python-based services for coordinating large-scale training runs. It ships with a reference TicTacToe environment, distributed actors and learners, replay storage, and operational tooling so you can iterate on new games and training algorithms quickly.

## Platform overview

Cartridge follows a hub-and-spoke architecture made up of specialized services:

- **Engine (Rust)** – hosts deterministic game implementations behind a stable gRPC contract so new cartridges can be registered without breaking existing clients.
- **Actor (Rust)** – runs game episodes against the engine and streams transitions to replay for experience collection.
- **Replay (Go)** – retains experience, supports uniform and prioritized sampling, and exposes statistics for monitoring buffer health.
- **Learner (Python)** – orchestrates SGD loops by pulling mini-batches from replay and publishing checkpoints to object storage.
- **Weights (Go)** – distributes model weights and exposes version metadata via Redis-backed coordination.
- **Orchestrator (Go)** – tracks experiment state, heartbeats, and control-plane commands for coordinating the fleet.
- **Web (Go/Svelte)** – provides a lightweight dashboard for inspecting runs and telemetry.

Each component communicates over protobuf-defined gRPC APIs, and shared contracts live under [`proto/`](proto/). Engine cartridges implement typed game logic that is erased behind a byte-level service interface, letting you evolve gameplay without touching the wire format.

## Repository layout

The repository is organized by service with supporting infrastructure, configuration, and documentation alongside the codebase:

```
services/                # Go, Rust, and Python microservices
proto/                   # gRPC API definitions shared across the stack
deployments/             # Docker Compose (local) and Kubernetes manifests
observability/           # Prometheus, Grafana, Loki, Tempo configuration
configs/                 # Runtime, experiment, and reward configuration files
infra/                   # Terraform modules and environment definitions
tests/                   # Integration, load, and golden-run suites
docs/                    # Architecture notes, design docs, and ADRs
```

Refer to [`docs/FILE_STRUCTURE.md`](docs/FILE_STRUCTURE.md) for a detailed, regularly updated tree with per-directory annotations.

## Getting started

### Prerequisites

Install the language toolchains and system dependencies used across the services:

- [Protocol Buffers compiler](https://github.com/protocolbuffers/protobuf/releases) **v25.1+**
- [Rust toolchain](https://www.rust-lang.org/tools/install)
- [Go 1.21+](https://go.dev/dl/)
- [Python 3.10+](https://www.python.org/downloads/) with [Poetry](https://python-poetry.org/)
- [Docker](https://www.docker.com/) and Docker Compose for local orchestration

You can validate your `protoc` installation with `protoc --version` before generating bindings.

### Clone and bootstrap

```bash
# Clone the repository
git clone https://github.com/<your-org>/cartridge.git
cd cartridge

# Install Python dependencies for the learner
cd services/learner-py
poetry install
cd -
```

Generate protobuf bindings for the language runtimes when you change a contract:

```bash
./tools/gen.sh
```

### Run the stack with Docker Compose

A curated Docker Compose file lives under `deployments/local`. It builds each service from source, starts observability tooling, and wires healthy dependencies automatically.

```bash
# From the repository root
cd deployments/local
docker compose up --build
```

The stack publishes the primary service ports to your host:

- Engine: `localhost:50051` (gRPC) and `localhost:9000` (metrics)
- Replay: `localhost:8081` (gRPC) and `localhost:9100` (metrics)
- Orchestrator API: `localhost:8080`
- Weights API: `localhost:8082`
- Learner metrics: `localhost:9001`
- Actor metrics: `localhost:9002`
- Prometheus / Grafana / Loki / Tempo: `localhost:9090`, `3000`, `3100`, `3200`

Configuration files mounted into the containers live in `configs/runtime/local` and `observability/` so you can tweak settings without rebuilding images.

### Running services from source

You can also run each component directly from your development environment.

#### Engine service (Rust)

```bash
cd services/engine-rust
cargo run -p engine-server
```

The engine workspace contains the typed game traits (`engine-core`), the tonic gRPC server (`engine-server`), generated protobuf bindings (`engine-proto`), and sample games like TicTacToe (`games-tictactoe`).

#### Replay service (Go)

```bash
cd services/replay-go
go run ./cmd/server -port 8080 -max-size 200000
```

The replay server exposes gRPC methods for storing transitions, sampling batches (uniform or prioritized), reporting buffer stats, and updating priorities, all backed by an in-memory store by default.

#### Orchestrator service (Go)

```bash
cd services/orchestrator-go
go run ./cmd/server -addr :8080
```

The orchestrator surfaces REST endpoints for creating runs, ingesting learner heartbeats, and managing command queues that coordinate the rest of the system.

#### Learner service (Python)

```bash
cd services/learner-py
poetry run learner --config configs/runtime/local/learner.yaml
```

The learner scaffolding focuses on configuration loading, replay integration, and training-loop orchestration so you can drop in custom algorithms over time.

#### Actor service (Rust)

```bash
cd services/actor-rust
cargo run -- --env-id tictactoe --batch-size 32
```

The Rust actor connects to the engine and replay services, runs episodes with a random policy, batches transitions, and streams them to replay. Flags can also be provided through `ACTOR_*` environment variables for containerized deployments.

#### Weights service (Go)

```bash
cd services/weights-go
go run ./cmd/server
```

This service brokers model artifacts and publishes version metadata, optionally integrating with Redis for coordination as demonstrated in the local Docker Compose stack.

## Testing and quality

Each language runtime uses its ecosystem’s tooling:

- Go services – `go test ./...`
- Rust services – `cargo test`
- Python packages – `poetry run pytest`

Continuous integration workflows under `.github/workflows/` run build, lint, and golden-run suites so pull requests receive fast feedback.

## Observability

Local deployments include Prometheus for metrics, Grafana for dashboards, Loki for logs, and Tempo for traces. Configuration for these systems lives under `observability/` and is mounted by the Docker Compose stack, so edits to dashboards or scrape configs take effect on the next restart.

## Further reading

The [`docs/`](docs/) directory captures deep dives into architecture, data flow, design decisions, and operational playbooks. Highlights include:

- [`ARCHITECTURE.md`](docs/ARCHITECTURE.md) – engine design, registry, and service layering.
- [`DATA_FLOW.md`](docs/DATA_FLOW.md) – end-to-end protocols between services.
- [`Individual Component Design/`](docs/Individual%20Component%20Design) – detailed specs for orchestrator, learner, replay, and more.
- [`SLO_CAPACITY.md`](docs/SLO_CAPACITY.md) and [`LOGGING_TRACING.md`](docs/LOGGING_TRACING.md) – operational targets and observability guidance.

Whether you are experimenting with a new cartridge or scaling production training, this repository provides the building blocks, tooling, and documentation needed to ship reinforcement learning workloads with confidence.
