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
| Control plane services | Cluster registry now has a minimal tenant-scoped enrollment API; gateway, identity, tenant, pricing, query, workflow, export, and audit mostly expose health only. | Users cannot operate the full product through the documented APIs. | Implement authenticated tenant, query, pricing, workflow, export, and audit service surfaces. |
| API authentication and tenancy | Cluster registry uses a gateway-style `X-Kube-Cost-Tenant-ID` tenant context; other implemented HTTP APIs still accept `tenant_id` directly from request parameters. | Tenant spoofing and data leakage risk until a real gateway verifies identity and injects tenant context. | Gateway-enforced OIDC identity, tenant authorization, and service-to-service identity. |
| Durable ingestion acknowledgement | Ingestion has historically advanced persisted sequence state after queue enqueue. | Agent may discard observations that have not reached durable storage. | Advance `persisted_through_sequence` only after persistence commits or durable raw archive write succeeds. |
| Replay and raw archive | Ingestion can write deterministic raw accepted batch files, externalize per-cluster sequence checkpoints, and inspect archived batches with a replay planning CLI. No Kafka-compatible stream or object-storage backend exists. | Corrections, schema replay, and disaster recovery are still incomplete beyond local raw capture and inspection. | Durable stream plus object-storage raw archive and filtered re-publication replay tooling. |
| Data lineage identity | Agent payloads now include namespace UID on namespaced child records and workload owner identity on container records; persistence falls back for older agents. | Historical rows and mixed-version agents may still carry namespace names in `namespace_uid`. | Add a normalizer/backfill path and richer workload resolution beyond direct owner references. |
| Billing-grade pricing | Pricing now imports tenant-scoped provider catalog intervals and billing charge lines; Allocation V1 still uses static demo prices. | Cost reports can retain provider source facts, but allocation does not yet consume effective rates or reconcile invoice totals. | Add effective-rate lookup, allocation integration, discounts, commitments, credits, FX, residual cost, and reconciliation. |
| Query and quality APIs | Query now exposes tenant-scoped `/api/v1/data-quality`, `/api/v1/usage`, `/api/v1/costs`, `/api/v1/allocation`, and recommendation read endpoints with bounded namespace/cluster grouping. Async query manifests, arbitrary label grouping, pagination cursors, and quality-enriched analytical responses are not implemented. | Product can expose initial synchronous analysis, but high-cardinality exploration and export-grade query workflows are still incomplete. | Add async query jobs, cursor pagination, allow-listed label grouping, result manifests, and quality annotations on analytical responses. |
| Recommendation workflow | Recommendation engine persists generated facts; query exposes list/detail reads; workflow records approve/reject/suppress/execute-request transitions and action audit rows. | Execution requests are recorded but not yet applied by an operator, so realized-savings verification is incomplete. | Add policy evaluation, executor handoff, verification, rollback tracking, and workflow state storage outside analytical ClickHouse. |
| Production deployment topology | Helm now supports per-component replicas, PDBs, topology spread, priority class, and optional gateway ingress; external PostgreSQL, stream, object storage, secrets management, observability, backups, and DR are still not packaged. | Local demos do not prove production readiness for stateful dependencies or cell operations. | Production cell topology with managed PostgreSQL, stream, object storage, secrets, observability, backups, and DR. |
| Test depth | Active tests are mostly unit and contract checks. | Regressions in end-to-end product behavior can pass. | Add E2E, integration, replay, tenant isolation, migration, performance, and chaos test gates. |

## Implementation order

1. Make ingestion acknowledgements durable-aware.
2. Add tenant-safe gateway and cluster enrollment minimum.
3. Resolve namespace/workload lineage in agent, proto, and persistence.
4. Add tenant-safe auth gateway enforcement for service entrypoints.
5. Add async query jobs, cursor pagination, and quality annotations.

## Compatibility policy

Each implementation step must preserve existing protobuf field numbers, API
paths, and ClickHouse column contracts unless a migration and compatibility note
are included. Tests must be added or updated with the change, and existing
contract tests must remain green.
