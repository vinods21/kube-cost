ALTER TABLE kube_cost.container
    ADD COLUMN IF NOT EXISTS owner_kind LowCardinality(String) DEFAULT '' AFTER deployment_uid;

ALTER TABLE kube_cost.container
    ADD COLUMN IF NOT EXISTS owner_uid String DEFAULT '' AFTER owner_kind;
