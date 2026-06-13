# Component Architecture

## Control plane

| Component | Responsibility | State |
|---|---|---|
| API Gateway/BFF | Authentication, rate limits, public API routing, response composition | Stateless |
| Identity and Authorization | Tenant membership, roles, policy evaluation | PostgreSQL/cache |
| Tenant Service | Organizations, accounts, projects, preferences, residency | PostgreSQL |
| Cluster Registry | Registration, credentials, capabilities, heartbeat, versions | PostgreSQL |
| Policy Service | Allocation, idle, shared-cost, recommendation, and automation policies | PostgreSQL + version events |
| Integration Service | Cloud accounts, billing exports, notifications, secret references | PostgreSQL/secret manager |
| Workflow Service | Recommendation approval and action lifecycle | PostgreSQL |
| Audit Service | Append-only administrative and access audit | PostgreSQL/object archive |

## Data plane

| Component | Responsibility | Interface |
|---|---|---|
| Agent Gateway | mTLS termination, admission control, sequencing, acknowledgement | gRPC streaming |
| Event Validator | Schema validation, size limits, tenant/cluster binding | Stream processor |
| Normalizer | Canonical units and resource identities | Kafka consume/produce |
| Metadata Resolver | Owner chains, namespace/team mappings, temporal labels | Stream/table joins |
| Pricing Service | Catalog, negotiated rates, commitments, FX, effective price lookup | gRPC + stream |
| Allocation Engine | Direct, idle, shared, and overhead allocation | Batch/stream jobs |
| Recommendation Engine | Rightsizing, scheduling, node, commitment, and waste analysis | Scheduled jobs |
| Query Service | Cost, usage, allocation, quality, and recommendation reads | gRPC/internal |
| Export Service | Asynchronous CSV/Parquet reports and billing exports | Jobs/object storage |

## Edge plane

| Agent module | Responsibility |
|---|---|
| Discovery | Watches nodes, namespaces, pods, controllers, PV/PVC, services, and selected CRDs |
| Metrics | Reads Metrics API and optional Prometheus-compatible sources |
| Identity | Resolves UID-based object identity and workload owner |
| Sampler | Builds fixed-window usage observations |
| Buffer | Durable local WAL, batching, retry, and quota management |
| Transport | Authenticated bidirectional stream and configuration updates |
| Diagnostics | Health, lag, dropped-field, and cardinality metrics |

## Component interaction rules

- Services own their transactional data and expose contracts instead of shared writes.
- Analytical components MAY read shared ClickHouse facts but MUST not mutate another component's source facts.
- PostgreSQL cross-service joins are prohibited in runtime paths.
- Kafka topics are integration contracts, not internal database tables.
- Configuration changes emit versioned events for deterministic recomputation.
- Long-running exports and recomputations are asynchronous jobs with idempotency keys.

## Observability

All components emit OpenTelemetry traces, RED metrics, structured logs, build version, tenant-safe correlation IDs, and dependency health. Cardinality controls prohibit raw pod UID, container ID, or arbitrary label values in metrics.
