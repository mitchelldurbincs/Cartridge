# Build stage
FROM rust:1.90 AS builder

# Install protoc
RUN apt-get update && apt-get install -y protobuf-compiler && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy the repository structure needed for the workspace build.
COPY proto/ proto/
COPY services/actor-rust/ services/actor-rust/
COPY services/engine-rust/ services/engine-rust/

WORKDIR /workspace/services/engine-rust

# Build the application
RUN cargo build --release --bin engine-server

# Runtime stage
FROM debian:bookworm-slim

# Install ca-certificates and curl for runtime diagnostics
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