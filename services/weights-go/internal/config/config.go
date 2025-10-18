package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config captures all runtime tunables for the weights service.
type Config struct {
	Server        ServerConfig
	Registry      RegistryConfig
	Redis         RedisConfig
	Compatibility CompatibilityConfig
	Observability ObservabilityConfig
}

// ServerConfig contains gRPC listener settings.
type ServerConfig struct {
	Host            string
	Port            int
	ShutdownTimeout time.Duration
}

// RegistryConfig controls how published versions are persisted.
type RegistryConfig struct {
	Backend        string
	PersistenceDSN string
	HistoryDepth   int
}

// RedisConfig configures the optional Redis compatibility publisher.
type RedisConfig struct {
	Enabled  bool
	Address  string
	Channel  string
	Password string
	Database int
	Timeout  time.Duration
}

// CompatibilityConfig toggles backward compatibility features.
type CompatibilityConfig struct {
	MirrorToRedis bool
}

// ObservabilityConfig captures metrics and tracing settings.
type ObservabilityConfig struct {
	MetricsAddress string
	TracingEnabled bool
}

// Load reads configuration from the process environment.
func Load() Config {
	cfg := Config{
		Server: ServerConfig{
			Host:            getEnvString("WEIGHTS_GRPC_HOST", "0.0.0.0"),
			Port:            getEnvInt("WEIGHTS_GRPC_PORT", 8081),
			ShutdownTimeout: getEnvDuration("WEIGHTS_SHUTDOWN_TIMEOUT", 15*time.Second),
		},
		Registry: RegistryConfig{
			Backend:        getEnvString("WEIGHTS_REGISTRY_BACKEND", "memory"),
			PersistenceDSN: os.Getenv("WEIGHTS_REGISTRY_DSN"),
			HistoryDepth:   getEnvInt("WEIGHTS_HISTORY_DEPTH", 2),
		},
		Redis: RedisConfig{
			Enabled:  getEnvBool("WEIGHTS_REDIS_ENABLED", false),
			Address:  getEnvString("WEIGHTS_REDIS_ADDRESS", "127.0.0.1:6379"),
			Channel:  getEnvString("WEIGHTS_REDIS_CHANNEL", "weights:updates"),
			Password: os.Getenv("WEIGHTS_REDIS_PASSWORD"),
			Database: getEnvInt("WEIGHTS_REDIS_DB", 0),
			Timeout:  getEnvDuration("WEIGHTS_REDIS_TIMEOUT", 2*time.Second),
		},
		Compatibility: CompatibilityConfig{
			MirrorToRedis: getEnvBool("WEIGHTS_MIRROR_REDIS", true),
		},
		Observability: ObservabilityConfig{
			MetricsAddress: getEnvString("WEIGHTS_METRICS_ADDRESS", ":9094"),
			TracingEnabled: getEnvBool("WEIGHTS_TRACING_ENABLED", false),
		},
	}

	if !cfg.Redis.Enabled {
		cfg.Compatibility.MirrorToRedis = false
	}

	return cfg
}

// Endpoint returns host:port for the gRPC server.
func (s ServerConfig) Endpoint() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func getEnvString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return def
}
