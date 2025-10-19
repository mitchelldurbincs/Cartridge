# Runtime Configuration Presets

This directory hosts example runtime configurations organised by environment. Files in the
`local/` folder mirror the values used by `deployments/local/docker-compose.yml` and can be
copied or sourced when running services outside of Docker Compose.

Each service exposes either YAML or environment variable based settings. Local presets are
intended as a starting point — tweak them to match your setup before exporting or running the
services manually.
