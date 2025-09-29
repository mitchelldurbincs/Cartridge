# Build stage
FROM golang:1.21 as builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o replay-server ./cmd/server

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS connections
RUN apk --no-cache add ca-certificates

# Copy the binary
COPY --from=builder /app/replay-server /usr/local/bin/replay-server

# Create non-root user
RUN adduser -D -u 1000 replay

USER replay

EXPOSE 8080

ENTRYPOINT ["replay-server"]