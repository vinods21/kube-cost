ALTER TABLE kube_cost.cluster
    ADD COLUMN IF NOT EXISTS operation LowCardinality(String) DEFAULT 'upsert' AFTER labels;

ALTER TABLE kube_cost.namespace
    ADD COLUMN IF NOT EXISTS operation LowCardinality(String) DEFAULT 'upsert' AFTER labels;

ALTER TABLE kube_cost.deployment
    ADD COLUMN IF NOT EXISTS operation LowCardinality(String) DEFAULT 'upsert' AFTER labels;

ALTER TABLE kube_cost.pod
    ADD COLUMN IF NOT EXISTS operation LowCardinality(String) DEFAULT 'upsert' AFTER labels;

ALTER TABLE kube_cost.container
    ADD COLUMN IF NOT EXISTS operation LowCardinality(String) DEFAULT 'upsert' AFTER gpu_request_milli;

ALTER TABLE kube_cost.node
    ADD COLUMN IF NOT EXISTS operation LowCardinality(String) DEFAULT 'upsert' AFTER labels;

DROP VIEW IF EXISTS kube_cost.current_cluster;
CREATE VIEW kube_cost.current_cluster AS
SELECT * EXCEPT (row_number)
FROM
(
    SELECT *, row_number() OVER
        (PARTITION BY tenant_id, cluster_id ORDER BY observed_at DESC, version DESC, event_id DESC) AS row_number
    FROM kube_cost.cluster FINAL
)
WHERE row_number = 1 AND operation = 'upsert';

DROP VIEW IF EXISTS kube_cost.current_namespace;
CREATE VIEW kube_cost.current_namespace AS
SELECT * EXCEPT (row_number)
FROM
(
    SELECT *, row_number() OVER
        (PARTITION BY tenant_id, cluster_id, namespace_uid ORDER BY observed_at DESC, version DESC, event_id DESC) AS row_number
    FROM kube_cost.namespace FINAL
)
WHERE row_number = 1 AND operation = 'upsert';

DROP VIEW IF EXISTS kube_cost.current_deployment;
CREATE VIEW kube_cost.current_deployment AS
SELECT * EXCEPT (row_number)
FROM
(
    SELECT *, row_number() OVER
        (PARTITION BY tenant_id, cluster_id, deployment_uid ORDER BY observed_at DESC, version DESC, event_id DESC) AS row_number
    FROM kube_cost.deployment FINAL
)
WHERE row_number = 1 AND operation = 'upsert';

DROP VIEW IF EXISTS kube_cost.current_pod;
CREATE VIEW kube_cost.current_pod AS
SELECT * EXCEPT (row_number)
FROM
(
    SELECT *, row_number() OVER
        (PARTITION BY tenant_id, cluster_id, pod_uid ORDER BY observed_at DESC, version DESC, event_id DESC) AS row_number
    FROM kube_cost.pod FINAL
)
WHERE row_number = 1 AND operation = 'upsert';

DROP VIEW IF EXISTS kube_cost.current_container;
CREATE VIEW kube_cost.current_container AS
SELECT * EXCEPT (row_number)
FROM
(
    SELECT *, row_number() OVER
        (PARTITION BY tenant_id, cluster_id, pod_uid, container_name ORDER BY observed_at DESC, version DESC, event_id DESC) AS row_number
    FROM kube_cost.container FINAL
)
WHERE row_number = 1 AND operation = 'upsert';

DROP VIEW IF EXISTS kube_cost.current_node;
CREATE VIEW kube_cost.current_node AS
SELECT * EXCEPT (row_number)
FROM
(
    SELECT *, row_number() OVER
        (PARTITION BY tenant_id, cluster_id, node_uid ORDER BY observed_at DESC, version DESC, event_id DESC) AS row_number
    FROM kube_cost.node FINAL
)
WHERE row_number = 1 AND operation = 'upsert';
