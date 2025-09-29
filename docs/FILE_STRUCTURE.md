cartridge/
├─ services/
│  ├─ orchestrator-go/
│  ├─ replay-go/
│  ├─ weights-go/
│  ├─ web-go/                         # UI/API (Svelte UI under /web-go/ui)
│  ├─ engine-rust/                    # console server (tonic) + registry
│  ├─ games/
│  │  ├─ tictactoe/
│  │  └─ <next-game>/
│  └─ learner-py/
│     ├─ learner/                     # training loop, checkpoint, IO
│     └─ scripts/                     # train.py, eval.py
│
├─ proto/
│  ├─ engine/v1/engine.proto
│  └─ experience/v1/experience.proto
│
├─ deployments/
│  ├─ local/                          # Docker Compose for dev
│  │  ├─ docker-compose.yml
│  │  ├─ docker-compose.observability.yml
│  │  ├─ promtail-config.yml
│  │  ├─ otel-collector.yaml          # local collector pipeline
│  │  └─ grafana-provisioning/        # local datasources/dashboards
│  ├─ k8s/                            # raw manifests for GKE (kustomize)
│  │  ├─ base/
│  │  │  ├─ orchestrator.yaml
│  │  │  ├─ replay.yaml
│  │  │  ├─ learner-job.yaml          # Indexed Job (Kueue)
│  │  │  ├─ actors-deploy.yaml
│  │  │  ├─ weights.yaml
│  │  │  ├─ web.yaml
│  │  │  ├─ rbac-secrets-netpol.yaml  # RBAC, NetworkPolicy, PodSecurity
│  │  │  ├─ serviceaccounts.yaml      # with Workload Identity annotations
│  │  │  └─ externalsecrets.yaml      # pull from Secret Manager (optional)
│  │  ├─ overlays/
│  │  │  ├─ dev/
│  │  │  └─ prod/
│  │  └─ kueue/                       # ClusterQueue/LocalQueue specs
│  └─ helm/                           # optional charts if you prefer Helm
│
├─ observability/
│  ├─ prometheus/
│  │  ├─ rules/                       # alert rules (yaml)
│  │  └─ scrape/                      # extra scrape configs (if any)
│  ├─ grafana/
│  │  ├─ dashboards/                  # JSON dashboards (actor/replay/learner)
│  │  └─ datasources/                 # Prom/Loki/Tempo/Thanos (if used)
│  ├─ loki/                           # Loki configs (local & helm values)
│  │  ├─ loki-values.gcs.yaml         # prod: boltdb-shipper + GCS backend
│  │  └─ loki-local.yaml              # dev: filesystem/MinIO
│  ├─ tempo/                          # Tempo configs (local & helm values)
│  │  ├─ tempo-values.gcs.yaml        # prod: GCS backend
│  │  └─ tempo-local.yaml
│  └─ otel-collector/
│     ├─ collector-local.yaml         # 100% sample to Tempo (dev)
│     └─ collector-prod.yaml          # tail sampling + enrich to Tempo (prod)
│
├─ infra/                              # Terraform (GCP) — cloud-side IaC
│  ├─ envs/
│  │  ├─ dev/
│  │  │  ├─ main.tf                   # compose modules below
│  │  │  ├─ variables.tf
│  │  │  ├─ terraform.tfvars
│  │  │  └─ backend.tf                # GCS remote state
│  │  └─ prod/
│  │     ├─ main.tf
│  │     ├─ variables.tf
│  │     ├─ terraform.tfvars
│  │     └─ backend.tf
│  ├─ modules/
│  │  ├─ network/                     # VPC, subnets, NAT, firewall
│  │  ├─ gke/                         # GKE cluster + nodepools (GPU pool opt.)
│  │  ├─ buckets/                     # GCS: runs, parquet, loki, tempo (+lifecycle)
│  │  ├─ sql/                         # Cloud SQL Postgres (private IP)
│  │  ├─ redis/                       # Memorystore (Redis) for weights/stats/streams
│  │  ├─ artifact_registry/           # Docker/repos
│  │  ├─ iam/                         # SAs, IAM roles, Workload Identity bindings
│  │  ├─ dns/                         # (optional) Cloud DNS, static IPs
│  │  └─ budgets/                     # billing budgets/alerts
│  └─ README.md                       # how to init/apply/destroy
│
├─ configs/
│  ├─ experiments/                    # experiment.yaml (DSL) samples
│  ├─ rewards/                        # RewardSpec files (env.version.yaml)
│  ├─ weights/                        # rollout configs (blue/green split)
│  └─ runtime/                        # service configs (limits, feature flags)
│
├─ schemas/
│  ├─ parquet/
│  │  ├─ steps.schema.md
│  │  └─ episodes.schema.md
│  └─ sql/
│     ├─ migrations/
│     │  ├─ 0001_init.sql
│     │  └─ 0002_episode_summary.sql
│     └─ queries/                     # UI/aggregator canned queries
│
├─ tools/
│  ├─ buf.gen.yaml                    # codegen targets for Go/Rust/Python
│  ├─ gen.sh                          # wrapper around buf generate
│  ├─ linters/                        # golangci-lint, ruff, clippy cfgs
│  └─ scripts/                        # dev helpers (orc, cp, ls, tf wrappers)
│
├─ tests/
│  ├─ golden/                         # blessed outputs (hashes, episodes.jsonl, run.json)
│  ├─ integration/                    # e2e compose tests
│  └─ load/                           # k6/vegeta profiles for Replay
│
├─ docs/
│  ├─ ARCHITECTURE.md
│  ├─ MVP.md
│  ├─ DATA_FLOW.md
│  ├─ LOGGING_TRACING.md
│  ├─ SLO_CAPACITY.md
│  ├─ FAILURE_BACKPRESSURE.md
│  ├─ PARQUET_SCHEMA_CATALOG.md
│  ├─ RUN_REGISTRY_SCHEMA.md
│  ├─ GOLDEN_RUN.md
│  ├─ GKE_BLUEPRINT.md
│  ├─ SECURITY_IAM.md                 # WI, SA-to-role matrix, NetPolicy
│  ├─ REWARD_SPEC.md
│  ├─ DEPENDENCIES.md                 # 1-pager dependency list
│  ├─ ADR/                            # short Architecture Decision Records
│  │  ├─ 0001-tempo-over-jaeger.md
│  │  ├─ 0002-redis-usage.md
│  │  ├─ 0003-parquet-schema-v1.md
│  │  └─ 0004-thanos-when-and-why.md
│  └─ GLOSSARY.md
│
├─ .github/workflows/
│  ├─ ci-build.yml                    # build & test (go/rust/python)
│  ├─ ci-lint.yml                     # golangci-lint, ruff, clippy
│  ├─ ci-golden.yml                   # runs golden run (CPU, compose)
│  └─ release-images.yml              # push images (with SBOM/signing)
│
├─ Makefile                           # gen, build, up/down, lint, fmt, golden, tf
├─ buf.yaml                           # buf module config
├─ .env.example                       # local minio/redis/postgres creds
└─ README.md
