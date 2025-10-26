//! Engine server binary
//!
//! Main entry point for the Cartridge engine server.

use std::env;

use cartridge_observability::{init_prometheus_metrics_from_env, init_tracing_default};
use engine_proto::engine_server::EngineServer;
use engine_server::{registry_init, EngineService};
use tonic::transport::Server;
use tracing::info;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    let log_level = init_tracing_default()?;
    info!(log_level = %log_level, "Tracing initialized");

    // Initialize Prometheus metrics
    init_prometheus_metrics_from_env("ENGINE_METRICS_ADDR")?;

    // Initialize the game registry
    registry_init::initialize_registry();

    // Get server address from environment or use default
    let addr = env::var("ENGINE_SERVER_ADDR")
        .unwrap_or_else(|_| "0.0.0.0:50051".to_string())
        .parse()?;

    // Create the service
    let engine_service = EngineService::new();

    info!(%addr, "Engine server starting");

    // Start the server
    Server::builder()
        .add_service(EngineServer::new(engine_service))
        .serve(addr)
        .await?;

    Ok(())
}
