//! Engine server binary
//!
//! Main entry point for the Cartridge engine server.

use std::env;
use std::net::SocketAddr;

use engine_proto::engine_server::EngineServer;
use engine_server::{registry_init, EngineService};
use metrics_exporter_prometheus::PrometheusBuilder;
use tonic::transport::Server;
use tracing::{debug, error, info};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt::init();

    initialize_metrics();

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

fn initialize_metrics() {
    match env::var("ENGINE_METRICS_ADDR") {
        Ok(addr) => match addr.parse::<SocketAddr>() {
            Ok(socket_addr) => {
                match PrometheusBuilder::new()
                    .with_http_listener(socket_addr)
                    .install_recorder()
                {
                    Ok(_) => info!(%socket_addr, "Prometheus metrics exporter initialized"),
                    Err(err) => error!(%err, "Failed to install Prometheus metrics exporter"),
                }
            }
            Err(err) => {
                error!(%err, address = %addr, "Invalid ENGINE_METRICS_ADDR value");
            }
        },
        Err(_) => {
            debug!("Prometheus metrics exporter disabled");
        }
    }
}
