package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type ClickHouseConfig struct {
	Address      string
	Database     string
	Username     string
	Password     string
	Secure       bool
	DialTimeout  time.Duration
	MaxOpenConns int
	MaxIdleConns int
}

type Repository struct {
	connection        clickhouse.Conn
	nodeHourlyCostUSD float64
}

func OpenRepository(config ClickHouseConfig, nodeHourlyCostUSD float64) (*Repository, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("ClickHouse address is required")
	}
	if config.Database == "" {
		config.Database = "kube_cost"
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = 5 * time.Second
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 10
	}
	if config.MaxIdleConns <= 0 {
		config.MaxIdleConns = 5
	}
	if nodeHourlyCostUSD <= 0 {
		nodeHourlyCostUSD = defaultNodeHourlyCostUSD
	}
	options := &clickhouse.Options{
		Addr: []string{config.Address},
		Auth: clickhouse.Auth{
			Database: config.Database,
			Username: config.Username,
			Password: config.Password,
		},
		DialTimeout:     config.DialTimeout,
		MaxOpenConns:    config.MaxOpenConns,
		MaxIdleConns:    config.MaxIdleConns,
		ConnMaxLifetime: time.Hour,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	}
	if config.Secure {
		options.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	connection, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("open ClickHouse connection: %w", err)
	}
	return &Repository{connection: connection, nodeHourlyCostUSD: nodeHourlyCostUSD}, nil
}

func (r *Repository) NamespaceCosts(ctx context.Context, query Query) ([]NamespaceCost, error) {
	sql, args := namespaceCostSQL(query, r.nodeHourlyCostUSD)
	rows, err := r.connection.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query namespace costs: %w", err)
	}
	defer rows.Close()

	var result []NamespaceCost
	for rows.Next() {
		var item NamespaceCost
		var bucketStart time.Time
		if err := rows.Scan(
			&item.TenantID,
			&item.ClusterID,
			&item.NamespaceUID,
			&item.NamespaceName,
			&bucketStart,
			&item.CPURequestCoreMilliseconds,
			&item.AllocationWeight,
			&item.AllocatedCost,
		); err != nil {
			return nil, fmt.Errorf("scan namespace cost row: %w", err)
		}
		item.BucketStart = bucketStart.UTC().Format(time.RFC3339)
		item.Currency = defaultCurrency
		item.AllocationMethod = allocationMethodCPU
		item.ComputationVersion = computationVersionV1
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read namespace cost rows: %w", err)
	}
	return result, nil
}

func (r *Repository) Ping(ctx context.Context) error {
	return r.connection.Ping(ctx)
}

func (r *Repository) Close() error {
	return r.connection.Close()
}

func namespaceCostSQL(query Query, nodeHourlyCostUSD float64) (string, []any) {
	namespaceWhere, namespaceArgs := metricWhere(query)
	totalWhere, totalArgs := metricWhere(query)
	args := append([]any{}, namespaceArgs...)
	args = append(args, totalArgs...)
	args = append(args, nodeHourlyCostUSD)

	sql := fmt.Sprintf(`
WITH namespace_requests AS
(
    SELECT
        tenant_id,
        cluster_id,
        node_uid,
        namespace_uid,
        toStartOfHour(bucket_start) AS bucket_start,
        sum(cpu_request_core_milliseconds) AS namespace_cpu_request_core_milliseconds
    FROM kube_cost.container_metrics_10s
    WHERE %s
    GROUP BY tenant_id, cluster_id, node_uid, namespace_uid, bucket_start
),
node_requests AS
(
    SELECT
        tenant_id,
        cluster_id,
        node_uid,
        toStartOfHour(bucket_start) AS bucket_start,
        sum(cpu_request_core_milliseconds) AS node_cpu_request_core_milliseconds
    FROM kube_cost.container_metrics_10s
    WHERE %s
    GROUP BY tenant_id, cluster_id, node_uid, bucket_start
    HAVING node_cpu_request_core_milliseconds > 0
),
allocated AS
(
    SELECT
        nr.tenant_id,
        nr.cluster_id,
        nr.namespace_uid,
        nr.bucket_start,
        nr.namespace_cpu_request_core_milliseconds,
        toFloat64(nr.namespace_cpu_request_core_milliseconds) / toFloat64(node_requests.node_cpu_request_core_milliseconds) AS allocation_weight,
        ? * (toFloat64(nr.namespace_cpu_request_core_milliseconds) / toFloat64(node_requests.node_cpu_request_core_milliseconds)) AS allocated_cost
    FROM namespace_requests AS nr
    INNER JOIN node_requests
        ON nr.tenant_id = node_requests.tenant_id
       AND nr.cluster_id = node_requests.cluster_id
       AND nr.node_uid = node_requests.node_uid
       AND nr.bucket_start = node_requests.bucket_start
    INNER JOIN kube_cost.current_node AS node
        ON nr.tenant_id = node.tenant_id
       AND nr.cluster_id = node.cluster_id
       AND nr.node_uid = node.node_uid
)
SELECT
    allocated.tenant_id,
    allocated.cluster_id,
    allocated.namespace_uid,
    if(empty(any(ns.namespace_name)), allocated.namespace_uid, any(ns.namespace_name)) AS namespace_name,
    allocated.bucket_start,
    sum(allocated.namespace_cpu_request_core_milliseconds) AS cpu_request_core_milliseconds,
    sum(allocated.allocation_weight) AS allocation_weight,
    sum(allocated.allocated_cost) AS allocated_cost
FROM allocated
LEFT JOIN kube_cost.current_namespace AS ns
    ON allocated.tenant_id = ns.tenant_id
   AND allocated.cluster_id = ns.cluster_id
   AND allocated.namespace_uid = ns.namespace_uid
GROUP BY
    allocated.tenant_id,
    allocated.cluster_id,
    allocated.namespace_uid,
    allocated.bucket_start
ORDER BY allocated.bucket_start, allocated.cluster_id, allocated.namespace_uid`, namespaceWhere, totalWhere)

	return sql, args
}

func metricWhere(query Query) (string, []any) {
	clauses := []string{
		"tenant_id = ?",
		"bucket_start >= ?",
		"bucket_start < ?",
		"cpu_request_core_milliseconds > 0",
	}
	args := []any{query.TenantID, query.Start, query.End}
	if query.ClusterID != "" {
		clauses = append(clauses, "cluster_id = ?")
		args = append(args, query.ClusterID)
	}
	return strings.Join(clauses, " AND "), args
}
