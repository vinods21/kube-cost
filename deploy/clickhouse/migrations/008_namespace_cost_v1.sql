CREATE TABLE IF NOT EXISTS kube_cost.namespace_cost_1h
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    namespace_name String,
    bucket_start DateTime('UTC'),
    allocation_method LowCardinality(String),
    currency FixedString(3),
    node_hourly_cost_usd Decimal(18, 9),
    cpu_request_core_milliseconds UInt64,
    allocation_weight Decimal(20, 12),
    allocated_cost Decimal(38, 9),
    computation_version String,
    computed_at DateTime64(3, 'UTC') DEFAULT now64(3),
    source LowCardinality(String),
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(bucket_start)
ORDER BY
(
    tenant_id,
    toDate(bucket_start),
    cluster_id,
    namespace_uid,
    bucket_start,
    allocation_method,
    computation_version
)
TTL bucket_start + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE VIEW IF NOT EXISTS kube_cost.current_namespace_cost_1h AS
SELECT *
FROM kube_cost.namespace_cost_1h FINAL;
