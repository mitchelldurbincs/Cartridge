use anyhow::Result;
use clap::Parser;
use metrics_exporter_prometheus::PrometheusBuilder;
use std::net::SocketAddr;
use std::sync::Arc;
use tokio::signal;
use tracing::{debug, error, info};

mod actor;
mod config;
mod policy;
mod proto {
    pub mod engine {
        pub mod v1 {
            tonic::include_proto!("engine.v1");
        }
    }
    pub mod replay {
        pub mod v1 {
            tonic::include_proto!("replay.v1");
        }
    }
}

use crate::actor::Actor;
use crate::config::Config;

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize tracing
    tracing_subscriber::fmt::init();

    // Parse configuration
    let config = Config::parse();

    // Validate configuration
    config.validate()?;

    initialize_metrics(config.metrics_addr.as_deref())?;

    info!(
        "Starting actor {} for environment {}",
        config.actor_id, config.env_id
    );
    info!(
        "Engine: {}, Replay: {}",
        config.engine_addr, config.replay_addr
    );

    // Create actor instance
    let actor = Actor::new(config).await?;
    let actor = Arc::new(actor);

    // Setup graceful shutdown
    let shutdown_actor = Arc::clone(&actor);
    let shutdown_handle = tokio::spawn(async move {
        signal::ctrl_c().await.expect("Failed to listen for ctrl+c");
        info!("Shutdown signal received, stopping actor...");
        shutdown_actor.shutdown().await;
    });

    // Run the actor
    let run_result = actor.run().await;

    // Wait for shutdown to complete
    shutdown_handle.abort();

    match run_result {
        Ok(_) => {
            info!("Actor completed successfully");
            Ok(())
        }
        Err(e) => {
            error!("Actor failed: {}", e);
            Err(e)
        }
    }
}

fn initialize_metrics(addr: Option<&str>) -> Result<()> {
    match addr {
        Some(addr) => {
            let socket_addr: SocketAddr = addr
                .parse()
                .map_err(|e| anyhow::anyhow!("invalid ACTOR_METRICS_ADDR '{}': {}", addr, e))?;
            PrometheusBuilder::new()
                .with_http_listener(socket_addr)
                .install_recorder()
                .map(|_| {
                    info!(%socket_addr, "Prometheus metrics exporter initialized");
                })
                .map_err(|e| anyhow::anyhow!("failed to install Prometheus recorder: {}", e))?
        }
        None => {
            debug!("Prometheus metrics exporter disabled");
        }
    }

    Ok(())
}
