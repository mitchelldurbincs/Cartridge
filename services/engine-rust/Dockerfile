# Build stage
FROM rust:1.70 as builder

# Install protoc
RUN apt-get update && apt-get install -y protobuf-compiler

WORKDIR /app

# Copy manifests
COPY Cargo.toml Cargo.lock ./

# Copy all crate directories
COPY engine-core/ engine-core/
COPY engine-server/ engine-server/
COPY engine-proto/ engine-proto/
COPY games-tictactoe/ games-tictactoe/

# Build the application
RUN cargo build --release --bin engine-server

# Runtime stage
FROM debian:bookworm-slim

# Install ca-certificates for HTTPS connections
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy the binary
COPY --from=builder /app/target/release/engine-server /usr/local/bin/engine-server

# Create non-root user
RUN useradd -r -u 1000 engine

USER engine

EXPOSE 50051

ENTRYPOINT ["engine-server"]