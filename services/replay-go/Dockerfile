# Build stage
FROM golang:1.22 AS builder

WORKDIR /workspace

# Copy the entire workspace to get access to orchestrator-go dependencies
COPY services/replay-go/ services/replay-go/
COPY services/orchestrator-go/internal/thirdparty/ services/orchestrator-go/internal/thirdparty/

# Set working directory to replay service
WORKDIR /workspace/services/replay-go

# Download dependencies
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /workspace/bin/replay-server ./cmd/server

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS connections
RUN apk --no-cache add ca-certificates

# Copy the binary
COPY --from=builder /workspace/bin/replay-server /usr/local/bin/replay-server

# Create non-root user
RUN adduser -D -u 1000 replay

USER replay

EXPOSE 8080

ENTRYPOINT ["replay-server"]