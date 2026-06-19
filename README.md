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
| `agent/` | CK-Kube Agent V1 inventory collector |
| `operators/` | Platform and guarded action operators |
| `internal/` | Shared process, gRPC, and controller startup scaffolding |
| `deploy/` | Compose, Helm, Kind, and ClickHouse assets |
| `tests/` | Contract, integration, end-to-end, performance, and chaos scaffolding |
| `scripts/` | Cross-platform build and development scripts |

## Local development

Prerequisites:

- Go 1.24 or newer
- Docker Engine with Docker Compose v2
- Kind
- Helm 3
- kubectl
- GNU Make

Start the complete local environment:

```text
make dev-up
```

This command:

1. Starts ClickHouse and Grafana with Docker Compose.
2. Creates the `kube-cost` Kind cluster when it does not exist.
3. Builds all service, agent, and operator development images.
4. Loads the images into Kind.
5. Installs or upgrades the platform and agent Helm charts.
6. Waits for Compose health checks and Kubernetes deployments.

Local endpoints:

| Component | Address | Credentials |
|---|---|---|
| Grafana | `http://localhost:3000` | `admin` / `admin` |
| ClickHouse HTTP | `http://localhost:8123` | `kube_cost` / `kube_cost` |
| ClickHouse native | `localhost:9000` | `kube_cost` / `kube_cost` |
| Kubernetes | context `kind-kube-cost` | local kubeconfig |

Grafana starts with the ClickHouse data source provisioned.

Remove the complete environment:

```text
make dev-down
```

Teardown deletes the Kind cluster, Helm releases, Docker Compose containers, and local ClickHouse/Grafana volumes.

On Windows, when GNU Make is unavailable, use:

```powershell
.\scripts\dev-up.ps1
.\scripts\dev-down.ps1
```

On Linux or macOS:

```sh
sh scripts/dev-up.sh
sh scripts/dev-down.sh
```

Protocol generation uses the repository-native Go generator; a system `protoc` installation is not required.

## Commands

```text
make dev-up
make dev-down
make build
make test
make proto
make compose-up
make kind-up
make helm-install
make helm-lint
make clickhouse-migrate
make clickhouse-benchmark
make clickhouse-integration-test
```

PowerShell equivalents are available under `scripts/`.

## Scope

The repository includes the Kubernetes inventory agent, ingestion transport,
ClickHouse inventory persistence, analytical schemas, deployment scaffolding,
and Cost Allocation Engine V1 for namespace cost, idle capacity, network,
control-plane, and system namespace cost classification, plus Optimization
Engine V1 for CPU p95 and memory p99 rightsizing recommendations, and a
Karpenter integration for NodePool, NodeClass, and NodeClaim scoring.
