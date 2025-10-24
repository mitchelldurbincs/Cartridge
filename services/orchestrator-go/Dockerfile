# To update digests, run:
# docker pull golang:1.22 && docker inspect golang:1.22 --format='{{index .RepoDigests 0}}'
# docker pull busybox:1.36 && docker inspect busybox:1.36 --format='{{index .RepoDigests 0}}'
# docker pull gcr.io/distroless/base-debian12:nonroot && docker inspect gcr.io/distroless/base-debian12:nonroot --format='{{index .RepoDigests 0}}'
FROM golang:1.22@sha256:ad20ca6242864c29171744bb7f83e2da7cdbf900f2f31a14e2d3c69aa48e185a AS builder

WORKDIR /workspace

# Copy dependency files first for better caching
COPY services/orchestrator-go/go.mod services/orchestrator-go/go.sum services/orchestrator-go/
WORKDIR /workspace/services/orchestrator-go
RUN go mod download

# Copy source code for the orchestrator module
COPY services/orchestrator-go/ /workspace/services/orchestrator-go/

# Build the orchestrator binary
RUN CGO_ENABLED=0 go build -o /workspace/bin/orchestrator ./cmd/server && \
    CGO_ENABLED=0 go build -o /workspace/bin/orchestrator-healthcheck ./cmd/healthcheck

FROM busybox:1.36@sha256:2919d0172f7524b2d8df9e50066a682669e6d170ac0f6a49676d54358fe970b5 AS busybox

FROM gcr.io/distroless/base-debian12:nonroot@sha256:6bcc2aef803bc4b42c5b7ec0e84feda5c431d0a65edbe27d411f4ac88a929bd1

COPY --from=busybox /bin/busybox /busybox/busybox

COPY --from=builder /workspace/bin/orchestrator /usr/local/bin/orchestrator
COPY --from=builder /workspace/bin/orchestrator-healthcheck /usr/local/bin/orchestrator-healthcheck

EXPOSE 8080

USER nonroot

ENTRYPOINT ["orchestrator"]
