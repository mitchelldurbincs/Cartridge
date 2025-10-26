# Repository Guidelines

## Project Structure & Module Organization
Cartridge is organized by service under `services/`, with language-specific runtimes: `orchestrator-go`, `weights-go`, `replay-go`, and `web-go` (Go), `engine-rust` and `actor-rust` (Rust), and `learner-py` (Python). Each service keeps its source under `cmd/` or `src/` and colocates tests. Shared protobuf contracts live in `proto/engine`, `proto/replay`, and `proto/weights`; regenerate stubs before shipping protocol changes. Runtime configs sit in `configs/`, deployment manifests in `deployments/`, and observability dashboards in `observability/`. High-level component designs are captured in `docs/Individual Component Design`.

## Prerequisites
Install the following before building services: **Protocol Buffers Compiler (protoc) v25.1+** (required for Rust gRPC services; install via `apt-get install protobuf-compiler` or download from https://github.com/protocolbuffers/protobuf/releases), **Rust toolchain** via rustup, **Go 1.21+**, **Python 3.10+** with Poetry, and **Docker/Docker Compose** for local deployments. Verify protoc with `protoc --version`.

## Build, Test, and Development Commands
- Go services: `cd services/orchestrator-go && go run ./cmd/server` to launch, `go test ./...` for unit suites; apply the same pattern for `services/weights-go` and `services/replay-go`.
- Rust services: `cd services/engine-rust && cargo check && cargo test`; use `cargo run -p engine-server` for the engine binary and mirror that workflow for `services/actor-rust`.
- Learner (Python): `cd services/learner-py && poetry install`, `poetry run pytest`, and `poetry run learner --config configs/runtime/dev.yaml` to exercise the training loop locally.

## Coding Style & Naming Conventions
Use formatter tooling before committing: `go fmt ./...` for Go, `cargo fmt --all` and `cargo clippy --all-targets --all-features` for Rust, and `poetry run ruff check .` plus `poetry run mypy` for Python. Follow idiomatic casing (`CamelCase` types, `snake_case` functions in Python, `LowerCamel` Go receivers). Python modules target 100-character lines per `pyproject.toml`. Keep protobuf packages versioned alongside schema changes and regenerate language bindings after edits.

## Testing Guidelines
Prefer fast unit coverage colocated with each component: Go tests reside in `_test.go` files, Rust uses `#[cfg(test)]` modules and integration crates under `engine-rust/`, and Python relies on `services/learner-py/tests/test_*.py` and async fixtures. Name tests after the behavior under test (e.g., `TestRunLifecycle`). Run the relevant suite (`go test ./...`, `cargo test`, `poetry run pytest`) before submitting; add integration checks in `tests/` when wiring multiple services.

## Commit & Pull Request Guidelines
Keep commits small, imperative, and scoped (e.g., `feat: scaffold weights service`, `docs: plan Redis integration`). Reference issue IDs or design docs when applicable. PRs should outline intent, summarize testing (`go test`, `cargo test`, etc.), and call out config or protocol changes. Include screenshots or API samples when touching user-facing surfaces, and request reviewers from the owning service.
