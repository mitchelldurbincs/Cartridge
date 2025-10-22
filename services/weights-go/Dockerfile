FROM golang:1.22 AS builder

WORKDIR /workspace

# Copy module sources and dependencies used via local replaces.
COPY services/weights-go/ services/weights-go/
COPY services/orchestrator-go/internal/thirdparty/zerolog services/orchestrator-go/internal/thirdparty/zerolog

WORKDIR /workspace/services/weights-go

# Build the weights binary.
RUN CGO_ENABLED=0 go build -o /workspace/bin/weights ./cmd/server

FROM busybox:1.36 AS busybox

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=busybox /bin/busybox /busybox/busybox

COPY --from=builder /workspace/bin/weights /usr/local/bin/weights

EXPOSE 8081 9094

USER nonroot

ENTRYPOINT ["weights"]
