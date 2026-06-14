-- Local benchmark fixture:
--   2,500 containers x 360 ten-second windows = 900,000 raw rows
--   100 nodes x 360 ten-second windows = 36,000 raw rows
-- Materialized views additionally generate scope and node rollups.

INSERT INTO kube_cost.container_metrics_10s
SELECT
    'benchmark' AS tenant_id,
    'cluster-1' AS cluster_id,
    concat('namespace-', toString(container_number % 25)) AS namespace_uid,
    concat('deployment-', toString(container_number % 250)) AS deployment_uid,
    concat('pod-', toString(container_number)) AS pod_uid,
    concat('node-', toString(container_number % 100)) AS node_uid,
    concat('container-', toString(container_number % 3)) AS container_name,
    toStartOfHour(now('UTC')) - INTERVAL 1 HOUR + toIntervalSecond(window_number * 10) AS bucket_start,
    toUInt16(10) AS bucket_seconds,
    toUInt64(500 + cityHash64(container_number, window_number) % 9500) AS cpu_usage_core_milliseconds,
    toUInt64(5000) AS cpu_request_core_milliseconds,
    toUInt64(10000) AS cpu_limit_core_milliseconds,
    toUInt64((64 + cityHash64(window_number, container_number) % 448) * 1024 * 1024 * 10) AS memory_working_set_byte_seconds,
    toUInt64(256 * 1024 * 1024 * 10) AS memory_request_byte_seconds,
    toUInt64(512 * 1024 * 1024 * 10) AS memory_limit_byte_seconds,
    toUInt64(cityHash64(container_number, window_number, 1) % 100000) AS network_rx_bytes,
    toUInt64(cityHash64(container_number, window_number, 2) % 100000) AS network_tx_bytes,
    toUInt64(cityHash64(container_number, window_number, 3) % 50000) AS filesystem_read_bytes,
    toUInt64(cityHash64(container_number, window_number, 4) % 50000) AS filesystem_write_bytes,
    toUInt64(0) AS gpu_usage_milli_seconds,
    toUInt64(0) AS gpu_request_milli_seconds,
    toUInt32(if(cityHash64(container_number, window_number) % 100000 = 0, 1, 0)) AS oom_events,
    toUInt32(cityHash64(container_number, window_number) % 5) AS cpu_throttled_periods,
    toUInt32(1) AS sample_count,
    'complete' AS quality,
    toDateTime64(bucket_start + INTERVAL 1 SECOND, 3, 'UTC') AS observed_at,
    toDateTime64(bucket_start + INTERVAL 2 SECOND, 3, 'UTC') AS ingested_at,
    generateUUIDv4() AS event_id,
    toUInt64(1) AS version
FROM (SELECT number AS container_number FROM numbers(2500)) AS containers
CROSS JOIN (SELECT number AS window_number FROM numbers(360)) AS windows
SETTINGS
    max_insert_threads = 4,
    max_threads = 8;

INSERT INTO kube_cost.node_metrics_10s
SELECT
    'benchmark' AS tenant_id,
    'cluster-1' AS cluster_id,
    concat('node-', toString(node_number)) AS node_uid,
    toStartOfHour(now('UTC')) - INTERVAL 1 HOUR + toIntervalSecond(window_number * 10) AS bucket_start,
    toUInt16(10) AS bucket_seconds,
    toUInt64(20000 + cityHash64(node_number, window_number) % 60000) AS cpu_usage_core_milliseconds,
    toUInt64(160000) AS cpu_allocatable_core_milliseconds,
    toUInt64((8 + cityHash64(window_number, node_number) % 16) * 1024 * 1024 * 1024 * 10) AS memory_working_set_byte_seconds,
    toUInt64(32 * 1024 * 1024 * 1024 * 10) AS memory_allocatable_byte_seconds,
    toUInt64(cityHash64(node_number, window_number, 1) % 10000000) AS network_rx_bytes,
    toUInt64(cityHash64(node_number, window_number, 2) % 10000000) AS network_tx_bytes,
    toUInt64(cityHash64(node_number, window_number, 3) % 1000000) AS filesystem_read_bytes,
    toUInt64(cityHash64(node_number, window_number, 4) % 1000000) AS filesystem_write_bytes,
    toUInt64(0) AS gpu_usage_milli_seconds,
    toUInt64(0) AS gpu_capacity_milli_seconds,
    toUInt32(1) AS sample_count,
    'complete' AS quality,
    toDateTime64(bucket_start + INTERVAL 1 SECOND, 3, 'UTC') AS observed_at,
    toDateTime64(bucket_start + INTERVAL 2 SECOND, 3, 'UTC') AS ingested_at,
    generateUUIDv4() AS event_id,
    toUInt64(1) AS version
FROM (SELECT number AS node_number FROM numbers(100)) AS nodes
CROSS JOIN (SELECT number AS window_number FROM numbers(360)) AS windows
SETTINGS
    max_insert_threads = 4,
    max_threads = 8;

INSERT INTO kube_cost.allocation_cost_1h
SELECT
    'benchmark',
    'cluster-1',
    concat('namespace-', toString(number % 25)),
    concat('deployment-', toString(number % 250)),
    concat('pod-', toString(number)),
    concat('project-', toString(number % 10)),
    concat('team-', toString(number % 20)),
    'benchmark',
    concat('cc-', toString(number % 5)),
    toStartOfDay(now('UTC')),
    'direct',
    ['cpu', 'memory', 'gpu'][number % 3 + 1],
    'amortized',
    'USD',
    toDecimal128(0.01 + (number % 100) / 10000, 9),
    toDecimal128(0, 9),
    toDecimal128(0, 9),
    toDecimal128(0, 9),
    toDecimal128(0, 9),
    toDecimal128(0, 9),
    toDecimal128(0.01 + (number % 100) / 10000, 9),
    toDecimal64(1, 12),
    '',
    'policy-v1',
    'benchmark-v1',
    'complete',
    generateUUIDv4(),
    toUInt64(1)
FROM numbers(2500);
