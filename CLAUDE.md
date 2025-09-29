# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Cartridge is a reinforcement learning platform for training AI agents to play games. It's built as a microservices architecture with distributed components handling game simulation, learning, replay storage, and orchestration.

## Architecture

The system consists of these main services:

- **Actor (Rust)** - Game actor service that plays games using trained models, generates experience data, and sends it to replay buffer
- **Engine (Rust)** - Game simulation service that provides a pluggable framework for different games using a generic protobuf contract. Games implement the `Game` trait with typed state/action/observation, which gets converted to an erased interface for gRPC communication.
- **Learner (Python)** - PyTorch-based reinforcement learning training service that implements RL algorithms and periodically saves model checkpoints
- **Replay (Go)** - Experience buffer service that stores and samples game transitions for training
- **Orchestrator (Go)** - Coordinates experiment runs and manages system state using chi web server and zerolog logging
- **Web (Go)** - Dashboard web server for monitoring runs, visualizing metrics, and managing experiments
- **Weights (Go)** - Model weight distribution service that publishes trained models to actors

## Key Design Principles

### Game Engine Extensibility
- All games share one stable protobuf contract - adding new games doesn't require server/proto changes
- Games declare their encodings and schema versions via Capabilities
- State/Action/Observation are encoded as bytes with declared formats (e.g., `state:packed_u8:v1`)
- Deterministic simulation using ChaCha20Rng seeding

### Data Flow
- Actor (Rust) → Engine (Rust): Game step requests via gRPC
- Actor (Rust) → Replay (Go): Experience transitions via gRPC streaming
- Learner ↔ Replay: Sample batches for SGD via gRPC
- Learner → Object Store: Model checkpoints as safetensors + manifest
- Weights (Go) → Actor (Rust): Model distribution via gRPC
- All services → Observability: Metrics/logs/traces to Prometheus/Loki/Tempo

## Development Commands

### Local Development
```bash
# Start all services locally
docker-compose -f deployments/local/docker-compose.yml up

# Individual service development - check each service directory for:
# - Rust: cargo build, cargo test, cargo clippy, rustfmt
# - Go: go build, go test, golangci-lint, make zero, gosec  
# - Python: poetry install, ruff (lint), black (format)
```

### Testing
- Rust services: Use `cargo test` and `criterion` for benchmarking
- Go services: Use `go test` with standard testing patterns
- Python services: Use pytest with poetry dependency management

### Code Quality Tools
- Rust: `cargo clippy` for linting, `rustfmt` for formatting
- Go: `golangci-lint` for linting, `gosec` for security analysis
- Python: `ruff` for linting, `black` for formatting

## Project Structure

- `services/` - Individual microservices (actor-rust, engine-rust, learner-py, orchestrator-go, replay-go, web-go, weights-go)
- `games/` - Game implementations (e.g., tictactoe)
- `proto/` - Protobuf definitions for service communication
- `deployments/` - Docker Compose (local) and Kubernetes (k8s) configurations
- `docs/` - Architecture documentation including detailed design docs
- `observability/` - Monitoring stack configuration (Grafana, Prometheus, Loki)
- `schemas/` - Data schemas for Parquet and SQL storage

## Technology Stack

- **Languages**: Rust (engine), Go (orchestration/web), Python (ML)
- **Frameworks**: Tonic (Rust gRPC), Chi (Go web), PyTorch (ML)
- **Infrastructure**: Docker Compose (local), Kubernetes with Tilt (prod)
- **Storage**: MinIO/GCS for objects, PostgreSQL for metadata, Redis for caching
- **Observability**: Prometheus metrics, Loki logging, Tempo/Jaeger tracing

## Adding New Games

1. Create game crate in `games/<env_id>` implementing the `Game` trait
2. Implement deterministic `reset/step` logic with ChaCha20Rng
3. Define `encode/decode` for State/Action/Obs types
4. Set Capabilities (action space, encodings, max_horizon, preferred_batch)
5. Register game factory in engine server
6. Add tests for determinism and encode/decode round-trips
7. Benchmark performance with criterion

## Common Patterns

- Use structured logging with correlation IDs across services
- Implement graceful shutdown handlers for all services
- Follow protobuf evolution practices (add fields, never renumber, use reserved)
- Store deterministic seeds and schema versions in artifacts for reproducibility
- Prefer message passing over shared state for concurrency