#!/usr/bin/env sh
set -eu

command -v protoc >/dev/null 2>&1

mkdir -p proto/gen/go
PROTOC_GEN_GO="$(go tool -n protoc-gen-go)"
PROTOC_GEN_GO_GRPC="$(go tool -n protoc-gen-go-grpc)"
find proto/cost -name '*.proto' -print | sort | xargs protoc -I proto \
  --plugin="protoc-gen-go=${PROTOC_GEN_GO}" \
  --plugin="protoc-gen-go-grpc=${PROTOC_GEN_GO_GRPC}" \
  --go_out=proto/gen/go --go_opt=paths=source_relative \
  --go-grpc_out=proto/gen/go --go-grpc_opt=paths=source_relative
