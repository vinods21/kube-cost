# Kubernetes Cost Intelligence Platform

Production-grade monorepo skeleton for the architecture defined in [docs/](docs/README.md).

## Stack

- Go 1.24+
- gRPC and Protocol Buffers
- ClickHouse
- controller-runtime
- Helm
- Docker Compose
- Kind

## Repository

| Path | Purpose |
|---|---|
| `proto/` | Versioned protobuf contracts |
| `services/` | Independently buildable platform services |
| `agent/` | In-cluster Kubernetes collection agent |
| `operators/` | Platform and guarded action operators |
| `internal/` | Shared process, gRPC, and controller startup scaffolding |
| `deploy/` | Compose, Helm, Kind, and ClickHouse assets |
| `tests/` | Contract, integration, end-to-end, performance, and chaos scaffolding |
| `scripts/` | Cross-platform build and development scripts |

## Prerequisites

Go 1.24 or newer is required. Protocol generation additionally requires `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc`. Deployment validation requires Docker, Helm, and Kind as appropriate.

## Commands

```text
make build
make test
make proto
make compose-up
make kind-up
make helm-lint
```

PowerShell equivalents are available under `scripts/`.

## Scope

This repository currently contains compile-time and deployment scaffolding only. Business logic, controllers, persistence behavior, allocation algorithms, and optimization algorithms are intentionally absent.
