# Execution Plan

## Delivery principles

- Build a vertical cost lineage before broad feature coverage.
- Gate phases on measured correctness, scale, security, and operability.
- Keep action automation disabled until recommendation quality is proven.
- Use synthetic, replay, and real canary clusters from the first data-plane milestone.

## Phase 0: Foundation and decisions

Deliver:

- Accept critical ADRs and establish ownership.
- Define SLOs, threat model, data classification, retention, and residency.
- Ratify API/proto compatibility and release policy.
- Stand up CI checks for contracts, documentation, migrations, artifacts, and supply chain.

Exit criteria: architecture review approval; named service/data owners; threat-model actions tracked; development environments reproducible.

## Phase 1: Minimum auditable cost vertical

Deliver:

- Cluster enrollment and read-only agent.
- Kubernetes inventory, CPU/memory requests and usage, node identity.
- Ingestion gateway, durable stream, raw archive, canonical ClickHouse facts.
- Public catalog pricing and hourly workload/node cost.
- Basic tenant, namespace, workload, node, and cluster cost API.
- Freshness, completeness, and price-coverage diagnostics.

Exit criteria: less than 1% unexplained duplicate/loss in replay; hourly results reproducible; p95 freshness under 10 minutes; cost lineage inspectable end to end.

## Phase 2: Billing-grade allocation

Deliver:

- Cloud billing imports, discounts, commitments, credits, and FX.
- Idle, shared, overhead, and unallocated cost policies.
- Temporal team/project mappings and policy dry runs.
- Reconciliation dashboards and finalized billing periods.
- Exports and budget/threshold alerts.

Exit criteria: at least 99% of eligible invoice cost classified; variance within provider-specific threshold; policy replay and rollback validated; finance sign-off on sample periods.

## Phase 3: Multi-cloud and scale

Deliver:

- Provider adapters for target clouds and major storage/network/LB charges.
- Regional-cell deployment and tenant routing.
- High-cardinality query controls, workload isolation, aggregate strategy.
- DR restore, chaos, performance, and noisy-neighbor testing.

Exit criteria: tested target cluster/pod scale at 2x expected peak; cell failure drills pass RTO/RPO; query and ingestion SLOs pass under concurrent load.

## Phase 4: Optimization insights

Deliver:

- Workload rightsizing, idle resource, scheduling, node, and commitment recommendations.
- Evidence, confidence, risk, expiry, suppression, and workflow APIs.
- Historical replay evaluation and shadow scoring.
- Realized-savings measurement.

Exit criteria: recommendation precision and rollback-risk thresholds agreed by product/SRE; no action execution enabled; canary users validate explainability.

## Phase 5: Karpenter and guarded automation

Deliver:

- Karpenter inventory/lifecycle correlation and compatibility matrix.
- NodePool/consolidation/spot insights with scheduling simulation.
- Action operator, typed approvals, canary rollout, verification, and rollback.
- Freeze windows, blast-radius controls, audit, and emergency disable.

Exit criteria: security review approved; failure injection proves halt/rollback; only low-risk action classes enabled per tenant opt-in.

## Workstreams

| Workstream | Continuous responsibilities |
|---|---|
| Contracts | APIs, protobuf, events, compatibility and deprecation |
| Edge | collection correctness, API pressure, buffering, upgrades |
| Data | schemas, lineage, replay, quality, retention |
| Cost | pricing, allocation, commitments, reconciliation |
| Optimization | evidence, simulation, evaluation, safety |
| Platform | cells, delivery, SRE, DR, security |
| Experience | query semantics, exports, workflows, explainability |

## Test strategy

- Unit/property tests for money, interval, and allocation invariants.
- Contract tests for producer/consumer compatibility.
- Golden datasets with hand-calculated allocation.
- Replay tests across schema and policy versions.
- Integration tests with ephemeral Kubernetes and provider fixtures.
- Performance tests for ingestion, ClickHouse merges, and queries.
- Chaos tests for agent disconnect, broker loss, database failover, and late data.
- Security tests for tenant isolation, RBAC, secret handling, and action allow lists.

## Governance

Architecture review occurs at each phase boundary. Schema, contract, allocation-semantic, or automation-safety changes require ADR review. Production readiness requires SLO dashboards, alerts, runbooks, capacity model, on-call ownership, backup restore evidence, and rollback procedure.

## Principal risks

| Risk | Mitigation |
|---|---|
| Metrics do not reconcile to invoices | billing imports, residual category, explicit quality |
| Label cardinality destabilizes storage | allow lists, budgets, promoted dimensions |
| Agent overloads Kubernetes API | watches, caching, adaptive intervals, performance gates |
| Price/commitment semantics differ by provider | adapter boundary, provider fixtures, lineage |
| Recommendations overstate savings | realizability simulation, confidence, verification |
| Automation disrupts workloads | typed actions, canaries, policy gates, rollback |
| Reprocessing changes historical reports unexpectedly | immutable versions, compare/promote workflow |
| Tenant data leakage | mandatory tenant keys, authorization, row policies, isolation tests |

## Definition of production ready

A capability is production ready only when its contracts are versioned, data lineage is queryable, quality states are exposed, tenant isolation is tested, SLOs and alerts exist, scale targets pass, restore/rollback is documented and tested, and an owning on-call team accepts it.
