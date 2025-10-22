FROM golang:1.22 AS builder

WORKDIR /workspace

# Copy source code for the orchestrator module
COPY services/orchestrator-go/ services/orchestrator-go/

WORKDIR /workspace/services/orchestrator-go

# Build the orchestrator binary
RUN CGO_ENABLED=0 go build -o /workspace/bin/orchestrator ./cmd/server && \
    CGO_ENABLED=0 go build -o /workspace/bin/orchestrator-healthcheck ./cmd/healthcheck

FROM busybox:1.36 AS busybox

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=busybox /bin/busybox /busybox/busybox

COPY --from=builder /workspace/bin/orchestrator /usr/local/bin/orchestrator
COPY --from=builder /workspace/bin/orchestrator-healthcheck /usr/local/bin/orchestrator-healthcheck

EXPOSE 8080

USER nonroot

ENTRYPOINT ["orchestrator"]
