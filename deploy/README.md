# Deployment

- `compose/` starts local ClickHouse and Grafana.
- `grafana/` contains local data source and dashboard provisioning.
- `kind/` defines a local multi-node Kubernetes cluster.
- `helm/kube-cost-platform/` deploys central services and operators.
- `helm/kube-cost-agent/` deploys the read-only in-cluster agent.
- `helm/local/` contains development resource overrides.
- `clickhouse/init/` contains bootstrap-only database and user assets.
- `clickhouse/migrations/` contains forward-only analytical schema migrations.

Use `make dev-up` and `make dev-down` as the supported complete lifecycle. Lower-level Compose, Kind, and Helm targets are available for focused development.
