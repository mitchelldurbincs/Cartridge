# Consolidated Dockerfile for Rust services (actor-rust, engine-rust)
# Usage:
#   docker build -f dockerfiles/rust.Dockerfile --build-arg SERVICE_NAME=actor-rust --build-arg BINARY_NAME=actor --build-arg USER_NAME=actor .
#   docker build -f dockerfiles/rust.Dockerfile --build-arg SERVICE_NAME=engine-rust --build-arg BINARY_NAME=engine-server --build-arg USER_NAME=engine .

ARG SERVICE_NAME
ARG BINARY_NAME
ARG USER_NAME

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
ARG SERVICE_NAME
WORKDIR /workspace
COPY services/actor-rust/ services/actor-rust/
COPY services/engine-rust/ services/engine-rust/
COPY proto/ proto/
WORKDIR /workspace/services/${SERVICE_NAME}
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=$SCCACHE_DIR,sharing=locked \
    cargo chef prepare --recipe-path recipe.json

# Builder stage
FROM base AS builder
ARG SERVICE_NAME
ARG BINARY_NAME
WORKDIR /workspace

# Copy proto files and both Rust services (for shared dependencies like cartridge-observability)
COPY proto/ proto/
COPY services/engine-rust/ services/engine-rust/
COPY services/actor-rust/ services/actor-rust/

# Build dependencies using cargo-chef
WORKDIR /workspace/services/${SERVICE_NAME}
COPY --from=planner /workspace/services/${SERVICE_NAME}/recipe.json recipe.json
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=$SCCACHE_DIR,sharing=locked \
    cargo chef cook --release --recipe-path recipe.json

# Build the application with real source
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=$SCCACHE_DIR,sharing=locked \
    cargo build --release $([ "${BINARY_NAME}" != "$(basename ${SERVICE_NAME})" ] && echo "--bin ${BINARY_NAME}" || echo "")

# Runtime stage
# To pin with SHA256, run: docker pull debian:bookworm-slim && docker inspect debian:bookworm-slim --format='{{index .RepoDigests 0}}'
# Then use: FROM debian:bookworm-slim@sha256:<hash>
FROM debian:bookworm-slim
ARG SERVICE_NAME
ARG BINARY_NAME
ARG USER_NAME

# Install curl for healthchecks and ca-certificates for HTTPS connections
RUN apt-get update \
    && apt-get install -y ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary
COPY --from=builder /workspace/services/${SERVICE_NAME}/target/release/${BINARY_NAME} /usr/local/bin/${BINARY_NAME}

# Create non-root user
RUN useradd -r -u 1000 ${USER_NAME}

# Create wrapper script with better logging (ARGs not available at runtime)
RUN printf '#!/bin/sh\nset -e\necho "Starting %s..."\nexec /usr/local/bin/%s "$@"\n' "${BINARY_NAME}" "${BINARY_NAME}" > /entrypoint.sh && \
    chmod +x /entrypoint.sh

USER ${USER_NAME}

ENTRYPOINT ["/entrypoint.sh"]
