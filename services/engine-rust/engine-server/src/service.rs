//! gRPC service implementation for the Engine server
//!
//! This module provides the Tonic-based gRPC server implementation that handles
//! all engine service methods with proper error handling and buffer management.

use std::pin::Pin;
use std::sync::Arc;
use std::time::Instant;

use dashmap::{mapref::entry::Entry, DashMap};
use engine_core::registry::{create_game, is_registered};
use engine_core::ErasedGame;
use engine_proto::{
    engine_server::Engine, BatchSimulateRequest, BoxSpec as ProtoBoxSpec, Capabilities,
    Encoding as ProtoEncoding, EngineId, MultiDiscrete as ProtoMultiDiscrete, ResetRequest,
    ResetResponse, SimResultChunk, StepRequest, StepResponse,
};
use tonic::{Request, Response, Result as TonicResult, Status};

use metrics::{counter, gauge, histogram};
use tokio_stream::Stream;

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

    /// Sample a random action from the action space
    fn sample_random_action(
        action_space: &engine_core::typed::ActionSpace,
        rng: &mut rand_chacha::ChaCha20Rng,
        out: &mut Vec<u8>,
    ) -> Result<(), String> {
        use engine_core::typed::ActionSpace;
        use rand::Rng;

        match action_space {
            ActionSpace::Discrete(n) => {
                if *n == 0 {
                    return Err("Discrete action space with n=0".to_string());
                }
                let action = rng.gen_range(0..*n);
                out.push(action as u8);
                Ok(())
            }
            ActionSpace::MultiDiscrete(nvec) => {
                for &n in nvec {
                    if n == 0 {
                        return Err("MultiDiscrete action space with n=0".to_string());
                    }
                    let action = rng.gen_range(0..n);
                    out.push(action as u8);
                }
                Ok(())
            }
            ActionSpace::Continuous { low, high, shape } => {
                if low.len() != high.len() {
                    return Err("Continuous action space low/high length mismatch".to_string());
                }

                // Calculate total size from shape
                let total_size: usize = shape.iter().map(|&s| s as usize).product();

                if total_size != low.len() {
                    return Err(format!(
                        "Continuous action space shape mismatch: shape product {} != low.len() {}",
                        total_size, low.len()
                    ));
                }

                for i in 0..low.len() {
                    let value = rng.gen_range(low[i]..high[i]);
                    out.extend_from_slice(&value.to_le_bytes());
                }
                Ok(())
            }
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
    type BatchSimulateStream = Pin<Box<dyn Stream<Item = Result<SimResultChunk, Status>> + Send>>;

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
                entry.into_ref()
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
            tracing::error!(
                error = %e,
                env_id = %env_id,
                seed = req.seed,
                hint_size = req.hint.len(),
                "Reset operation failed"
            );
            counter!(
                "engine_rpc_failures_total",
                1,
                "method" => "reset",
                "error" => "reset_failed"
            );
            Self::observe_rpc_latency("reset", start);
            return Err(Status::internal(format!(
                "Reset failed: {} (env_id={}, seed={}, hint_size={})",
                e,
                env_id,
                req.seed,
                req.hint.len()
            )));
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
        let key = (env_id.clone(), build_id.clone());

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
                    tracing::error!(
                        error = %e,
                        env_id = %env_id,
                        state_size = req.state.len(),
                        action_size = req.action.len(),
                        "Step operation failed"
                    );
                    counter!(
                        "engine_rpc_failures_total",
                        1,
                        "method" => "step",
                        "error" => "step_failed"
                    );
                    Self::observe_rpc_latency("step", start);
                    return Err(Status::internal(format!(
                        "Step failed: {} (env_id={}, state_size={}, action_size={})",
                        e,
                        env_id,
                        req.state.len(),
                        req.action.len()
                    )));
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

    async fn batch_simulate(
        &self,
        request: Request<BatchSimulateRequest>,
    ) -> TonicResult<Response<Self::BatchSimulateStream>> {
        counter!("engine_rpc_requests_total", 1, "method" => "batch_simulate");
        let start = Instant::now();
        let req = request.into_inner();

        // Validate engine_id
        let engine_id = match req.id {
            Some(id) => id,
            None => {
                counter!(
                    "engine_rpc_failures_total",
                    1,
                    "method" => "batch_simulate",
                    "error" => "missing_engine_id"
                );
                Self::observe_rpc_latency("batch_simulate", start);
                return Err(Status::invalid_argument("Missing engine_id"));
            }
        };

        // Validate env_id is registered
        if !is_registered(&engine_id.env_id) {
            counter!(
                "engine_rpc_failures_total",
                1,
                "method" => "batch_simulate",
                "error" => "unknown_env"
            );
            Self::observe_rpc_latency("batch_simulate", start);
            return Err(Status::not_found(format!(
                "Unknown env_id: {}",
                engine_id.env_id
            )));
        }

        // Validate trajectories are provided
        if req.trajectories.is_empty() {
            counter!(
                "engine_rpc_failures_total",
                1,
                "method" => "batch_simulate",
                "error" => "empty_batch"
            );
            Self::observe_rpc_latency("batch_simulate", start);
            return Err(Status::invalid_argument(
                "No trajectories provided in batch",
            ));
        }

        // Create game instance for capabilities lookup
        let game = match create_game(&engine_id.env_id) {
            Some(game) => game,
            None => {
                counter!(
                    "engine_rpc_failures_total",
                    1,
                    "method" => "batch_simulate",
                    "error" => "create_failed"
                );
                Self::observe_rpc_latency("batch_simulate", start);
                return Err(Status::internal("Failed to create game instance"));
            }
        };

        let capabilities = game.capabilities();

        // Clone necessary data for the async stream
        let buffer_pool = self.buffer_pool.clone();
        let trajectories = req.trajectories;
        let return_states = req.return_states;
        let return_observations = req.return_observations;

        counter!(
            "engine_rpc_success_total",
            1,
            "method" => "batch_simulate"
        );
        counter!(
            "engine_batch_simulate_trajectories_total",
            trajectories.len() as u64,
        );

        Self::observe_rpc_latency("batch_simulate", start);

        // Create streaming response
        let stream = async_stream::stream! {
            use rand::SeedableRng;
            use rand_chacha::ChaCha20Rng;
            use engine_proto::{SimStep, TrajectoryResult};

            let mut results = Vec::new();

            for (trajectory_id, config) in trajectories.iter().enumerate() {
                // Create a new game instance for this trajectory
                let mut game = match create_game(&engine_id.env_id) {
                    Some(g) => g,
                    None => {
                        tracing::error!(
                            env_id = %engine_id.env_id,
                            trajectory_id = trajectory_id,
                            "Failed to create game instance for trajectory"
                        );
                        counter!(
                            "engine_batch_simulate_trajectory_failures_total",
                            1,
                            "error" => "create_failed"
                        );
                        continue;
                    }
                };

                // Get buffers from pool
                let mut state_buf = buffer_pool.get_state_buffer();
                let mut obs_buf = buffer_pool.get_obs_buffer();
                let mut action_buf = buffer_pool.get_action_buffer();

                // Reset the game with the trajectory's seed
                if let Err(e) = game.reset(config.seed, &config.hint, &mut state_buf, &mut obs_buf) {
                    tracing::error!(
                        error = %e,
                        env_id = %engine_id.env_id,
                        trajectory_id = trajectory_id,
                        seed = config.seed,
                        "Reset failed for trajectory"
                    );
                    counter!(
                        "engine_batch_simulate_trajectory_failures_total",
                        1,
                        "error" => "reset_failed"
                    );
                    buffer_pool.return_state_buffer(state_buf);
                    buffer_pool.return_obs_buffer(obs_buf);
                    buffer_pool.return_action_buffer(action_buf);
                    continue;
                }

                // Initialize RNG for action sampling (use trajectory seed + offset for diversity)
                let mut action_rng = ChaCha20Rng::seed_from_u64(config.seed.wrapping_add(12345));

                let mut steps = Vec::new();
                let mut total_reward = 0.0f32;
                let max_steps = if config.max_steps > 0 {
                    config.max_steps
                } else {
                    capabilities.max_horizon
                };

                // Simulate trajectory
                for step_num in 0..max_steps {
                    // Sample random action from action space
                    action_buf.clear();
                    if let Err(e) = Self::sample_random_action(&capabilities.action_space, &mut action_rng, &mut action_buf) {
                        tracing::error!(
                            error = %e,
                            env_id = %engine_id.env_id,
                            trajectory_id = trajectory_id,
                            step = step_num,
                            "Failed to sample random action"
                        );
                        counter!(
                            "engine_batch_simulate_trajectory_failures_total",
                            1,
                            "error" => "action_sample_failed"
                        );
                        break;
                    }

                    // Record current state and observation if requested
                    let state_copy = if return_states {
                        state_buf.clone()
                    } else {
                        Vec::new()
                    };
                    let obs_copy = if return_observations {
                        obs_buf.clone()
                    } else {
                        Vec::new()
                    };
                    let action_copy = action_buf.clone();

                    // Take step
                    let mut next_state_buf = buffer_pool.get_state_buffer();
                    let mut next_obs_buf = buffer_pool.get_obs_buffer();

                    let (reward, step_done, _info) = match game.step(
                        &state_buf,
                        &action_buf,
                        &mut next_state_buf,
                        &mut next_obs_buf,
                    ) {
                        Ok(result) => result,
                        Err(e) => {
                            tracing::error!(
                                error = %e,
                                env_id = %engine_id.env_id,
                                trajectory_id = trajectory_id,
                                step = step_num,
                                "Step failed for trajectory"
                            );
                            counter!(
                                "engine_batch_simulate_trajectory_failures_total",
                                1,
                                "error" => "step_failed"
                            );
                            buffer_pool.return_state_buffer(next_state_buf);
                            buffer_pool.return_obs_buffer(next_obs_buf);
                            break;
                        }
                    };

                    total_reward += reward;

                    // Record step
                    steps.push(SimStep {
                        state: state_copy,
                        action: action_copy,
                        obs: obs_copy,
                        reward,
                        done: step_done,
                    });

                    // Update state and obs for next iteration
                    buffer_pool.return_state_buffer(state_buf);
                    buffer_pool.return_obs_buffer(obs_buf);
                    state_buf = next_state_buf;
                    obs_buf = next_obs_buf;

                    if step_done {
                        break;
                    }
                }

                // Return buffers to pool
                buffer_pool.return_state_buffer(state_buf);
                buffer_pool.return_obs_buffer(obs_buf);
                buffer_pool.return_action_buffer(action_buf);

                // Create trajectory result
                let total_steps = steps.len() as u32;
                results.push(TrajectoryResult {
                    trajectory_id: trajectory_id as u32,
                    steps,
                    total_steps,
                    total_reward,
                });

                counter!(
                    "engine_batch_simulate_steps_total",
                    total_steps as u64,
                );
                histogram!(
                    "engine_batch_simulate_trajectory_steps",
                    total_steps as f64,
                );
                histogram!(
                    "engine_batch_simulate_trajectory_reward",
                    total_reward as f64,
                );
            }

            // Send final chunk with all results
            yield Ok(SimResultChunk {
                trajectories: results,
                final_chunk: true,
            });
        };

        Ok(Response::new(Box::pin(stream)))
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
    use tokio_stream::StreamExt;

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
    async fn test_batch_simulate_basic() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(BatchSimulateRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            trajectories: vec![
                engine_proto::TrajectoryConfig {
                    seed: 42,
                    hint: Vec::new(),
                    max_steps: 9,
                },
                engine_proto::TrajectoryConfig {
                    seed: 123,
                    hint: Vec::new(),
                    max_steps: 9,
                },
            ],
            return_states: true,
            return_observations: true,
        });

        let response = service.batch_simulate(request).await.unwrap();
        let mut stream = response.into_inner();

        // Collect all chunks
        let mut all_trajectories = Vec::new();
        while let Some(chunk_result) = stream.next().await {
            let chunk = chunk_result.unwrap();
            all_trajectories.extend(chunk.trajectories);

            if chunk.final_chunk {
                break;
            }
        }

        // Verify we got 2 trajectories
        assert_eq!(all_trajectories.len(), 2);

        // Verify trajectory IDs
        assert_eq!(all_trajectories[0].trajectory_id, 0);
        assert_eq!(all_trajectories[1].trajectory_id, 1);

        // Verify steps are not empty and have proper data
        for traj in &all_trajectories {
            assert!(!traj.steps.is_empty());
            assert!(traj.total_steps > 0);
            assert_eq!(traj.total_steps, traj.steps.len() as u32);

            // Check that states, actions, and observations are populated
            for step in &traj.steps {
                assert!(!step.state.is_empty()); // return_states was true
                assert!(!step.action.is_empty());
                assert!(!step.obs.is_empty()); // return_observations was true
            }
        }
    }

    #[tokio::test]
    async fn test_batch_simulate_without_states_and_obs() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(BatchSimulateRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            trajectories: vec![engine_proto::TrajectoryConfig {
                seed: 42,
                hint: Vec::new(),
                max_steps: 5,
            }],
            return_states: false,
            return_observations: false,
        });

        let response = service.batch_simulate(request).await.unwrap();
        let mut stream = response.into_inner();

        // Collect all chunks
        let mut all_trajectories = Vec::new();
        while let Some(chunk_result) = stream.next().await {
            let chunk = chunk_result.unwrap();
            all_trajectories.extend(chunk.trajectories);

            if chunk.final_chunk {
                break;
            }
        }

        assert_eq!(all_trajectories.len(), 1);

        // Verify states and observations are empty
        for traj in &all_trajectories {
            for step in &traj.steps {
                assert!(step.state.is_empty()); // return_states was false
                assert!(!step.action.is_empty()); // actions are always returned
                assert!(step.obs.is_empty()); // return_observations was false
            }
        }
    }

    #[tokio::test]
    async fn test_batch_simulate_deterministic() {
        setup_test_registry();

        let service = EngineService::new();

        // Run first simulation
        let request1 = Request::new(BatchSimulateRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            trajectories: vec![engine_proto::TrajectoryConfig {
                seed: 12345,
                hint: Vec::new(),
                max_steps: 5,
            }],
            return_states: true,
            return_observations: true,
        });

        let response1 = service.batch_simulate(request1).await.unwrap();
        let mut stream1 = response1.into_inner();

        let mut traj1 = Vec::new();
        while let Some(chunk_result) = stream1.next().await {
            let chunk = chunk_result.unwrap();
            traj1.extend(chunk.trajectories);
        }

        // Run second simulation with same seed
        let service2 = EngineService::new();
        let request2 = Request::new(BatchSimulateRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            trajectories: vec![engine_proto::TrajectoryConfig {
                seed: 12345,
                hint: Vec::new(),
                max_steps: 5,
            }],
            return_states: true,
            return_observations: true,
        });

        let response2 = service2.batch_simulate(request2).await.unwrap();
        let mut stream2 = response2.into_inner();

        let mut traj2 = Vec::new();
        while let Some(chunk_result) = stream2.next().await {
            let chunk = chunk_result.unwrap();
            traj2.extend(chunk.trajectories);
        }

        // Verify determinism: same seed should produce same trajectories
        assert_eq!(traj1.len(), traj2.len());
        assert_eq!(traj1[0].total_steps, traj2[0].total_steps);
        assert_eq!(traj1[0].total_reward, traj2[0].total_reward);

        // Verify step-by-step equality
        for (step1, step2) in traj1[0].steps.iter().zip(traj2[0].steps.iter()) {
            assert_eq!(step1.state, step2.state);
            assert_eq!(step1.action, step2.action);
            assert_eq!(step1.obs, step2.obs);
            assert_eq!(step1.reward, step2.reward);
            assert_eq!(step1.done, step2.done);
        }
    }

    #[tokio::test]
    async fn test_batch_simulate_missing_engine_id() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(BatchSimulateRequest {
            id: None,
            trajectories: vec![engine_proto::TrajectoryConfig {
                seed: 42,
                hint: Vec::new(),
                max_steps: 5,
            }],
            return_states: false,
            return_observations: false,
        });

        let result = service.batch_simulate(request).await;
        assert!(result.is_err());

        if let Err(err) = result {
            assert_eq!(err.code(), tonic::Code::InvalidArgument);
        }
    }

    #[tokio::test]
    async fn test_batch_simulate_unknown_game() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(BatchSimulateRequest {
            id: Some(EngineId {
                env_id: "unknown-game".to_string(),
                build_id: "test".to_string(),
            }),
            trajectories: vec![engine_proto::TrajectoryConfig {
                seed: 42,
                hint: Vec::new(),
                max_steps: 5,
            }],
            return_states: false,
            return_observations: false,
        });

        let result = service.batch_simulate(request).await;
        assert!(result.is_err());

        if let Err(err) = result {
            assert_eq!(err.code(), tonic::Code::NotFound);
        }
    }

    #[tokio::test]
    async fn test_batch_simulate_empty_trajectories() {
        setup_test_registry();

        let service = EngineService::new();
        let request = Request::new(BatchSimulateRequest {
            id: Some(EngineId {
                env_id: "tictactoe".to_string(),
                build_id: "test".to_string(),
            }),
            trajectories: Vec::new(),
            return_states: false,
            return_observations: false,
        });

        let result = service.batch_simulate(request).await;
        assert!(result.is_err());

        if let Err(err) = result {
            assert_eq!(err.code(), tonic::Code::InvalidArgument);
        }
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
