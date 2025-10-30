# Consolidated Dockerfile for Go services (orchestrator-go, weights-go)
# Usage:
#   docker build -f dockerfiles/go.Dockerfile --build-arg SERVICE_NAME=orchestrator-go --build-arg BINARY_NAME=orchestrator --build-arg BUILD_HEALTHCHECK=true .
#   docker build -f dockerfiles/go.Dockerfile --build-arg SERVICE_NAME=weights-go --build-arg BINARY_NAME=weights --build-arg BUILD_HEALTHCHECK=false .

ARG SERVICE_NAME
ARG BINARY_NAME
ARG BUILD_HEALTHCHECK=false

FROM golang:1.24 AS builder

ARG SERVICE_NAME
ARG BINARY_NAME
ARG BUILD_HEALTHCHECK

WORKDIR /workspace

# Copy service source and shared dependencies
COPY services/${SERVICE_NAME}/ services/${SERVICE_NAME}/
COPY services/orchestrator-go/internal/thirdparty/ services/orchestrator-go/internal/thirdparty/

WORKDIR /workspace/services/${SERVICE_NAME}

# Build the main binary with explicit ldflags for static linking
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o /workspace/bin/${BINARY_NAME} ./cmd/server

# Conditionally build healthcheck binary (only for orchestrator)
RUN if [ "$BUILD_HEALTHCHECK" = "true" ]; then \
        CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o /workspace/bin/${BINARY_NAME}-healthcheck ./cmd/healthcheck; \
    fi

FROM alpine:3.19 AS busybox

# Use alpine for better debugging and logging support
FROM alpine:3.19

ARG BINARY_NAME
ARG BUILD_HEALTHCHECK

# Install ca-certificates for HTTPS and curl for healthchecks
RUN apk add --no-cache ca-certificates curl

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Copy binaries and create a runtime-friendly entrypoint
COPY --from=builder /workspace/bin/${BINARY_NAME} /usr/local/bin/${BINARY_NAME}
COPY --from=builder /workspace/bin/${BINARY_NAME}-healthcheck /usr/local/bin/ 2>/dev/null || true

# Create an entrypoint script that will work at runtime
RUN printf '#!/bin/sh\nset -e\necho "Starting %s..."\nexec /usr/local/bin/%s "$@"\n' "${BINARY_NAME}" "${BINARY_NAME}" > /entrypoint.sh && \
    chmod +x /entrypoint.sh

USER appuser

ENTRYPOINT ["/entrypoint.sh"]
