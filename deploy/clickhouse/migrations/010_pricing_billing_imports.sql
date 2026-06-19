CREATE TABLE IF NOT EXISTS kube_cost.catalog_price_interval
(
    tenant_id String,
    provider LowCardinality(String),
    account_id String,
    region LowCardinality(String),
    service LowCardinality(String),
    sku String,
    resource_type LowCardinality(String),
    purchase_option LowCardinality(String),
    unit LowCardinality(String),
    currency FixedString(3),
    unit_price Decimal(38, 9),
    effective_start DateTime('UTC'),
    effective_end Nullable(DateTime('UTC')),
    source LowCardinality(String),
    price_version String,
    attributes String,
    ingested_at DateTime64(3, 'UTC'),
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(effective_start)
ORDER BY
(
    tenant_id,
    provider,
    account_id,
    region,
    service,
    sku,
    resource_type,
    purchase_option,
    effective_start,
    price_version
)
TTL effective_start + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS kube_cost.billing_charge
(
    tenant_id String,
    charge_id String,
    provider LowCardinality(String),
    account_id String,
    billing_period_start DateTime('UTC'),
    billing_period_end DateTime('UTC'),
    usage_start DateTime('UTC'),
    usage_end DateTime('UTC'),
    service LowCardinality(String),
    sku String,
    resource_id String,
    cost_category LowCardinality(String),
    currency FixedString(3),
    list_cost Decimal(38, 9),
    net_cost Decimal(38, 9),
    amortized_cost Decimal(38, 9),
    invoiced_cost Decimal(38, 9),
    credits Decimal(38, 9),
    taxes Decimal(38, 9),
    invoice_id String,
    source LowCardinality(String),
    attributes String,
    ingested_at DateTime64(3, 'UTC'),
    version UInt64
)
ENGINE = ReplacingMergeTree(version)
PARTITION BY toYYYYMM(billing_period_start)
ORDER BY
(
    tenant_id,
    provider,
    account_id,
    billing_period_start,
    charge_id
)
TTL billing_period_start + INTERVAL 7 YEAR DELETE
SETTINGS index_granularity = 8192;
