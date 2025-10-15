# Weights Service Redis Integration Design - CLAUDE

**Document Version**: 1.0
**Created**: October 2024
**Author**: Claude Code Assistant
**Status**: Design Phase

## Executive Summary

This document outlines the design for integrating Redis as a persistent storage backend for the Cartridge weights service, extending the existing in-memory registry implementation to provide distributed storage, pub/sub notifications, and backward compatibility with existing Redis-based systems.

## Background

The current weights service (`services/weights-go`) implements an in-memory registry for weight metadata persistence. While functional for development and testing, the in-memory approach has limitations:

- **No Persistence**: Data is lost on service restart
- **Single Instance**: Cannot scale horizontally
- **No Distributed Access**: Other services cannot directly access weight metadata
- **Limited History**: Memory constraints limit version history depth

Redis integration addresses these concerns while maintaining the existing clean abstractions.

## Design Goals

### Primary Objectives
1. **Persistent Storage**: Weight metadata survives service restarts
2. **Distributed Access**: Multiple service instances can share weight state
3. **Pub/Sub Notifications**: Real-time updates for weight consumers
4. **Backward Compatibility**: Support existing Redis-based integrations
5. **Performance**: Sub-millisecond read latency for current weights

### Non-Goals
- **Complex Querying**: Advanced search/filter operations (use PostgreSQL for analytics)
- **Large Data Storage**: Inline weight data should remain under 1MB
- **Distributed Locking**: Registry operations are idempotent by design

## Architecture Overview

### Current Implementation
```
┌─────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ gRPC Client │───▶│ Weights Service  │───▶│ MemoryRegistry  │
└─────────────┘    └──────────────────┘    └─────────────────┘
```

### Proposed Redis Integration
```
┌─────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ gRPC Client │───▶│ Weights Service  │───▶│   RedisRegistry │
└─────────────┘    └──────────────────┘    └─────────────────┘
                                                     │
                   ┌─────────────────────────────────┘
                   │
                   ▼
            ┌─────────────┐    ┌──────────────┐    ┌─────────────┐
            │    Redis    │◀──▶│   Pub/Sub    │───▶│  Consumers  │
            │   Cluster   │    │   Channel    │    │ (Actor/Web) │
            └─────────────┘    └──────────────┘    └─────────────┘
```

## Technical Specification

### Redis Registry Implementation

#### Key Structure
```
weights:current:{run_id}     → JSON(VersionSnapshot)      # Latest version
weights:history:{run_id}     → ZSET(step → JSON)          # Version history
weights:watchers:{run_id}    → SET(watcher_id)            # Active watchers
weights:metadata             → HASH(run_id → metadata)    # Index metadata
```

#### Interface Implementation
```go
type RedisRegistry struct {
    client       redis.UniversalClient
    pubsub       *redis.PubSub
    logger       *zerolog.Logger
    historyCap   int
    watcherMgr   *WatcherManager
}

func (r *RedisRegistry) Upsert(ctx context.Context, input service.PublishInput) (service.VersionSnapshot, error)
func (r *RedisRegistry) Current(ctx context.Context, runID string) (service.VersionSnapshot, error)
func (r *RedisRegistry) Subscribe(runID string, opts service.WatchOptions) (<-chan service.VersionSnapshot, func())
```

### Configuration Extension

#### Environment Variables
```bash
# Registry Backend Selection
WEIGHTS_REGISTRY_BACKEND=redis|memory|postgres  # default: memory

# Redis Connection
WEIGHTS_REDIS_ADDRESS=redis://localhost:6379/0   # Redis connection string
WEIGHTS_REDIS_PASSWORD=secret                    # Optional auth
WEIGHTS_REDIS_TLS_ENABLED=false                  # TLS support
WEIGHTS_REDIS_POOL_SIZE=10                       # Connection pool size

# Pub/Sub Configuration
WEIGHTS_REDIS_CHANNEL=weights:updates             # Notification channel
WEIGHTS_REDIS_BATCH_SIZE=100                     # Batch notification size

# Performance Tuning
WEIGHTS_HISTORY_DEPTH=50                         # Max versions per run
WEIGHTS_CACHE_TTL=3600                           # Cache expiry (seconds)
WEIGHTS_RETRY_ATTEMPTS=3                         # Retry failed operations
```

#### Config Structure Updates
```go
type RegistryConfig struct {
    Backend        string        // "memory", "redis", "postgres"
    PersistenceDSN string        // Connection string
    HistoryDepth   int           // Max versions to retain

    // Redis-specific settings
    Redis RedisRegistryConfig
}

type RedisRegistryConfig struct {
    Address        string
    Password       string
    TLS            bool
    PoolSize       int
    Channel        string
    BatchSize      int
    CacheTTL       time.Duration
    RetryAttempts  int
}
```

## Implementation Plan

### Phase 1: Redis Registry Core (Week 1)

#### Tasks
1. **Add Redis Dependencies**
   ```bash
   go get github.com/redis/go-redis/v9
   ```

2. **Implement RedisRegistry Interface**
   - File: `internal/registry/redis.go`
   - Implement `Registry` interface methods
   - Add connection management and error handling

3. **Update Configuration**
   - Extend `config.go` with Redis settings
   - Add registry backend factory pattern

4. **Basic Testing**
   - Unit tests for Redis operations
   - Integration tests with Redis container

#### Key Implementation Details

**Upsert Operation**
```go
func (r *RedisRegistry) Upsert(ctx context.Context, input service.PublishInput) (service.VersionSnapshot, error) {
    snapshot := service.VersionSnapshot{
        RunID:       input.RunID,
        Step:        input.Step,
        Checksum:    input.Checksum,
        ArtifactURI: input.ArtifactURI,
        InlineBytes: input.InlineBytes,
        Metadata:    input.Metadata,
        PublishedAt: input.PublishedAt,
    }

    pipe := r.client.TxPipeline()

    // Store current version
    currentKey := fmt.Sprintf("weights:current:%s", input.RunID)
    pipe.Set(ctx, currentKey, mustMarshal(snapshot), 0)

    // Add to sorted history
    historyKey := fmt.Sprintf("weights:history:%s", input.RunID)
    pipe.ZAdd(ctx, historyKey, redis.Z{
        Score:  float64(input.Step),
        Member: mustMarshal(snapshot),
    })

    // Trim history to max depth
    pipe.ZRemRangeByRank(ctx, historyKey, 0, -int64(r.historyCap)-1)

    // Publish notification
    pipe.Publish(ctx, r.config.Channel, mustMarshal(notification{
        Type:     "weights_updated",
        RunID:    input.RunID,
        Snapshot: snapshot,
    }))

    _, err := pipe.Exec(ctx)
    return snapshot, err
}
```

### Phase 2: Pub/Sub Integration (Week 2)

#### Watcher Management
```go
type WatcherManager struct {
    mu       sync.RWMutex
    watchers map[string]map[string]chan<- service.VersionSnapshot
    pubsub   *redis.PubSub
}

func (wm *WatcherManager) Subscribe(runID string, opts service.WatchOptions) (<-chan service.VersionSnapshot, func()) {
    ch := make(chan service.VersionSnapshot, 1)

    wm.mu.Lock()
    watcherID := generateWatcherID()
    if wm.watchers[runID] == nil {
        wm.watchers[runID] = make(map[string]chan<- service.VersionSnapshot)
    }
    wm.watchers[runID][watcherID] = ch
    wm.mu.Unlock()

    // Replay latest if requested
    if opts.ReplayLatest {
        go wm.replayLatest(runID, ch)
    }

    cancel := func() {
        wm.mu.Lock()
        delete(wm.watchers[runID], watcherID)
        if len(wm.watchers[runID]) == 0 {
            delete(wm.watchers, runID)
        }
        wm.mu.Unlock()
        close(ch)
    }

    return ch, cancel
}
```

### Phase 3: Advanced Features (Week 3)

#### High Availability
- Redis Cluster support
- Automatic failover handling
- Circuit breaker for Redis operations

#### Monitoring & Observability
- Prometheus metrics for Redis operations
- Connection pool monitoring
- Pub/sub message tracking

#### Performance Optimizations
- Connection pooling
- Pipelining for batch operations
- Local caching for frequently accessed weights

## Data Flow Examples

### Weight Publication Flow
```
1. Learner publishes weights via gRPC
2. Weights service validates input
3. RedisRegistry.Upsert() executes:
   a. Store current version: SET weights:current:{run_id}
   b. Add to history: ZADD weights:history:{run_id}
   c. Trim old history: ZREMRANGEBYRANK
   d. Publish notification: PUBLISH weights:updates
4. Active watchers receive notifications
5. Actor services fetch updated weights
```

### Weight Consumption Flow
```
1. Actor requests current weights via gRPC
2. RedisRegistry.Current() executes:
   a. GET weights:current:{run_id}
   b. Deserialize JSON to VersionSnapshot
3. Actor downloads weights from ArtifactURI
4. Actor validates using Checksum
```

### Streaming Subscription Flow
```
1. Web dashboard subscribes to weights stream
2. WatcherManager registers subscription
3. Redis SUBSCRIBE to weights:updates channel
4. On weight updates:
   a. Redis publishes notification
   b. WatcherManager routes to subscribers
   c. Subscribers receive VersionSnapshot
```

## Error Handling & Resilience

### Connection Failures
- **Retry Logic**: Exponential backoff with jitter
- **Circuit Breaker**: Fail fast after consecutive failures
- **Fallback**: Optional in-memory cache for critical reads

### Data Consistency
- **Atomic Operations**: Use Redis transactions for multi-key updates
- **Conflict Resolution**: Last-writer-wins for same step number
- **Validation**: Checksum verification on read operations

### Monitoring
```go
type RedisMetrics struct {
    Operations    prometheus.CounterVec    // Success/failure by operation
    Latency      prometheus.HistogramVec  // Operation latency
    Connections  prometheus.GaugeVec      // Active connections
    PubSubMsgs   prometheus.CounterVec    // Pub/sub message volume
}
```

## Migration Strategy

### Phase 1: Development Environment
1. Add Redis container to `docker-compose.yml`
2. Update environment configuration
3. Test with existing weights service clients

### Phase 2: Backward Compatibility
1. Support multiple backend types concurrently
2. Add registry backend factory
3. Maintain existing API contracts

### Phase 3: Production Rollout
1. Deploy Redis cluster in production
2. Configure weights service with Redis backend
3. Monitor performance and adjust configuration
4. Migrate existing weight data (if applicable)

## Testing Strategy

### Unit Tests
- Redis registry interface compliance
- Error handling scenarios
- Data serialization/deserialization
- Connection management

### Integration Tests
- Redis container with testcontainers
- Multi-instance coordination
- Pub/sub message delivery
- Failover scenarios

### Performance Tests
- Load testing with concurrent clients
- Pub/sub scalability testing
- Memory usage profiling
- Network partition simulation

## Configuration Examples

### Development (Docker Compose)
```yaml
# deployments/local/docker-compose.yml
services:
  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
    command: redis-server --appendonly yes

  weights:
    environment:
      WEIGHTS_REGISTRY_BACKEND: redis
      WEIGHTS_REDIS_ADDRESS: redis://redis:6379/0
      WEIGHTS_HISTORY_DEPTH: 10
```

### Production (Kubernetes)
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: weights-config
data:
  WEIGHTS_REGISTRY_BACKEND: "redis"
  WEIGHTS_REDIS_ADDRESS: "redis://redis-cluster:6379/0"
  WEIGHTS_REDIS_TLS_ENABLED: "true"
  WEIGHTS_HISTORY_DEPTH: "100"
  WEIGHTS_REDIS_POOL_SIZE: "20"
```

## Security Considerations

### Authentication
- Redis AUTH password support
- Optional TLS encryption for data in transit
- Network segmentation for Redis access

### Data Protection
- Sensitive metadata filtering
- Optional encryption for inline bytes
- Audit logging for weight access

### Access Control
- Service-level authentication via mTLS
- Rate limiting for weight publication
- Resource quotas per run ID

## Success Metrics

### Performance Targets
- **Read Latency**: < 5ms p99 for current weight lookups
- **Write Latency**: < 20ms p99 for weight publication
- **Throughput**: > 1000 weight updates/second
- **Availability**: 99.9% uptime with Redis failover

### Monitoring Indicators
- Redis connection pool utilization
- Pub/sub subscription health
- Background compaction performance
- Memory usage growth patterns

## Future Enhancements

### Potential Extensions
1. **PostgreSQL Backend**: For complex queries and analytics
2. **Multi-Region Replication**: Geographic distribution
3. **Compression**: Reduce memory footprint for large weights
4. **Versioning API**: REST endpoints for weight history
5. **Webhook Notifications**: HTTP callbacks for weight updates

This Redis integration provides a solid foundation for production-grade weight distribution while maintaining the clean abstractions established in the current implementation.