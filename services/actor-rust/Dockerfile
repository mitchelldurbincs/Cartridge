# Build stage
FROM rust:1.90 AS builder

# Install protoc
RUN apt-get update && apt-get install -y protobuf-compiler && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy the repository structure needed for building the actor crate.
COPY proto/ proto/
COPY services/engine-rust/ services/engine-rust/
COPY services/actor-rust/ services/actor-rust/

WORKDIR /workspace/services/actor-rust

# Build the application
RUN cargo build --release

# Runtime stage
FROM debian:bookworm-slim

# Install ca-certificates for HTTPS connections
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy the binary
COPY --from=builder /workspace/services/actor-rust/target/release/actor /usr/local/bin/actor

# Create non-root user
RUN useradd -r -u 1000 actor

USER actor

ENTRYPOINT ["actor"]