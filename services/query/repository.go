package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type Repository interface {
	DataQuality(context.Context, DataQualityQuery) ([]DataQualitySignal, error)
	Ping(context.Context) error
	Close() error
}

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

type ClickHouseRepository struct {
	connection clickhouse.Conn
}

func OpenRepository(config ClickHouseConfig) (*ClickHouseRepository, error) {
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
	return &ClickHouseRepository{connection: connection}, nil
}

func (r *ClickHouseRepository) DataQuality(ctx context.Context, query DataQualityQuery) ([]DataQualitySignal, error) {
	sql, args := dataQualitySQL(query)
	rows, err := r.connection.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query data quality: %w", err)
	}
	defer rows.Close()

	var result []DataQualitySignal
	for rows.Next() {
		var signal DataQualitySignal
		var latestBucket time.Time
		var latestIngested time.Time
		if err := rows.Scan(
			&signal.Source,
			&signal.Grain,
			&signal.ClusterID,
			&signal.RecordCount,
			&latestBucket,
			&latestIngested,
		); err != nil {
			return nil, fmt.Errorf("scan data quality row: %w", err)
		}
		if signal.RecordCount > 0 {
			signal.LatestBucketStart = &latestBucket
			signal.LatestIngestedAt = &latestIngested
		}
		result = append(result, signal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read data quality rows: %w", err)
	}
	return result, nil
}

func (r *ClickHouseRepository) Ping(ctx context.Context) error {
	return r.connection.Ping(ctx)
}

func (r *ClickHouseRepository) Close() error {
	return r.connection.Close()
}

func dataQualitySQL(query DataQualityQuery) (string, []any) {
	where, args := tenantWhere(query)
	sql := fmt.Sprintf(`
SELECT
    source,
    grain,
    cluster_id,
    count() AS record_count,
    max(bucket_start) AS latest_bucket_start,
    max(ingested_at) AS latest_ingested_at
FROM
(
    SELECT
        'container_metrics_10s' AS source,
        '10s' AS grain,
        cluster_id,
        bucket_start,
        ingested_at
    FROM kube_cost.container_metrics_10s
    WHERE %s
    UNION ALL
    SELECT
        'node_metrics_10s' AS source,
        '10s' AS grain,
        cluster_id,
        bucket_start,
        ingested_at
    FROM kube_cost.node_metrics_10s
    WHERE %s
)
GROUP BY source, grain, cluster_id
ORDER BY cluster_id, source`, where, where)
	return sql, append(args, args...)
}

func tenantWhere(query DataQualityQuery) (string, []any) {
	clauses := []string{"tenant_id = ?"}
	args := []any{query.TenantID}
	if query.ClusterID != "" {
		clauses = append(clauses, "cluster_id = ?")
		args = append(args, query.ClusterID)
	}
	return strings.Join(clauses, " AND "), args
}
