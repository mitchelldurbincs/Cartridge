Go 
	* Libraries
		* https://github.com/go-chi/chi - web server
		* zerolog - logging
	* Tools
	* Responsibilities
		* Orchestrate Experience generation
		* Replay Aggregator 
		* Web server
		* Results Aggregator
* Rust
	* Libraries
		* thiserror and anyhow - errors
		* https://github.com/bheisler/criterion.rs - benchmarking
		* rand_chacha - determinism
		* tokio - concurrency
		* tonic - proto client
		* prometheus
		* tracing
		* tracing-opentelemetry
		* prost - proto
		* rayon - for batching 
	* Tools
		* cargo clippy
		* rustfmt
	* Responsibilities
		* Plays the games
		* Create a framework for each new game, but have a foundation for how a game can be played? if that makes sense. (be able to play multiple games in the future)
		* Game simulation
		* Stateless - sees a state, completes the action and returns
		* Batch processing
* Python
	* Libraries
		* PyTorch - ML / reinforcement learning
	* Tools
		* ruff - lint
		* black - format
		* poetry - dependency management
	* Responsibilities
		* Train the models with RL algorithms
		* Periodically save checkpoints
* CI/CD
	* GitHub Actions
		* golangci-lint
		* make zero
		* gosec
	* Docker compose 
		* local dev 
	* kind 
		* checking the k8 stuff locally 
* K8s
	* Use Tilt
	* Prometheus Operator
* Web Server
	* System Status - how many pods are running
* Logging 
	* Use Loki

MUCH LATER
* Go 
	* MatchMaker service so i can implement tournaments 

---

# What the user gets out of the dashboard

## A. Overview (home)

- **Now Playing:** active runs, status, ETA, steps/sec, GPU util.
- **Cluster glance:** pods by service (api/engine/trainer), replicas (from K8s), pending Jobs.
- **Top Signals:** reward curve sparkline, win rate, recent alerts (p95 latency, heartbeat stale).
- **Big buttons:** “New Experiment”, “Run Eval”, “Replay Browser”.

## B. Runs view

- Table: run id, experiment, status, steps, samples/sec, last heartbeat, cost estimate
- Click → detail page with:
    - **Live metrics** (reward, loss, lr, entropy, KL), **resource charts** (GPU/CPU/mem).
    - **Artifacts**: trajectory shards, checkpoints (download/open).
    - **Logs**: trainer/engine logs (tailing), trace links.
    - **Control**: Pause/Resume/Terminate (with confirmation).

## C. Leaderboard / Evaluations

- Compare models (A/B, tournament history).
- ELO (or TrueSkill later), win rate by map/seed bucket
- “Run evaluation” form (pick two models, seeds, N matches) → creates an Eval Job.

## D. Replay theatre

- Pick a replay shard → timeline scrubber → 1×/4×/8× playback.
- Exprt short GIF of a highlight (fun!).

## E. Cluster view (ops-lite)

- Engines: desired/ready replicas, queue depth, steps/sec per pod.
- Trainers: Jobs running/pending/succeeded/failed
- Quick links to Grafana/Tempo if you need deep dive

---

# Should the user configure experiments here?

**Yes—with guardrails.** Make the _experiment_ an immutable template; make the _run_ a concrete execution derived from a template (with optional, whitelisted overrides).

## UX model

- **Create Experiment** (form):
    
    - **Game** (env_id, map/rules hint)
        
    - **Algorithm** (e.g., PPO)
        
    - **Trainer knobs** (batch size, gamma, GAE λ, clip range, entropy coeff)
        
    - **Simulation** (horizon, parallel envs, rollout length)
        
    - **Seeds** (seed_base, per-episode scheme)
        
    - **Resources** (GPU=1, CPU/mem requests)
        
    - **Retention** (shard size/rotation, checkpoint cadence, N to keep)
        
- **Start Run** from an experiment:
    
    - Optional **overrides** for a small, **whitelisted** set (e.g., learning rate, rollout length).
        
    - Everything else stays fixed → true reproducibility.
        

## Live tuning vs immutable config

- **Immutable:** env rules, model arch, optimizer type, horizon, object-store layout.
    
- **Mutable at runtime (bounded):** learning rate, entropy coeff, clip range, evaluation interval.
    
    - Show sliders with min/max, tooltips, and **“apply”** that sends a **signed control event** to the trainer.
        
    - Every change is **audited** (who/when/old→new).
        

> Rule of thumb: “If changing it invalidates the science, make it immutable; if it’s a safety/throughput knob, allow bounded runtime tuning.”

---

# Data contracts that make the UI easy

## Experiment config (example schema)

```json
{
  "env_id": "generals",
  "algo": "PPO",
  "trainer": {
    "lr": 3e-4,
    "gamma": 0.99,
    "gae_lambda": 0.95,
    "clip_range": 0.2,
    "entropy_coef": 0.01,
    "max_grad_norm": 0.5
  },
  "simulation": {
    "horizon": 512,
    "parallel_envs": 32,
    "rollout_len": 128
  },
  "seeds": { "seed_base": 42, "per_episode": "seed_base+episode_idx" },
  "resources": { "gpu": 1, "cpu": "2", "memory": "4Gi" },
  "retention": { "shard_target_mb": 128, "checkpoint_every_steps": 200_000, "keep_checkpoints": 5 },
  "overrides_whitelist": ["trainer.lr", "trainer.entropy_coef", "trainer.clip_range"]
}
```

## REST endpoints (MVP)

- `POST /experiments` → `{experiment_id}`
    
- `POST /runs` `{experiment_id, overrides?}` → `{run_id}`
    
- `POST /runs/{id}/pause|resume|terminate`
    
- `POST /runs/{id}/tune` `{path:"trainer.lr", value:2e-4}` _(server validates against whitelist & bounds)_
    
- `GET /runs/:id/metrics?keys=...` (time series)
    
- `GET /runs/:id/artifacts` (list of shards/checkpoints)
    
- `POST /evals` `{model_a, model_b, n_matches, seed_plan}`
    

## WebSocket events

`RunProgress, CheckpointCreated, EvalResult, TuneApplied, AlertTriggered`

---

# Safety rails for configuration

- **Bounds & validation** in backend (min/max/step) and reflected in the UI widgets.
    
- **Idempotency-Key** on all mutating ops.
    
- **Preview JSON**: “Here’s the exact config/run manifest we’ll launch” with a **diff** against the experiment template.
    
- **RBAC** (later): viewers vs operators vs admins.
    
- **Audit log**: CSV/JSON export of all actions on a run.
    

---

# How the dashboard gets its data

- **Metrics:** Prometheus (API latency, steps/sec, loss, GPU util).
    
- **Traces:** Tempo/Jaeger deep links when a request feels slow.
    
- **Status:** `api-go`’s DB + WebSocket push for live updates.
    
- **K8s state:** `api-go` exposes a summarized view of pods/Jobs (api-go watches K8s and stores minimal state so UI doesn’t need cluster creds).
    

---

# Minimal page set to build first (fast dopamine)

1. **Overview** (cards + sparklines + “New Experiment”).
    
2. **Run detail** (live charts + artifacts + pause/resume + “Tune” drawer).
    
3. **Eval launcher** (A vs B) + **Leaderboard**.
    
4. **Replay browser** (even a simple frame scrubber).
    

---

# Implementation tips

- Drive forms from a **JSON Schema** → automatic validation + server-side re-validation.
    
- Use **server-sent events or WS** for updates; don’t poll.
    
- Keep **run pages URL-addressable** (`/runs/:id`), so you can share links.
    
- Add a **“Copy curl”** for every action (devs love it).
    
- Expose a **dry-run** mode for “Start Run” that only validates and renders the final Job spec.
    

---

## TL;DR

- Dashboard = **TV + Control Room**.
    
- Users **see**: pods, steps/sec, lr/loss, win rate, artifacts, alerts.
    
- Users **do**: create experiments (immutable templates), start runs (with whitelisted overrides), safely **tune a few knobs live**, launch evals, watch replays.
    
- Everything is **bounded, audited, and reproducible**.
    

If you want, I’ll sketch the exact UI panels and the JSON schemas (with min/max/step) so you can wire the Svelte form in a day.