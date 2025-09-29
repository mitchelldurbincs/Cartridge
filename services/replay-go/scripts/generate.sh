#!/bin/bash
set -e

# Generate protobuf files
protoc --go_out=pkg/proto --go_opt=paths=source_relative \
       --go-grpc_out=pkg/proto --go-grpc_opt=paths=source_relative \
       --proto_path=../../proto \
       replay/v1/replay.proto

echo "Generated protobuf files in pkg/proto/"