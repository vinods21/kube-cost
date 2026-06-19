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
- Optimization Engine V1 heuristics for CPU and memory rightsizing.
- Karpenter snapshot and scoring API behind an opt-in integrations mode.
- Helm, Kind, Docker Compose, and local development scripts.

## Product gaps

| Gap | Current state | Product risk | Target capability |
|---|---|---|---|
| Control plane services | Gateway, identity, tenant, pricing, query, workflow, export, and audit mostly expose health only. | Users cannot operate the product through the documented APIs. | Implement authenticated tenant, cluster, query, pricing, workflow, export, and audit service surfaces. |
| API authentication and tenancy | Implemented HTTP APIs accept `tenant_id` directly from request parameters. | Tenant spoofing and data leakage risk. | Gateway-enforced OIDC identity, tenant authorization, and service-to-service identity. |
| Durable ingestion acknowledgement | Ingestion has historically advanced persisted sequence state after queue enqueue. | Agent may discard observations that have not reached durable storage. | Advance `persisted_through_sequence` only after persistence commits or durable raw archive write succeeds. |
| Replay and raw archive | No Kafka-compatible stream or object-storage raw envelope archive exists. | Corrections, schema replay, and disaster recovery are incomplete. | Durable stream plus raw accepted envelope archive and replay tooling. |
| Data lineage identity | Namespaced child records temporarily use namespace name where namespace UID is unavailable; container records lack deployment identity. | Namespace, workload, team, and project attribution can be incorrect. | Add/derive stable namespace and workload identity before allocation depends on child rows. |
| Billing-grade pricing | Allocation V1 uses static demo prices. | Cost reports cannot reconcile to provider invoices. | Provider catalogs, billing imports, discounts, commitments, credits, FX, residual cost, and reconciliation. |
| Query and quality APIs | General `/costs`, `/usage`, `/allocation`, `/data-quality`, and async query APIs are not implemented. | Product cannot expose auditable cost analysis beyond the narrow V1 namespace API. | Implement query service and quality diagnostics with freshness, completeness, lineage, and cardinality controls. |
| Recommendation workflow | Recommendation engine logs generated findings but does not persist or expose workflow APIs. | Users cannot review, approve, suppress, or measure recommendations. | Persist recommendation facts and implement list/detail/approve/reject/suppress/execute-request APIs. |
| Production deployment topology | Local deployment has ClickHouse and Grafana; Helm deploys many health-only services. | Local demos do not prove production readiness. | Production cell topology with ingress, PostgreSQL, stream, object storage, secrets, observability, backups, and DR. |
| Test depth | Active tests are mostly unit and contract checks. | Regressions in end-to-end product behavior can pass. | Add E2E, integration, replay, tenant isolation, migration, performance, and chaos test gates. |

## Implementation order

1. Make ingestion acknowledgements durable-aware.
2. Add tenant-safe gateway and cluster enrollment minimum.
3. Resolve namespace/workload lineage in agent, proto, and persistence.
4. Implement query and data-quality APIs for the current ClickHouse facts.
5. Persist and expose recommendation workflow state.
6. Add first provider pricing and billing import path.
7. Add replay/archive infrastructure and tests.
8. Expand production deployment and operational readiness checks.

## Compatibility policy

Each implementation step must preserve existing protobuf field numbers, API
paths, and ClickHouse column contracts unless a migration and compatibility note
are included. Tests must be added or updated with the change, and existing
contract tests must remain green.
