CREATE TABLE IF NOT EXISTS kube_cost.container_metrics_10s
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    deployment_uid String,
    pod_uid String,
    node_uid String,
    container_name String,
    bucket_start DateTime('UTC'),
    bucket_seconds UInt16,
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
    oom_events UInt32,
    cpu_throttled_periods UInt32,
    sample_count UInt32,
    quality LowCardinality(String),
    observed_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
    event_id UUID,
    version UInt64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(bucket_start)
ORDER BY
(
    tenant_id,
    toDate(bucket_start),
    cluster_id,
    namespace_uid,
    deployment_uid,
    pod_uid,
    container_name,
    bucket_start,
    event_id
)
TTL bucket_start + INTERVAL 1 DAY DELETE
SETTINGS index_granularity = 8192;

ALTER TABLE kube_cost.container_metrics_10s
    ADD INDEX IF NOT EXISTS idx_container_node node_uid TYPE bloom_filter(0.01) GRANULARITY 4;

CREATE TABLE IF NOT EXISTS kube_cost.node_metrics_10s
(
    tenant_id String,
    cluster_id String,
    node_uid String,
    bucket_start DateTime('UTC'),
    bucket_seconds UInt16,
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
    sample_count UInt32,
    quality LowCardinality(String),
    observed_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3),
    event_id UUID,
    version UInt64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(bucket_start)
ORDER BY
(
    tenant_id,
    toDate(bucket_start),
    cluster_id,
    node_uid,
    bucket_start,
    event_id
)
TTL bucket_start + INTERVAL 1 DAY DELETE
SETTINGS index_granularity = 8192;
