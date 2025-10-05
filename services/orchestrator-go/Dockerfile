FROM golang:1.22 AS builder

WORKDIR /workspace

# Copy source code for the orchestrator module
COPY services/orchestrator-go/ services/orchestrator-go/

WORKDIR /workspace/services/orchestrator-go

# Build the orchestrator binary
RUN CGO_ENABLED=0 go build -o /workspace/bin/orchestrator ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=builder /workspace/bin/orchestrator /usr/local/bin/orchestrator

EXPOSE 8080

USER nonroot

ENTRYPOINT ["orchestrator"]
