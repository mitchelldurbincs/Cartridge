//! gRPC service implementation for the Engine server
//!
//! This module provides the Tonic-based gRPC server implementation that handles
//! all engine service methods with proper error handling and buffer management.

use std::collections::{hash_map::Entry, HashMap};
use std::sync::Arc;

use engine_core::registry::{create_game, is_registered};
use engine_core::ErasedGame;
use engine_proto::{
    engine_server::Engine, BoxSpec as ProtoBoxSpec, Capabilities, Encoding as ProtoEncoding,
    EngineId, MultiDiscrete as ProtoMultiDiscrete, ResetRequest, ResetResponse, StepRequest,
    StepResponse,
};
use tokio::sync::Mutex;
use tonic::{Request, Response, Result as TonicResult, Status};

use crate::buffers::BufferPool;

/// Engine gRPC service implementation
#[derive(Debug)]
pub struct EngineService {
    buffer_pool: BufferPool,
    game_cache: Arc<Mutex<HashMap<(String, String), Box<dyn ErasedGame>>>>,
}

impl EngineService {
    /// Create a new engine service
    pub fn new() -> Self {
        Self {
            buffer_pool: BufferPool::with_capacity(100, 100, 50, 512),
            game_cache: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    /// Create a new engine service with custom buffer pool
    pub fn with_buffer_pool(buffer_pool: BufferPool) -> Self {
        Self {
            buffer_pool,
            game_cache: Arc::new(Mutex::new(HashMap::new())),
        }
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
        let engine_id = request.into_inner();

        // Validate env_id
        if !is_registered(&engine_id.env_id) {
            return Err(Status::not_found(format!(
                "Unknown env_id: {}",
                engine_id.env_id
            )));
        }

        // Create game instance to get capabilities
        let game = create_game(&engine_id.env_id)
            .ok_or_else(|| Status::internal("Failed to create game instance"))?;

        let capabilities = game.capabilities();
        let proto_caps = Self::capabilities_to_proto(&capabilities);

        Ok(Response::new(proto_caps))
    }

    async fn reset(&self, request: Request<ResetRequest>) -> TonicResult<Response<ResetResponse>> {
        let req = request.into_inner();

        let engine_id = req
            .id
            .ok_or_else(|| Status::invalid_argument("Missing engine_id"))?;

        let env_id = engine_id.env_id.clone();
        let build_id = engine_id.build_id.clone();

        // Get buffers from pool
        let mut state_buf = self.buffer_pool.get_state_buffer();
        let mut obs_buf = self.buffer_pool.get_obs_buffer();

        let mut cache = self.game_cache.lock().await;

        let game = match cache.entry((env_id.clone(), build_id)) {
            Entry::Occupied(entry) => entry.into_mut(),
            Entry::Vacant(entry) => {
                let game = create_game(&env_id)
                    .ok_or_else(|| Status::not_found(format!("Unknown env_id: {}", env_id)))?;
                entry.insert(game)
            }
        };

        // Perform reset
        game.reset(req.seed, &req.hint, &mut state_buf, &mut obs_buf)
            .map_err(|e| Status::internal(format!("Reset failed: {}", e)))?;

        drop(cache);

        let response = ResetResponse {
            state: state_buf.clone(),
            obs: obs_buf.clone(),
        };

        // Return buffers to pool
        self.buffer_pool.return_state_buffer(state_buf);
        self.buffer_pool.return_obs_buffer(obs_buf);

        Ok(Response::new(response))
    }

    async fn step(&self, request: Request<StepRequest>) -> TonicResult<Response<StepResponse>> {
        let req = request.into_inner();

        let engine_id = req
            .id
            .ok_or_else(|| Status::invalid_argument("Missing engine_id"))?;

        if !is_registered(&engine_id.env_id) {
            return Err(Status::not_found(format!(
                "Unknown env_id: {}",
                engine_id.env_id
            )));
        }

        let key = (engine_id.env_id.clone(), engine_id.build_id.clone());

        let mut cache = self.game_cache.lock().await;
        let game = match cache.get_mut(&key) {
            Some(game) => game,
            None => {
                return Err(Status::failed_precondition(
                    "Game not initialized - call reset before step",
                ))
            }
        };

        // Get buffers from pool
        let mut new_state_buf = self.buffer_pool.get_state_buffer();
        let mut obs_buf = self.buffer_pool.get_obs_buffer();

        // Perform step
        let (reward, done, info) = game
            .step(&req.state, &req.action, &mut new_state_buf, &mut obs_buf)
            .map_err(|e| Status::internal(format!("Step failed: {}", e)))?;

        drop(cache);

        let response = StepResponse {
            state: new_state_buf.clone(),
            obs: obs_buf.clone(),
            reward,
            done,
            info,
        };

        // Return buffers to pool
        self.buffer_pool.return_state_buffer(new_state_buf);
        self.buffer_pool.return_obs_buffer(obs_buf);

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

    #[derive(Default)]
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
        ) -> (Self::Obs, f32, bool) {
            self.step_calls += 1;
            let random = rng.next_u32();
            state.0 = random as u64;
            let obs = RngObs(random as f32);
            let reward = random as f32 + self.step_calls as f32;
            (obs, reward, false)
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
