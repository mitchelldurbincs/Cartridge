package config

import (
	"fmt"
	"time"
)

// Config holds all actor configuration
type Config struct {
	// Service endpoints
	EngineAddr string `mapstructure:"engine_addr"`
	ReplayAddr string `mapstructure:"replay_addr"`

	// Actor settings
	ActorID     string `mapstructure:"actor_id"`
	EnvID       string `mapstructure:"env_id"`

	// Episode management
	MaxEpisodes   int           `mapstructure:"max_episodes"`
	EpisodeTimeout time.Duration `mapstructure:"episode_timeout"`

	// Batch settings
	BatchSize     int `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`

	// Logging
	LogLevel string `mapstructure:"log_level"`
}

// Default returns a config with sensible defaults
func Default() *Config {
	return &Config{
		EngineAddr:     "localhost:50051",
		ReplayAddr:     "localhost:8080",
		ActorID:        "actor-1",
		EnvID:          "tictactoe",
		MaxEpisodes:    -1, // unlimited
		EpisodeTimeout: 30 * time.Second,
		BatchSize:      32,
		FlushInterval:  5 * time.Second,
		LogLevel:       "info",
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.EngineAddr == "" {
		return fmt.Errorf("engine_addr is required")
	}
	if c.ReplayAddr == "" {
		return fmt.Errorf("replay_addr is required")
	}
	if c.EnvID == "" {
		return fmt.Errorf("env_id is required")
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be positive")
	}
	if c.EpisodeTimeout <= 0 {
		return fmt.Errorf("episode_timeout must be positive")
	}
	return nil
}