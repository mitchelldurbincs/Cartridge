// Generated protobuf code wrapped in module structure
pub mod engine {
    pub mod v1 {
        tonic::include_proto!("engine.v1");
    }
}

// Re-export frequently used items at the crate root so downstream crates can follow
// the conventional `engine_proto::TypeName` style without having to know the nested
// module structure.
pub use self::engine::v1::capabilities;
pub use self::engine::v1::engine_client::{self, EngineClient};
pub use self::engine::v1::engine_server::{self, Engine, EngineServer};
pub use self::engine::v1::{
    BatchSimulateRequest, BoxSpec, Capabilities, Encoding, EngineId, MultiDiscrete, ResetRequest,
    ResetResponse, SimResultChunk, StepRequest, StepResponse,
};
