CREATE VIEW IF NOT EXISTS kube_cost.current_cluster AS
SELECT *
FROM kube_cost.cluster FINAL
WHERE valid_to IS NULL;

CREATE VIEW IF NOT EXISTS kube_cost.current_namespace AS
SELECT *
FROM kube_cost.namespace FINAL
WHERE valid_to IS NULL;

CREATE VIEW IF NOT EXISTS kube_cost.current_deployment AS
SELECT *
FROM kube_cost.deployment FINAL
WHERE valid_to IS NULL;

CREATE VIEW IF NOT EXISTS kube_cost.current_pod AS
SELECT *
FROM kube_cost.pod FINAL
WHERE valid_to IS NULL;

CREATE VIEW IF NOT EXISTS kube_cost.current_container AS
SELECT *
FROM kube_cost.container FINAL
WHERE valid_to IS NULL;

CREATE VIEW IF NOT EXISTS kube_cost.current_node AS
SELECT *
FROM kube_cost.node FINAL
WHERE valid_to IS NULL;

CREATE VIEW IF NOT EXISTS kube_cost.open_recommendation AS
SELECT *
FROM kube_cost.recommendation FINAL
WHERE status IN ('open', 'acknowledged', 'approved', 'executing');
