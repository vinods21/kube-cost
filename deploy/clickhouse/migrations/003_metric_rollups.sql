CREATE TABLE IF NOT EXISTS kube_cost.scope_metrics_5m
(
    tenant_id String,
    cluster_id String,
    scope_type LowCardinality(String),
    scope_id String,
    bucket_start DateTime('UTC'),
    bucket_seconds UInt64,
    cpu_usage_core_milliseconds UInt64,
    cpu_request_core_milliseconds UInt64,
    cpu_limit_core_milliseconds UInt64,
    memory_working_set_byte_seconds UInt64,
    memory_request_byte_seconds UInt64,
    memory_limit_byte_seconds UInt64,
    network_rx_bytes UInt64,
    network_tx_bytes UInt64,
    filesystem_read_bytes UInt64,
    filesystem_write_bytes UInt64,
    gpu_usage_milli_seconds UInt64,
    gpu_request_milli_seconds UInt64,
    oom_events UInt64,
    cpu_throttled_periods UInt64,
    sample_count UInt64
)
ENGINE = SummingMergeTree
PARTITION BY toYYYYMM(bucket_start)
ORDER BY (tenant_id, scope_type, toDate(bucket_start), cluster_id, scope_id, bucket_start)
TTL bucket_start + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS kube_cost.container_metrics_10s_to_scope_5m
TO kube_cost.scope_metrics_5m
AS
SELECT
    tenant_id,
    cluster_id,
    scope.1 AS scope_type,
    scope.2 AS scope_id,
    toStartOfFiveMinutes(bucket_start) AS bucket_start,
    sum(toUInt64(bucket_seconds)) AS bucket_seconds,
    sum(cpu_usage_core_milliseconds) AS cpu_usage_core_milliseconds,
    sum(cpu_request_core_milliseconds) AS cpu_request_core_milliseconds,
    sum(cpu_limit_core_milliseconds) AS cpu_limit_core_milliseconds,
    sum(memory_working_set_byte_seconds) AS memory_working_set_byte_seconds,
    sum(memory_request_byte_seconds) AS memory_request_byte_seconds,
    sum(memory_limit_byte_seconds) AS memory_limit_byte_seconds,
    sum(network_rx_bytes) AS network_rx_bytes,
    sum(network_tx_bytes) AS network_tx_bytes,
    sum(filesystem_read_bytes) AS filesystem_read_bytes,
    sum(filesystem_write_bytes) AS filesystem_write_bytes,
    sum(gpu_usage_milli_seconds) AS gpu_usage_milli_seconds,
    sum(gpu_request_milli_seconds) AS gpu_request_milli_seconds,
    sum(toUInt64(oom_events)) AS oom_events,
    sum(toUInt64(cpu_throttled_periods)) AS cpu_throttled_periods,
    sum(toUInt64(sample_count)) AS sample_count
FROM kube_cost.container_metrics_10s
ARRAY JOIN
[
    tuple('cluster', cluster_id),
    tuple('namespace', namespace_uid),
    tuple('deployment', deployment_uid),
    tuple('pod', pod_uid),
    tuple('container', concat(pod_uid, '/', container_name))
] AS scope
WHERE scope.2 != ''
GROUP BY tenant_id, cluster_id, scope_type, scope_id, bucket_start;

CREATE TABLE IF NOT EXISTS kube_cost.scope_metrics_1h AS kube_cost.scope_metrics_5m
ENGINE = SummingMergeTree
PARTITION BY toYYYYMM(bucket_start)
ORDER BY (tenant_id, scope_type, toDate(bucket_start), cluster_id, scope_id, bucket_start)
TTL bucket_start + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS kube_cost.scope_metrics_5m_to_1h
TO kube_cost.scope_metrics_1h
AS
SELECT
    tenant_id,
    cluster_id,
    scope_type,
    scope_id,
    toStartOfHour(bucket_start) AS bucket_start,
    sum(bucket_seconds) AS bucket_seconds,
    sum(cpu_usage_core_milliseconds) AS cpu_usage_core_milliseconds,
    sum(cpu_request_core_milliseconds) AS cpu_request_core_milliseconds,
    sum(cpu_limit_core_milliseconds) AS cpu_limit_core_milliseconds,
    sum(memory_working_set_byte_seconds) AS memory_working_set_byte_seconds,
    sum(memory_request_byte_seconds) AS memory_request_byte_seconds,
    sum(memory_limit_byte_seconds) AS memory_limit_byte_seconds,
    sum(network_rx_bytes) AS network_rx_bytes,
    sum(network_tx_bytes) AS network_tx_bytes,
    sum(filesystem_read_bytes) AS filesystem_read_bytes,
    sum(filesystem_write_bytes) AS filesystem_write_bytes,
    sum(gpu_usage_milli_seconds) AS gpu_usage_milli_seconds,
    sum(gpu_request_milli_seconds) AS gpu_request_milli_seconds,
    sum(oom_events) AS oom_events,
    sum(cpu_throttled_periods) AS cpu_throttled_periods,
    sum(sample_count) AS sample_count
FROM kube_cost.scope_metrics_5m
GROUP BY tenant_id, cluster_id, scope_type, scope_id, bucket_start;

CREATE TABLE IF NOT EXISTS kube_cost.scope_metrics_1d AS kube_cost.scope_metrics_5m
ENGINE = SummingMergeTree
PARTITION BY toYear(bucket_start)
ORDER BY (tenant_id, scope_type, toDate(bucket_start), cluster_id, scope_id)
TTL bucket_start + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS kube_cost.scope_metrics_1h_to_1d
TO kube_cost.scope_metrics_1d
AS
SELECT
    tenant_id,
    cluster_id,
    scope_type,
    scope_id,
    toStartOfDay(bucket_start) AS bucket_start,
    sum(bucket_seconds) AS bucket_seconds,
    sum(cpu_usage_core_milliseconds) AS cpu_usage_core_milliseconds,
    sum(cpu_request_core_milliseconds) AS cpu_request_core_milliseconds,
    sum(cpu_limit_core_milliseconds) AS cpu_limit_core_milliseconds,
    sum(memory_working_set_byte_seconds) AS memory_working_set_byte_seconds,
    sum(memory_request_byte_seconds) AS memory_request_byte_seconds,
    sum(memory_limit_byte_seconds) AS memory_limit_byte_seconds,
    sum(network_rx_bytes) AS network_rx_bytes,
    sum(network_tx_bytes) AS network_tx_bytes,
    sum(filesystem_read_bytes) AS filesystem_read_bytes,
    sum(filesystem_write_bytes) AS filesystem_write_bytes,
    sum(gpu_usage_milli_seconds) AS gpu_usage_milli_seconds,
    sum(gpu_request_milli_seconds) AS gpu_request_milli_seconds,
    sum(oom_events) AS oom_events,
    sum(cpu_throttled_periods) AS cpu_throttled_periods,
    sum(sample_count) AS sample_count
FROM kube_cost.scope_metrics_1h
GROUP BY tenant_id, cluster_id, scope_type, scope_id, bucket_start;

CREATE TABLE IF NOT EXISTS kube_cost.node_metrics_5m
(
    tenant_id String,
    cluster_id String,
    node_uid String,
    bucket_start DateTime('UTC'),
    bucket_seconds UInt64,
    cpu_usage_core_milliseconds UInt64,
    cpu_allocatable_core_milliseconds UInt64,
    memory_working_set_byte_seconds UInt64,
    memory_allocatable_byte_seconds UInt64,
    network_rx_bytes UInt64,
    network_tx_bytes UInt64,
    filesystem_read_bytes UInt64,
    filesystem_write_bytes UInt64,
    gpu_usage_milli_seconds UInt64,
    gpu_capacity_milli_seconds UInt64,
    sample_count UInt64
)
ENGINE = SummingMergeTree
PARTITION BY toYYYYMM(bucket_start)
ORDER BY (tenant_id, toDate(bucket_start), cluster_id, node_uid, bucket_start)
TTL bucket_start + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS kube_cost.node_metrics_10s_to_5m
TO kube_cost.node_metrics_5m
AS
SELECT
    tenant_id,
    cluster_id,
    node_uid,
    toStartOfFiveMinutes(bucket_start) AS bucket_start,
    sum(toUInt64(bucket_seconds)) AS bucket_seconds,
    sum(cpu_usage_core_milliseconds) AS cpu_usage_core_milliseconds,
    sum(cpu_allocatable_core_milliseconds) AS cpu_allocatable_core_milliseconds,
    sum(memory_working_set_byte_seconds) AS memory_working_set_byte_seconds,
    sum(memory_allocatable_byte_seconds) AS memory_allocatable_byte_seconds,
    sum(network_rx_bytes) AS network_rx_bytes,
    sum(network_tx_bytes) AS network_tx_bytes,
    sum(filesystem_read_bytes) AS filesystem_read_bytes,
    sum(filesystem_write_bytes) AS filesystem_write_bytes,
    sum(gpu_usage_milli_seconds) AS gpu_usage_milli_seconds,
    sum(gpu_capacity_milli_seconds) AS gpu_capacity_milli_seconds,
    sum(toUInt64(sample_count)) AS sample_count
FROM kube_cost.node_metrics_10s
GROUP BY tenant_id, cluster_id, node_uid, bucket_start;

CREATE TABLE IF NOT EXISTS kube_cost.node_metrics_1h
(
    tenant_id String,
    cluster_id String,
    node_uid String,
    bucket_start DateTime('UTC'),
    bucket_seconds UInt64,
    cpu_usage_core_milliseconds UInt64,
    cpu_allocatable_core_milliseconds UInt64,
    memory_working_set_byte_seconds UInt64,
    memory_allocatable_byte_seconds UInt64,
    network_rx_bytes UInt64,
    network_tx_bytes UInt64,
    filesystem_read_bytes UInt64,
    filesystem_write_bytes UInt64,
    gpu_usage_milli_seconds UInt64,
    gpu_capacity_milli_seconds UInt64,
    sample_count UInt64
)
ENGINE = SummingMergeTree
PARTITION BY toYYYYMM(bucket_start)
ORDER BY (tenant_id, toDate(bucket_start), cluster_id, node_uid, bucket_start)
TTL bucket_start + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS kube_cost.node_metrics_5m_to_1h
TO kube_cost.node_metrics_1h
AS
SELECT
    tenant_id,
    cluster_id,
    node_uid,
    toStartOfHour(bucket_start) AS bucket_start,
    sum(bucket_seconds) AS bucket_seconds,
    sum(cpu_usage_core_milliseconds) AS cpu_usage_core_milliseconds,
    sum(cpu_allocatable_core_milliseconds) AS cpu_allocatable_core_milliseconds,
    sum(memory_working_set_byte_seconds) AS memory_working_set_byte_seconds,
    sum(memory_allocatable_byte_seconds) AS memory_allocatable_byte_seconds,
    sum(network_rx_bytes) AS network_rx_bytes,
    sum(network_tx_bytes) AS network_tx_bytes,
    sum(filesystem_read_bytes) AS filesystem_read_bytes,
    sum(filesystem_write_bytes) AS filesystem_write_bytes,
    sum(gpu_usage_milli_seconds) AS gpu_usage_milli_seconds,
    sum(gpu_capacity_milli_seconds) AS gpu_capacity_milli_seconds,
    sum(sample_count) AS sample_count
FROM kube_cost.node_metrics_5m
GROUP BY tenant_id, cluster_id, node_uid, bucket_start;

CREATE TABLE IF NOT EXISTS kube_cost.node_metrics_1d
(
    tenant_id String,
    cluster_id String,
    node_uid String,
    bucket_start DateTime('UTC'),
    bucket_seconds UInt64,
    cpu_usage_core_milliseconds UInt64,
    cpu_allocatable_core_milliseconds UInt64,
    memory_working_set_byte_seconds UInt64,
    memory_allocatable_byte_seconds UInt64,
    network_rx_bytes UInt64,
    network_tx_bytes UInt64,
    filesystem_read_bytes UInt64,
    filesystem_write_bytes UInt64,
    gpu_usage_milli_seconds UInt64,
    gpu_capacity_milli_seconds UInt64,
    sample_count UInt64
)
ENGINE = SummingMergeTree
PARTITION BY toYear(bucket_start)
ORDER BY (tenant_id, toDate(bucket_start), cluster_id, node_uid)
TTL bucket_start + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS kube_cost.node_metrics_1h_to_1d
TO kube_cost.node_metrics_1d
AS
SELECT
    tenant_id,
    cluster_id,
    node_uid,
    toStartOfDay(bucket_start) AS bucket_start,
    sum(bucket_seconds) AS bucket_seconds,
    sum(cpu_usage_core_milliseconds) AS cpu_usage_core_milliseconds,
    sum(cpu_allocatable_core_milliseconds) AS cpu_allocatable_core_milliseconds,
    sum(memory_working_set_byte_seconds) AS memory_working_set_byte_seconds,
    sum(memory_allocatable_byte_seconds) AS memory_allocatable_byte_seconds,
    sum(network_rx_bytes) AS network_rx_bytes,
    sum(network_tx_bytes) AS network_tx_bytes,
    sum(filesystem_read_bytes) AS filesystem_read_bytes,
    sum(filesystem_write_bytes) AS filesystem_write_bytes,
    sum(gpu_usage_milli_seconds) AS gpu_usage_milli_seconds,
    sum(gpu_capacity_milli_seconds) AS gpu_capacity_milli_seconds,
    sum(sample_count) AS sample_count
FROM kube_cost.node_metrics_1h
GROUP BY tenant_id, cluster_id, node_uid, bucket_start;
