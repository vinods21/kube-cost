# Deployment Architecture

## Environments

- Local development: single-node dependencies and synthetic data.
- Integration: ephemeral per-change environment for contracts and migrations.
- Staging: production topology at reduced scale with canary tenants/clusters.
- Production: regional cells with independent failure domains.

## Regional cell

Each cell contains API ingress, control services, ingestion gateways, Kafka-compatible brokers, PostgreSQL HA, ClickHouse cluster, object storage access, caches, workers, observability collectors, and secret/KMS integration. Global services provide identity federation, tenant routing, artifact distribution, and sanitized fleet health.

Tenant data is pinned to a home region. Cross-region replication follows residency policy and does not make the global plane a data bypass.

## Kubernetes topology

- Separate namespaces and node pools for ingress, control, ingestion, analytics workers, and stateful data.
- Pod anti-affinity/topology spread across zones.
- PodDisruptionBudgets sized to preserve quorum and serving capacity.
- Priority classes protect ingestion and stateful recovery.
- NetworkPolicies default deny.
- Workload identity replaces static cloud credentials.

## Stateful services

- PostgreSQL: managed HA preferred, point-in-time recovery, tested logical backups.
- ClickHouse: replicated shards across zones, keeper quorum, object-storage backups.
- Kafka: three or more brokers across zones, replication factor three, min in-sync replicas two.
- Object storage: versioning, lifecycle, checksum manifests, regional policy.

## Delivery

GitOps promotes signed immutable images and charts through environments. Database migrations are separate pre-deployment stages. Progressive delivery starts with internal/canary tenants, monitors SLO and data parity, then expands. Feature flags are tenant-aware and time-bounded.

## Scaling

- Gateway scales on active streams, bytes, acknowledgement latency, and CPU.
- Stream processors scale on partition lag and throughput.
- Query services scale on concurrency and latency.
- Workers scale on queued scope-hours and completion deadlines.
- Stateful capacity planning uses disk growth, merge/compaction headroom, and recovery bandwidth.

## Disaster recovery

- Control state: cross-region backup and documented restore into a replacement cell.
- Analytical state: ClickHouse backups plus raw event replay from object storage.
- Stream loss: brokers are not the sole archive.
- Quarterly restore tests validate RPO/RTO and contract compatibility.

## Platform security

Artifacts are signed and scanned; SBOMs are retained. Admission policy requires approved registries, non-root execution, read-only root filesystems where possible, resource limits, seccomp, and dropped capabilities. Administrative access is just-in-time and audited.

## SLO operations

Use burn-rate alerts for availability and freshness. Capacity alerts precede disk, partition, and WAL exhaustion. Runbooks cover ingestion lag, ClickHouse merge pressure, billing import failure, price gaps, tenant query abuse, certificate rotation, and regional failover.
