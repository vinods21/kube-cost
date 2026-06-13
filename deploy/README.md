# Deployment

- `compose/` starts local ClickHouse, PostgreSQL, and Kafka-compatible infrastructure.
- `kind/` defines a local multi-node Kubernetes cluster.
- `helm/kube-cost-platform/` deploys central services and operators.
- `helm/kube-cost-agent/` deploys the read-only in-cluster agent.
- `clickhouse/init/` contains bootstrap-only database assets.
