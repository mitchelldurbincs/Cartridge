// Library interface for actor-rust
// This allows benchmarks and tests to access internal modules

pub mod actor;
pub mod config;
pub mod policy;

// Protobuf generated code
pub mod proto {
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

// Re-export commonly used types
pub use actor::Actor;
pub use config::Config;
pub use policy::RandomPolicy;
