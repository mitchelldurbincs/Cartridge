# Consolidated Dockerfile for Go services (orchestrator-go, weights-go)
# Usage:
#   docker build -f dockerfiles/go.Dockerfile --build-arg SERVICE_NAME=orchestrator-go --build-arg BINARY_NAME=orchestrator --build-arg BUILD_HEALTHCHECK=true .
#   docker build -f dockerfiles/go.Dockerfile --build-arg SERVICE_NAME=weights-go --build-arg BINARY_NAME=weights --build-arg BUILD_HEALTHCHECK=false .

ARG SERVICE_NAME
ARG BINARY_NAME
ARG BUILD_HEALTHCHECK=false

FROM golang:1.22 AS builder

ARG SERVICE_NAME
ARG BINARY_NAME
ARG BUILD_HEALTHCHECK

WORKDIR /workspace

# Copy service source and shared dependencies
COPY services/${SERVICE_NAME}/ services/${SERVICE_NAME}/
COPY services/orchestrator-go/internal/thirdparty/ services/orchestrator-go/internal/thirdparty/

WORKDIR /workspace/services/${SERVICE_NAME}

# Build the main binary
RUN CGO_ENABLED=0 go build -o /workspace/bin/${BINARY_NAME} ./cmd/server

# Conditionally build healthcheck binary (only for orchestrator)
RUN if [ "$BUILD_HEALTHCHECK" = "true" ]; then \
        CGO_ENABLED=0 go build -o /workspace/bin/${BINARY_NAME}-healthcheck ./cmd/healthcheck; \
    fi

# Create entrypoint wrapper script
RUN printf '#!/bin/sh\nexec /usr/local/bin/%s "$@"\n' "${BINARY_NAME}" > /workspace/bin/entrypoint.sh && \
    chmod +x /workspace/bin/entrypoint.sh

FROM busybox:1.36 AS busybox

FROM gcr.io/distroless/base-debian12:nonroot

ARG BINARY_NAME
ARG BUILD_HEALTHCHECK

COPY --from=busybox /bin/busybox /busybox/busybox

# Copy all binaries (includes main binary and optional healthcheck)
COPY --from=builder /workspace/bin/${BINARY_NAME}* /usr/local/bin/
COPY --from=builder /workspace/bin/entrypoint.sh /entrypoint.sh

USER nonroot

ENTRYPOINT ["/busybox/busybox", "sh", "/entrypoint.sh"]
