CREATE TABLE IF NOT EXISTS kube_cost.recommendation
(
    tenant_id String,
    recommendation_id String,
    cluster_id String,
    namespace_uid String,
    target_kind LowCardinality(String),
    target_uid String,
    recommendation_type LowCardinality(String),
    safety_class LowCardinality(String),
    status LowCardinality(String),
    analysis_window_start DateTime('UTC'),
    analysis_window_end DateTime('UTC'),
    generated_at DateTime64(3, 'UTC'),
    expires_at DateTime64(3, 'UTC'),
    current_configuration String,
    proposed_configuration String,
    evidence String,
    currency FixedString(3),
    monthly_gross_savings Decimal(38, 9),
    monthly_net_savings Decimal(38, 9),
    confidence Decimal(6, 5),
    risk_score Decimal(6, 5),
    policy_version String,
    model_version String,
    computation_version String,
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(generated_at)
ORDER BY
(
    tenant_id,
    cluster_id,
    recommendation_type,
    target_kind,
    target_uid,
    recommendation_id
)
TTL toDateTime(generated_at) + INTERVAL 25 MONTH DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.recommendation_action
(
    tenant_id String,
    recommendation_id String,
    action_id UUID,
    action LowCardinality(String),
    actor_type LowCardinality(String),
    actor_id String,
    reason String,
    occurred_at DateTime64(3, 'UTC'),
    execution_id String,
    result LowCardinality(String),
    details String
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (tenant_id, recommendation_id, occurred_at, action_id)
TTL toDateTime(occurred_at) + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;
