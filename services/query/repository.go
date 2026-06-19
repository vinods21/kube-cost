package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/shopspring/decimal"
)

var ErrRecommendationNotFound = errors.New("recommendation not found")

type Repository interface {
	DataQuality(context.Context, DataQualityQuery) ([]DataQualitySignal, error)
	Recommendations(context.Context, RecommendationQuery) ([]RecommendationResult, error)
	Recommendation(context.Context, string, string) (RecommendationResult, error)
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

func (r *ClickHouseRepository) Recommendations(ctx context.Context, query RecommendationQuery) ([]RecommendationResult, error) {
	sql, args, err := recommendationsSQL(query)
	if err != nil {
		return nil, err
	}
	rows, err := r.connection.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query recommendations: %w", err)
	}
	defer rows.Close()

	var result []RecommendationResult
	for rows.Next() {
		recommendation, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, recommendation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read recommendation rows: %w", err)
	}
	return result, nil
}

func (r *ClickHouseRepository) Recommendation(ctx context.Context, tenantID, recommendationID string) (RecommendationResult, error) {
	sql := recommendationsSelectSQL(`
WHERE tenant_id = ? AND recommendation_id = ?
ORDER BY generated_at DESC
LIMIT 1`)
	rows, err := r.connection.Query(ctx, sql, tenantID, recommendationID)
	if err != nil {
		return RecommendationResult{}, fmt.Errorf("query recommendation: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return RecommendationResult{}, fmt.Errorf("read recommendation row: %w", err)
		}
		return RecommendationResult{}, ErrRecommendationNotFound
	}
	recommendation, err := scanRecommendation(rows)
	if err != nil {
		return RecommendationResult{}, err
	}
	return recommendation, nil
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

func recommendationsSQL(query RecommendationQuery) (string, []any, error) {
	where, args, err := recommendationsWhere(query)
	if err != nil {
		return "", nil, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return recommendationsSelectSQL(fmt.Sprintf(`
WHERE %s
ORDER BY monthly_net_savings DESC, generated_at DESC, recommendation_id
LIMIT %d`, where, limit)), args, nil
}

func recommendationsWhere(query RecommendationQuery) (string, []any, error) {
	clauses := []string{"tenant_id = ?"}
	args := []any{query.TenantID}
	if query.ClusterID != "" {
		clauses = append(clauses, "cluster_id = ?")
		args = append(args, query.ClusterID)
	}
	if query.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	}
	if query.RecommendationType != "" {
		clauses = append(clauses, "recommendation_type = ?")
		args = append(args, query.RecommendationType)
	}
	if query.TargetKind != "" {
		clauses = append(clauses, "target_kind = ?")
		args = append(args, query.TargetKind)
	}
	if query.TargetUID != "" {
		clauses = append(clauses, "target_uid = ?")
		args = append(args, query.TargetUID)
	}
	if query.MinimumMonthlySavings != "" {
		minimumSavings, err := decimal.NewFromString(query.MinimumMonthlySavings)
		if err != nil {
			return "", nil, fmt.Errorf("invalid minimum monthly savings: %w", err)
		}
		clauses = append(clauses, "monthly_net_savings >= ?")
		args = append(args, minimumSavings)
	}
	return strings.Join(clauses, " AND "), args, nil
}

func recommendationsSelectSQL(suffix string) string {
	return fmt.Sprintf(`
SELECT
    tenant_id,
    recommendation_id,
    cluster_id,
    namespace_uid,
    target_kind,
    target_uid,
    recommendation_type,
    safety_class,
    status,
    analysis_window_start,
    analysis_window_end,
    generated_at,
    expires_at,
    current_configuration,
    proposed_configuration,
    evidence,
    currency,
    monthly_gross_savings,
    monthly_net_savings,
    confidence,
    risk_score,
    policy_version,
    model_version,
    computation_version,
    version
FROM kube_cost.recommendation FINAL
%s`, suffix)
}

type recommendationScanner interface {
	Scan(dest ...any) error
}

func scanRecommendation(scanner recommendationScanner) (RecommendationResult, error) {
	var recommendation RecommendationResult
	var currentConfiguration string
	var proposedConfiguration string
	var evidence string
	var monthlyGrossSavings decimal.Decimal
	var monthlyNetSavings decimal.Decimal
	var confidence decimal.Decimal
	var riskScore decimal.Decimal
	if err := scanner.Scan(
		&recommendation.TenantID,
		&recommendation.RecommendationID,
		&recommendation.ClusterID,
		&recommendation.NamespaceUID,
		&recommendation.TargetKind,
		&recommendation.TargetUID,
		&recommendation.RecommendationType,
		&recommendation.SafetyClass,
		&recommendation.Status,
		&recommendation.AnalysisWindowStart,
		&recommendation.AnalysisWindowEnd,
		&recommendation.GeneratedAt,
		&recommendation.ExpiresAt,
		&currentConfiguration,
		&proposedConfiguration,
		&evidence,
		&recommendation.Currency,
		&monthlyGrossSavings,
		&monthlyNetSavings,
		&confidence,
		&riskScore,
		&recommendation.PolicyVersion,
		&recommendation.ModelVersion,
		&recommendation.ComputationVersion,
		&recommendation.Version,
	); err != nil {
		return RecommendationResult{}, fmt.Errorf("scan recommendation row: %w", err)
	}
	recommendation.CurrentConfiguration = jsonRawMessage(currentConfiguration)
	recommendation.ProposedConfiguration = jsonRawMessage(proposedConfiguration)
	recommendation.Evidence = jsonRawMessage(evidence)
	recommendation.MonthlyGrossSavings = monthlyGrossSavings.String()
	recommendation.MonthlyNetSavings = monthlyNetSavings.String()
	recommendation.Confidence = confidence.String()
	recommendation.RiskScore = riskScore.String()
	return recommendation, nil
}
