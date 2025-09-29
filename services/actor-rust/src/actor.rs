use anyhow::{anyhow, Result};
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tokio::time::{interval, timeout};
use tonic::{transport::Channel, Request};
use tracing::{debug, error, info};

use crate::config::Config;
use crate::policy::{Policy, RandomPolicy};
use crate::proto::engine::v1::{
    engine_client::EngineClient, EngineId, ResetRequest, StepRequest,
};
use crate::proto::replay::v1::{
    replay_client::ReplayClient, StoreBatchRequest, Transition,
};

pub struct Actor {
    config: Config,
    engine_client: EngineClient<Channel>,
    replay_client: ReplayClient<Channel>,
    policy: Arc<Mutex<Box<dyn Policy>>>,
    episode_count: Arc<Mutex<u32>>,
    transition_buffer: Arc<Mutex<Vec<Transition>>>,
    shutdown_signal: Arc<Mutex<bool>>,
}

impl Actor {
    pub async fn new(config: Config) -> Result<Self> {
        // Connect to engine service
        info!("Connecting to engine service at {}", config.engine_addr);
        let engine_channel = tonic::transport::Endpoint::new(config.engine_addr.clone())?
            .connect()
            .await
            .map_err(|e| anyhow!("Failed to connect to engine at {}: {}", config.engine_addr, e))?;

        // Connect to replay service
        info!("Connecting to replay service at {}", config.replay_addr);
        let replay_channel = tonic::transport::Endpoint::new(config.replay_addr.clone())?
            .connect()
            .await
            .map_err(|e| anyhow!("Failed to connect to replay at {}: {}", config.replay_addr, e))?;

        let mut engine_client = EngineClient::new(engine_channel);
        let replay_client = ReplayClient::new(replay_channel);

        // Get game capabilities to configure policy
        info!("Fetching capabilities for environment: {}", config.env_id);
        let capabilities_request = Request::new(EngineId {
            env_id: config.env_id.clone(),
            build_id: "actor-rust".to_string(),
        });

        let capabilities_response = engine_client
            .get_capabilities(capabilities_request)
            .await
            .map_err(|e| anyhow!("Failed to get capabilities for {}: {}", config.env_id, e))?;

        let capabilities = capabilities_response.into_inner();

        // Create random policy based on action space
        let policy = RandomPolicy::new(&capabilities)
            .map_err(|e| anyhow!("Failed to create policy: {}", e))?;

        info!(
            "Actor {} initialized for environment {}",
            config.actor_id, config.env_id
        );
        info!(
            "Game capabilities: max_horizon={}, preferred_batch={}",
            capabilities.max_horizon, capabilities.preferred_batch
        );

        Ok(Self {
            config,
            engine_client,
            replay_client,
            policy: Arc::new(Mutex::new(Box::new(policy))),
            episode_count: Arc::new(Mutex::new(0)),
            transition_buffer: Arc::new(Mutex::new(Vec::new())),
            shutdown_signal: Arc::new(Mutex::new(false)),
        })
    }

    pub async fn run(&self) -> Result<()> {
        info!("Actor {} starting main loop", self.config.actor_id);

        // Setup flush timer for partial batches
        let mut flush_timer = interval(self.config.flush_interval());

        loop {
            // Check shutdown signal
            if *self.shutdown_signal.lock().unwrap() {
                info!("Shutdown signal received, stopping actor");
                break;
            }

            tokio::select! {
                _ = flush_timer.tick() => {
                    // Flush partial batches periodically
                    let buffer_len = self.transition_buffer.lock().unwrap().len();
                    if buffer_len > 0 {
                        debug!("Periodic flush: {} transitions in buffer", buffer_len);
                        if let Err(e) = self.flush_buffer().await {
                            error!("Failed to flush buffer: {}", e);
                        }
                    }
                }

                _ = tokio::time::sleep(Duration::from_millis(1)) => {
                    // Check episode limit
                    let current_episode_count = *self.episode_count.lock().unwrap();
                    if self.config.max_episodes > 0 && current_episode_count >= self.config.max_episodes as u32 {
                        info!("Reached maximum episodes ({}), stopping", self.config.max_episodes);
                        break;
                    }

                    // Run an episode
                    match self.run_episode().await {
                        Ok(_) => {
                            let mut count = self.episode_count.lock().unwrap();
                            *count += 1;
                            if *count % 10 == 0 {
                                info!("Completed {} episodes", *count);
                            }
                        }
                        Err(e) => {
                            let count = *self.episode_count.lock().unwrap();
                            error!("Episode {} failed: {}", count + 1, e);
                            // Continue with next episode rather than stopping
                        }
                    }
                }
            }
        }

        // Flush any remaining transitions
        self.flush_buffer().await?;
        info!("Actor stopped gracefully");
        Ok(())
    }

    pub async fn shutdown(&self) {
        *self.shutdown_signal.lock().unwrap() = true;
        info!("Shutdown signal set");
    }

    async fn run_episode(&self) -> Result<()> {
        let episode_count = *self.episode_count.lock().unwrap();

        // Reset the game
        let reset_request = Request::new(ResetRequest {
            id: Some(EngineId {
                env_id: self.config.env_id.clone(),
                build_id: "actor-rust".to_string(),
            }),
            seed: SystemTime::now().duration_since(UNIX_EPOCH)?.as_nanos() as u64,
            hint: vec![],
        });

        let reset_response = timeout(
            self.config.episode_timeout(),
            self.engine_client.clone().reset(reset_request),
        )
        .await
        .map_err(|_| anyhow!("Reset timed out"))?
        .map_err(|e| anyhow!("Failed to reset game: {}", e))?;

        let reset_data = reset_response.into_inner();
        let episode_id = format!("{}-ep-{}-{}",
            self.config.actor_id,
            episode_count,
            SystemTime::now().duration_since(UNIX_EPOCH)?.as_secs()
        );

        let mut current_state = reset_data.state;
        let mut current_obs = reset_data.obs;
        let mut step_number = 0u32;

        debug!("Started episode {}", episode_id);

        loop {
            // Select action using policy
            let action = {
                let mut policy = self.policy.lock().unwrap();
                policy.select_action(&current_obs)
                    .map_err(|e| anyhow!("Failed to select action: {}", e))?
            };

            // Take step in environment
            let step_request = Request::new(StepRequest {
                id: Some(EngineId {
                    env_id: self.config.env_id.clone(),
                    build_id: "actor-rust".to_string(),
                }),
                state: current_state.clone(),
                action: action.clone(),
            });

            let step_response = timeout(
                self.config.episode_timeout(),
                self.engine_client.clone().step(step_request),
            )
            .await
            .map_err(|_| anyhow!("Step timed out"))?
            .map_err(|e| anyhow!("Failed to step environment: {}", e))?;

            let step_data = step_response.into_inner();

            // Create transition
            let transition = Transition {
                id: format!("{}-step-{}", episode_id, step_number),
                env_id: self.config.env_id.clone(),
                episode_id: episode_id.clone(),
                step_number,
                state: current_state.clone(),
                action,
                next_state: step_data.state.clone(),
                observation: current_obs.clone(),
                next_observation: step_data.obs.clone(),
                reward: step_data.reward,
                done: step_data.done,
                priority: 1.0, // Default priority
                timestamp: SystemTime::now().duration_since(UNIX_EPOCH)?.as_secs(),
                metadata: std::collections::HashMap::new(),
            };

            // Add to buffer
            {
                let mut buffer = self.transition_buffer.lock().unwrap();
                buffer.push(transition);

                // Flush buffer if full
                if buffer.len() >= self.config.batch_size {
                    drop(buffer); // Release lock before async call
                    self.flush_buffer().await?;
                }
            }

            // Check if episode is done
            if step_data.done {
                debug!(
                    "Episode {} completed in {} steps, final reward: {:.2}",
                    episode_id,
                    step_number + 1,
                    step_data.reward
                );
                break;
            }

            // Update state for next step
            current_state = step_data.state;
            current_obs = step_data.obs;
            step_number += 1;
        }

        Ok(())
    }

    async fn flush_buffer(&self) -> Result<()> {
        let transitions = {
            let mut buffer = self.transition_buffer.lock().unwrap();
            if buffer.is_empty() {
                return Ok(());
            }
            let transitions = buffer.clone();
            buffer.clear();
            transitions
        };

        debug!("Flushing {} transitions to replay service", transitions.len());

        let request = Request::new(StoreBatchRequest { transitions });

        self.replay_client
            .clone()
            .store_batch(request)
            .await
            .map_err(|e| anyhow!("Failed to store batch: {}", e))?;

        Ok(())
    }
}