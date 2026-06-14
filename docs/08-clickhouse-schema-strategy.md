# ClickHouse Schema Strategy

## Role

ClickHouse is the analytical system of record for normalized observations, pricing, cost facts, allocation facts, quality, and recommendations. It is not the source of truth for mutable control-plane workflows.

## Database layers

| Layer | Purpose | Examples |
|---|---|---|
| Landing | Short-retention validated event payloads | observations_ingest, prices_ingest |
| Canonical | Deduplicated typed facts at source grain | container_usage_1m, node_inventory_history |
| Derived | Priced and allocated facts | workload_cost_1h, shared_allocation_1h |
| Aggregate | Query-optimized rollups | namespace_cost_1d, tenant_cost_1mo |
| Serving | Stable views and dictionaries | current dimensions, API views |

## Table strategy

- Use replicated MergeTree-family engines in production.
- Partition primarily by month and tenant shard; avoid one partition per tenant.
- Order high-volume facts by `(tenant_id, date, cluster_id, primary dimensions, event_time)`.
- Use deterministic `event_id` and computation version for idempotent replacement.
- Keep raw source facts append-only; corrections append a higher version.
- Use LowCardinality only for bounded dimensions, never arbitrary labels.
- Store money in fixed-point decimals and resource quantities in canonical integer/fixed-point units.

## Canonical tables

- `container_usage_1m`: requests, limits, usage integrals, runtime, identity, metadata version, quality.
- `pod_lifecycle`: scheduled/running/ready intervals and owner history.
- `node_inventory_history`: instance type, capacity, architecture, purchase option, provider identity.
- `volume_usage_1h`, `network_usage_1h`, `load_balancer_usage_1h`.
- `catalog_price_interval`, `billing_charge`, `commitment_allocation`, `fx_rate`.
- `resource_cost_1h`: direct priced cost by resource component.
- `allocation_cost_1h`: destination dimensions, source category, rule, weight, policy version.
- `recommendation_fact`: evidence summaries and projected savings.
- `data_quality_1h`: expected/received samples, pricing and identity coverage.

## Labels and dimensions

Frequently queried dimensions such as namespace, workload, team, project, environment, and cost center receive typed columns. Arbitrary labels use a bounded map for filtering only, with ingestion allow lists, value length limits, and per-tenant cardinality budgets. Promoted dimensions are configured through versioned mappings and backfilled deliberately.

## Materialization

- Materialized views produce technical rollups only when their semantics are stable.
- Business-policy allocation is computed by versioned jobs, not hidden in materialized views.
- Projections optimize common tenant/time and cluster/time paths.
- Dictionaries serve small, slowly changing mappings; historical joins use interval tables.

## Reprocessing

A recomputation writes a new `computation_version` for the affected tenant/time/scope. Serving views select the active version recorded in a computation manifest. Activation is atomic from the API perspective. Previous versions remain temporarily for rollback and audit.

## Retention and tiering

TTL moves older hourly facts to colder volumes where supported, then deletes according to policy. Daily/monthly aggregates outlive minute facts. Object storage retains raw envelopes and Parquet exports for disaster recovery and deep replay.

The initial physical implementation retains 10-second facts for 24 hours,
5-minute rollups for 90 days, hourly facts for 25 months, and daily facts for
7 years. See [ClickHouse Physical Schema](19-clickhouse-physical-schema.md) for
DDL, materialized views, partitioning, cardinality, and benchmark assumptions.

## Multi-tenancy

- Tenant ID is mandatory in every table, view, query predicate, and row-level policy.
- Distributed tables route by stable tenant hash while splitting very large tenants by cluster.
- Query settings enforce per-tenant memory, concurrency, scan, and result limits.
- Dedicated clusters remain an enterprise isolation option without changing contracts.

## Operations

Track parts per partition, merge backlog, mutation backlog, replication lag, query p95/p99, rejected inserts, disk watermarks, compression ratio, and tenant scan volume. Schema changes use expand/backfill/switch/contract phases; synchronous mutations on large tables are prohibited.
