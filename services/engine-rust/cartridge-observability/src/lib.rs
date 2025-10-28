//! Cartridge Observability
//!
//! Shared observability utilities for Cartridge services, providing consistent
//! tracing and metrics initialization across all Rust services.

use anyhow::{anyhow, Result};
use metrics_exporter_prometheus::PrometheusBuilder;
use once_cell::sync::OnceCell;
use std::env;
use std::net::SocketAddr;
use tracing::level_filters::LevelFilter;
use tracing::{debug, info};
use tracing_subscriber::EnvFilter;

/// Initialize tracing with configurable log filter.
///
/// This function sets up the tracing subscriber with the following priority:
/// 1. `RUST_LOG` environment variable (if valid)
/// 2. The provided `fallback_log_level` parameter
///
/// # Arguments
/// * `fallback_log_level` - The log level or filter directive to use if RUST_LOG is not set or invalid
///
/// # Returns
/// * `Ok(LevelFilter)` - The most permissive level allowed by the active filter
/// * `Err` - If the fallback_log_level is invalid or tracing initialization fails
///
/// # Example
/// ```no_run
/// use cartridge_observability::init_tracing;
///
/// let level = init_tracing("info").expect("Failed to initialize tracing");
/// println!("Tracing initialized at level: {}", level);
/// ```
pub fn init_tracing(fallback_log_level: &str) -> Result<LevelFilter> {
    // Validate that the provided fallback is a known level before building filters so
    // we surface a consistent error message for invalid inputs.
    let validated_level = fallback_log_level.parse::<LevelFilter>().map_err(|_| {
        anyhow!(
            "invalid log level '{}', expected one of trace, debug, info, warn, error",
            fallback_log_level
        )
    })?;

    let fallback_filter = EnvFilter::try_new(fallback_log_level).map_err(|e| {
        anyhow!(
            "invalid log level or filter '{}': {}",
            fallback_log_level,
            e
        )
    })?;

    let mut filter = fallback_filter;
    let mut selected_level = validated_level;

    if let Ok(value) = env::var("RUST_LOG") {
        match EnvFilter::try_new(value.clone()) {
            Ok(env_filter) => {
                selected_level = env_filter.max_level_hint().unwrap_or(LevelFilter::TRACE);
                info!(
                    rust_log = %value,
                    "Using RUST_LOG environment variable for log filter"
                );
                filter = env_filter;
            }
            Err(e) => {
                eprintln!(
                    "Failed to parse RUST_LOG '{}': {}. Falling back to configured log filter.",
                    value, e
                );
            }
        }
    }

    tracing_subscriber::fmt()
        .with_env_filter(filter)
        .try_init()
        .map_err(|e| anyhow!("failed to initialize tracing subscriber: {}", e))?;

    Ok(selected_level)
}

/// Initialize tracing with default INFO level.
///
/// This is a convenience wrapper around `init_tracing` that uses "info" as the default level.
///
/// # Returns
/// * `Ok(LevelFilter)` - The selected log level that was applied
/// * `Err` - If tracing initialization fails
///
/// # Example
/// ```no_run
/// use cartridge_observability::init_tracing_default;
///
/// init_tracing_default().expect("Failed to initialize tracing");
/// ```
pub fn init_tracing_default() -> Result<LevelFilter> {
    init_tracing("info")
}

/// Initialize Prometheus metrics exporter.
///
/// If an address is provided, starts an HTTP server on that address to serve
/// Prometheus metrics. If no address is provided, metrics collection is disabled.
///
/// # Arguments
/// * `addr` - Optional socket address (e.g., "0.0.0.0:9090") to listen on
///
/// # Returns
/// * `Ok(())` - Metrics exporter initialized successfully or disabled
/// * `Err` - If the address is invalid or the exporter fails to start
///
/// # Example
/// ```no_run
/// use cartridge_observability::init_prometheus_metrics;
///
/// // Enable metrics on port 9090
/// init_prometheus_metrics(Some("0.0.0.0:9090")).expect("Failed to start metrics");
///
/// // Disable metrics
/// init_prometheus_metrics(None).expect("Failed to initialize");
/// ```
static PROMETHEUS_INITIALIZED: OnceCell<()> = OnceCell::new();

pub fn init_prometheus_metrics(addr: Option<&str>) -> Result<()> {
    match addr {
        Some(addr) => {
            let socket_addr: SocketAddr = addr
                .parse()
                .map_err(|e| anyhow!("invalid metrics address '{}': {}", addr, e))?;

            let already_initialized = PROMETHEUS_INITIALIZED.get().is_some();

            if already_initialized {
                info!(%socket_addr, "Prometheus metrics exporter already running");
            } else {
                info!(%socket_addr, "Starting Prometheus metrics exporter");
            }

            if !already_initialized {
                PrometheusBuilder::new()
                    .with_http_listener(socket_addr)
                    .install()
                    .map_err(|e| anyhow!("failed to install Prometheus exporter: {}", e))?;
                let _ = PROMETHEUS_INITIALIZED.set(());
            }

            if !already_initialized {
                info!(%socket_addr, "Prometheus metrics exporter initialized");
            }
        }
        None => {
            debug!("Prometheus metrics exporter disabled");
        }
    }

    Ok(())
}

/// Initialize Prometheus metrics from environment variable.
///
/// Reads the metrics address from the specified environment variable.
/// If the variable is not set, metrics are disabled.
///
/// # Arguments
/// * `env_var` - Name of the environment variable containing the metrics address
///
/// # Returns
/// * `Ok(())` - Metrics exporter initialized successfully or disabled
/// * `Err` - If the address is invalid or the exporter fails to start
///
/// # Example
/// ```no_run
/// use cartridge_observability::init_prometheus_metrics_from_env;
///
/// // Read from ENGINE_METRICS_ADDR environment variable
/// init_prometheus_metrics_from_env("ENGINE_METRICS_ADDR")
///     .expect("Failed to initialize metrics");
/// ```
pub fn init_prometheus_metrics_from_env(env_var: &str) -> Result<()> {
    let addr = env::var(env_var).ok();
    init_prometheus_metrics(addr.as_deref())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{Read, Write};
    use std::net::{TcpListener, TcpStream};
    use std::thread;
    use std::time::Duration;

    #[test]
    fn test_init_prometheus_metrics_none() {
        // Should not panic when metrics are disabled
        assert!(init_prometheus_metrics(None).is_ok());
    }

    #[test]
    fn test_init_prometheus_metrics_valid_addr() {
        // Bind to an ephemeral port to reserve it, then drop the listener
        let listener = TcpListener::bind("127.0.0.1:0").expect("failed to bind test listener");
        let addr = listener.local_addr().unwrap();
        drop(listener);

        // Initialize exporter and give it a moment to start listening
        init_prometheus_metrics(Some(&addr.to_string())).expect("failed to init metrics");
        thread::sleep(Duration::from_millis(50));

        // Connect to the metrics endpoint and perform a simple HTTP GET
        let mut stream = TcpStream::connect(addr).expect("failed to connect to metrics exporter");
        stream
            .write_all(b"GET /metrics HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")
            .expect("failed to write request");

        let mut response = String::new();
        stream
            .read_to_string(&mut response)
            .expect("failed to read response");

        assert!(
            response.starts_with("HTTP/1.1 200"),
            "unexpected response: {}",
            response
        );
    }

    #[test]
    fn test_init_prometheus_metrics_invalid_addr() {
        // Should return error for invalid address
        let result = init_prometheus_metrics(Some("invalid"));
        assert!(result.is_err());
    }

    #[test]
    fn test_invalid_log_level() {
        // Should return error for invalid log level
        let result = init_tracing("invalid_level");
        assert!(result.is_err());
    }
}
