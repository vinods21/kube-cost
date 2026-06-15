# ClickHouse Physical Schema

## Scope

The schema implements temporal Kubernetes inventory, 10-second metrics,
technical 5-minute, 1-hour, and 1-day rollups, versioned cost facts, and
recommendation analytics. The DDL is under
`deploy/clickhouse/migrations/`.

The local schema uses `MergeTree` engines. Production deployments replace fact
tables with `ReplicatedMergeTree` or replicated equivalents and expose
`Distributed` tables without changing column contracts.

## Tables

### Inventory

`cluster`, `namespace`, `deployment`, `pod`, `container`, and `node` are
valid-time history tables. A new row is written when an observed property
changes. `valid_from` is inclusive and `valid_to` is exclusive. Kubernetes UIDs
are stored as strings because imported and synthetic clusters cannot be assumed
to use RFC 4122 values.

The `current_*` views expose rows whose valid interval is open. Inventory uses
`ReplacingMergeTree(version)` because no incremental aggregate consumes these
tables.

Inventory writes are append-only:

- Create and update events append `operation = 'upsert'`.
- Delete events append `operation = 'delete'` tombstones.
- Current-state views rank events per stable entity identity by observation
  time, sequence version, and event ID, then exclude latest delete tombstones.
- Retried agent event IDs are projected deterministically into UUID values and
  collapse under `ReplacingMergeTree ... FINAL`.

The write path does not issue ClickHouse mutations. `valid_from` records event
time; `valid_to` remains available for a later interval-compaction job. History
queries derive intervals from ordered event versions until that compaction is
implemented.

The current agent contract does not carry namespace UID on namespaced child
records, so those rows temporarily store namespace name in `namespace_uid`.
Container records also lack deployment identity. These lineage gaps must be
resolved by a normalizer or a future contract field before cost attribution
depends on them.

### Metrics

`container_metrics_10s` and `node_metrics_10s` are canonical immutable facts.
Resource gauges are stored as resource-time integrals:

- CPU: core-milliseconds.
- Memory: byte-seconds.
- GPU: milli-GPU-seconds.
- Network and filesystem: bytes.

This makes every rollup additive. Utilization is calculated as usage integral
divided by allocatable, requested, or limited integral, rather than averaging
already-computed percentages.

`scope_metrics_*` stores cluster, namespace, deployment, pod, and container
rollups using `(scope_type, scope_id)`. Node rollups remain separate because
node capacity has different semantics from workload requests.

Incremental materialized views cascade:

```text
container_metrics_10s -> scope_metrics_5m -> scope_metrics_1h -> scope_metrics_1d
node_metrics_10s      -> node_metrics_5m  -> node_metrics_1h  -> node_metrics_1d
```

Canonical metric writers MUST deduplicate deterministic event IDs before
insertion. In a replicated deployment they MUST also use deterministic insert
deduplication tokens. Incremental materialized views process inserted blocks;
they do not retract an aggregate when a source row is later replaced.
Corrections therefore use a new computation pipeline or explicit compensating
facts, not mutation of canonical metric rows.

### Cost

- `resource_cost_1h` stores direct priced resource facts.
- `allocation_cost_1h` stores policy-versioned direct, idle, shared, overhead,
  credit, and unallocated results.
- `cost_1d` is a technical additive materialized rollup.

Allocation and pricing jobs write immutable computation versions. Serving APIs
select an active computation version from control-plane state; materialized
views never choose business policy.

Money uses `Decimal(38, 9)` and an explicit ISO currency. Components and cost
bases are bounded `LowCardinality(String)` dimensions so new enum values remain
forward compatible.

### Recommendations

`recommendation` is the analytical recommendation fact and
`recommendation_action` is an append-only analytical action history. Workflow
authority remains in the transactional control plane. Large evidence and
configuration bodies are JSON strings until their stable, frequently queried
fields justify promoted typed columns.

## Partition strategy

| Tables | Partition | Primary ordering rationale |
|---|---|---|
| Inventory | month of `valid_from` | tenant, cluster, stable UID, valid time |
| 10-second and 5-minute facts | month of bucket | tenant and date pruning without daily partition explosion |
| Hourly facts | month of bucket | 25-month operational window |
| Daily facts | year of bucket | long retention with bounded partition count |
| Recommendations/actions | month of creation/action | lifecycle and audit time scans |

There is deliberately no partition per tenant or cluster. At 10,000 clusters,
that would create excessive active parts and coordination overhead. Stable
tenant hashing is a production sharding concern, not a partition key.

## Retention strategy

| Grain/data | Default hot retention | Purpose |
|---|---:|---|
| 10-second metrics | 24 hours | incident analysis and short-window optimization |
| 5-minute metrics | 90 days | operational trends and recommendation evidence |
| 1-hour metrics and cost | 25 months | budgeting, allocation, and year-over-year comparison |
| 1-day metrics and cost | 7 years | long-term reporting and compliance |
| Pod/container history | 25 months | workload lineage matching hourly facts |
| Cluster/namespace/deployment/node history | 7 years | stable reporting dimensions |
| Recommendation facts | 25 months | optimization analysis |
| Recommendation actions | 7 years | audit history |

Production storage policies MAY move hourly and daily parts to object-backed
cold volumes before deletion. Raw envelopes remain in object storage for replay.
Changing TTL is an expand/backfill/switch operation and must be capacity-tested.

## Expected cardinality

The architecture target is 10,000 clusters and 5 million active pods. Planning
assumptions:

| Dimension | Assumption |
|---|---:|
| Containers per pod | 1.7 |
| Active containers | 8.5 million |
| Nodes per cluster | 20 |
| Active nodes | 200,000 |
| Deployments per cluster | 50 |
| Active deployments | 500,000 |
| Namespaces per cluster | 10 |
| Active namespaces | 100,000 |

Unfiltered fleet-wide row generation would be:

| Grain | Container/workload rows per day | Node rows per day |
|---|---:|---:|
| 10 seconds | 73.44 billion container rows | 1.728 billion |
| 5 minutes | 4.064 billion scope rows | 57.6 million |
| 1 hour | 338.64 million scope rows | 4.8 million |
| 1 day | 14.11 million scope rows | 200,000 |

The scope estimate includes cluster, namespace, deployment, pod, and container
rows. At roughly 80-160 compressed bytes per wide 10-second row, fleet-wide
container collection would consume approximately 5.9-11.8 TB per day before
replication. Therefore 10-second collection MUST be targeted by tenant,
cluster, workload cohort, or short diagnostic window. Normal fleet collection
should enter at 5-minute grain or use an aggregation service before ClickHouse.
The schema supports high resolution; it does not make unrestricted
high-resolution collection economical.

## Benchmark

`make clickhouse-benchmark` applies migrations, inserts:

- 900,000 container 10-second rows,
- 36,000 node 10-second rows,
- 2,500 hourly cost rows,
- all resulting materialized rollups,

then runs raw trend, namespace rollup, node utilization, daily cost, and storage
compression queries. Results are machine-dependent and are not acceptance
thresholds by themselves.

Initial local acceptance targets on a four-core developer machine are:

| Operation | Target |
|---|---:|
| Fixture load including materialized views | under 60 seconds |
| Single-container 10-second trend | under 250 ms warm |
| Top namespaces over one hour | under 500 ms warm |
| Node utilization over one hour | under 500 ms warm |
| Daily namespace cost | under 500 ms warm |

Production benchmark gates must additionally measure insert rows per second,
materialized-view write amplification, active parts, merge backlog, compressed
bytes per row, p95/p99 query latency, and replica lag at representative tenant
skew.

## Applying migrations

Start local dependencies, then run:

```text
make clickhouse-migrate
```

`make dev-up` applies the same forward-only, idempotent migrations after
ClickHouse becomes healthy.
