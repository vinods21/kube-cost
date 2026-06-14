-- Q1: raw 10-second container trend.
SELECT
    bucket_start,
    sum(cpu_usage_core_milliseconds) / 1000 / 10 AS cpu_cores
FROM kube_cost.container_metrics_10s
WHERE tenant_id = 'benchmark'
  AND pod_uid = 'pod-42'
  AND bucket_start >= toStartOfHour(now('UTC')) - INTERVAL 1 HOUR
  AND bucket_start < toStartOfHour(now('UTC'))
GROUP BY bucket_start
ORDER BY bucket_start
FORMAT Null;

-- Q2: top namespaces from the 5-minute rollup.
SELECT
    scope_id,
    sum(cpu_usage_core_milliseconds) AS cpu_milliseconds
FROM kube_cost.scope_metrics_5m
WHERE tenant_id = 'benchmark'
  AND scope_type = 'namespace'
  AND bucket_start >= toStartOfHour(now('UTC')) - INTERVAL 1 HOUR
  AND bucket_start < toStartOfHour(now('UTC'))
GROUP BY scope_id
ORDER BY cpu_milliseconds DESC
LIMIT 20
FORMAT Null;

-- Q3: node utilization from the 5-minute rollup.
SELECT
    node_uid,
    sum(cpu_usage_core_milliseconds) / nullIf(sum(cpu_allocatable_core_milliseconds), 0) AS cpu_utilization,
    sum(memory_working_set_byte_seconds) / nullIf(sum(memory_allocatable_byte_seconds), 0) AS memory_utilization
FROM kube_cost.node_metrics_5m
WHERE tenant_id = 'benchmark'
  AND bucket_start >= toStartOfHour(now('UTC')) - INTERVAL 1 HOUR
  AND bucket_start < toStartOfHour(now('UTC'))
GROUP BY node_uid
ORDER BY cpu_utilization DESC
FORMAT Null;

-- Q4: daily allocated cost by namespace and component.
SELECT
    namespace_uid,
    component,
    sum(allocated_cost) AS allocated_cost
FROM kube_cost.cost_1d
WHERE tenant_id = 'benchmark'
  AND bucket_start = toStartOfDay(now('UTC'))
  AND computation_version = 'benchmark-v1'
GROUP BY namespace_uid, component
ORDER BY allocated_cost DESC
FORMAT Null;

-- Q5: physical storage and compression.
SELECT
    table,
    sum(rows) AS rows,
    formatReadableSize(sum(bytes_on_disk)) AS bytes_on_disk,
    round(sum(data_uncompressed_bytes) / nullIf(sum(data_compressed_bytes), 0), 2) AS compression_ratio
FROM system.parts
WHERE active
  AND database = 'kube_cost'
  AND table IN
  (
      'container_metrics_10s',
      'scope_metrics_5m',
      'scope_metrics_1h',
      'scope_metrics_1d',
      'node_metrics_10s',
      'node_metrics_5m',
      'cost_1d'
  )
GROUP BY table
ORDER BY table;
