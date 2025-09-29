use anyhow::Result;
use clap::Parser;
use std::sync::Arc;
use tokio::signal;
use tracing::{info, error};

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

    info!("Starting actor {} for environment {}", config.actor_id, config.env_id);
    info!("Engine: {}, Replay: {}", config.engine_addr, config.replay_addr);

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