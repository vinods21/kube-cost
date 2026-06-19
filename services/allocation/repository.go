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
	connection clickhouse.Conn
	options    AllocationOptions
}

func OpenRepository(config ClickHouseConfig, options AllocationOptions) (*Repository, error) {
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
	options = normalizeAllocationOptions(options)
	clickhouseOptions := &clickhouse.Options{
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
		clickhouseOptions.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	connection, err := clickhouse.Open(clickhouseOptions)
	if err != nil {
		return nil, fmt.Errorf("open ClickHouse connection: %w", err)
	}
	return &Repository{connection: connection, options: options}, nil
}

func (r *Repository) NamespaceCosts(ctx context.Context, query Query) ([]NamespaceCost, error) {
	sql, args := namespaceCostSQL(query, r.options)
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
			&item.NetworkBytes,
			&item.AllocationWeight,
			&item.DirectCost,
			&item.IdleCost,
			&item.NetworkCost,
			&item.ControlPlaneCost,
			&item.SystemNamespaceCost,
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

func namespaceCostSQL(query Query, options AllocationOptions) (string, []any) {
	options = normalizeAllocationOptions(options)
	where, args := metricWhere(query)
	args = append(args,
		options.NodeHourlyCostUSD,
		options.NetworkCostPerGiBUSD,
		options.ControlPlaneHourlyCostUSD,
		options.NodeHourlyCostUSD,
	)

	sql := fmt.Sprintf(`
WITH usage AS
(
    SELECT
        tenant_id,
        cluster_id,
        node_uid,
        namespace_uid,
        toStartOfHour(bucket_start) AS bucket_start,
        sum(cpu_request_core_milliseconds) AS cpu_request_core_milliseconds,
        sum(network_rx_bytes + network_tx_bytes) AS network_bytes
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
    FROM usage
    GROUP BY tenant_id, cluster_id, node_uid, bucket_start
    HAVING node_cpu_request_core_milliseconds > 0
),
cluster_requests AS
(
    SELECT
        tenant_id,
        cluster_id,
        bucket_start,
        sum(cpu_request_core_milliseconds) AS cluster_cpu_request_core_milliseconds
    FROM usage
    GROUP BY tenant_id, cluster_id, bucket_start
    HAVING cluster_cpu_request_core_milliseconds > 0
),
allocated AS
(
    SELECT
        usage.tenant_id,
        usage.cluster_id,
        usage.namespace_uid,
        if(empty(any(ns.namespace_name)), usage.namespace_uid, any(ns.namespace_name)) AS namespace_name,
        usage.bucket_start,
        sum(usage.cpu_request_core_milliseconds) AS cpu_request_core_milliseconds,
        sum(usage.network_bytes) AS network_bytes,
        sum(toFloat64(usage.cpu_request_core_milliseconds) / greatest(toFloat64(node.allocatable_cpu_millicores) * 3600, toFloat64(node_requests.node_cpu_request_core_milliseconds))) AS allocation_weight,
        sum(? * (toFloat64(usage.cpu_request_core_milliseconds) / greatest(toFloat64(node.allocatable_cpu_millicores) * 3600, toFloat64(node_requests.node_cpu_request_core_milliseconds)))) AS direct_cost,
        toFloat64(0) AS idle_cost,
        sum((toFloat64(usage.network_bytes) / 1073741824) * ?) AS network_cost,
        sum(? * (toFloat64(usage.cpu_request_core_milliseconds) / toFloat64(cluster_requests.cluster_cpu_request_core_milliseconds))) AS control_plane_cost
    FROM usage
    INNER JOIN node_requests
        ON usage.tenant_id = node_requests.tenant_id
       AND usage.cluster_id = node_requests.cluster_id
       AND usage.node_uid = node_requests.node_uid
       AND usage.bucket_start = node_requests.bucket_start
    INNER JOIN cluster_requests
        ON usage.tenant_id = cluster_requests.tenant_id
       AND usage.cluster_id = cluster_requests.cluster_id
       AND usage.bucket_start = cluster_requests.bucket_start
    INNER JOIN kube_cost.current_node AS node
        ON usage.tenant_id = node.tenant_id
       AND usage.cluster_id = node.cluster_id
       AND usage.node_uid = node.node_uid
    LEFT JOIN kube_cost.current_namespace AS ns
        ON usage.tenant_id = ns.tenant_id
       AND usage.cluster_id = ns.cluster_id
       AND usage.namespace_uid = ns.namespace_uid
    GROUP BY usage.tenant_id, usage.cluster_id, usage.namespace_uid, usage.bucket_start
),
idle AS
(
    SELECT
        node_requests.tenant_id,
        node_requests.cluster_id,
        '__idle__' AS namespace_uid,
        '__idle__' AS namespace_name,
        node_requests.bucket_start,
        toUInt64(0) AS cpu_request_core_milliseconds,
        toUInt64(0) AS network_bytes,
        toFloat64(0) AS allocation_weight,
        toFloat64(0) AS direct_cost,
        sum(? * greatest(
            greatest(toFloat64(node.allocatable_cpu_millicores) * 3600, toFloat64(node_requests.node_cpu_request_core_milliseconds)) - toFloat64(node_requests.node_cpu_request_core_milliseconds),
            0
        ) / greatest(toFloat64(node.allocatable_cpu_millicores) * 3600, toFloat64(node_requests.node_cpu_request_core_milliseconds))) AS idle_cost,
        toFloat64(0) AS network_cost,
        toFloat64(0) AS control_plane_cost
    FROM node_requests
    INNER JOIN kube_cost.current_node AS node
        ON node_requests.tenant_id = node.tenant_id
       AND node_requests.cluster_id = node.cluster_id
       AND node_requests.node_uid = node.node_uid
    GROUP BY node_requests.tenant_id, node_requests.cluster_id, node_requests.bucket_start
)
SELECT
    tenant_id,
    cluster_id,
    namespace_uid,
    namespace_name,
    bucket_start,
    cpu_request_core_milliseconds,
    network_bytes,
    allocation_weight,
    direct_cost,
    idle_cost,
    network_cost,
    control_plane_cost,
    if(namespace_name IN ('kube-system', 'kube-public', 'kube-node-lease'), direct_cost + network_cost + control_plane_cost, 0) AS system_namespace_cost,
    direct_cost + idle_cost + network_cost + control_plane_cost AS allocated_cost
FROM
(
    SELECT * FROM allocated
    UNION ALL
    SELECT * FROM idle
)
ORDER BY bucket_start, cluster_id, namespace_uid`, where)

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
