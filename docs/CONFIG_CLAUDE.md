# Configuration Management Analysis and Recommendations

## Executive Summary

This document provides a comprehensive analysis of configuration management across all Cartridge services. The analysis reveals inconsistent configuration patterns, missing configurations for several services, and opportunities for standardization.

## Current Configuration State

### Existing Configuration Patterns

#### 1. **Learner Service (Python)** ✅ Well-Configured
- **Location**: `deployments/local/config/learner.yaml`
- **Pattern**: YAML-based configuration with Pydantic validation
- **Strengths**:
  - Comprehensive configuration covering all aspects (replay, training, algorithm, checkpoints, weights, control)
  - Strong validation with type checking and constraints
  - CLI override support (`--override key=value`)
  - Environment-agnostic design
- **Configuration Sections**:
  - Replay buffer settings
  - Training hyperparameters (learning rate, device, dimensions)
  - Algorithm parameters (PPO-specific settings)
  - Checkpoint management
  - Weight distribution via Redis
  - Orchestrator integration and heartbeat

#### 2. **Actor Service (Rust)** ✅ Well-Configured
- **Location**: `services/actor-rust/src/config.rs`
- **Pattern**: Clap CLI with environment variable fallbacks
- **Strengths**:
  - Command-line and environment variable support
  - Validation logic built-in
  - Type-safe configuration with Serde
  - Clear documentation and help text
- **Configuration Options**:
  - Engine/Replay service addresses
  - Actor identification and environment selection
  - Batch processing parameters
  - Timeout and logging settings

#### 3. **Orchestrator Service (Go)** ✅ Well-Configured
- **Location**: `services/orchestrator-go/internal/config/config.go`
- **Pattern**: Environment variables with structured Go types
- **Strengths**:
  - Comprehensive server, database, NATS, and health configurations
  - Type-safe with reasonable defaults
  - Environment variable driven
- **Configuration Sections**:
  - HTTP server settings
  - PostgreSQL database configuration
  - NATS messaging configuration
  - Health monitoring thresholds

#### 4. **Weights Service (Go)** ✅ Well-Configured
- **Location**: `services/weights-go/internal/config/config.go`
- **Pattern**: Environment variables with structured Go types
- **Strengths**:
  - Comprehensive configuration for all service aspects
  - Redis integration configuration
  - Observability settings
- **Configuration Sections**:
  - gRPC server settings
  - Registry backend configuration
  - Redis compatibility settings
  - Metrics and tracing configuration

### Services with Minimal/Missing Configuration

#### 1. **Engine Service (Rust)** ⚠️ Minimal Configuration
- **Current State**: Hardcoded environment variables in `main.rs`
- **Issues**:
  - Only `ENGINE_SERVER_ADDR` and `ENGINE_METRICS_ADDR` configured
  - No structured configuration pattern
  - Missing game registry configuration
  - No timeout or connection pool settings

#### 2. **Replay Service (Go)** ⚠️ Minimal Configuration
- **Current State**: Command-line flags only in `cmd/server/main.go`
- **Issues**:
  - Only port and max-size configurable
  - No storage backend configuration options
  - Missing performance tuning parameters
  - No observability configuration

#### 3. **Web Service (Go)** ❌ Missing Entirely
- **Current State**: Service directory exists but is empty
- **Missing**:
  - No configuration structure defined
  - No server configuration
  - No integration endpoints configured
  - No authentication/authorization settings

## Configuration Organization Issues

### 1. **Inconsistent Patterns**
- **Rust Services**: Mix of Clap (Actor) vs hardcoded env vars (Engine)
- **Go Services**: Consistent env var pattern but different helper functions
- **Python Services**: YAML with CLI overrides (only Learner exists)

### 2. **Location Inconsistency**
- Configuration files scattered across:
  - `/configs/` (exists but empty subdirectories)
  - `/deployments/local/config/` (only learner.yaml)
  - Service-specific internal directories
  - Environment variables in docker-compose

### 3. **Missing Global Configuration**
- No centralized service discovery configuration
- No shared logging/metrics configuration
- No environment-specific settings (dev/staging/prod)

## Missing Critical Configurations

### 1. **Database Configuration**
- Orchestrator has PostgreSQL config but docker-compose doesn't use it
- No database migration or connection pooling settings
- Missing schema versioning configuration

### 2. **Observability Stack**
- No centralized logging configuration
- Missing metrics collection standardization
- No tracing configuration across services

### 3. **Security Settings**
- No TLS/mTLS configuration
- Missing authentication/authorization settings
- No secret management configuration

### 4. **Service Mesh Configuration**
- No service discovery settings
- Missing load balancing configuration
- No circuit breaker or retry policies

## Docker Compose Configuration Issues

### Current Environment Variables in docker-compose.yml:
```yaml
# Engine
ENGINE_SERVER_ADDR: 0.0.0.0:50051

# Replay
REPLAY_PORT: "8080"
REPLAY_MAX_SIZE: "200000"

# Weights
WEIGHTS_REDIS_ENABLED: "true"
WEIGHTS_REDIS_ADDRESS: redis:6379

# Actor
ACTOR_ENGINE_ADDR: http://engine:50051
ACTOR_REPLAY_ADDR: http://replay:8080
ACTOR_ACTOR_ID: actor-rust-1
ACTOR_ENV_ID: tictactoe
ACTOR_BATCH_SIZE: "64"
ACTOR_FLUSH_INTERVAL: "5"
ACTOR_LOG_LEVEL: info
```

**Issues**:
- Missing orchestrator environment variables
- No database configuration (PostgreSQL not even included)
- Inconsistent port configurations
- No observability stack configuration

## Recommendations

### 1. **Standardize Configuration Patterns**

#### For Go Services:
```go
// Standard pattern for all Go services
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig  // If needed
    Logging  LoggingConfig
    Metrics  MetricsConfig
}

func Load() (*Config, error) {
    // Environment variable loading with defaults
}
```

#### For Rust Services:
```rust
#[derive(Parser, Serialize, Deserialize)]
struct Config {
    #[command(flatten)]
    server: ServerConfig,

    #[command(flatten)]
    logging: LoggingConfig,
}
```

#### For Python Services:
```python
# Continue YAML + Pydantic pattern established by learner
class ServiceConfig(BaseModel):
    server: ServerConfig
    logging: LoggingConfig
```

### 2. **Reorganize Configuration Structure**

**Recommended Directory Structure**:
```
/configs/
├── base/                     # Base configurations
│   ├── logging.yaml
│   ├── metrics.yaml
│   └── database.yaml
├── environments/
│   ├── local/
│   │   ├── docker-compose.yml
│   │   ├── services/
│   │   │   ├── learner.yaml
│   │   │   ├── orchestrator.yaml
│   │   │   ├── web.yaml
│   │   │   └── engine.yaml
│   ├── development/
│   ├── staging/
│   └── production/
└── schemas/                  # JSON schemas for validation
    ├── learner-config.schema.json
    ├── orchestrator-config.schema.json
    └── web-config.schema.json
```

### 3. **Implement Missing Configurations**

#### Engine Service Configuration:
```rust
#[derive(Parser)]
pub struct EngineConfig {
    #[arg(long, env = "ENGINE_SERVER_ADDR", default_value = "0.0.0.0:50051")]
    pub server_addr: String,

    #[arg(long, env = "ENGINE_METRICS_ADDR", default_value = "0.0.0.0:9090")]
    pub metrics_addr: String,

    #[arg(long, env = "ENGINE_MAX_CONNECTIONS", default_value = "100")]
    pub max_connections: usize,

    #[arg(long, env = "ENGINE_REQUEST_TIMEOUT", default_value = "30s")]
    pub request_timeout: Duration,
}
```

#### Replay Service Configuration:
```go
type ReplayConfig struct {
    Server   ServerConfig
    Storage  StorageConfig
    Sampling SamplingConfig
    Metrics  MetricsConfig
}

type StorageConfig struct {
    Backend     string        // memory, postgresql, etc.
    MaxSize     uint64
    Retention   time.Duration
}
```

#### Web Service Configuration:
```go
type WebConfig struct {
    Server        ServerConfig
    Orchestrator  OrchestratorConfig
    Authentication AuthConfig
    Frontend      FrontendConfig
}

type OrchestratorConfig struct {
    BaseURL string
    Timeout time.Duration
}
```

### 4. **Configuration Validation and Management**

#### JSON Schema Validation:
- Create JSON schemas for all configuration formats
- Implement validation in CI/CD pipelines
- Generate documentation from schemas

#### Environment-Specific Overrides:
```yaml
# Base configuration
base: &base
  logging:
    level: info
    format: json

# Environment-specific overrides
local:
  <<: *base
  logging:
    level: debug
    format: console

production:
  <<: *base
  logging:
    level: error
    sampling: 0.1
```

### 5. **Database Integration**

#### Add PostgreSQL to docker-compose:
```yaml
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: cartridge
      POSTGRES_USER: cartridge
      POSTGRES_PASSWORD: cartridge
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./configs/local/database/init.sql:/docker-entrypoint-initdb.d/init.sql

volumes:
  postgres-data:
```

#### Update Orchestrator Configuration:
```yaml
orchestrator:
  environment:
    DB_HOST: postgres
    DB_PORT: 5432
    DB_NAME: cartridge
    DB_USER: cartridge
    DB_PASSWORD: cartridge
  depends_on:
    - postgres
```

### 6. **Observability Stack Configuration**

#### Add Monitoring Services:
```yaml
services:
  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./configs/local/prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana:latest
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
    ports:
      - "3000:3000"
    volumes:
      - grafana-data:/var/lib/grafana
```

## Implementation Priority

### Phase 1 (High Priority)
1. **Create missing web service configuration**
2. **Standardize Engine and Replay service configurations**
3. **Add PostgreSQL to local development stack**
4. **Reorganize /configs directory structure**

### Phase 2 (Medium Priority)
1. **Implement JSON schema validation**
2. **Create environment-specific configuration management**
3. **Add observability stack configurations**
4. **Standardize configuration loading across all services**

### Phase 3 (Low Priority)
1. **Implement configuration hot-reloading**
2. **Add configuration management UI**
3. **Create configuration templates and generators**
4. **Implement configuration encryption for secrets**

## Conclusion

The Cartridge project has a solid foundation for configuration management in some services (Learner, Actor, Orchestrator, Weights) but lacks consistency and completeness across the entire system. The main issues are:

1. **Missing web service configuration entirely**
2. **Inconsistent configuration patterns across languages**
3. **Scattered configuration files and missing centralization**
4. **Missing database integration in local development**
5. **No observability stack configuration**

Implementing the recommendations above will provide a robust, consistent, and maintainable configuration management system that scales from local development to production deployment.