# Performance Tests

Performance workloads live here.

- `clickhouse/reset.sql` removes the reserved `benchmark` tenant fixture.
- `clickhouse/generate.sql` creates a deterministic-scale local fixture.
- `clickhouse/queries.sql` exercises raw metrics, rollups, cost, and storage.

Run the ClickHouse workload with `make clickhouse-benchmark`.
