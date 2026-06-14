CREATE TABLE IF NOT EXISTS kube_cost.resource_cost_1h
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    deployment_uid String,
    pod_uid String,
    container_name String,
    node_uid String,
    project String,
    team String,
    environment LowCardinality(String),
    cost_center String,
    bucket_start DateTime('UTC'),
    component LowCardinality(String),
    cost_basis LowCardinality(String),
    currency FixedString(3),
    usage_quantity Decimal(38, 9),
    usage_unit LowCardinality(String),
    list_cost Decimal(38, 9),
    net_cost Decimal(38, 9),
    amortized_cost Decimal(38, 9),
    invoiced_cost Decimal(38, 9),
    price_version String,
    computation_version String,
    quality LowCardinality(String),
    source_event_id UUID,
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
    component,
    cost_basis,
    bucket_start,
    computation_version,
    source_event_id
)
TTL bucket_start + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.allocation_cost_1h
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    deployment_uid String,
    pod_uid String,
    project String,
    team String,
    environment LowCardinality(String),
    cost_center String,
    bucket_start DateTime('UTC'),
    source_category LowCardinality(String),
    component LowCardinality(String),
    cost_basis LowCardinality(String),
    currency FixedString(3),
    direct_cost Decimal(38, 9),
    idle_cost Decimal(38, 9),
    shared_cost Decimal(38, 9),
    overhead_cost Decimal(38, 9),
    credit_cost Decimal(38, 9),
    unallocated_cost Decimal(38, 9),
    allocated_cost Decimal(38, 9),
    allocation_weight Decimal(20, 12),
    allocation_rule_id String,
    allocation_policy_version String,
    computation_version String,
    quality LowCardinality(String),
    source_event_id UUID,
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
    component,
    cost_basis,
    bucket_start,
    allocation_policy_version,
    computation_version,
    source_event_id
)
TTL bucket_start + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.cost_1d
(
    tenant_id String,
    cluster_id String,
    namespace_uid String,
    deployment_uid String,
    project String,
    team String,
    environment LowCardinality(String),
    cost_center String,
    bucket_start DateTime('UTC'),
    source_category LowCardinality(String),
    component LowCardinality(String),
    cost_basis LowCardinality(String),
    currency FixedString(3),
    direct_cost Decimal(38, 9),
    idle_cost Decimal(38, 9),
    shared_cost Decimal(38, 9),
    overhead_cost Decimal(38, 9),
    credit_cost Decimal(38, 9),
    unallocated_cost Decimal(38, 9),
    allocated_cost Decimal(38, 9),
    allocation_policy_version String,
    computation_version String
)
ENGINE = SummingMergeTree
PARTITION BY toYear(bucket_start)
ORDER BY
(
    tenant_id,
    toDate(bucket_start),
    cluster_id,
    namespace_uid,
    deployment_uid,
    project,
    team,
    environment,
    cost_center,
    source_category,
    component,
    cost_basis,
    currency,
    allocation_policy_version,
    computation_version
)
TTL bucket_start + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS kube_cost.allocation_cost_1h_to_1d
TO kube_cost.cost_1d
AS
SELECT
    tenant_id,
    cluster_id,
    namespace_uid,
    deployment_uid,
    project,
    team,
    environment,
    cost_center,
    toStartOfDay(bucket_start) AS bucket_start,
    source_category,
    component,
    cost_basis,
    currency,
    sum(direct_cost) AS direct_cost,
    sum(idle_cost) AS idle_cost,
    sum(shared_cost) AS shared_cost,
    sum(overhead_cost) AS overhead_cost,
    sum(credit_cost) AS credit_cost,
    sum(unallocated_cost) AS unallocated_cost,
    sum(allocated_cost) AS allocated_cost,
    allocation_policy_version,
    computation_version
FROM kube_cost.allocation_cost_1h
GROUP BY
    tenant_id,
    cluster_id,
    namespace_uid,
    deployment_uid,
    project,
    team,
    environment,
    cost_center,
    bucket_start,
    source_category,
    component,
    cost_basis,
    currency,
    allocation_policy_version,
    computation_version;
