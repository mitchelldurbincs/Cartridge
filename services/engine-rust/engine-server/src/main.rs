//! Engine server binary
//! 
//! Main entry point for the Cartridge engine server.

use std::env;
use tonic::transport::Server;
use engine_proto::engine_server::EngineServer;
use engine_server::{EngineService, registry_init};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt::init();
    
    // Initialize the game registry
    registry_init::initialize_registry();
    
    // Get server address from environment or use default
    let addr = env::var("ENGINE_SERVER_ADDR")
        .unwrap_or_else(|_| "0.0.0.0:50051".to_string())
        .parse()?;
    
    // Create the service
    let engine_service = EngineService::new();
    
    println!("Engine server starting on {}", addr);
    
    // Start the server
    Server::builder()
        .add_service(EngineServer::new(engine_service))
        .serve(addr)
        .await?;
    
    Ok(())
}