# Cartridge Observability Stack

This directory contains the complete observability infrastructure for the Cartridge reinforcement learning platform. The stack includes metrics collection (Prometheus), visualization (Grafana), log aggregation (Loki), and distributed tracing (Tempo).

## Overview

The observability stack monitors all Cartridge services and provides comprehensive visibility into:
- **Training Progress**: SGD steps, losses, entropy, checkpoint timing
- **RPC Performance**: Request rates, latencies, error rates, cache behavior
- **System Health**: Service availability, resource utilization, error tracking
- **Data Flow**: Replay buffer operations, transition throughput, queue depths
- **Model Distribution**: Weights publishing, subscriber counts

## Quick Start

### 1. Start the Observability Stack

```bash
# From the observability directory
cd observability
docker-compose up -d

# Verify services are running
docker-compose ps
```

### 2. Start Cartridge Services

```bash
# From the deployments/local directory
cd ../deployments/local
docker-compose up -d
```

### 3. Access Dashboards

- **Grafana**: http://localhost:3000
  - Username: `admin`
  - Password: `admin`
- **Prometheus**: http://localhost:9090
- **Loki**: http://localhost:3100
- **Tempo**: http://localhost:3200

## Architecture

### Components

| Component | Port | Purpose |
|-----------|------|---------|
| **Prometheus** | 9090 | Metrics collection and storage |
| **Grafana** | 3000 | Visualization and dashboards |
| **Loki** | 3100 | Log aggregation |
| **Tempo** | 3200, 4317, 4318 | Distributed tracing |

### Metrics Endpoints

All Cartridge services expose Prometheus-compatible metrics:

| Service | Metrics Port | Endpoint |
|---------|--------------|----------|
| Engine | 9000 | http://engine:9000/metrics |
| Learner | 9001 | http://learner:9001/metrics |
| Actor | 9002 | http://actor:9002/metrics |
| Orchestrator | 8080 | http://orchestrator:8080/metrics |
| Replay | 9090 | http://replay:9090/metrics |
| Weights | 9094 | http://weights:9094/metrics |
| Web | 9107 | http://web:9107/metrics |

## Dashboards

The stack includes three pre-configured Grafana dashboards:

### 1. Cartridge System Overview
**Path**: Cartridge → System Overview

High-level view of the entire platform:
- Service health status (UP/DOWN)
- Training progress (SGD steps rate)
- Episode returns across environments
- System-wide request rates
- Error rates by service
- Data pipeline health (queues, buffers)

**Use Cases**:
- Quick system health check
- Identifying performance bottlenecks
- Monitoring training runs
- Detecting anomalies

### 2. Cartridge Learner Service
**Path**: Cartridge → Learner Service

Detailed training metrics:
- Total SGD steps and training rate
- Heartbeat status
- Policy and value losses
- Policy entropy
- Replay sample results and latency
- Prefetch queue depth
- Checkpoint duration
- Weights publish metrics

**Use Cases**:
- Training run monitoring
- Debugging convergence issues
- Optimizing hyperparameters
- Identifying replay bottlenecks

### 3. Cartridge Engine Service
**Path**: Cartridge → Engine Service

RPC and performance metrics:
- Registered games count
- Cached game instances
- RPC request rates by method
- Success vs failure rates
- Error rates
- RPC latency percentiles (P50, P95, P99)
- Game cache hit rates
- Buffer pool utilization

**Use Cases**:
- Performance tuning
- Identifying slow RPCs
- Cache optimization
- Resource monitoring

## Alerts

Prometheus alert rules are configured in `prometheus/alerts.yml`:

### Critical Alerts
- **ServiceDown**: Service unreachable for > 2 minutes
- **LearnerHeartbeatFailure**: Learner can't communicate with orchestrator

### Warning Alerts
- **HighEngineRPCFailureRate**: >5% RPC failures
- **HighReplaySampleErrorRate**: >5% sampling errors
- **TrainingStalled**: No SGD steps for 10 minutes
- **SlowCheckpointing**: P95 checkpoint duration > 60s
- **HighActorEpisodeFailureRate**: >10% episode failures

See `prometheus/alerts.yml` for complete alert definitions.

## Metrics Inventory

Comprehensive documentation of all metrics is available in:
- **Main Docs**: `/docs/METRICS.md`

Key metric categories:
- **Counters**: Total events (SGD steps, requests, errors)
- **Gauges**: Current values (losses, queue depths, cache entries)
- **Histograms**: Distributions (latencies, durations)

## Configuration

### Prometheus Configuration
**File**: `prometheus/prometheus.yml`

- **Scrape Interval**: 15 seconds
- **Retention**: 30 days
- **Service Discovery**: Static configs for all Cartridge services
- **Alert Rules**: Loaded from `alerts.yml`

### Grafana Configuration
**Files**:
- `grafana/grafana.ini`: Server settings
- `grafana/datasources.yml`: Prometheus, Loki, Tempo datasources
- `grafana/dashboards/*.json`: Dashboard definitions

**Default Credentials**:
- Username: `admin`
- Password: `admin`

⚠️ **Change these in production!**

### Loki Configuration
**File**: `loki/loki-config.yml`

- **Retention**: 30 days
- **Storage**: Filesystem-based (local development)
- **Ingestion Limits**: 10MB/s rate, 20MB burst

### Tempo Configuration
**File**: `tempo/tempo-config.yml`

- **Receivers**: OTLP (gRPC/HTTP), Jaeger, Zipkin
- **Storage**: Local filesystem
- **Metrics Generation**: Service graphs and span metrics

## Usage Examples

### Querying Metrics with PromQL

**Training rate over last 5 minutes**:
```promql
rate(learner_sgd_steps_total[5m])
```

**Engine RPC P95 latency**:
```promql
histogram_quantile(0.95, rate(engine_rpc_latency_seconds_bucket[5m]))
```

**Replay buffer error rate**:
```promql
sum(rate(replay_sample_requests_total{result="error"}[5m])) /
sum(rate(replay_sample_requests_total[5m]))
```

**Actor episode success rate by environment**:
```promql
sum(rate(actor_episode_results_total{result="success"}[5m])) by (env_id)
```

### Viewing Logs (Loki)

Access Loki queries through Grafana's Explore interface:

1. Navigate to Grafana → Explore
2. Select "Loki" datasource
3. Use LogQL queries:

```logql
{job="learner"} |= "error"
{service="engine"} | json | level="error"
{job="orchestrator"} |= "heartbeat"
```

### Distributed Tracing (Tempo)

When services are instrumented with OTLP:

1. Navigate to Grafana → Explore
2. Select "Tempo" datasource
3. Search by trace ID or service name
4. View trace timeline and span details

## Maintenance

### Data Retention

**Prometheus**: 30 days (configurable in `prometheus.yml`)
```yaml
--storage.tsdb.retention.time=30d
```

**Loki**: 30 days (configured in `loki-config.yml`)
```yaml
retention_period: 720h
```

**Tempo**: 1 hour for blocks (configured in `tempo-config.yml`)
```yaml
block_retention: 1h
```

### Storage Volumes

Data is persisted in Docker volumes:
- `prometheus-data`: Prometheus time-series database
- `grafana-data`: Grafana dashboards and settings
- `loki-data`: Log chunks and indices
- `tempo-data`: Trace blocks and WAL

**Clear all observability data**:
```bash
docker-compose down -v
```

### Backup

**Export Grafana dashboards**:
```bash
# From Grafana UI: Dashboard → Share → Export → Save to file
# Or via API:
curl -u admin:admin http://localhost:3000/api/dashboards/uid/cartridge-learner | jq .
```

**Export Prometheus data**:
```bash
# Take snapshot
docker exec prometheus promtool tsdb snapshot /prometheus
```

## Troubleshooting

### Services Not Appearing in Grafana

1. Check Prometheus targets: http://localhost:9090/targets
2. Verify all targets show "UP" status
3. Check network connectivity:
   ```bash
   docker exec prometheus wget -O- http://engine:9000/metrics
   ```

### Missing Metrics

1. Verify service is exposing metrics:
   ```bash
   curl http://localhost:9000/metrics  # Engine
   curl http://localhost:9001/metrics  # Learner
   ```

2. Check Prometheus scrape errors:
   - Navigate to http://localhost:9090/targets
   - Look for error messages

3. Verify service environment variables:
   ```bash
   docker exec engine env | grep METRICS
   ```

### High Memory Usage

**Prometheus**:
- Reduce retention period
- Decrease scrape frequency
- Add metric filtering (relabel_configs)

**Loki**:
- Reduce retention period
- Decrease ingestion rate limits
- Enable compaction

### Dashboards Not Loading

1. Check Grafana logs:
   ```bash
   docker logs grafana
   ```

2. Verify dashboard provisioning:
   ```bash
   docker exec grafana ls /etc/grafana/provisioning/dashboards
   ```

3. Reload provisioning:
   ```bash
   docker restart grafana
   ```

## Network Configuration

The observability stack uses two networks:

1. **observability_network**: Internal network for observability components
2. **cartridge_network**: Shared network with Cartridge services

This allows Prometheus to scrape metrics from all Cartridge services while keeping observability components isolated.

## Advanced Configuration

### Enable AlertManager

Uncomment the `alertmanager` service in `docker-compose.yml`:

```yaml
alertmanager:
  image: prom/alertmanager:v0.26.0
  # ... (see docker-compose.yml)
```

Create `alertmanager/config.yml`:
```yaml
route:
  receiver: 'slack'

receivers:
  - name: 'slack'
    slack_configs:
      - api_url: 'YOUR_SLACK_WEBHOOK_URL'
        channel: '#alerts'
```

### Enable Node Exporter

Uncomment the `node-exporter` service to monitor host system metrics (CPU, memory, disk, network).

### Enable Promtail

Uncomment the `promtail` service to collect logs from Docker containers and ship to Loki.

## Production Considerations

When deploying to production:

1. **Change default passwords** in `grafana/grafana.ini`
2. **Enable authentication** for Prometheus and Loki
3. **Use external storage** (S3/GCS) for Loki and Tempo
4. **Set up AlertManager** with proper routing
5. **Configure TLS** for all HTTP endpoints
6. **Use persistent volumes** with backup strategy
7. **Implement access control** (Grafana roles, Prometheus auth)
8. **Monitor the monitoring stack** itself

## Resources

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [Loki Documentation](https://grafana.com/docs/loki/)
- [Tempo Documentation](https://grafana.com/docs/tempo/)
- [PromQL Guide](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Cartridge Metrics Inventory](/docs/METRICS.md)

## Support

For issues or questions:
- Check existing dashboards and alerts
- Review Prometheus targets and scrape health
- Consult `/docs/METRICS.md` for metric definitions
- Check service logs for instrumentation errors
