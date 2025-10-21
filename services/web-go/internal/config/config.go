package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration for the web service.
type Config struct {
	Server        ServerConfig
	Observability ObservabilityConfig
	Orchestrator  OrchestratorConfig
}

// ServerConfig controls the primary HTTP listener.
type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// ObservabilityConfig configures metrics and logging exports.
type ObservabilityConfig struct {
	MetricsAddress string
	LogLevel       string
}

// OrchestratorConfig configures how the service speaks to the orchestrator.
type OrchestratorConfig struct {
	Endpoint       string
	RequestTimeout time.Duration
}

// Load reads configuration from files within the provided paths and environment variables.
// Environment variables use the WEB_ prefix, for example WEB_SERVER_PORT.
func Load(paths ...string) (Config, error) {
	values := map[string]string{}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if err := mergeConfigFile(values, filepath.Join(p, "config.yaml")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
		if err := mergeConfigFile(values, filepath.Join(p, "config.yml")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
		if err := mergeJSONFile(values, filepath.Join(p, "config.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}

	cfg := Config{
		Server: ServerConfig{
			Host:            getString("WEB_SERVER_HOST", values, "server.host", "0.0.0.0"),
			Port:            getInt("WEB_SERVER_PORT", values, "server.port", 8080),
			ReadTimeout:     getDuration("WEB_SERVER_READ_TIMEOUT", values, "server.read_timeout", 15*time.Second),
			WriteTimeout:    getDuration("WEB_SERVER_WRITE_TIMEOUT", values, "server.write_timeout", 15*time.Second),
			ShutdownTimeout: getDuration("WEB_SERVER_SHUTDOWN_TIMEOUT", values, "server.shutdown_timeout", 20*time.Second),
		},
		Observability: ObservabilityConfig{
			MetricsAddress: getString("WEB_OBSERVABILITY_METRICS_ADDRESS", values, "observability.metrics_address", ":9107"),
			LogLevel:       getString("WEB_OBSERVABILITY_LOG_LEVEL", values, "observability.log_level", "info"),
		},
		Orchestrator: OrchestratorConfig{
			Endpoint:       getString("WEB_ORCHESTRATOR_ENDPOINT", values, "orchestrator.endpoint", "http://localhost:9000"),
			RequestTimeout: getDuration("WEB_ORCHESTRATOR_REQUEST_TIMEOUT", values, "orchestrator.request_timeout", 5*time.Second),
		},
	}

	return cfg, nil
}

// Address returns host:port for the HTTP server.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func mergeConfigFile(dst map[string]string, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var section string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, ":") && !strings.Contains(line, " ") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		if section != "" {
			key = section + "." + key
		}
		if value != "" {
			dst[key] = value
		}
	}
	return scanner.Err()
}

func mergeJSONFile(dst map[string]string, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("config: parse json %s: %w", path, err)
	}

	flattenJSON(dst, "", raw)
	return nil
}

func flattenJSON(dst map[string]string, prefix string, raw map[string]any) {
	for key, value := range raw {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		switch typed := value.(type) {
		case map[string]any:
			flattenJSON(dst, fullKey, typed)
		case string:
			dst[fullKey] = typed
		case float64:
			dst[fullKey] = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", typed), "0"), ".")
		default:
			dst[fullKey] = fmt.Sprintf("%v", typed)
		}
	}
}

func getString(envKey string, values map[string]string, key string, def string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if v, ok := values[key]; ok && v != "" {
		return v
	}
	return def
}

func getInt(envKey string, values map[string]string, key string, def int) int {
	if v := os.Getenv(envKey); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	if v, ok := values[key]; ok {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return def
}

func getDuration(envKey string, values map[string]string, key string, def time.Duration) time.Duration {
	if v := os.Getenv(envKey); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	if v, ok := values[key]; ok {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return def
}
