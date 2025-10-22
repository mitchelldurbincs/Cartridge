package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	var (
		addr    string
		timeout time.Duration
	)

	flag.StringVar(&addr, "addr", envOrDefault("ORCHESTRATOR_HEALTHCHECK_ADDR", "http://localhost:8080/metrics"), "Address of the orchestrator metrics endpoint")
	flag.DurationVar(&timeout, "timeout", envDuration("ORCHESTRATOR_HEALTHCHECK_TIMEOUT", 5*time.Second), "Request timeout")
	flag.Parse()

	client := http.Client{Timeout: timeout}
	resp, err := client.Get(addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "unexpected status code: %s\n", resp.Status)
		os.Exit(1)
	}

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		fmt.Fprintf(os.Stderr, "failed to discard response body: %v\n", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}
