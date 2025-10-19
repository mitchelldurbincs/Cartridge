# Cartridge Configuration Review (Codex)

## Snapshot
- Central `configs/` hierarchy is empty despite multiple docs promising runtime presets, leaving contributors without canonical samples.
- Learner is the only service shipping a real config file (`deployments/local/config/learner.yaml`) that exercises the validated schema in `services/learner-py/learner/config.py#L14-L178`, but its location conflicts with the documented layout.
- Weights service is wired to environment variables through `services/weights-go/internal/config/config.go#L10-L128` and is exercised in Compose, yet lacks checked-in examples or a `.env` template.
- Orchestrator defines a rich config struct (`services/orchestrator-go/internal/config/config.go#L10-L111`) but the binary only honours the `-addr` flag (`services/orchestrator-go/cmd/server/main.go#L22-L59`), so none of the database/event settings can be provided today.
- Engine (`services/engine-rust/engine-server/src/main.rs#L24-L52`) and Actor (`services/actor-rust/src/config.rs#L14-L82`) correctly read from environment/flags, but there is no authoritative reference under `configs/`.
- Replay still relies on CLI flags (`services/replay-go/cmd/server/main.go#L18-L110`); the Compose file sets `REPLAY_PORT`/`REPLAY_MAX_SIZE`, yet those variables are unused (`deployments/local/docker-compose.yml#L12-L44`).

## Current Inventory
| Component | Mechanism | Source of Truth | Assessment |
| --- | --- | --- | --- |
| Learner (Python) | YAML/JSON config + CLI overrides | `deployments/local/config/learner.yaml#L1-L39` | Schema validated, working, but should be relocated to `configs/runtime/` and accompanied by prod/dev variants. |
| Weights (Go) | Environment variables | `services/weights-go/internal/config/config.go#L10-L128` | Implementation loads values as expected; provide sample `.env`/doc and ensure Compose sets `WEIGHTS_REGISTRY_BACKEND` when switching modes. |
| Orchestrator (Go) | Intended env-driven config | `services/orchestrator-go/internal/config/config.go#L10-L111` | Unused; wire `config.Load` into `cmd/server` and document expected variables. |
| Replay (Go) | CLI flags | `services/replay-go/cmd/server/main.go#L18-L61` | Works locally, but env vars in Compose are ignored—either teach the binary to read them or drop the unused entries. |
| Engine (Rust) | Env vars (`ENGINE_SERVER_ADDR`, `ENGINE_METRICS_ADDR`) | `services/engine-rust/engine-server/src/main.rs#L24-L52` | Behaviour matches docs; add sample values to a shared `.env` to ease local bootstrapping. |
| Actor (Rust) | Clap flags + env mirrors | `services/actor-rust/src/config.rs#L14-L82` | Healthy parsing/validation path; missing reference config/README for required values. |
| Learner Control Plane | HTTP endpoints | `services/orchestrator-go/internal/http/server.go#L32-L145` | Depends on orchestrator config gaps; once orchestration config is wired, document interplay (heartbeat cadence, DB DSN). |
| Web (Go) | N/A yet | `services/web-go/` | No scaffolding or config choices available. |

## Findings
### Centralised Config Layout Is Drifting
Documentation (for example `docs/FILE_STRUCTURE.md#L1-L107`) expects runtime presets under `configs/runtime/`, but the directory is empty. The lone checked-in example instead lives under `deployments/local/config/`, which makes automation and discovery harder.

### Environment-Only Services Need Reference Material
Weights, engine, and actor can be tuned purely via environment variables, yet there is no `.env.example` enumerating the knobs nor docs mapping them to deployment targets. This slows onboarding and encourages ad-hoc value setting.

### Orchestrator Config Plumbing Is Missing
Although the orchestrator exposes getters for DB/NATS/health timing, the server never calls `config.Load()`. As a result, docker-compose users cannot toggle in-memory vs. Postgres, heartbeat windows, or event sinks without modifying code.

### Replay Flag/Env Mismatch
Compose sets `REPLAY_PORT` and `REPLAY_MAX_SIZE` (`deployments/local/docker-compose.yml#L12-L21`), but the binary reads only `-port`/`-max-size` flags. This mismatch can confuse operators and should be resolved by accepting env fallbacks or by removing the unused settings.

### Missing Web Config Decisions
Because `services/web-go/` is empty, there is no documented configuration surface for the web front end (API host, asset roots, session handling). Capturing defaults early will anchor future implementation.

## Recommendations
1. **Populate `configs/` with canonical presets.** Move `deployments/local/config/learner.yaml` to `configs/runtime/local/learner.yaml`, add a README outlining expected overrides, and introduce stub files for other services (e.g., `configs/runtime/local/orchestrator.env`, `configs/runtime/local/weights.env`).
2. **Document env surfaces.** Extend `docs/` with a short reference mapping each service’s env vars/flags to their semantics (or fold into the service READMEs) so engineers know which knobs are live today.
3. **Wire orchestrator config.** Update `cmd/server/main.go` to call `config.Load()`; honour database/event settings and expose them via flags for local overrides.
4. **Align replay configuration.** Either add env parsing for `REPLAY_PORT`/`REPLAY_MAX_SIZE` or remove those variables from Compose to avoid false signalling.
5. **Scaffold web config early.** When bootstrapping `services/web-go`, publish a plan (`services/web-go/PLAN.md`) and a `configs/runtime/local/web.yaml` placeholder capturing expected upstream addresses and feature flags.
6. **Adopt a shared `.env.example`.** List the cross-service environment variables (engine, actor, weights, orchestrator) to streamline local setups and CI secrets management.
