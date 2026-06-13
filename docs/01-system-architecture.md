# System Architecture

## Scope

The platform converts cluster observations, cloud prices, billing data, and allocation policy into queryable cost facts and explainable optimization recommendations.

## Context

Actors:

- Platform administrators configure tenants, clusters, policies, budgets, and integrations.
- FinOps users analyze spend, allocation, trends, and savings opportunities.
- Engineering teams inspect workload cost and recommendation evidence.
- Cluster agents export metadata, resource usage, and lifecycle events.
- Cloud adapters import price catalogs, billing exports, commitment coverage, and account metadata.
- Automation systems consume alerts, reports, and approved actions.

## Logical architecture

1. **Edge collection plane**: per-cluster agent discovers resources, samples metrics, enriches identity, buffers, and streams immutable observation envelopes.
2. **Ingestion plane**: authenticates agents, validates contracts, deduplicates, persists raw envelopes, and publishes normalized events.
3. **Enrichment plane**: joins observations with workload ownership, node/provider identity, prices, discounts, commitments, and allocation policy.
4. **Analytics plane**: materializes resource usage, cost, allocation, efficiency, and recommendation facts in ClickHouse.
5. **Control plane**: manages tenants, cluster registration, policy, integrations, workflows, RBAC, and audit logs in PostgreSQL.
6. **Experience plane**: exposes query APIs, exports, alerts, dashboards, and recommendation workflows.

## Primary data flow

`Agent -> Ingestion Gateway -> Durable Stream -> Normalizers -> Raw Object Archive -> ClickHouse facts -> Query API`

Parallel flows:

- `Cloud billing/catalog -> Provider adapters -> Durable Stream -> Pricing and billing facts`
- `Policy/configuration -> PostgreSQL -> versioned policy events -> allocation/recommendation jobs`
- `Recommendation engine -> recommendation store -> approval workflow -> optional operator action`

## Architectural invariants

- Every record is scoped by `tenant_id`; cluster data also carries stable `cluster_id`.
- Observation events are immutable and idempotent.
- Derived facts retain lineage to observation, price, FX, and policy versions.
- Event time and ingestion time are both retained.
- Reprocessing creates a new computation version; it never silently overwrites lineage.
- Missing data is represented as quality state, not converted to zero.
- Agent credentials cannot query tenant data or mutate platform policy.
- Recommendations do not execute until policy and approval gates permit them.

## Availability and scale

- Edge collection continues through control-plane outages using bounded disk spooling.
- Ingestion is horizontally scalable and partitioned by tenant and cluster.
- Analytics jobs are replayable from Kafka and object storage.
- Query workloads are isolated from ingestion through ClickHouse replicas and workload settings.
- Target initial scale: 10,000 clusters, 5 million pods, 1-minute samples, 13 months hot analytical retention, configurable cold archive.

## Security and compliance

- mTLS workload identity for agents; OIDC for users and service accounts.
- Encryption in transit and at rest; tenant-scoped envelope encryption for sensitive integration credentials.
- Least-privilege Kubernetes RBAC and cloud IAM.
- Immutable audit events for configuration, access, approvals, and action execution.
- Configurable metadata allow/deny lists to prevent secret or sensitive label collection.
- Regional data residency is enforced at routing and storage boundaries.

## Reliability objectives

| Capability | Objective |
|---|---|
| Agent ingestion availability | 99.95% monthly |
| Query API availability | 99.9% monthly |
| Freshness, p95 | under 10 minutes |
| Daily allocation completion | by 02:00 tenant local time |
| Event loss | zero after gateway acknowledgement |
| RPO / RTO | 15 minutes / 2 hours for control state |

## Failure model

- Duplicate delivery is expected and removed by deterministic event identity.
- Late data is accepted within configurable correction windows.
- Price gaps use explicit fallback status and trigger reconciliation.
- Partial provider outages degrade affected enrichment only.
- Poison events enter a tenant-safe dead-letter stream with replay tooling.
- Backpressure propagates to agent batching before disk quotas are exhausted.
