# Recommended Next Steps

Based on the current design documents, here is a prioritized roadmap for the next iteration of the Cartridge project.

## 1. Solidify the Engine Service Foundation
- Scaffold the Rust engine workspace (engine-core, engine-server, engine-proto, and an initial game crate) so the architecture in `ARCHITECTURE.md` has a concrete home.
- Implement the stable gRPC contract and registry/adapter pattern described in the design docs to unlock add-a-game velocity.
- Deliver a deterministic reference game (e.g., tic-tac-toe) with encode/decode coverage and criterion benchmarks to validate performance expectations.

## 2. Define Protobuf Contracts and Code Generation Flow
- Author the engine protobufs and experience schemas that appear across `DESIGN_DOC.md` and the engine implementation plan.
- Wire up Buf/cargo build integration so all services (Go, Rust, Python) consume generated clients from a single source of truth.

## 3. Stand Up the Replay and Learner Loop Skeletons
- Create minimal Go replay service endpoints that satisfy the data flows captured in `DATA_FLOW.md`.
- Stub the Python learner with dataset sampling over gRPC and checkpoint emission to object storage so the orchestrator can observe end-to-end traffic.

## 4. Start the Dashboard MVP Slice
- Follow the "Minimal page set" outlined in `DESIGN_DOC.md` to build the Overview and Run Detail pages backed by mocked data from the orchestrator API.
- Establish the REST/WebSocket contracts early so front-end work can parallelize with backend development.

## 5. Instrumentation and Ops Readiness
- Implement baseline Prometheus metrics and tracing hooks in each service per the observability expectations in the documentation.
- Prepare docker-compose assets for local development so the multi-service topology can be exercised before moving to Kubernetes.

## 6. Document Gaps and Open Questions
- Fill in the placeholder docs (`MVP.md`, `DEPENDENCIES.md`, `LOGGING_TRACING.md`, `SLO_CAPACITY.md`, etc.) with the concrete decisions made during the above steps.
- Capture any new architecture decisions in the ADR folder to keep design intent synchronized with implementation.

These milestones establish the core service loop, developer tooling, and UI surface area needed to iterate on reinforcement learning experiments with confidence.
