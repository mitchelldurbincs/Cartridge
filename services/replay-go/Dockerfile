# Build stage
FROM golang:1.24 AS builder

# Install protoc and Go protobuf plugins
RUN apt-get update && apt-get install -y protobuf-compiler && rm -rf /var/lib/apt/lists/*
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

WORKDIR /workspace

# Copy proto files and replay service
COPY proto/ proto/
COPY services/replay-go/ services/replay-go/
COPY services/orchestrator-go/internal/thirdparty/ services/orchestrator-go/internal/thirdparty/

# Set working directory to replay service
WORKDIR /workspace/services/replay-go

# Generate protobuf code
RUN mkdir -p pkg/proto && \
    protoc --go_out=pkg/proto --go_opt=paths=source_relative \
           --go-grpc_out=pkg/proto --go-grpc_opt=paths=source_relative \
           --proto_path=../../proto \
           replay/v1/replay.proto

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