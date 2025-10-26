use cartridge_actor::actor::Actor;
use cartridge_actor::config::Config;
use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;
use tokio::net::TcpListener;
use tokio::sync::Mutex;
use tonic::transport::Server;
use tonic::{Request, Response, Status};

// Import protobuf types
use cartridge_actor::proto::engine::v1::engine_server::{Engine, EngineServer};
use cartridge_actor::proto::engine::v1::{
    capabilities, Capabilities, Encoding, EngineId, ResetRequest, ResetResponse, SimResultChunk,
    StepRequest, StepResponse,
};
use cartridge_actor::proto::replay::v1::replay_server::{Replay, ReplayServer};
use cartridge_actor::proto::replay::v1::{
    ClearRequest, ClearResponse, GetStatsRequest, SampleRequest, SampleResponse,
    StoreBatchRequest, StoreBatchResponse, StoreTransitionRequest, StoreTransitionResponse,
    StatsResponse, UpdatePrioritiesRequest, UpdatePrioritiesResponse,
};

/// Mock Engine that simulates a fast game environment
#[derive(Debug, Clone)]
struct MockEngine {
    episode_length: u32,
    step_counter: Arc<Mutex<u32>>,
}

impl MockEngine {
    fn new(episode_length: u32) -> Self {
        Self {
            episode_length,
            step_counter: Arc::new(Mutex::new(0)),
        }
    }
}

#[tonic::async_trait]
impl Engine for MockEngine {
    type BatchSimulateStream = std::pin::Pin<
        Box<dyn futures::Stream<Item = Result<SimResultChunk, Status>> + Send>,
    >;
    async fn get_capabilities(
        &self,
        _request: Request<EngineId>,
    ) -> Result<Response<Capabilities>, Status> {
        Ok(Response::new(Capabilities {
            id: Some(EngineId {
                env_id: "bench-env".to_string(),
                build_id: "bench-1.0".to_string(),
            }),
            enc: Some(Encoding {
                state: "packed_u8:v1".to_string(),
                action: "discrete:v1".to_string(),
                obs: "packed_u8:v1".to_string(),
                schema_version: 1,
            }),
            max_horizon: self.episode_length,
            action_space: Some(capabilities::ActionSpace::DiscreteN(9)),
            preferred_batch: 32,
        }))
    }

    async fn reset(
        &self,
        _request: Request<ResetRequest>,
    ) -> Result<Response<ResetResponse>, Status> {
        let mut counter = self.step_counter.lock().await;
        *counter = 0;

        Ok(Response::new(ResetResponse {
            state: vec![0u8; 9], // Mock state
            obs: vec![0u8; 9],   // Mock observation
        }))
    }

    async fn step(
        &self,
        _request: Request<StepRequest>,
    ) -> Result<Response<StepResponse>, Status> {
        let mut counter = self.step_counter.lock().await;
        *counter += 1;

        let done = *counter >= self.episode_length;

        Ok(Response::new(StepResponse {
            state: vec![*counter as u8; 9], // Mock next state
            obs: vec![*counter as u8; 9],   // Mock observation
            reward: 0.0,
            done,
            info: 0, // Mock info bits
        }))
    }

    async fn batch_simulate(
        &self,
        _request: Request<cartridge_actor::proto::engine::v1::BatchSimulateRequest>,
    ) -> Result<Response<Self::BatchSimulateStream>, Status> {
        // Not implemented for benchmarking
        Err(Status::unimplemented("batch_simulate not needed for benchmarks"))
    }
}

/// Mock Replay that accepts batches without doing any work
#[derive(Debug, Clone)]
struct MockReplay;

#[tonic::async_trait]
impl Replay for MockReplay {
    async fn store_transition(
        &self,
        _request: Request<StoreTransitionRequest>,
    ) -> Result<Response<StoreTransitionResponse>, Status> {
        Ok(Response::new(StoreTransitionResponse {
            transition_id: "mock-id".to_string(),
            success: true,
            error_message: String::new(),
        }))
    }

    async fn store_batch(
        &self,
        request: Request<StoreBatchRequest>,
    ) -> Result<Response<StoreBatchResponse>, Status> {
        let count = request.get_ref().transitions.len() as u32;
        Ok(Response::new(StoreBatchResponse {
            transition_ids: vec![],
            stored_count: count,
            failed_count: 0,
            error_messages: vec![],
        }))
    }

    async fn sample(
        &self,
        _request: Request<SampleRequest>,
    ) -> Result<Response<SampleResponse>, Status> {
        Ok(Response::new(SampleResponse {
            transitions: vec![],
            total_available: 0,
            weights: vec![],
        }))
    }

    async fn get_stats(
        &self,
        _request: Request<GetStatsRequest>,
    ) -> Result<Response<StatsResponse>, Status> {
        Ok(Response::new(StatsResponse {
            total_transitions: 0,
            total_episodes: 0,
            transitions_by_env: std::collections::HashMap::new(),
            oldest_timestamp: 0,
            newest_timestamp: 0,
            storage_bytes: 0,
        }))
    }

    async fn update_priorities(
        &self,
        _request: Request<UpdatePrioritiesRequest>,
    ) -> Result<Response<UpdatePrioritiesResponse>, Status> {
        Ok(Response::new(UpdatePrioritiesResponse {
            updated_count: 0,
            error_messages: vec![],
        }))
    }

    async fn clear(
        &self,
        _request: Request<ClearRequest>,
    ) -> Result<Response<ClearResponse>, Status> {
        Ok(Response::new(ClearResponse {
            cleared_count: 0,
            remaining_count: 0,
        }))
    }
}

/// Start a mock engine server and return its address
async fn start_mock_engine(episode_length: u32) -> SocketAddr {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    let engine = MockEngine::new(episode_length);
    let svc = EngineServer::new(engine);

    tokio::spawn(async move {
        Server::builder()
            .add_service(svc)
            .serve_with_incoming(tokio_stream::wrappers::TcpListenerStream::new(listener))
            .await
            .unwrap();
    });

    // Wait a bit for server to start
    tokio::time::sleep(Duration::from_millis(100)).await;
    addr
}

/// Start a mock replay server and return its address
async fn start_mock_replay() -> SocketAddr {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    let replay = MockReplay;
    let svc = ReplayServer::new(replay);

    tokio::spawn(async move {
        Server::builder()
            .add_service(svc)
            .serve_with_incoming(tokio_stream::wrappers::TcpListenerStream::new(listener))
            .await
            .unwrap();
    });

    // Wait a bit for server to start
    tokio::time::sleep(Duration::from_millis(100)).await;
    addr
}

/// Run N episodes and measure throughput
async fn run_episodes(config: Config, num_episodes: i32) {
    let actor = Actor::new(config)
        .await
        .expect("Failed to create actor");

    // Run the specified number of episodes
    for _ in 0..num_episodes {
        actor
            .run_episode()
            .await
            .expect("Failed to run episode");
    }
}

fn benchmark_episode_throughput(c: &mut Criterion) {
    let runtime = tokio::runtime::Runtime::new().unwrap();

    let mut group = c.benchmark_group("actor_episode_throughput");

    // Test different episode lengths
    for episode_length in [10, 50, 100] {
        group.throughput(Throughput::Elements(10)); // 10 episodes

        group.bench_with_input(
            BenchmarkId::new("episodes", episode_length),
            &episode_length,
            |b, &episode_length| {
                b.to_async(&runtime).iter(|| async move {
                    // Start mock services
                    let engine_addr = start_mock_engine(episode_length).await;
                    let replay_addr = start_mock_replay().await;

                    let config = Config {
                        engine_addr: format!("http://{}", engine_addr),
                        replay_addr: format!("http://{}", replay_addr),
                        actor_id: "bench-actor".to_string(),
                        env_id: "bench-env".to_string(),
                        max_episodes: 10,
                        episode_timeout_secs: 30,
                        batch_size: 32,
                        flush_interval_secs: 5,
                        log_level: "error".to_string(),
                        metrics_addr: None,
                    };

                    run_episodes(config, 10).await;
                });
            },
        );
    }

    group.finish();
}

fn benchmark_single_episode(c: &mut Criterion) {
    let runtime = tokio::runtime::Runtime::new().unwrap();

    let mut group = c.benchmark_group("actor_single_episode");

    // Test different episode lengths
    for episode_length in [10, 50, 100] {
        group.bench_with_input(
            BenchmarkId::new("steps", episode_length),
            &episode_length,
            |b, &episode_length| {
                b.to_async(&runtime).iter(|| async move {
                    // Start mock services
                    let engine_addr = start_mock_engine(episode_length).await;
                    let replay_addr = start_mock_replay().await;

                    let config = Config {
                        engine_addr: format!("http://{}", engine_addr),
                        replay_addr: format!("http://{}", replay_addr),
                        actor_id: "bench-actor".to_string(),
                        env_id: "bench-env".to_string(),
                        max_episodes: 1,
                        episode_timeout_secs: 30,
                        batch_size: 32,
                        flush_interval_secs: 5,
                        log_level: "error".to_string(),
                        metrics_addr: None,
                    };

                    run_episodes(config, 1).await;
                });
            },
        );
    }

    group.finish();
}

criterion_group!(benches, benchmark_episode_throughput, benchmark_single_episode);
criterion_main!(benches);
