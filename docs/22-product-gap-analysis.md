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
| Control plane services | Cluster registry, pricing imports, query reads, and recommendation workflow now expose minimal tenant-scoped V1 APIs through the gateway. Identity, tenant, export, and audit mostly expose health only. | Users cannot operate the full product lifecycle through the documented APIs. | Implement identity, tenant membership, export, audit, and richer workflow service surfaces. |
| API authentication and tenancy | Gateway resolves static bearer token mappings to trusted tenant headers and strips caller-supplied tenant headers before proxying public V1 HTTP APIs. Backend services still accept the tenant header directly, and OIDC/JWKS, tenant membership, and service-to-service identity are not implemented. | Direct backend exposure can still bypass tenant authority, and static token mapping is not production identity. | Add OIDC/JWKS authn, tenant membership checks, backend gateway-only enforcement, service-to-service identity, and network policy defaults. |
| Durable ingestion acknowledgement | Ingestion has historically advanced persisted sequence state after queue enqueue. | Agent may discard observations that have not reached durable storage. | Advance `persisted_through_sequence` only after persistence commits or durable raw archive write succeeds. |
| Replay and raw archive | Ingestion can write deterministic raw accepted batch files, externalize per-cluster sequence checkpoints, and inspect archived batches with a replay planning CLI. No Kafka-compatible stream or object-storage backend exists. | Corrections, schema replay, and disaster recovery are still incomplete beyond local raw capture and inspection. | Durable stream plus object-storage raw archive and filtered re-publication replay tooling. |
| Data lineage identity | Agent payloads now include namespace UID on namespaced child records and workload owner identity on container records; persistence falls back for older agents. A lineage normalizer can dry-run or apply append-only inventory replacement rows where namespace names were stored in `namespace_uid`. | Historical immutable metric and cost facts still require raw replay or derived fact rebuilds after inventory repair. | Add richer workload resolution beyond direct owner references and automate replay/rebuild workflows for affected immutable facts. |
| Billing-grade pricing | Pricing now imports tenant-scoped provider catalog intervals and billing charge lines; Allocation V1 still uses static demo prices. | Cost reports can retain provider source facts, but allocation does not yet consume effective rates or reconcile invoice totals. | Add effective-rate lookup, allocation integration, discounts, commitments, credits, FX, residual cost, and reconciliation. |
| Query and quality APIs | Query now exposes tenant-scoped `/api/v1/data-quality`, `/api/v1/usage`, `/api/v1/costs`, `/api/v1/allocation`, and recommendation read endpoints with bounded namespace/cluster grouping. Async query manifests, arbitrary label grouping, pagination cursors, and quality-enriched analytical responses are not implemented. | Product can expose initial synchronous analysis, but high-cardinality exploration and export-grade query workflows are still incomplete. | Add async query jobs, cursor pagination, allow-listed label grouping, result manifests, and quality annotations on analytical responses. |
| Recommendation workflow | Recommendation engine persists generated facts; query exposes list/detail reads; workflow records approve/reject/suppress/execute-request transitions and action audit rows. | Execution requests are recorded but not yet applied by an operator, so realized-savings verification is incomplete. | Add policy evaluation, executor handoff, verification, rollback tracking, and workflow state storage outside analytical ClickHouse. |
| Production deployment topology | Helm now supports per-component replicas, PDBs, topology spread, priority class, and optional gateway ingress; external PostgreSQL, stream, object storage, secrets management, observability, backups, and DR are still not packaged. | Local demos do not prove production readiness for stateful dependencies or cell operations. | Production cell topology with managed PostgreSQL, stream, object storage, secrets, observability, backups, and DR. |
| Test depth | A baseline `make compatibility-check` gate now runs all Go tests plus default and production-style Helm renders; ClickHouse integration and performance scripts exist separately. | Regressions in full end-to-end product behavior, dependency failure, replay, and scale can still pass unless the heavier gates are run. | Add automated E2E, replay-with-ClickHouse, tenant isolation, performance, and chaos gates in CI. |

## Implementation order

1. Make ingestion acknowledgements durable-aware.
2. Add tenant-safe gateway and cluster enrollment minimum.
3. Resolve namespace/workload lineage in agent, proto, and persistence.
4. Add backend gateway-only enforcement and service-to-service identity.
5. Add async query jobs, cursor pagination, and quality annotations.

## Compatibility policy

Each implementation step must preserve existing protobuf field numbers, API
paths, and ClickHouse column contracts unless a migration and compatibility note
are included. Tests must be added or updated with the change, and existing
contract tests must remain green.
