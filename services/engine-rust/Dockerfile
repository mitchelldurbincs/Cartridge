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

# Create dummy source files for all workspace members to build dependencies
RUN mkdir -p services/engine-rust/engine-core/src && \
    echo "pub fn dummy() {}" > services/engine-rust/engine-core/src/lib.rs && \
    mkdir -p services/engine-rust/engine-server/src && \
    echo "fn main() {}" > services/engine-rust/engine-server/src/main.rs && \
    mkdir -p services/engine-rust/engine-proto/src && \
    echo "pub fn dummy() {}" > services/engine-rust/engine-proto/src/lib.rs && \
    mkdir -p services/engine-rust/games-tictactoe/src && \
    echo "pub fn dummy() {}" > services/engine-rust/games-tictactoe/src/lib.rs

# Build dependencies only - this layer caches all dependency compilation
WORKDIR /workspace/services/engine-rust
RUN cargo build --release --bin engine-server || true

# Now copy the actual source code
WORKDIR /workspace
COPY services/actor-rust/ services/actor-rust/
COPY services/engine-rust/ services/engine-rust/

# Build the application with real source
WORKDIR /workspace/services/engine-rust
RUN cargo build --release --bin engine-server

# Runtime stage
# To pin with SHA256, run: docker pull debian:bookworm-slim && docker inspect debian:bookworm-slim --format='{{index .RepoDigests 0}}'
# Then use: FROM debian:bookworm-slim@sha256:<hash>
FROM debian:bookworm-slim

# Install only ca-certificates (needed for HTTPS connections)
RUN apt-get update \
    && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary
COPY --from=builder /workspace/services/engine-rust/target/release/engine-server /usr/local/bin/engine-server

# Create non-root user
RUN useradd -r -u 1000 engine

USER engine

EXPOSE 50051

ENTRYPOINT ["engine-server"]