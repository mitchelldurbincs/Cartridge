# Local Runtime Presets

The files in this directory capture the default configuration values for running Cartridge
services on a developer workstation. They are consumed by the Docker Compose setup in
`deployments/local/` and can also be referenced when exporting environment variables manually.

- `learner.yaml` — baseline PPO learner configuration used by `services/learner-py`. The
  Redis channel under `weights.channel` should match `WEIGHTS_REDIS_CHANNEL` in
  `weights.env` so that weight notifications fan out consistently.
- `*.env` files — shell-friendly environment variables for services that read from the process
environment. Source these files (e.g. `source orchestrator.env`) before starting the service
binary to replicate the Compose defaults.
