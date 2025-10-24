// Generated protobuf code
tonic::include_proto!("engine.v1");

// The generated code nests everything under the `engine::v1` module. Re-export the
// frequently used items at the crate root so downstream crates can follow the
// conventional `engine_proto::TypeName` style without having to know the nested
// module structure produced by `tonic::include_proto!`.
pub use engine::v1::capabilities;
pub use engine::v1::engine_client::{self, EngineClient};
pub use engine::v1::engine_server::{self, Engine, EngineServer};
pub use engine::v1::{
    BatchSimulateRequest, BoxSpec, Capabilities, Encoding, EngineId, MultiDiscrete, ResetRequest,
    ResetResponse, SimResultChunk, StepRequest, StepResponse,
};
