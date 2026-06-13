#!/usr/bin/env sh
set -eu

command -v protoc >/dev/null 2>&1
command -v protoc-gen-go >/dev/null 2>&1
command -v protoc-gen-go-grpc >/dev/null 2>&1

mkdir -p proto/gen/go
find proto/cost -name '*.proto' -print | sort | xargs protoc -I proto \
  --go_out=proto/gen/go --go_opt=paths=source_relative \
  --go-grpc_out=proto/gen/go --go-grpc_opt=paths=source_relative
