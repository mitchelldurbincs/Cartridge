# Build stage
# To pin with SHA256, run: docker pull rust:1.90 && docker inspect rust:1.90 --format='{{index .RepoDigests 0}}'
# Then use: FROM rust:1.90@sha256:<hash>
FROM rust:1.90 AS builder

# Install protoc
RUN apt-get update && apt-get install -y protobuf-compiler && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy manifests first for dependency caching
# This layer will be cached unless Cargo.toml changes
COPY services/actor-rust/Cargo.toml services/actor-rust/Cargo.lock* services/actor-rust/
COPY services/actor-rust/build.rs services/actor-rust/
COPY proto/ proto/

# Create dummy source to build dependencies
RUN mkdir -p services/actor-rust/src && \
    echo "fn main() {}" > services/actor-rust/src/main.rs

# Build dependencies only - this layer caches all dependency compilation
WORKDIR /workspace/services/actor-rust
RUN cargo build --release || true

# Now copy the actual source code
WORKDIR /workspace
COPY services/engine-rust/ services/engine-rust/
COPY services/actor-rust/ services/actor-rust/

# Build the application with real source
WORKDIR /workspace/services/actor-rust
RUN cargo build --release

# Runtime stage
# To pin with SHA256, run: docker pull debian:bookworm-slim && docker inspect debian:bookworm-slim --format='{{index .RepoDigests 0}}'
# Then use: FROM debian:bookworm-slim@sha256:<hash>
FROM debian:bookworm-slim

# Install curl for healthchecks and ca-certificates for HTTPS connections
RUN apt-get update \
    && apt-get install -y ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary
COPY --from=builder /workspace/services/actor-rust/target/release/actor /usr/local/bin/actor

# Create non-root user
RUN useradd -r -u 1000 actor

USER actor

ENTRYPOINT ["actor"]