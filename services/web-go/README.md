# Web Service (Go)

The web-go service exposes the HTTP API that powers the Cartridge dashboard. It surfaces health
information, proxies run metadata from the orchestrator, and publishes Prometheus metrics for
monitoring.

## Prerequisites

- Go 1.22+

## Configuration

Configuration is loaded from environment variables prefixed with `WEB_` and optionally from YAML
files in `configs/web-go`. See [`configs/web-go/config.yaml`](../../configs/web-go/config.yaml) for the
available fields.

## Running locally

```bash
cd services/web-go
go run ./cmd/server
```

The service listens on the address defined in `server.host`/`server.port`. Prometheus metrics are
exposed on `/metrics` and a simple health check lives at `/healthz`.

## Testing

```bash
cd services/web-go
go test ./...
```

`go test` runs the handler unit tests with mocked orchestrator clients.

## Development notes

- The orchestrator client is currently an in-memory stub. Replace it with the real client once the
  upstream API is ready (`internal/orchestrator` contains TODOs).
- Requests are instrumented with Prometheus metrics and structured logs using Zerolog.
