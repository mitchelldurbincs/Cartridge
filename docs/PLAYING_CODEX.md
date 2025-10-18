# PLAYING CODEX – Human Play Architecture Blueprint

## Intent
- Enable human players to challenge Cartridge agents through the existing web entry point without compromising the infrastructure-first charter of the project.
- Preserve the “cartridge” metaphor: every new game should feel like a hot-swappable module that plugs into the platform with predictable hooks.
- Keep the runtime surface clean so infrastructure, agent training, and gameplay concerns evolve independently.

## Guiding Principles
- **Strict separation of concerns**: simulation lives in Engine, agent inference remains in Actor/Learner/Weights, human interaction flows through Web, and orchestration stitches them together.
- **Manifest-driven extensibility**: a game is described by contracts and metadata rather than bespoke wiring.
- **Session awareness everywhere**: every component understands the lifecycle of a human vs. agent match and emits events the Replay/Observability stack can consume.
- **Fail-safe defaults**: human play should degrade gracefully (e.g., AI timeouts return best-known move, network hiccups trigger reconnection flow).

## Target Service Topology

```
Browser UI
   │
   ▼
Web Service  ──▶ Session Gateway (new module under orchestrator-go)
                 │             │
                 │             ├──▶ Engine gRPC (per-game rules + state transitions)
                 │             └──▶ Agent Bridge (new facade over actor/weights inference)
                 ▼
Replay/Telemetry (session events, replays, analytics)
```

### Session Gateway (new orchestrator-go module)
- Stateless API tier that owns human-session lifecycle (`CreateSession → Tick → Complete`).
- Maintains in-memory view backed by a durable store (Redis/Postgres) for reconnects and spectator joins.
- Enforces generic session contract: players, current actor, legal actions, clocks, metadata.
- Streams canonical events (`SessionCreated`, `MoveCommitted`, `SessionClosed`) to Replay service.

### Agent Bridge
- Wraps existing actor-runtime endpoints so Session Gateway can request moves with a uniform contract: `GetMove(agent_descriptor, board_state)`.
- Handles model preloading, timeout policy, and move validation against Engine responses.
- Supports personas/difficulty via declarative configuration tied to game manifests.

### Web Service Enhancements
- Adds human session APIs (REST + WebSocket) that forward to Session Gateway.
- Hosts client bundles generated from game cartridges and negotiates capabilities (input methods, board layout, time controls).
- Persists user preferences and match history; integrates with auth/identity if introduced later.

## Game Cartridge Model

```
games/
└── {game_id}/
    ├── manifest.yaml       # declarative capabilities
    ├── ui/                 # web-go bundle (React/Svelte/etc.)
    ├── engine/
    │   └── ruleset.go      # Engine registration + adapters
    └── agents/
        └── personas.yaml   # references to weights + behavior knobs
```

### Manifest Schema (conceptual)
- `game_id`, `display_name`, `version`, `engine_module` (path or fully-qualified name).
- `state_contract`: protobuf messages already implemented in `proto/engine`; manifest maps UI expectations to these types.
- `interaction_model`: turn structure, clock policy, concurrency (simultaneous vs sequential turns), legal action descriptor.
- `ui_capabilities`: viewport hints, required assets, optional tutorials.
- `agent_personas`: named difficulty levels with pointers to weights service artifacts and inference parameters.
- `session_policies`: max turn time, disconnect grace period, resign rules.

### Cartridge Lifecycle
1. **Register** engine module + protobuf contract under `proto/engine`.
2. **Install** manifest, UI bundle, and agent persona config under `games/{game_id}`.
3. **Advertise** new cartridge to Web via registry API (`ListGames` returns manifest-derived summary).
4. **Deploy** updated Engine/Actor binaries with the new ruleset, no change to other services required.

## Session Flow (Human vs Agent)
1. Player selects a cartridge via Web → Web reads manifest summary → calls `CreateSession(game_id, player_profile, agent_persona)`.
2. Session Gateway materializes baseline state from Engine (`NewGame` RPC) and subscribes to Engine updates.
3. Human input arrives via WebSocket → Session Gateway validates, submits to Engine (`ApplyAction`) → receives canonical state → broadcasts to clients.
4. When it is the agent’s turn, Session Gateway invokes Agent Bridge to request a move → Bridge queries Actor runtime with manifest-provided context (legal actions, inference temperature, optional search depth) → Relay move back through Engine for confirmation.
5. Gateway emits structured events to Replay (for post-game review) and Observability (metrics, traces).
6. Session closes gracefully; Web fetches summary + evaluation hints from Agent Bridge if available.

## Data & Observability
- **Session Store**: redis-like cache keyed by `session_id` storing latest Engine snapshot and human metadata; long-term persistence via Replay service.
- **Event Bus**: reuse existing pub/sub (NATS/Kafka if present) to fan-out gameplay events to analytics, leaderboards, or tournament services.
- **Metrics**: latency (human input → Engine ack), agent inference duration, reconnect frequency, AI timeout incidents.
- **Tracing**: correlate Web → Session Gateway → Engine → Agent Bridge spans for troubleshooting user-reported issues.

## Security & Safety Considerations
- Gate human endpoints behind auth rate-limits to prevent DDoS against inference.
- Sanitize human input server-side using Engine’s validation; never trust UI validation alone.
- Enforce move provenance: all state updates originate from Engine; Web only renders confirmed states.
- Provide explicit circuit breakers for runaway matches (e.g., repeated invalid moves, agent failures).

## Rollout Strategy
1. **Spike**: prototype Session Gateway with a single cartridge (e.g., TicTacToe) using canned agent responses.
2. **Bridge Integration**: connect to real Actor inference, define timeout + retry policy, surface telemetry.
3. **Manifest Adoption**: migrate existing games onto manifest spec; build lint tooling that validates completeness.
4. **UI Library**: publish starter kit that consumes manifest and wires state hooks; share guidelines in `docs/Individual Component Design`.
5. **Operational Hardening**: add load tests, chaos scenarios (agent crash, engine slowdown), and automated replay validation.

## Open Questions
- Do we want spectators/streaming at MVP, or defer to a later phase?
- Should human-auth live inside Web or a dedicated Identity service?
- How do we version agent personas when weights are regenerated frequently?
- What is the SLA for agent-latency vs. human timeout, and how does that interact with training pipelines?

## Next Steps for Alignment
- Validate manifest schema against current protobuf contracts; adjust Engine/Actor adapters where needed.
- Schedule architecture review with service owners (Engine, Web, Orchestrator, Replay) to confirm boundaries.
- Produce DX documentation: “Add a Cartridge” guide, manifest template, UI starter implementation plan.
- Define MVP success metrics (match completion rate, UI latency budget) and include them in observability dashboards.

