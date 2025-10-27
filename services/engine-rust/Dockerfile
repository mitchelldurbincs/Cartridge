# Base stage with build tools
# To pin with SHA256, run: docker pull rust:1.90 && docker inspect rust:1.90 --format='{{index .RepoDigests 0}}'
# Then use: FROM rust:1.90@sha256:<hash>
FROM rust:1.90 AS base

# Install protoc and build tools
RUN apt-get update && apt-get install -y protobuf-compiler && rm -rf /var/lib/apt/lists/*
RUN cargo install sccache --version ^0.7
RUN cargo install cargo-chef --version ^0.1
ENV RUSTC_WRAPPER=sccache SCCACHE_DIR=/sccache

# Planner stage to generate cargo-chef recipe
FROM base AS planner
WORKDIR /workspace
COPY services/engine-rust/ services/engine-rust/
COPY services/actor-rust/ services/actor-rust/
COPY proto/ proto/
WORKDIR /workspace/services/engine-rust
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=$SCCACHE_DIR,sharing=locked \
    cargo chef prepare --recipe-path recipe.json

# Builder stage
FROM base AS builder
WORKDIR /workspace

# Copy proto files needed for build
COPY proto/ proto/

# Build dependencies using cargo-chef
WORKDIR /workspace/services/engine-rust
COPY --from=planner /workspace/services/engine-rust/recipe.json recipe.json
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=$SCCACHE_DIR,sharing=locked \
    cargo chef cook --release --recipe-path recipe.json

# Copy actual source code
WORKDIR /workspace
COPY services/actor-rust/ services/actor-rust/
COPY services/engine-rust/ services/engine-rust/

# Build the application with real source
WORKDIR /workspace/services/engine-rust
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=$SCCACHE_DIR,sharing=locked \
    cargo build --release --bin engine-server

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