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
}

func OpenRepository(config ClickHouseConfig) (*Repository, error) {
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
	return &Repository{connection: connection}, nil
}

func (r *Repository) Samples(ctx context.Context, query Query, window time.Duration) ([]Sample, error) {
	sql, args := samplesSQL(query, window)
	rows, err := r.connection.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query optimization samples: %w", err)
	}
	defer rows.Close()

	var result []Sample
	for rows.Next() {
		var sample Sample
		if err := rows.Scan(
			&sample.TenantID,
			&sample.ClusterID,
			&sample.ScopeType,
			&sample.ScopeID,
			&sample.BucketStart,
			&sample.BucketSeconds,
			&sample.CPUUsageCoreMS,
			&sample.CPURequestCoreMS,
			&sample.CPULimitCoreMS,
			&sample.MemoryWorkingSetBytes,
			&sample.MemoryRequestBytes,
			&sample.MemoryLimitBytes,
		); err != nil {
			return nil, fmt.Errorf("scan optimization sample: %w", err)
		}
		result = append(result, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read optimization samples: %w", err)
	}
	return result, nil
}

func (r *Repository) Ping(ctx context.Context) error {
	return r.connection.Ping(ctx)
}

func (r *Repository) Close() error {
	return r.connection.Close()
}

func samplesSQL(query Query, window time.Duration) (string, []any) {
	where, args := sampleWhere(query, window)
	sql := fmt.Sprintf(`
SELECT
    tenant_id,
    cluster_id,
    scope_type,
    scope_id,
    bucket_start,
    bucket_seconds,
    cpu_usage_core_milliseconds,
    cpu_request_core_milliseconds,
    cpu_limit_core_milliseconds,
    memory_working_set_byte_seconds,
    memory_request_byte_seconds,
    memory_limit_byte_seconds
FROM kube_cost.scope_metrics_1h
WHERE %s
ORDER BY tenant_id, cluster_id, scope_type, scope_id, bucket_start`, where)
	return sql, args
}

func sampleWhere(query Query, window time.Duration) (string, []any) {
	clauses := []string{
		"tenant_id = ?",
		"scope_type = 'container'",
		"bucket_start >= ?",
		"bucket_start < ?",
		"sample_count > 0",
	}
	args := []any{query.TenantID, query.End.Add(-window), query.End}
	if query.ClusterID != "" {
		clauses = append(clauses, "cluster_id = ?")
		args = append(args, query.ClusterID)
	}
	return strings.Join(clauses, " AND "), args
}
