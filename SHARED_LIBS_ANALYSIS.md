# Cartridge Codebase - Shared Libraries Analysis

## Executive Summary

This analysis identifies significant opportunities for creating shared libraries in the Cartridge codebase to reduce code duplication and establish consistent patterns across services. The analysis covers both Rust services (actor-rust, engine-rust) and Go services (orchestrator-go, replay-go, web-go, weights-go).

**Key Findings:**
- **Rust Services**: Opportunity for 2-3 shared workspace crates (already using workspace structure)
- **Go Services**: Opportunity for 2-4 shared packages/modules
- **Cross-Service**: Opportunity for shared patterns in observability, configuration, and gRPC setup

---

## Rust Services Analysis

### Current State
- **Actor-rust**: Standalone binary with all code in `src/`
- **Engine-rust**: Already using workspace structure with multiple crates
  - `engine-core`: Core game traits and types
  - `engine-proto`: Protocol buffer definitions
  - `engine-server`: gRPC server implementation
  - `games-tictactoe`: Game implementation

### Shared Library Opportunities

#### 1. **cartridge-observability** (New Shared Crate)

**Current Duplication:**
- Both `actor-rust` and `engine-server` implement similar metrics/tracing initialization
- Shared dependencies: `tracing`, `tracing-subscriber`, `metrics`, `metrics-exporter-prometheus`

**Files Involved:**
- `/home/user/Cartridge/services/actor-rust/src/main.rs` (lines 101-151)
- `/home/user/Cartridge/services/engine-rust/engine-server/src/main.rs` (lines 43-63)

**Duplicated Code Pattern:**
```rust
// actor-rust (101-126 lines)
fn initialize_tracing(log_level: &str) -> Result<LevelFilter> { ... }

// engine-server (17-23 lines)
tracing_subscriber::fmt::init();

// Both also implement:
fn initialize_metrics(addr: Option<&str>) -> Result<()> { ... }
```

**Proposed Shared Module:**
```rust
pub mod observability {
    pub fn init_tracing(log_level: &str) -> Result<LevelFilter>
    pub fn init_prometheus_metrics(addr: Option<&str>) -> Result<()>
}
```

**Estimated Savings:**
- ~150 lines of duplicated code
- Consistent behavior across services
- Centralized configuration for observability stack

---

#### 2. **cartridge-config** (New Shared Crate)

**Current Duplication:**
- `actor-rust` uses `clap` + `serde` for configuration
- Only actor-rust needs this currently, but engine-server could benefit from CLI argument handling

**Files Involved:**
- `/home/user/Cartridge/services/actor-rust/src/config.rs` (170 lines)
- `/home/user/Cartridge/services/actor-rust/src/main.rs` (lines 36-40)

**Pattern:**
```rust
#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    #[arg(long, env = "ACTOR_*", default_value = "...")]
    pub field: Type,
}

impl Config {
    pub fn validate(&self) -> Result<()> { ... }
}
```

**Proposed Shared Utilities:**
- Base trait for configuration validation
- Helper macros for environment variable parsing
- Standard validation error types

**Estimated Savings:**
- ~80 lines in actor-rust
- Framework for other services to use when adding CLI support

---

#### 3. **cartridge-grpc-client** (Enhancement)

**Current Duplication:**
- Both actor-rust connects to external gRPC services (engine, replay)
- Shared pattern: Channel creation, endpoint resolution, error handling

**Files Involved:**
- `/home/user/Cartridge/services/actor-rust/src/actor.rs` (lines 28-68)

**Current Pattern:**
```rust
let engine_channel = tonic::transport::Endpoint::new(config.engine_addr.clone())?
    .connect()
    .await
    .map_err(|e| anyhow!("Failed to connect to engine at {}: {}", config.engine_addr, e))?;

let engine_client = EngineClient::new(engine_channel);
```

**Proposed Utilities:**
- Generic client connector with retry logic
- Connection pooling helpers
- Standardized connection error types

---

## Go Services Analysis

### Current State
- **orchestrator-go**: Full HTTP server with comprehensive features (3059 lines internal)
- **replay-go**: gRPC service (1573 lines internal)
- **web-go**: HTTP server + orchestrator client
- **weights-go**: gRPC service with Redis backend

### Shared Library Opportunities

#### 1. **github.com/cartridge/shared/config** (New Package)

**Current Duplication Across Services:**

**Files:**
- `/home/user/Cartridge/services/orchestrator-go/internal/config/config.go` (110 lines)
- `/home/user/Cartridge/services/web-go/internal/config/config.go` (191 lines)
- `/home/user/Cartridge/services/weights-go/internal/config/config.go` (128 lines)

**Duplicated Patterns:**
```go
// All three services implement identical functions:
func getEnvString(key, defaultValue string) string { ... }
func getEnvInt(key string, defaultValue int) int { ... }
func getEnvDuration(key string, defaultValue time.Duration) time.Duration { ... }
func getEnvBool(key string, def bool) bool { ... }  // Only in weights-go
```

**Proposed Shared Package Structure:**
```
shared/config/
├── env.go          // Helper functions for env vars
├── server.go       // ServerConfig types & helpers
├── types.go        // Common config types
└── loader.go       // Generic config loading utilities
```

**Reusable Types:**
```go
// Shared ServerConfig pattern
type ServerConfig struct {
    Host            string
    Port            int
    ReadTimeout     time.Duration
    WriteTimeout    time.Duration
    ShutdownTimeout time.Duration
}

func (s ServerConfig) Address() string { ... }
func (s ServerConfig) Endpoint() string { ... }
```

**Estimated Savings:**
- ~160 lines of environment helper functions
- ~80 lines of ServerConfig boilerplate
- Consistent configuration behavior across all Go services

---

#### 2. **github.com/cartridge/shared/logging** (New Package)

**Current Duplication:**

**Files:**
- `/home/user/Cartridge/services/orchestrator-go/cmd/server/main.go` (line 25)
- `/home/user/Cartridge/services/web-go/cmd/server/main.go` (line 25)
- `/home/user/Cartridge/services/weights-go/cmd/server/main.go` (line 28)
- `/home/user/Cartridge/services/web-go/internal/logging/logging.go`

**Duplicated Logger Initialization:**
```go
// All services do similar initialization
logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
```

**Proposed Shared Package:**
```go
// shared/logging/
func NewLogger() *zerolog.Logger { ... }
func SetLevel(logger *zerolog.Logger, level string) *zerolog.Logger { ... }
func WithContext(logger *zerolog.Logger, fields map[string]interface{}) *zerolog.Logger { ... }
```

**Additional Shared Utilities:**
- `web-go/internal/logging/logging.go` functions (`ParseLevel`, `ShouldLog`) could be standardized
- Custom log formatter templates for consistency
- Request ID/correlation ID helpers

**Estimated Savings:**
- ~20 lines per service (4 services × 20 = 80 lines)
- Consistent log levels and formatting across all services

---

#### 3. **github.com/cartridge/shared/observability** (New Package)

**Current Duplication:**

**Metrics Setup Patterns - Each Service Re-implements Metrics Collection:**

Files with Duplicated Patterns:
- `/home/user/Cartridge/services/orchestrator-go/internal/metrics/metrics.go` (107 lines)
- `/home/user/Cartridge/services/replay-go/internal/metrics/metrics.go` (175 lines)
- `/home/user/Cartridge/services/web-go/internal/http/metrics.go` (28 lines)
- `/home/user/Cartridge/services/weights-go/internal/observability/metrics.go` (110 lines)

**Common Pattern:**
```go
// Every service implements similar patterns:
1. Create prometheus.Registerer
2. Create Collectors (Counter, Histogram, Gauge)
3. Register metrics
4. Provide Handler() for HTTP endpoint
5. Methods to record metrics

// Common helpers:
func (c *Collector) Handler() http.Handler {
    return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}
```

**Shared Histogram/Counter Creation:**
```go
// orchestrator-go/internal/metrics/metrics.go (34-44)
heartbeatLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
    Name:    "orchestrator_heartbeat_latency_seconds",
    Help:    "Time elapsed between consecutive heartbeats per run.",
    Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120},
})

apiRequestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Name:    "orchestrator_http_request_duration_seconds",
    Help:    "Latency for orchestrator HTTP API requests.",
    Buckets: prometheus.DefBuckets,
}, []string{"method", "route", "status"})
```

**Proposed Shared Package:**
```
shared/observability/
├── metrics.go        // BaseCollector, standard metric types
├── buckets.go        // Predefined histogram buckets
├── middleware.go     // HTTP/gRPC metrics middleware
└── tracing.go        // Tracing utilities
```

**Shared Types & Functions:**
```go
// BaseCollector pattern
type MetricsCollector interface {
    Handler() http.Handler
}

// Predefined bucket constants
var (
    HTTPLatencyBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
    RPCLatencyBuckets  = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}
    HeartbeatBuckets   = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120}
)

// HTTP metrics middleware
func HTTPMetricsMiddleware(name string, c MetricsCollector) func(http.Handler) http.Handler { ... }

// gRPC metrics interceptor
func GRPCMetricsInterceptor(c MetricsCollector) grpc.UnaryServerInterceptor { ... }
```

**Estimated Savings:**
- ~150-200 lines of duplicated metrics setup code
- Consistent metric naming and bucketing across services
- Reduced maintenance burden for adding metrics to new services

**Why This Matters:**
Currently each service:
1. Creates its own `Collector` type
2. Registers metrics independently
3. Uses different bucket sizes for histograms
4. Implements `Handler()` method identically

---

#### 4. **github.com/cartridge/shared/http** (New Package)

**Current Duplication:**

**Middleware Implementation - Web & Orchestrator Services Re-implement Similar Middleware:**

Files:
- `/home/user/Cartridge/services/orchestrator-go/internal/middleware/middleware.go` (167 lines)
- `/home/user/Cartridge/services/web-go/internal/http/middleware.go` (111 lines)
- `/home/user/Cartridge/services/web-go/internal/http/instrumentation.go` (44 lines)

**Overlapping Middleware:**

1. **Correlation ID Middleware** (Both Implement Identically)
```go
// orchestrator-go/middleware (141-154)
func CorrelationID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        correlationID := r.Header.Get("X-Correlation-ID")
        if correlationID == "" {
            correlationID = uuid.New().String()
            r.Header.Set("X-Correlation-ID", correlationID)
        }
        w.Header().Set("X-Correlation-ID", correlationID)
        next.ServeHTTP(w, r)
    })
}

// web-go/middleware (15-29) - requestIDMiddleware with similar logic
```

2. **Request Logging Middleware**
- orchestrator-go implements full `RequestLogger` with zerolog (167 lines)
- web-go implements simpler recovery middleware (44 lines)
- Both capture request/response metadata

3. **Recovery Middleware**
- Both implement panic recovery (web-go has cleaner implementation)
- Both log panic information

4. **Response Writer Wrapping**
- Both implement custom `responseWriter` to capture status codes
- Used by request logging and instrumentation

**Proposed Shared Package:**
```
shared/http/
├── middleware.go      // Common middleware (correlation ID, recovery, logging, timeouts)
├── response.go        // ResponseWriter wrapper utilities
├── instrumentation.go // HTTP request instrumentation
├── errors.go          // HTTP error response helpers
└── handlers.go        // Common handler utilities
```

**Shared Middleware:**
```go
// Shared middleware functions
func CorrelationIDMiddleware() func(http.Handler) http.Handler { ... }
func RequestLoggerMiddleware(logger *zerolog.Logger) func(http.Handler) http.Handler { ... }
func RecoveryMiddleware(logger *zerolog.Logger) func(http.Handler) http.Handler { ... }
func RequestTimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler { ... }
func RealIPMiddleware() func(http.Handler) http.Handler { ... }

// Response utilities
func WriteJSON(w http.ResponseWriter, status int, data interface{}) error { ... }
func WriteError(w http.ResponseWriter, status int, message string) error { ... }

// Error mapping
func MapError(err error) (int, string) { ... }  // Maps domain errors to HTTP status codes
```

**Common Response Writer:**
```go
type ResponseWriter struct {
    http.ResponseWriter
    StatusCode int
    BytesWritten int
    StartTime time.Time
}
```

**Estimated Savings:**
- ~100+ lines of duplicated middleware code
- Consistent error handling across services
- Standardized request/response patterns
- Easier to implement metrics middleware uniformly

**Why This Matters:**
- Both HTTP services need correlation tracking
- Both need request logging (orchestra-go for API, web-go for service logs)
- Both need recovery from panics
- Both need to capture request metrics (status, latency, size)
- Shared implementation ensures consistency

---

#### 5. **github.com/cartridge/shared/server** (New Package)

**Current Duplication - Server Startup & Shutdown Patterns:**

Files:
- `/home/user/Cartridge/services/orchestrator-go/cmd/server/main.go` (100 lines)
- `/home/user/Cartridge/services/replay-go/cmd/server/main.go` (150+ lines)
- `/home/user/Cartridge/services/web-go/cmd/server/main.go` (77 lines)
- `/home/user/Cartridge/services/weights-go/cmd/server/main.go` (113 lines)

**Common Pattern Across All Services:**
```go
// 1. Create listeners (HTTP or gRPC)
// 2. Setup graceful shutdown handlers
// 3. Setup signal handling (SIGTERM, SIGINT)
// 4. Start server in goroutine
// 5. Wait for shutdown signal
// 6. Graceful shutdown with timeout
// 7. Force shutdown if timeout exceeded
// 8. Log completion
```

**HTTP Server Pattern (Orchestrator & Web):**
```go
// orchestrator-go/main.go (70-100)
srv := &http.Server{
    Addr:              addr,
    Handler:           h.Routes(),
    ReadHeaderTimeout: 10 * time.Second,
    ReadTimeout:       readTimeout,
    WriteTimeout:      writeTimeout,
}

done := make(chan struct{})
go func() {
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        logger.Fatal().Err(err).Msg("http server failed")
    }
    close(done)
}()

sig := make(chan os.Signal, 1)
signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
<-sig

ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
defer cancel()
if err := srv.Shutdown(ctx); err != nil {
    logger.Error().Err(err).Msg("graceful shutdown failed")
}
```

**gRPC Server Pattern (Replay & Weights):**
```go
// replay-go/main.go (90-150)
grpcServer := grpc.NewServer()
lis, err := net.Listen("tcp", addr)

done := make(chan struct{})
go func() {
    if err := grpcServer.Serve(lis); err != nil {
        logger.Fatalf("Failed to serve: %v", err)
    }
}()

c := make(chan os.Signal, 1)
signal.Notify(c, os.Interrupt, syscall.SIGTERM)
<-c

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

grpcServer.GracefulStop()
// with timeout fallback to Stop()
```

**Proposed Shared Package:**
```
shared/server/
├── http.go          // HTTP server setup & shutdown
├── grpc.go          // gRPC server setup & shutdown
├── signals.go       // Signal handling
└── health.go        // Health check setup
```

**Shared HTTP Server Functions:**
```go
type HTTPServerConfig struct {
    Host            string
    Port            int
    Handler         http.Handler
    ReadTimeout     time.Duration
    WriteTimeout    time.Duration
    ShutdownTimeout time.Duration
}

func RunHTTPServer(ctx context.Context, cfg HTTPServerConfig, logger *zerolog.Logger) error {
    // Handles server creation, startup, signal listening, and graceful shutdown
}
```

**Shared gRPC Server Functions:**
```go
type GRPCServerConfig struct {
    Host            string
    Port            int
    ShutdownTimeout time.Duration
    // Additional gRPC options
}

func RunGRPCServer(ctx context.Context, cfg GRPCServerConfig, registerFn func(*grpc.Server), logger *zerolog.Logger) error {
    // Handles server creation, startup, signal listening, and graceful shutdown
}
```

**Estimated Savings:**
- ~40-50 lines per service (4 services × 45 = 180 lines)
- Consistent shutdown behavior across all services
- Centralized error handling for server lifecycle
- Easier to add common server features (like health checks)

**Benefits:**
- Reduces main() complexity significantly
- Ensures consistent signal handling across services
- Makes adding features like health checks uniform
- Reduces likelihood of graceful shutdown bugs

---

## Summary by Language

### Rust Opportunities

| Library | Type | Lines Saved | Priority |
|---------|------|-------------|----------|
| cartridge-observability | Crate | ~150 | High |
| cartridge-config | Crate | ~80 | Medium |
| cartridge-grpc-client | Module | ~60 | Low |

**Total Rust Savings:** ~290 lines + better code reuse

---

### Go Opportunities

| Package | Lines Saved | Priority | Complexity |
|---------|-------------|----------|-----------|
| shared/config | ~240 | High | Low |
| shared/logging | ~100 | Medium | Low |
| shared/observability | ~150 | High | Medium |
| shared/http | ~100 | High | Medium |
| shared/server | ~180 | High | Medium |

**Total Go Savings:** ~770 lines + consistency improvements

---

## Implementation Roadmap

### Phase 1 (Immediate - High Impact, Low Complexity)
1. **shared/config** - Go config helpers
   - 20+ lines per service ✓ immediate
   - No breaking changes, purely additive

2. **shared/logging** - Go logging utilities
   - Simple wrapper functions
   - Can be adopted gradually

### Phase 2 (Short Term - Medium Complexity)
1. **shared/observability** - Metrics base types & helpers
   - Standardize histogram buckets
   - Common metric registration patterns
   - Reduce per-service metrics code ~150 lines

2. **cartridge-observability** - Rust observability crate
   - Extract tracing/metrics initialization
   - Add to workspace.dependencies

### Phase 3 (Medium Term - Higher Complexity)
1. **shared/http** - HTTP middleware library
   - Coordinate between web-go and orchestrator-go
   - Ensure compatibility
   - ~100 lines saved per service

2. **shared/server** - Server startup/shutdown utilities
   - HTTP and gRPC variants
   - Significant cleanup of main() functions
   - ~180 lines saved across services

### Phase 4 (Optional Enhancements)
1. **cartridge-config** - Rust config utilities
2. **cartridge-grpc-client** - Rust gRPC client helpers

---

## Specific Code Examples

### Example 1: Shared Config Package (Go)

Current orchestrator-go/internal/config/config.go implementation:
```go
func getEnvString(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if intValue, err := strconv.Atoi(value); err == nil {
            return intValue
        }
    }
    return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
    if value := os.Getenv(key); value != "" {
        if duration, err := time.ParseDuration(value); err == nil {
            return duration
        }
    }
    return defaultValue
}
```

**Same pattern repeated in:**
- web-go/internal/config/config.go
- weights-go/internal/config/config.go

**Proposed Shared Implementation:**
```go
// shared/config/env.go
package config

import (
    "os"
    "strconv"
    "time"
)

func GetString(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func GetInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if intValue, err := strconv.Atoi(value); err == nil {
            return intValue
        }
    }
    return defaultValue
}

func GetDuration(key string, defaultValue time.Duration) time.Duration {
    if value := os.Getenv(key); value != "" {
        if duration, err := time.ParseDuration(value); err == nil {
            return duration
        }
    }
    return defaultValue
}

func GetBool(key string, defaultValue bool) bool {
    if value := os.Getenv(key); value != "" {
        if parsed, err := strconv.ParseBool(value); err == nil {
            return parsed
        }
    }
    return defaultValue
}
```

**Usage in services:**
```go
import "github.com/cartridge/shared/config"

cfg := Config{
    Server: ServerConfig{
        Port:            config.GetInt("PORT", 8080),
        ReadTimeout:     config.GetDuration("READ_TIMEOUT", 30*time.Second),
    },
}
```

---

### Example 2: Shared HTTP Middleware (Go)

**Current Duplication:**
Both orchestrator-go and web-go implement correlation ID middleware:

orchestrator-go (141-154):
```go
func CorrelationID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        correlationID := r.Header.Get("X-Correlation-ID")
        if correlationID == "" {
            correlationID = uuid.New().String()
            r.Header.Set("X-Correlation-ID", correlationID)
        }
        w.Header().Set("X-Correlation-ID", correlationID)
        next.ServeHTTP(w, r)
    })
}
```

web-go (21-29):
```go
func requestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := atomic.AddUint64(&idCounter, 1)
        requestID := formatRequestID(id)
        ctx := context.WithValue(r.Context(), requestIDKey, requestID)
        w.Header().Set("X-Request-ID", requestID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

**Proposed Shared Implementation:**
```go
// shared/http/middleware.go
package http

import (
    "net/http"
    "github.com/google/uuid"
)

func CorrelationIDMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            correlationID := r.Header.Get("X-Correlation-ID")
            if correlationID == "" {
                correlationID = uuid.New().String()
                r.Header.Set("X-Correlation-ID", correlationID)
            }
            w.Header().Set("X-Correlation-ID", correlationID)
            next.ServeHTTP(w, r)
        })
    }
}
```

**Usage:**
```go
router.Use(http.CorrelationIDMiddleware())
```

---

### Example 3: Shared Observability Metrics (Go)

**Current Pattern** (orchestrator-go/internal/metrics):
```go
func NewCollector(reg prometheus.Registerer) *Collector {
    if reg == nil {
        reg = prometheus.DefaultRegisterer
    }
    
    gatherer := prometheus.DefaultGatherer
    if g, ok := reg.(prometheus.Gatherer); ok {
        gatherer = g
    }
    
    heartbeatLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "orchestrator_heartbeat_latency_seconds",
        Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120},
    })
    
    // Register and return
}

func (c *Collector) Handler() http.Handler {
    gatherer := prometheus.DefaultGatherer
    if c != nil && c.gatherer != nil {
        gatherer = c.gatherer
    }
    return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}
```

**Same pattern in:**
- replay-go/internal/metrics
- weights-go/internal/observability/metrics
- web-go/internal/http/metrics

**Proposed Shared Utilities:**
```go
// shared/observability/metrics.go
package observability

import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    // Standard histogram buckets for different use cases
    HTTPLatencyBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
    RPC LatencyBuckets  = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}
)

type MetricsCollector interface {
    Handler() http.Handler
}

// Helper to create metrics handler
func NewMetricsHandler(reg prometheus.Registerer) http.Handler {
    if reg == nil {
        reg = prometheus.DefaultRegisterer
    }
    gatherer := prometheus.DefaultGatherer
    if g, ok := reg.(prometheus.Gatherer); ok {
        gatherer = g
    }
    return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}
```

---

### Example 4: Rust Observability Crate

**Current code in actor-rust/src/main.rs (101-151):**
```rust
fn initialize_tracing(log_level: &str) -> Result<LevelFilter> {
    let fallback_level = log_level.parse::<LevelFilter>()?;
    let selected_level = match env::var("RUST_LOG") {
        Ok(value) => match value.parse::<LevelFilter>() {
            Ok(level) => level,
            Err(e) => {
                eprintln!("failed to parse RUST_LOG: {}. Falling back.", e);
                fallback_level
            }
        },
        Err(_) => fallback_level,
    };
    tracing_subscriber::fmt()
        .with_max_level(selected_level)
        .try_init()?;
    Ok(selected_level)
}

fn initialize_metrics(addr: Option<&str>) -> Result<()> {
    match addr {
        Some(addr) => {
            let socket_addr: SocketAddr = addr.parse()?;
            PrometheusBuilder::new()
                .with_http_listener(socket_addr)
                .install()?;
            info!("Prometheus metrics exporter initialized");
        }
        None => {
            debug!("Prometheus metrics exporter disabled");
        }
    }
    Ok(())
}
```

**Proposed Shared Crate:**
```rust
// crates/cartridge-observability/src/lib.rs
use anyhow::Result;
use tracing::level_filters::LevelFilter;
use std::net::SocketAddr;

pub fn init_tracing(log_level: &str) -> Result<LevelFilter> {
    let fallback_level = log_level.parse::<LevelFilter>()?;
    let selected_level = match std::env::var("RUST_LOG") {
        Ok(value) => match value.parse::<LevelFilter>() {
            Ok(level) => level,
            Err(e) => {
                eprintln!("failed to parse RUST_LOG: {}. Falling back.", e);
                fallback_level
            }
        },
        Err(_) => fallback_level,
    };
    tracing_subscriber::fmt()
        .with_max_level(selected_level)
        .try_init()?;
    Ok(selected_level)
}

pub fn init_prometheus_metrics(addr: Option<&str>) -> Result<()> {
    use metrics_exporter_prometheus::PrometheusBuilder;
    use std::net::SocketAddr;
    
    match addr {
        Some(addr) => {
            let socket_addr: SocketAddr = addr.parse()?;
            PrometheusBuilder::new()
                .with_http_listener(socket_addr)
                .install()?;
        }
        None => {}
    }
    Ok(())
}
```

**Usage in services:**
```rust
use cartridge_observability::{init_tracing, init_prometheus_metrics};

#[tokio::main]
async fn main() -> Result<()> {
    let config = Config::parse();
    
    let _level = init_tracing(&config.log_level)?;
    init_prometheus_metrics(config.metrics_addr.as_deref())?;
    
    // ... rest of main
}
```

---

## Testing Implications

### New Test Files Needed
1. **shared/config** - 40-50 lines of tests
   - Test env var parsing with various input types
   - Test defaults when env vars not set
   - Test error handling

2. **shared/logging** - 30-40 lines
   - Test logger creation
   - Test level parsing

3. **shared/observability** - 60-80 lines
   - Test metrics handler creation
   - Test metric collection

4. **shared/http** - 80-100 lines
   - Test middleware ordering
   - Test recovery behavior
   - Test correlation ID propagation

5. **shared/server** - 100-120 lines
   - Test server startup/shutdown
   - Test signal handling
   - Test timeout behavior

---

## Risk Assessment & Mitigation

### Risks
1. **Breaking existing code** - Higher risk for refactoring existing modules
   - Mitigation: Create new packages, migrate gradually
   
2. **Version mismatches** - Shared deps could cause conflicts
   - Mitigation: Pin versions in workspace/go.mod
   
3. **Over-engineering** - Creating packages that don't see reuse
   - Mitigation: Start with Phase 1 (high confidence items)

### Dependencies to Manage
- Rust: Shared workspace dependencies already in place for engine-rust
- Go: Need to create modules path structure (github.com/cartridge/shared/*)

---

## Implementation Priority Matrix

```
HIGH VALUE, LOW EFFORT:
  ✓ shared/config (Go)
  ✓ cartridge-observability (Rust)

HIGH VALUE, MEDIUM EFFORT:
  → shared/observability (Go)
  → shared/http (Go)
  → shared/server (Go)

MEDIUM VALUE, LOW EFFORT:
  → shared/logging (Go)

MEDIUM VALUE, MEDIUM EFFORT:
  → cartridge-config (Rust)

LOW VALUE, MEDIUM EFFORT:
  → cartridge-grpc-client (Rust)
```

---

## Conclusion

The Cartridge codebase has significant opportunities for consolidating common patterns, particularly in:

1. **Configuration management** (~240 Go lines saved)
2. **Observability/Metrics** (~250+ Go lines saved, ~150 Rust lines)
3. **HTTP middleware & server setup** (~280 Go lines saved)
4. **Logging initialization** (~100 Go lines saved)

**Total Estimated Savings: ~1,060+ lines of code** across the codebase, with additional benefits in:
- Consistency across services
- Reduced maintenance burden
- Easier addition of new services
- Standardized error handling and logging
- Common patterns for observability

**Recommended Start:** Begin with Phase 1 (shared/config and shared/logging) given their low implementation complexity and immediate ROI.

