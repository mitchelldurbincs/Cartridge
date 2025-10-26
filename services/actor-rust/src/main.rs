use anyhow::Result;
use cartridge_observability::{init_prometheus_metrics, init_tracing};
use clap::Parser;
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
    // Print early diagnostic message to stderr before any initialization
    eprintln!("Actor service starting...");

    // Parse configuration
    let config = Config::parse();
    eprintln!("Configuration parsed successfully");

    // Validate configuration
    config.validate()?;
    eprintln!("Configuration validated successfully");

    // Initialize tracing with the configured level or RUST_LOG override
    let selected_level = init_tracing(&config.log_level)?;
    info!(log_level = %selected_level, "Tracing initialized");
    debug!(config = ?config, "Actor configuration loaded");

    // Log the max_episodes setting to help debug
    let max_episode_description = if config.max_episodes < 0 {
        "unlimited".to_string()
    } else {
        config.max_episodes.to_string()
    };
    info!(
        max_episodes = config.max_episodes,
        "Actor will run {} episodes",
        max_episode_description
    );

    // Initialize Prometheus metrics
    init_prometheus_metrics(config.metrics_addr.as_deref())?;

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
