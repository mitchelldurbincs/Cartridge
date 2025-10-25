# Build stage
# To pin with SHA256, run: docker pull rust:1.90 && docker inspect rust:1.90 --format='{{index .RepoDigests 0}}'
# Then use: FROM rust:1.90@sha256:<hash>
FROM rust:1.90 AS builder

# Install protoc
RUN apt-get update && apt-get install -y protobuf-compiler && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy workspace manifests first for dependency caching
# This layer will be cached unless Cargo.toml files change
COPY services/engine-rust/Cargo.toml services/engine-rust/
COPY services/engine-rust/Cargo.lock* services/engine-rust/
COPY services/engine-rust/engine-core/Cargo.toml services/engine-rust/engine-core/
COPY services/engine-rust/engine-server/Cargo.toml services/engine-rust/engine-server/
COPY services/engine-rust/engine-proto/Cargo.toml services/engine-rust/engine-proto/
COPY services/engine-rust/engine-proto/build.rs services/engine-rust/engine-proto/
COPY services/engine-rust/games-tictactoe/Cargo.toml services/engine-rust/games-tictactoe/
COPY proto/ proto/

# Debug: Verify proto files were copied correctly
RUN echo "=== Verifying proto file structure ===" && \
    echo "Current directory: $(pwd)" && \
    echo "Listing /workspace contents:" && \
    ls -la /workspace/ && \
    echo "" && \
    echo "Listing proto directory:" && \
    ls -la proto/ 2>&1 || echo "ERROR: proto/ directory not found!" && \
    echo "" && \
    echo "Listing proto/engine:" && \
    ls -la proto/engine/ 2>&1 || echo "ERROR: proto/engine/ directory not found!" && \
    echo "" && \
    echo "Listing proto/engine/v1:" && \
    ls -la proto/engine/v1/ 2>&1 || echo "ERROR: proto/engine/v1/ directory not found!" && \
    echo "" && \
    echo "Checking for engine.proto file:" && \
    ls -la proto/engine/v1/engine.proto 2>&1 || echo "ERROR: proto/engine/v1/engine.proto not found!" && \
    echo "=== End proto file verification ==="

# Create dummy source files for all workspace members to build dependencies
RUN echo "=== Creating dummy source files ===" && \
    mkdir -p services/engine-rust/engine-core/src && \
    echo "pub fn dummy() {}" > services/engine-rust/engine-core/src/lib.rs && \
    mkdir -p services/engine-rust/engine-server/src && \
    echo "fn main() {}" > services/engine-rust/engine-server/src/main.rs && \
    mkdir -p services/engine-rust/engine-proto/src && \
    echo "pub fn dummy() {}" > services/engine-rust/engine-proto/src/lib.rs && \
    mkdir -p services/engine-rust/games-tictactoe/src && \
    echo "pub fn dummy() {}" > services/engine-rust/games-tictactoe/src/lib.rs && \
    echo "Dummy source files created" && \
    echo "=== End dummy source files creation ==="

# Debug: Verify directory structure before dependency build
RUN echo "=== Directory structure before dependency build ===" && \
    echo "Current directory: $(pwd)" && \
    echo "Services directory structure:" && \
    find services/engine-rust -type f -name "*.toml" -o -name "build.rs" | sort && \
    echo "" && \
    echo "Proto directory structure:" && \
    find proto -type f -name "*.proto" | sort && \
    echo "=== End directory structure verification ==="

# Build dependencies only - this layer caches all dependency compilation
WORKDIR /workspace/services/engine-rust
RUN echo "=== Building dependencies ===" && \
    echo "Working directory: $(pwd)" && \
    echo "Verifying proto path from engine-proto perspective:" && \
    echo "CARGO_MANIFEST_DIR would be: $(pwd)/engine-proto" && \
    echo "Proto path should be: $(pwd)/engine-proto/../../../proto" && \
    ls -la ../../../proto/engine/v1/engine.proto 2>&1 || echo "ERROR: Cannot find proto from relative path!" && \
    echo "" && \
    echo "Starting cargo build with verbose output:" && \
    cargo build --release --bin engine-server -vv 2>&1 || true && \
    echo "=== Dependency build completed (errors allowed) ==="

# Now copy the actual source code
WORKDIR /workspace
COPY services/actor-rust/ services/actor-rust/
COPY services/engine-rust/ services/engine-rust/

# Debug: Verify source code was copied correctly
RUN echo "=== Verifying source code after copy ===" && \
    echo "Current directory: $(pwd)" && \
    echo "Engine-rust directory structure:" && \
    find services/engine-rust -type f \( -name "*.rs" -o -name "*.toml" -o -name "build.rs" \) | head -20 && \
    echo "" && \
    echo "Verifying proto files still exist:" && \
    ls -la proto/engine/v1/engine.proto 2>&1 || echo "ERROR: Proto file missing after source copy!" && \
    echo "=== End source code verification ==="

# Build the application with real source
WORKDIR /workspace/services/engine-rust
RUN echo "=== Building final application ===" && \
    echo "Working directory: $(pwd)" && \
    echo "Verifying proto path one more time:" && \
    ls -la ../../proto/engine/v1/engine.proto 2>&1 || echo "ERROR: Proto not accessible from build directory!" && \
    echo "" && \
    echo "Starting final cargo build with verbose output:" && \
    cargo build --release --bin engine-server -vv && \
    echo "=== Final build completed successfully ==="

# Runtime stage
# To pin with SHA256, run: docker pull debian:bookworm-slim && docker inspect debian:bookworm-slim --format='{{index .RepoDigests 0}}'
# Then use: FROM debian:bookworm-slim@sha256:<hash>
FROM debian:bookworm-slim

# Install curl for healthchecks and ca-certificates for HTTPS connections
RUN apt-get update \
    && apt-get install -y ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary
COPY --from=builder /workspace/services/engine-rust/target/release/engine-server /usr/local/bin/engine-server

# Create non-root user
RUN useradd -r -u 1000 engine

USER engine

EXPOSE 50051

ENTRYPOINT ["engine-server"]