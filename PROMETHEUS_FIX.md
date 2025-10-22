# Prometheus Metrics Fix - Summary

## Problem Identified

Grafana was not receiving any data because Prometheus was **dropping all scraped metrics** due to overly restrictive `metric_relabel_configs`.

## Root Cause

Each scrape job in `observability/prometheus/prometheus.yml` had a configuration like this:

```yaml
metric_relabel_configs:
  - source_labels: [__name__]
    regex: 'engine_.*'
    action: keep
```

This configuration means: "ONLY keep metrics whose names match `engine_.*`"

**What was being dropped:**
- Standard Prometheus metrics (`up`, `scrape_duration_seconds`, `scrape_samples_scraped`)
- Go runtime metrics (`go_gc_duration_seconds`, `go_goroutines`, `process_cpu_seconds_total`)
- Any service-specific metrics that don't exactly match the pattern

Since services typically export standard Prometheus/runtime metrics BEFORE custom application metrics, and these patterns were too strict, **all metrics were being dropped**.

## Changes Made

**File:** `observability/prometheus/prometheus.yml`

Removed the restrictive `metric_relabel_configs` from all scrape jobs:
- ✅ `engine` job
- ✅ `learner` job
- ✅ `orchestrator` job
- ✅ `replay` job
- ✅ `weights` job
- ✅ `web` job
- ✅ `actor` job

**Backup created:** `observability/prometheus/prometheus.yml.backup`

## What to Do Next

1. **Restart Prometheus** to load the new configuration:
   ```bash
   docker compose -f deployments/local/docker-compose.yml restart prometheus
   ```

2. **Verify metrics are being scraped:**
   - Open Prometheus UI: http://localhost:9090
   - Go to Status → Targets
   - All targets should show as "UP" (green)
   - Check that "Last Scrape" shows a recent timestamp

3. **Verify data in Grafana:**
   - Open Grafana: http://localhost:3000 (admin/admin)
   - Run a simple query like `up` or `go_goroutines`
   - You should now see data points

4. **Expected metrics you should see:**
   - `up{job="engine"}` - Service health (1 = up, 0 = down)
   - `up{job="learner"}` - Learner service health
   - `go_goroutines{job="engine"}` - Go runtime metrics
   - `process_cpu_seconds_total` - CPU usage
   - Plus any custom metrics your services export

## Future Considerations

If you want to reduce metric cardinality in production (to save storage/costs), instead of using `action: keep` with strict patterns, consider:

1. **Drop specific high-cardinality metrics:**
   ```yaml
   metric_relabel_configs:
     - source_labels: [__name__]
       regex: '(go_gc_heap_.*|histogram_bucket_.*)'
       action: drop
   ```

2. **Keep service metrics but allow standard metrics too:**
   ```yaml
   metric_relabel_configs:
     - source_labels: [__name__]
       regex: '(engine_.*|up|go_.*|process_.*)'
       action: keep
   ```

## Verification Commands

After restarting Prometheus, verify with:

```bash
# Check Prometheus targets
curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health, lastScrape: .lastScrape}'

# Query for "up" metric to see which services are being scraped
curl -s 'http://localhost:9090/api/v1/query?query=up' | jq '.data.result[] | {job: .metric.job, value: .value[1]}'

# Check total metrics being stored
curl -s 'http://localhost:9090/api/v1/query?query=count(up)' | jq '.data.result[0].value[1]'
```
