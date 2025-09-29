# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Generate protobuf code (requires buf to be available)
# Note: In production, you'd want to copy pre-generated proto files
# or install buf in the build stage

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/actor cmd/actor/main.go

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 actor && \
    adduser -D -s /bin/sh -u 1000 -G actor actor

WORKDIR /home/actor

# Copy binary from builder
COPY --from=builder /app/bin/actor .

# Change ownership
RUN chown actor:actor actor

# Switch to non-root user
USER actor

# Expose no ports (client-only service)

# Default environment variables
ENV ACTOR_ENGINE_ADDR=engine:50051
ENV ACTOR_REPLAY_ADDR=replay:8080
ENV ACTOR_ENV_ID=tictactoe
ENV ACTOR_LOG_LEVEL=info

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD pgrep actor || exit 1

# Run the actor
ENTRYPOINT ["./actor"]