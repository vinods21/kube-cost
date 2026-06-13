CREATE USER IF NOT EXISTS grafana_reader IDENTIFIED WITH sha256_password BY 'grafana_reader';
GRANT SELECT ON kube_cost.* TO grafana_reader;
