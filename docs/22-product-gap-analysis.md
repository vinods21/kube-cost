# Product Gap Analysis

## Scope

This document tracks the gap between the intended Kubernetes cost intelligence
platform and the current implementation. It is a delivery checklist, not a
replacement for the architecture documents.

## Current implementation baseline

- Agent inventory and metrics collection.
- Agent-to-ingestion gRPC stream with mTLS identity validation.
- In-memory ingestion queue and ClickHouse persistence for inventory and metric
  facts.
- ClickHouse local schema, serving views, and benchmark fixtures.
- Namespace Cost V1 HTTP API using static node, control-plane, and network
  pricing.
- Optimization Engine V1 heuristics for CPU and memory rightsizing with
  persisted recommendation facts.
- Karpenter snapshot and scoring API behind an opt-in integrations mode.
- Helm, Kind, Docker Compose, and local development scripts.

## Product gaps

| Gap | Current state | Product risk | Target capability |
|---|---|---|---|
| Control plane services | Cluster registry, pricing imports/lookups, query reads, recommendation workflow, and in-process export job manifests now expose minimal tenant-scoped V1 APIs through the gateway. Identity, tenant, and audit mostly expose health only. | Users cannot operate the full product lifecycle through the documented APIs. | Implement identity, tenant membership, audit, durable export delivery, and richer workflow service surfaces. |
| API authentication and tenancy | Gateway resolves static bearer token mappings to trusted tenant headers, strips caller-supplied tenant headers, and can enforce gateway-only shared secret plus HMAC-signed backend requests on fronted backends. Helm can render opt-in NetworkPolicy defaults for gateway/backend, agent/ingestion, and operator metrics ingress. OIDC/JWKS, tenant membership, and certificate-backed workload identity are not implemented. | Static token and shared-secret/HMAC mapping is still bootstrap identity, not production authn/authz. | Add OIDC/JWKS authn, tenant membership checks, and certificate-backed service identity. |
| Durable ingestion acknowledgement | Ingestion advances `persisted_through_sequence` only after accepted batches are archived when enabled, acquired queue leases are committed by the persistence worker, and the external sequence checkpoint is saved when configured. | The remaining risk is deployment-specific durability of the chosen archive/checkpoint backends, not premature acknowledgement in the stream protocol. | Back the archive and checkpoint stores with production object storage or durable stream infrastructure. |
| Replay and raw archive | Ingestion can write deterministic raw accepted batch files, externalize per-cluster sequence checkpoints, and inspect archived batches with a replay planning CLI. No Kafka-compatible stream or object-storage backend exists. | Corrections, schema replay, and disaster recovery are still incomplete beyond local raw capture and inspection. | Durable stream plus object-storage raw archive and filtered re-publication replay tooling. |
| Data lineage identity | Agent payloads now include namespace UID on namespaced child records and workload owner identity on container records; persistence falls back for older agents. A lineage normalizer can dry-run or apply append-only inventory replacement rows where namespace names were stored in `namespace_uid`. | Historical immutable metric and cost facts still require raw replay or derived fact rebuilds after inventory repair. | Add richer workload resolution beyond direct owner references and automate replay/rebuild workflows for affected immutable facts. |
| Billing-grade pricing | Pricing now imports tenant-scoped provider catalog intervals and billing charge lines and exposes effective catalog price lookup with account/SKU wildcard fallback. Allocation V1 still uses static demo prices. | Cost reports can retain and resolve provider source facts, but allocation does not yet consume effective rates or reconcile invoice totals. | Add allocation integration, discounts, commitments, credits, FX, residual cost, and reconciliation. |
| Query and quality APIs | Query now exposes tenant-scoped `/api/v1/data-quality`, `/api/v1/usage`, `/api/v1/costs`, `/api/v1/allocation`, recommendation reads, bounded namespace/cluster/promoted-dimension grouping, opaque cursors, optional freshness/coverage summaries, in-process async query jobs with checksummed inline manifests, and in-process export job manifests. Durable async job/export storage, arbitrary raw label grouping, and object-storage result delivery are not implemented. | Product can expose initial synchronous analysis and short-lived async/export jobs, but high-cardinality exploration and export-grade workflows are still incomplete. | Add durable async job/export storage, cardinality-budgeted raw label grouping, and object-storage result delivery. |
| Recommendation workflow | Recommendation engine persists generated facts; query exposes list/detail reads; workflow records approve/reject/suppress/execute-request transitions, action audit rows, and structured execution handoff metadata. | Execution handoffs are recorded but not yet applied by an operator, so realized-savings verification is incomplete. | Add executor application, policy evaluation, verification, rollback tracking, and workflow state storage outside analytical ClickHouse. |
| Production deployment topology | Helm now supports per-component replicas, PDBs, topology spread, priority class, and optional gateway ingress; external PostgreSQL, stream, object storage, secrets management, observability, backups, and DR are still not packaged. | Local demos do not prove production readiness for stateful dependencies or cell operations. | Production cell topology with managed PostgreSQL, stream, object storage, secrets, observability, backups, and DR. |
| Test depth | A baseline `make compatibility-check` gate now runs all Go tests plus default and production-style Helm renders; ClickHouse integration and performance scripts exist separately. | Regressions in full end-to-end product behavior, dependency failure, replay, and scale can still pass unless the heavier gates are run. | Add automated E2E, replay-with-ClickHouse, tenant isolation, performance, and chaos gates in CI. |

## Implementation order

1. Make ingestion acknowledgements durable-aware.
2. Add tenant-safe gateway and cluster enrollment minimum.
3. Resolve namespace/workload lineage in agent, proto, and persistence.
4. Add service-to-service identity.
5. Add durable async query storage, cardinality-budgeted raw label grouping, and export/object-storage result manifests.

## Compatibility policy

Each implementation step must preserve existing protobuf field numbers, API
paths, and ClickHouse column contracts unless a migration and compatibility note
are included. Tests must be added or updated with the change, and existing
contract tests must remain green.
