# Weights Service Implementation TODO

This document outlines the remaining work to complete the weights service implementation based on the design in `WEIGHTS.md`.

## High Priority - Core Functionality

### 1. gRPC Server Implementation
- [ ] Generate protobuf stubs from `proto/weights/weights.proto`
  - Add protobuf generation to build pipeline
  - Generate Go bindings with proper package paths
- [ ] Implement gRPC handlers in `internal/grpc/` package
  - `PublishWeights` handler calling `service.Publish()`
  - `StreamWeights` handler with proper streaming semantics
  - `GetCurrent` handler for unary requests
- [ ] Wire gRPC server in `cmd/server/main.go`
  - Set up gRPC listener with proper shutdown handling
  - Register service handlers
  - Add health check endpoint

### 2. Redis Compatibility Bridge
- [ ] Implement Redis publisher in `internal/redis/` package
  - Connect to Redis using config settings
  - Publish WeightsBlob messages to configured channel
  - Handle connection failures gracefully
- [ ] Integrate Redis publisher into service layer
  - Mirror publications when `MirrorToRedis` is enabled
  - Ensure atomic registry + Redis operations
  - Add Redis-specific error handling

### 3. Persistent Storage Backends
- [ ] Redis registry implementation (`internal/registry/redis.go`)
  - Store version metadata in Redis hashes
  - Implement bounded history with TTLs
  - Handle Redis connection pooling
- [ ] PostgreSQL registry implementation (`internal/registry/postgres.go`)
  - Design schema for versions table
  - Implement CRUD operations with proper indexing
  - Add migration scripts
- [ ] Registry factory in config layer
  - Switch between memory/redis/postgres based on config
  - Validate backend-specific configuration

## Medium Priority - Production Readiness

### 4. Observability & Monitoring
- [ ] Metrics instrumentation
  - `weights_published_total` counter (matching learner)
  - `weights_publish_duration_seconds` histogram
  - `weights_subscribers_active` gauge
  - `weights_registry_errors_total` counter
- [ ] Distributed tracing
  - Add OpenTelemetry spans to gRPC handlers
  - Trace registry operations
  - Propagate trace context to Redis/DB operations
- [ ] Structured logging enhancements
  - Add correlation IDs to all log messages
  - Log subscriber connect/disconnect events
  - Add debug logging for streaming operations

### 5. Error Handling & Resilience
- [ ] Circuit breaker for Redis operations
- [ ] Retry logic with exponential backoff
- [ ] Graceful degradation when Redis unavailable
- [ ] Connection pooling and timeouts for all external deps
- [ ] Proper context cancellation throughout

### 6. Testing Suite
- [ ] Unit tests for service layer (`internal/service/service_test.go`)
- [ ] Registry implementation tests (memory, redis, postgres)
- [ ] Integration tests with real gRPC server
- [ ] Load testing for subscriber fan-out
- [ ] Benchmark tests for high-frequency publishing

## Lower Priority - Enhancements

### 7. Advanced Features
- [ ] Delta/segment streaming support
  - Extend proto with chunk-based streaming
  - Implement chunked upload/download
  - Add resumable transfer capabilities
- [ ] Multi-run tenancy
  - Namespace isolation between experiments
  - Per-tenant rate limiting
  - Tenant-specific retention policies
- [ ] Retention policies
  - Configurable version history limits
  - Automatic cleanup of old artifacts
  - Archive to cold storage integration

### 8. Deployment & Operations
- [ ] Dockerfile optimization
  - Multi-stage build with minimal runtime image
  - Proper signal handling and graceful shutdown
  - Health check endpoint for container orchestration
- [ ] Kubernetes manifests (`deployments/k8s/base/weights.yaml`)
  - Service, Deployment, ConfigMap resources
  - Resource limits and requests
  - Liveness and readiness probes
- [ ] Helm chart for flexible deployment
- [ ] CI/CD pipeline integration
  - Build and test automation
  - Container image publishing
  - Deployment automation

### 9. Security & Compliance
- [ ] Input sanitization and validation
- [ ] Rate limiting per client/run
- [ ] TLS configuration for gRPC
- [ ] Audit logging for weight publications
- [ ] Secret management for external connections

### 10. Documentation & Examples
- [ ] API documentation generation from proto
- [ ] Client usage examples (Go, Python, Rust)
- [ ] Migration guide from Redis-only setup
- [ ] Troubleshooting guide
- [ ] Performance tuning recommendations

## Implementation Order Recommendation

1. **Phase 1**: gRPC server + basic handlers (items 1)
2. **Phase 2**: Redis compatibility bridge (item 2)
3. **Phase 3**: Persistent backends + testing (items 3, 6)
4. **Phase 4**: Production readiness (items 4, 5)
5. **Phase 5**: Advanced features as needed (items 7-10)

## Success Criteria

- [ ] Learner can successfully publish weights via gRPC
- [ ] Actors receive real-time weight updates via streaming
- [ ] Redis compatibility maintains existing workflows
- [ ] Service handles 1000+ concurrent subscribers
- [ ] Sub-100ms publish-to-subscriber latency
- [ ] Graceful handling of Redis/DB unavailability
- [ ] Comprehensive observability for production debugging

## Notes

- Maintain backward compatibility throughout rollout
- Consider feature flags for gradual feature enablement
- Performance test with realistic load patterns
- Coordinate with learner/actor teams for integration testing