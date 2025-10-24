//! gRPC service implementation for the Engine server
//!
//! This module provides the Tonic-based gRPC server implementation that handles
//! all engine service methods with proper error handling and buffer management.

use std::sync::Arc;
use std::time::Instant;

use dashmap::{mapref::entry::Entry, DashMap};
use engine_core::registry::{create_game, is_registered};
use engine_core::ErasedGame;
use engine_proto::{
    engine_server::Engine, BoxSpec as ProtoBoxSpec, Capabilities, Encoding as ProtoEncoding,
    EngineId, MultiDiscrete as ProtoMultiDiscrete, ResetRequest, ResetResponse, StepRequest,
    StepResponse,
};
use tonic::{Request, Response, Result as TonicResult, Status};

use metrics::{counter, gauge, histogram};

use crate::buffers::BufferPool;

/// Engine gRPC service implementation
#[derive(Debug)]
pub struct EngineService {
    buffer_pool: BufferPool,
    game_cache: Arc<DashMap<(Arc<str>, Arc<str>), Box<dyn ErasedGame>>>,
}

impl EngineService {
    /// Create a new engine service
    pub fn new() -> Self {
        Self {
            buffer_pool: BufferPool::with_capacity(100, 100, 50, 512),
            game_cache: Arc::new(DashMap::new()),
        }
    }

    /// Create a new engine service with custom buffer pool
    pub fn with_buffer_pool(buffer_pool: BufferPool) -> Self {
        Self {
            buffer_pool,
            game_cache: Arc::new(DashMap::new()),
        }
    }

    fn observe_rpc_latency(method: &'static str, start: Instant) {
        histogram!(
            "engine_rpc_latency_seconds",
            start.elapsed().as_secs_f64(),
            "method" => method
        );
    }

    /// Convert internal capabilities to protobuf format
    fn capabilities_to_proto(caps: &engine_core::typed::Capabilities) -> Capabilities {
        let encoding = ProtoEncoding {
            state: caps.encoding.state.clone(),
            action: caps.encoding.action.clone(),
            obs: caps.encoding.obs.clone(),
            schema_version: caps.encoding.schema_version,
        };

        let action_space = match &caps.action_space {
            engine_core::typed::ActionSpace::Discrete(n) => {
                Some(engine_proto::capabilities::ActionSpace::DiscreteN(*n))
            }
            engine_core::typed::ActionSpace::MultiDiscrete(nvec) => {
                Some(engine_proto::capabilities::ActionSpace::Multi(
                    ProtoMultiDiscrete { nvec: nvec.clone() },
                ))
            }
            engine_core::typed::ActionSpace::Continuous { low, high, shape } => Some(
                engine_proto::capabilities::ActionSpace::Continuous(ProtoBoxSpec {
                    low: low.clone(),
                    high: high.clone(),
                    shape: shape.clone(),
                }),
            ),
        };

        Capabilities {
            id: Some(EngineId {
                env_id: caps.id.env_id.clone(),
                build_id: caps.id.build_id.clone(),
            }),
            enc: Some(encoding),
            max_horizon: caps.max_horizon,
            action_space,
            preferred_batch: caps.preferred_batch,
        }
    }
}

impl Default for EngineService {
    fn default() -> Self {
        Self::new()
    }
}

#[tonic::async_trait]
impl Engine for EngineService {
    async fn get_capabilities(
        &self,
        request: Request<EngineId>,
    ) -> TonicResult<Response<Capabilities>> {
        counter!("engine_rpc_requests_total", 1, "method" => "get_capabilities");
        let start = Instant::now();
        let engine_id = request.into_inner();

        // Validate env_id
        if !is_registered(&engine_id.env_id) {
            counter!(
                "engine_rpc_failures_total",
                1,
                "method" => "get_capabilities",
                "error" => "unknown_env"
            );
            Self::observe_rpc_latency("get_capabilities", start);
            return Err(Status::not_found(format!(
                "Unknown env_id: {}",
                engine_id.env_id
            )));
        }

        // Create game instance to get capabilities
        let game = match create_game(&engine_id.env_id) {
            Some(game) => game,
            None => {
                counter!(
                    "engine_rpc_failures_total",
                    1,
                    "method" => "get_capabilities",
                    "error" => "create_failed"
                );
                Self::observe_rpc_latency("get_capabilities", start);
                return Err(Status::internal("Failed to create game instance"));
            }
        };

        let capabilities = game.capabilities();
        let proto_caps = Self::capabilities_to_proto(&capabilities);

        counter!(
            "engine_rpc_success_total",
            1,
            "method" => "get_capabilities"
        );

        Self::observe_rpc_latency("get_capabilities", start);

        Ok(Response::new(proto_caps))
    }

    async fn reset(&self, request: Request<ResetRequest>) -> TonicResult<Response<ResetResponse>> {
        counter!("engine_rpc_requests_total", 1, "method" => "reset");
        let start = Instant::now();
        let req = request.into_inner();

        let engine_id = match req.id {
            Some(id) => id,
            None => {
                counter!(
                    "engine_rpc_failures_total",
                    1,
                    "method" => "reset",
                    "error" => "missing_engine_id"
                );
                Self::observe_rpc_latency("reset", start);
                return Err(Status::invalid_argument("Missing engine_id"));
            }
        };

        // Convert to Arc<str> for efficient cache key
        let env_id: Arc<str> = Arc::from(engine_id.env_id.as_str());
        let build_id: Arc<str> = Arc::from(engine_id.build_id.as_str());
        let key = (env_id.clone(), build_id.clone());

        // Get buffers from pool
        let mut state_buf = self.buffer_pool.get_state_buffer();
        let mut obs_buf = self.buffer_pool.get_obs_buffer();

        // Use DashMap's entry API for lock-free concurrent access
        let mut game_entry = match self.game_cache.entry(key) {
            Entry::Occupied(entry) => {
                counter!("engine_game_cache_hits_total", 1, "method" => "reset");
                entry.into_mut()
            }
            Entry::Vacant(entry) => {
                counter!("engine_game_cache_misses_total", 1, "method" => "reset");
                let Some(game) = create_game(env_id.as_ref()) else {
                    counter!(
                        "engine_rpc_failures_total",
                        1,
                        "method" => "reset",
                        "error" => "unknown_env"
                    );
                    Self::observe_rpc_latency("reset", start);
                    return Err(Status::not_found(format!("Unknown env_id: {}", env_id)));
                };
                entry.insert(game)
            }
        };

        gauge!("engine_game_cache_entries", self.game_cache.len() as f64);

        // Perform reset
        if let Err(e) =
            game_entry
                .value_mut()
                .reset(req.seed, &req.hint, &mut state_buf, &mut obs_buf)
        {
            counter!(
                "engine_rpc_failures_total",
                1,
                "method" => "reset",
                "error" => "reset_failed"
            );
            Self::observe_rpc_latency("reset", start);
            return Err(Status::internal(format!("Reset failed: {}", e)));
        }

        // Explicitly drop the entry to release the lock
        drop(game_entry);

        // Move buffers into response instead of cloning
        let response = ResetResponse {
            state: std::mem::take(&mut state_buf),
            obs: std::mem::take(&mut obs_buf),
        };

        // Return now-empty buffers to pool (they'll be cleared automatically)
        self.buffer_pool.return_state_buffer(state_buf);
        self.buffer_pool.return_obs_buffer(obs_buf);

        counter!(
            "engine_rpc_success_total",
            1,
            "method" => "reset"
        );

        Self::observe_rpc_latency("reset", start);

        Ok(Response::new(response))
    }

    async fn step(&self, request: Request<StepRequest>) -> TonicResult<Response<StepResponse>> {
        counter!("engine_rpc_requests_total", 1, "method" => "step");
        let start = Instant::now();
        let req = request.into_inner();

        let engine_id = match req.id {
            Some(id) => id,
            None => {
                counter!(
                    "engine_rpc_failures_total",
                    1,
                    "method" => "step",
                    "error" => "missing_engine_id"
                );
                Self::observe_rpc_latency("step", start);
                return Err(Status::invalid_argument("Missing engine_id"));
            }
        };

        if !is_registered(&engine_id.env_id) {
            counter!(
                "engine_rpc_failures_total",
                1,
                "method" => "step",
                "error" => "unknown_env"
            );
            Self::observe_rpc_latency("step", start);
            return Err(Status::not_found(format!(
                "Unknown env_id: {}",
                engine_id.env_id
            )));
        }

        // Convert to Arc<str> for efficient cache key
        let env_id: Arc<str> = Arc::from(engine_id.env_id.as_str());
        let build_id: Arc<str> = Arc::from(engine_id.build_id.as_str());
        let key = (env_id, build_id);

        // Use DashMap's get_mut for fine-grained locking
        let mut game_entry = match self.game_cache.get_mut(&key) {
            Some(entry) => {
                counter!("engine_game_cache_hits_total", 1, "method" => "step");
                gauge!("engine_game_cache_entries", self.game_cache.len() as f64);
                entry
            }
            None => {
                counter!("engine_game_cache_misses_total", 1, "method" => "step");
                counter!(
                    "engine_rpc_failures_total",
                    1,
                    "method" => "step",
                    "error" => "not_initialized"
                );
                Self::observe_rpc_latency("step", start);
                return Err(Status::failed_precondition(
                    "Game not initialized - call reset before step",
                ));
            }
        };

        // Get buffers from pool
        let mut new_state_buf = self.buffer_pool.get_state_buffer();
        let mut obs_buf = self.buffer_pool.get_obs_buffer();

        // Perform step
        let (reward, done, info) =
            match game_entry.step(&req.state, &req.action, &mut new_state_buf, &mut obs_buf) {
                Ok(result) => result,
                Err(e) => {
                    counter!(
                        "engine_rpc_failures_total",
                        1,
                        "method" => "step",
                        "error" => "step_failed"
                    );
                    Self::observe_rpc_latency("step", start);
                    return Err(Status::internal(format!("Step failed: {}", e)));
                }
            };

        // Explicitly drop the entry to release the fine-grained lock
        drop(game_entry);

        // Move buffers into response instead of cloning
        let response = StepResponse {
            state: std::mem::take(&mut new_state_buf),
            obs: std::mem::take(&mut obs_buf),
            reward,
            done,
            info,
        };

        // Return now-empty buffers to pool
        self.buffer_pool.return_state_buffer(new_state_buf);
        self.buffer_pool.return_obs_buffer(obs_buf);

        counter!(
            "engine_rpc_success_total",
            1,
            "method" => "step"
        );

        Self::observe_rpc_latency("step", start);

        Ok(Response::new(response))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use engine_core::registry::{clear_registry, register_game};
    use engine_core::typed::{
        ActionSpace, Capabilities as TypedCapabilities, DecodeError, EncodeError, Encoding,
        EngineId as TypedEngineId, Game,
    };
    use engine_core::GameAdapter;
    use games_tictactoe::TicTacToe;
    use rand::RngCore;

    fn setup_test_registry() {
        clear_registry();
        register_game("tictactoe".to_string(), || {
            Box::new(GameAdapter::new(TicTacToe::new()))
        });
    }

    fn setup_rng_test_registry() {
        clear_registry();
        register_game("rng-test".to_string(), || {
            Box::new(GameAdapter::new(RngStepGame::default()))
        });
    }

    #[derive(Default, Debug)]
    struct RngStepGame {
        step_calls: u32,
    }

    #[derive(Clone, Copy)]
    struct RngState(u64);

    #[derive(Clone, Copy)]
    struct RngObs(f32);

    impl Game for RngStepGame {
        type State = RngState;
        type Action = ();
        type Obs = RngObs;

        fn engine_id(&self) -> TypedEngineId {
            TypedEngineId {
                env_id: "rng-test".to_string(),
                build_id: "test-build".to_string(),
            }
        }

        fn capabilities(&self) -> TypedCapabilities {
            TypedCapabilities {
                id: self.engine_id(),
                encoding: Encoding {
                    state: "rng-state".to_string(),
                    action: "rng-action".to_string(),
                    obs: "rng-obs".to_string(),
                    schema_version: 1,
                },
                max_horizon: 100,
                action_space: ActionSpace::Discrete(1),
                preferred_batch: 1,
            }
        }

        fn reset(
            &mut self,
            rng: &mut rand_chacha::ChaCha20Rng,
            _hint: &[u8],
        ) -> (Self::State, Self::Obs) {
            self.step_calls = 0;
            let state = RngState(rng.next_u64());
            let obs = RngObs(rng.next_u32() as f32);
            (state, obs)
        }

        fn step(
            &mut self,
            state: &mut Self::State,
            _action: Self::Action,
            rng: &mut rand_chacha::ChaCha20Rng,
        ) -> (Self::Obs, f32, bool, u64) {
            self.step_calls += 1;
            let random = rng.next_u32();
            state.0 = random as u64;
            let obs = RngObs(random as f32);
            let reward = random as f32 + self.step_calls as f32;
            let info = (state.0 << 32) | u64::from(self.step_calls);
            (obs, reward, false, info)
        }

        fn encode_state(state: &Self::State, out: &mut Vec<u8>) -> Result<(), EncodeError> {
            out.extend_from_slice(&state.0.to_le_bytes());
            Ok(())
        }

        fn decode_state(buf: &[u8]) -> Result<Self::State, DecodeError> {
            if buf.len() != 8 {
                return Err(DecodeError::InvalidLength {
                    expected: 8,
                    actual: buf.len(),
                });
            }
            let mut array = [0u8; 8];
            array.copy_from_slice(buf);
            Ok(RngState(u64::from_le_bytes(array)))
        }

        fn encode_action(_action: &Self::Action, _out: &mut Vec<u8>) -> Result<(), EncodeError> {
            Ok(())
        }

        fn decode_action(buf: &[u8]) -> Result<Self::Action, DecodeError> {
            if buf.is_empty() {
                Ok(())
            } else {
                Err(DecodeError::InvalidLength {
                    expected: 0,
                    actual: buf.len(),
                })
            }
        }

        fn encode_obs(obs: &Self::Obs, out: &mut Vec<u8>) -> Result<(), EncodeError> {
            out.extend_from_slice(&obs.0.to_le_bytes());
            Ok(())
        }
    }

    #[tokio::test]
    async fn test_get_capabilities() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(EngineId {
            env_id: "tictactoe".to_string(),
            build_id: "test".to_string(),
        });

        let response = service.get_capabilities(request).await.unwrap();
        let caps = response.into_inner();

        assert!(caps.id.is_some());
        assert_eq!(caps.id.unwrap().env_id, "tictactoe");
        assert_eq!(caps.max_horizon, 9);
    }

    #[tokio::test]
    async fn test_get_capabilities_unknown_game() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(EngineId {
            env_id: "unknown".to_string(),
            build_id: "test".to_string(),
        });

        let result = service.get_capabilities(request).await;
        assert!(result.is_err());

        let err = result.unwrap_err();
        assert_eq!(err.code(), tonic::Code::NotFound);
    }

    #[tokio::test]
    async fn test_reset() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(ResetRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            seed: 42,
            hint: Vec::new(),
        });

        let response = service.reset(request).await.unwrap();
        let reset_resp = response.into_inner();

        assert!(!reset_resp.state.is_empty());
        assert!(!reset_resp.obs.is_empty());

        // TicTacToe state should be 11 bytes
        assert_eq!(reset_resp.state.len(), 11);
        // TicTacToe obs should be 29 * 4 = 116 bytes (29 f32 values)
        assert_eq!(reset_resp.obs.len(), 116);
    }

    #[tokio::test]
    async fn test_step() {
        setup_test_registry();

        let service = EngineService::new();

        // First reset the game
        let reset_request = Request::new(ResetRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            seed: 42,
            hint: Vec::new(),
        });

        let reset_response = service.reset(reset_request).await.unwrap();
        let reset_resp = reset_response.into_inner();

        // Now take a step
        let step_request = Request::new(StepRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            state: reset_resp.state,
            action: vec![4], // Place in center
        });

        let step_response = service.step(step_request).await.unwrap();
        let step_resp = step_response.into_inner();

        assert!(!step_resp.state.is_empty());
        assert!(!step_resp.obs.is_empty());
        assert!(!step_resp.done); // Game should not be done after one move
        assert_eq!(step_resp.reward, 0.0); // No reward for ongoing game
        assert_eq!(step_resp.info & 0x1FF, 0x1FFu64 & !(1u64 << 4));
    }

    #[tokio::test]
    async fn test_step_invalid_engine() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(StepRequest {
            id: Some(EngineId {
                env_id: "unknown".to_string(),
                build_id: "test".to_string(),
            }),
            state: vec![0; 11],
            action: vec![0],
        });

        let result = service.step(request).await;
        assert!(result.is_err());

        let err = result.unwrap_err();
        assert_eq!(err.code(), tonic::Code::NotFound);
    }

    #[tokio::test]
    async fn test_buffer_pool_integration() {
        setup_test_registry();

        let buffer_pool = BufferPool::with_capacity(2, 2, 2, 64);
        let service = EngineService::with_buffer_pool(buffer_pool.clone());

        let initial_stats = buffer_pool.stats();
        assert_eq!(initial_stats.available_state_buffers, 2);

        // Perform reset - should use and return buffers
        let request = Request::new(ResetRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            seed: 42,
            hint: Vec::new(),
        });

        let _response = service.reset(request).await.unwrap();

        // Buffers should be returned to pool
        let final_stats = buffer_pool.stats();
        assert_eq!(final_stats.available_state_buffers, 2);
        assert_eq!(final_stats.available_obs_buffers, 2);
    }

    #[tokio::test]
    async fn test_step_rng_progression_is_deterministic() {
        setup_rng_test_registry();

        let service = EngineService::new();
        let engine_id = EngineId {
            env_id: "rng-test".to_string(),
            build_id: "test-build".to_string(),
        };

        let reset_request = Request::new(ResetRequest {
            id: Some(engine_id.clone()),
            seed: 7,
            hint: Vec::new(),
        });

        let reset_response = service.reset(reset_request).await.unwrap();
        let reset_data = reset_response.into_inner();

        let first_step_request = Request::new(StepRequest {
            id: Some(engine_id.clone()),
            state: reset_data.state.clone(),
            action: Vec::new(),
        });

        let first_step = service.step(first_step_request).await.unwrap().into_inner();

        let second_step_request = Request::new(StepRequest {
            id: Some(engine_id.clone()),
            state: first_step.state.clone(),
            action: Vec::new(),
        });

        let second_step = service
            .step(second_step_request)
            .await
            .unwrap()
            .into_inner();

        assert_ne!(first_step.reward, second_step.reward);
        assert_ne!(first_step.info, second_step.info);

        let service_again = EngineService::new();

        let reset_again = Request::new(ResetRequest {
            id: Some(engine_id.clone()),
            seed: 7,
            hint: Vec::new(),
        });

        let reset_again_data = service_again.reset(reset_again).await.unwrap().into_inner();

        let first_again = service_again
            .step(Request::new(StepRequest {
                id: Some(engine_id.clone()),
                state: reset_again_data.state.clone(),
                action: Vec::new(),
            }))
            .await
            .unwrap()
            .into_inner();

        let second_again = service_again
            .step(Request::new(StepRequest {
                id: Some(engine_id.clone()),
                state: first_again.state.clone(),
                action: Vec::new(),
            }))
            .await
            .unwrap()
            .into_inner();

        assert_eq!(first_step.reward, first_again.reward);
        assert_eq!(first_step.info, first_again.info);
        assert_eq!(second_step.reward, second_again.reward);
        assert_eq!(second_step.info, second_again.info);
    }
}
