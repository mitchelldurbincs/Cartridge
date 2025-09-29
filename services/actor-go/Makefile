# Actor Service Makefile

.PHONY: build test clean proto run docker

# Default target
all: build

# Build the actor binary
build:
	go build -o bin/actor cmd/actor/main.go

# Generate protobuf code
proto:
	buf generate

# Run tests
test:
	go test ./...

# Run tests with race detection
test-race:
	go test -race ./...

# Run integration tests (requires services running)
test-integration:
	go test -tags=integration ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf pkg/proto/

# Run actor with tictactoe
run:
	go run cmd/actor/main.go --env-id tictactoe

# Run actor with debug logging
run-debug:
	go run cmd/actor/main.go --env-id tictactoe --log-level debug

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Build Docker image
docker:
	docker build -t cartridge/actor .

# Development helpers
dev-setup: deps proto

# Full CI pipeline
ci: fmt lint test test-race

# Help target
help:
	@echo "Available targets:"
	@echo "  build          - Build the actor binary"
	@echo "  proto          - Generate protobuf code"
	@echo "  test           - Run unit tests"
	@echo "  test-race      - Run tests with race detection"
	@echo "  test-integration - Run integration tests"
	@echo "  clean          - Clean build artifacts"
	@echo "  run            - Run actor with tictactoe"
	@echo "  run-debug      - Run actor with debug logging"
	@echo "  deps           - Download and tidy dependencies"
	@echo "  fmt            - Format Go code"
	@echo "  lint           - Lint Go code"
	@echo "  docker         - Build Docker image"
	@echo "  dev-setup      - Setup for development"
	@echo "  ci             - Run CI pipeline"
	@echo "  help           - Show this help"