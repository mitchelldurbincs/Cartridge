package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all orchestrator configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	NATS     NATSConfig
	Health   HealthConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port            int
	Host            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// NATSConfig holds NATS configuration
type NATSConfig struct {
	URL     string
	Subject string
}

// HealthConfig holds health monitoring configuration
type HealthConfig struct {
	CheckInterval         time.Duration
	HeartbeatStaleAfter   time.Duration
	HeartbeatUnresponsive time.Duration
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnvInt("PORT", 8080),
			Host:            getEnvString("HOST", "0.0.0.0"),
			ReadTimeout:     getEnvDuration("READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getEnvDuration("WRITE_TIMEOUT", 30*time.Second),
			ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Database: DatabaseConfig{
			Host:     getEnvString("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnvString("DB_USER", "postgres"),
			Password: getEnvString("DB_PASSWORD", ""),
			DBName:   getEnvString("DB_NAME", "cartridge"),
			SSLMode:  getEnvString("DB_SSL_MODE", "disable"),
		},
		NATS: NATSConfig{
			URL:     getEnvString("NATS_URL", "nats://localhost:4222"),
			Subject: getEnvString("NATS_SUBJECT", "run-status"),
		},
		Health: HealthConfig{
			CheckInterval:         getEnvDuration("HEALTH_CHECK_INTERVAL", 15*time.Second),
			HeartbeatStaleAfter:   getEnvDuration("HEARTBEAT_STALE_AFTER", 45*time.Second),
			HeartbeatUnresponsive: getEnvDuration("HEARTBEAT_UNRESPONSIVE", 135*time.Second),
		},
	}

	return cfg, nil
}

// ConnectionString returns the database connection string
func (d DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode)
}

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}