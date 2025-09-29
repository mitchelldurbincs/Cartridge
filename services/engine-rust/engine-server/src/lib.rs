//! Engine server implementation
//! 
//! This crate provides the gRPC server implementation for the Cartridge engine service.

pub mod service;
pub mod buffers;
pub mod registry_init;

// Re-export main types
pub use service::EngineService;
pub use buffers::BufferPool;