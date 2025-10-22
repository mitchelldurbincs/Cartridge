use anyhow::{anyhow, Result};
use clap::Parser;
use serde::{Deserialize, Serialize};
use std::net::SocketAddr;
use std::time::Duration;
use tracing::level_filters::LevelFilter;

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
#[command(name = "actor")]
#[command(about = "Cartridge RL Actor Service")]
#[command(
    long_about = "Actor service that runs game episodes and collects experience data.

The actor connects to the engine service to simulate games and sends
transition data to the replay service for training."
)]
pub struct Config {
    /// Engine service address
    #[arg(
        long,
        env = "ACTOR_ENGINE_ADDR",
        default_value = "http://localhost:50051"
    )]
    pub engine_addr: String,

    /// Replay service address
    #[arg(
        long,
        env = "ACTOR_REPLAY_ADDR",
        default_value = "http://localhost:8080"
    )]
    pub replay_addr: String,

    /// Unique actor identifier
    #[arg(long, env = "ACTOR_ACTOR_ID", default_value = "actor-rust-1")]
    pub actor_id: String,

    /// Environment ID to run (e.g., tictactoe)
    #[arg(long, env = "ACTOR_ENV_ID", default_value = "tictactoe")]
    pub env_id: String,

    /// Maximum episodes to run (-1 for unlimited)
    #[arg(long, env = "ACTOR_MAX_EPISODES", default_value = "-1")]
    pub max_episodes: i32,

    /// Timeout per episode in seconds
    #[arg(long, env = "ACTOR_EPISODE_TIMEOUT", default_value = "30")]
    pub episode_timeout_secs: u64,

    /// Batch size for replay buffer
    #[arg(long, env = "ACTOR_BATCH_SIZE", default_value = "32")]
    pub batch_size: usize,

    /// Interval to flush partial batches in seconds
    #[arg(long, env = "ACTOR_FLUSH_INTERVAL", default_value = "5")]
    pub flush_interval_secs: u64,

    /// Log level (trace, debug, info, warn, error)
    #[arg(long, env = "ACTOR_LOG_LEVEL", default_value = "info")]
    pub log_level: String,

    /// Optional Prometheus metrics exporter bind address (e.g. 0.0.0.0:9002)
    #[arg(long, env = "ACTOR_METRICS_ADDR")]
    pub metrics_addr: Option<String>,
}

impl Config {
    pub fn validate(&self) -> Result<()> {
        if self.actor_id.is_empty() {
            return Err(anyhow!("actor_id cannot be empty"));
        }

        if self.env_id.is_empty() {
            return Err(anyhow!("env_id cannot be empty"));
        }

        if self.batch_size == 0 {
            return Err(anyhow!("batch_size must be greater than 0"));
        }

        if self.episode_timeout_secs == 0 {
            return Err(anyhow!("episode_timeout_secs must be greater than 0"));
        }

        if self.flush_interval_secs == 0 {
            return Err(anyhow!("flush_interval_secs must be greater than 0"));
        }

        if self.log_level.parse::<LevelFilter>().is_err() {
            return Err(anyhow!(
                "invalid log level '{}', expected one of trace, debug, info, warn, error",
                self.log_level
            ));
        }

        if let Some(addr) = &self.metrics_addr {
            addr.parse::<SocketAddr>()
                .map_err(|e| anyhow!("invalid metrics bind address '{}': {}", addr, e))?;
        }

        Ok(())
    }

    pub fn episode_timeout(&self) -> Duration {
        Duration::from_secs(self.episode_timeout_secs)
    }

    pub fn flush_interval(&self) -> Duration {
        Duration::from_secs(self.flush_interval_secs)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn base_config() -> Config {
        Config {
            engine_addr: "http://localhost:50051".into(),
            replay_addr: "http://localhost:8080".into(),
            actor_id: "actor".into(),
            env_id: "env".into(),
            max_episodes: 1,
            episode_timeout_secs: 30,
            batch_size: 4,
            flush_interval_secs: 5,
            log_level: "info".into(),
            metrics_addr: None,
        }
    }

    #[test]
    fn validate_accepts_valid_configuration() {
        let cfg = base_config();
        assert!(cfg.validate().is_ok());
    }

    #[test]
    fn validate_rejects_empty_actor_id() {
        let mut cfg = base_config();
        cfg.actor_id.clear();
        let err = cfg.validate().unwrap_err();
        assert!(err.to_string().contains("actor_id"));
    }

    #[test]
    fn validate_rejects_zero_batch_size() {
        let mut cfg = base_config();
        cfg.batch_size = 0;
        let err = cfg.validate().unwrap_err();
        assert!(err.to_string().contains("batch_size"));
    }

    #[test]
    fn validate_rejects_invalid_metrics_address() {
        let mut cfg = base_config();
        cfg.metrics_addr = Some("not-an-addr".into());
        let err = cfg.validate().unwrap_err();
        assert!(err.to_string().contains("invalid metrics bind address"));
    }

    #[test]
    fn validate_rejects_invalid_log_level() {
        let mut cfg = base_config();
        cfg.log_level = "nope".into();
        let err = cfg.validate().unwrap_err();
        assert!(err.to_string().contains("invalid log level"));
    }
}
