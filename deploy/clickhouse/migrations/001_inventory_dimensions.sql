CREATE DATABASE IF NOT EXISTS kube_cost;

CREATE TABLE IF NOT EXISTS kube_cost.cluster
(
    tenant_id String,
    cluster_id String,
    cluster_name String,
    provider LowCardinality(String),
    account_id String,
    region LowCardinality(String),
    kubernetes_version LowCardinality(String),
    labels Map(String, String),
    valid_from DateTime64(3, 'UTC'),
    valid_to Nullable(DateTime64(3, 'UTC')),
    observed_at DateTime64(3, 'UTC'),
    event_id UUID,
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(valid_from)
ORDER BY (tenant_id, cluster_id, valid_from, event_id)
TTL toDateTime(valid_from) + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.namespace
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    namespace_name String,
    phase LowCardinality(String),
    team String,
    project String,
    environment LowCardinality(String),
    cost_center String,
    labels Map(String, String),
    valid_from DateTime64(3, 'UTC'),
    valid_to Nullable(DateTime64(3, 'UTC')),
    observed_at DateTime64(3, 'UTC'),
    event_id UUID,
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(valid_from)
ORDER BY (tenant_id, cluster_id, namespace_uid, valid_from, event_id)
TTL toDateTime(valid_from) + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.deployment
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    deployment_uid String,
    namespace_name String,
    deployment_name String,
    desired_replicas UInt32,
    available_replicas UInt32,
    strategy LowCardinality(String),
    team String,
    project String,
    environment LowCardinality(String),
    cost_center String,
    labels Map(String, String),
    valid_from DateTime64(3, 'UTC'),
    valid_to Nullable(DateTime64(3, 'UTC')),
    observed_at DateTime64(3, 'UTC'),
    event_id UUID,
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(valid_from)
ORDER BY (tenant_id, cluster_id, namespace_uid, deployment_uid, valid_from, event_id)
TTL toDateTime(valid_from) + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.pod
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    deployment_uid String,
    pod_uid String,
    node_uid String,
    namespace_name String,
    deployment_name String,
    pod_name String,
    phase LowCardinality(String),
    qos_class LowCardinality(String),
    owner_kind LowCardinality(String),
    owner_uid String,
    scheduled_at Nullable(DateTime64(3, 'UTC')),
    started_at Nullable(DateTime64(3, 'UTC')),
    finished_at Nullable(DateTime64(3, 'UTC')),
    labels Map(String, String),
    valid_from DateTime64(3, 'UTC'),
    valid_to Nullable(DateTime64(3, 'UTC')),
    observed_at DateTime64(3, 'UTC'),
    event_id UUID,
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(valid_from)
ORDER BY (tenant_id, cluster_id, namespace_uid, pod_uid, valid_from, event_id)
TTL toDateTime(valid_from) + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.container
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    deployment_uid String,
    pod_uid String,
    container_name String,
    container_id String,
    image String,
    image_id String,
    restart_count UInt32,
    cpu_request_millicores UInt64,
    cpu_limit_millicores UInt64,
    memory_request_bytes UInt64,
    memory_limit_bytes UInt64,
    gpu_request_milli UInt64,
    valid_from DateTime64(3, 'UTC'),
    valid_to Nullable(DateTime64(3, 'UTC')),
    observed_at DateTime64(3, 'UTC'),
    event_id UUID,
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(valid_from)
ORDER BY (tenant_id, cluster_id, namespace_uid, pod_uid, container_name, valid_from, event_id)
TTL toDateTime(valid_from) + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.node
(
    tenant_id String,
    cluster_id String,
    node_uid String,
    node_name String,
    provider_id String,
    instance_type LowCardinality(String),
    architecture LowCardinality(String),
    operating_system LowCardinality(String),
    region LowCardinality(String),
    zone LowCardinality(String),
    purchase_option LowCardinality(String),
    capacity_cpu_millicores UInt64,
    allocatable_cpu_millicores UInt64,
    capacity_memory_bytes UInt64,
    allocatable_memory_bytes UInt64,
    capacity_gpu_milli UInt64,
    labels Map(String, String),
    valid_from DateTime64(3, 'UTC'),
    valid_to Nullable(DateTime64(3, 'UTC')),
    observed_at DateTime64(3, 'UTC'),
    event_id UUID,
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(valid_from)
ORDER BY (tenant_id, cluster_id, node_uid, valid_from, event_id)
TTL toDateTime(valid_from) + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;
