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
	Usage(context.Context, AnalyticsQuery) ([]UsageRow, error)
	Costs(context.Context, AnalyticsQuery) (CostMetadata, []CostRow, error)
	Allocation(context.Context, AnalyticsQuery) (CostMetadata, []AllocationRow, error)
	Recommendations(context.Context, RecommendationQuery) ([]RecommendationResult, error)
	Recommendation(context.Context, string, string) (RecommendationResult, error)
	Ping(context.Context) error
	Close() error
}

type CostMetadata struct {
	Currency           string
	ComputationVersion string
	ComputedAt         time.Time
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

func (r *ClickHouseRepository) Usage(ctx context.Context, query AnalyticsQuery) ([]UsageRow, error) {
	sql, args := usageSQL(query)
	rows, err := r.connection.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query usage: %w", err)
	}
	defer rows.Close()

	var result []UsageRow
	for rows.Next() {
		var item UsageRow
		var cpuUsage decimal.Decimal
		var cpuRequest decimal.Decimal
		var cpuLimit decimal.Decimal
		var memoryWorkingSet decimal.Decimal
		var memoryRequest decimal.Decimal
		var memoryLimit decimal.Decimal
		var gpuUsage decimal.Decimal
		var gpuRequest decimal.Decimal
		if err := rows.Scan(
			&item.TenantID,
			&item.ClusterID,
			&item.NamespaceUID,
			&item.NamespaceName,
			&cpuUsage,
			&cpuRequest,
			&cpuLimit,
			&memoryWorkingSet,
			&memoryRequest,
			&memoryLimit,
			&item.NetworkBytes,
			&item.FilesystemBytes,
			&gpuUsage,
			&gpuRequest,
			&item.OOMEvents,
			&item.CPUThrottledPeriods,
			&item.SampleCount,
		); err != nil {
			return nil, fmt.Errorf("scan usage row: %w", err)
		}
		item.CPUUsageCoreHours = cpuUsage.String()
		item.CPURequestCoreHours = cpuRequest.String()
		item.CPULimitCoreHours = cpuLimit.String()
		item.MemoryWorkingSetGiBHours = memoryWorkingSet.String()
		item.MemoryRequestGiBHours = memoryRequest.String()
		item.MemoryLimitGiBHours = memoryLimit.String()
		item.GPUUsageMilliHours = gpuUsage.String()
		item.GPURequestMilliHours = gpuRequest.String()
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read usage rows: %w", err)
	}
	return result, nil
}

func (r *ClickHouseRepository) Costs(ctx context.Context, query AnalyticsQuery) (CostMetadata, []CostRow, error) {
	sql, args := costsSQL(query)
	rows, err := r.connection.Query(ctx, sql, args...)
	if err != nil {
		return CostMetadata{}, nil, fmt.Errorf("query costs: %w", err)
	}
	defer rows.Close()

	var metadata CostMetadata
	var result []CostRow
	for rows.Next() {
		var item CostRow
		var directCost decimal.Decimal
		var idleCost decimal.Decimal
		var networkCost decimal.Decimal
		var controlPlaneCost decimal.Decimal
		var systemNamespaceCost decimal.Decimal
		var allocatedCost decimal.Decimal
		if err := rows.Scan(
			&item.TenantID,
			&item.ClusterID,
			&item.NamespaceUID,
			&item.NamespaceName,
			&metadata.Currency,
			&metadata.ComputationVersion,
			&metadata.ComputedAt,
			&directCost,
			&idleCost,
			&networkCost,
			&controlPlaneCost,
			&systemNamespaceCost,
			&allocatedCost,
		); err != nil {
			return CostMetadata{}, nil, fmt.Errorf("scan cost row: %w", err)
		}
		item.DirectCost = directCost.String()
		item.IdleCost = idleCost.String()
		item.NetworkCost = networkCost.String()
		item.ControlPlaneCost = controlPlaneCost.String()
		item.SystemNamespaceCost = systemNamespaceCost.String()
		item.AllocatedCost = allocatedCost.String()
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return CostMetadata{}, nil, fmt.Errorf("read cost rows: %w", err)
	}
	return metadata, result, nil
}

func (r *ClickHouseRepository) Allocation(ctx context.Context, query AnalyticsQuery) (CostMetadata, []AllocationRow, error) {
	sql, args := allocationSQL(query)
	rows, err := r.connection.Query(ctx, sql, args...)
	if err != nil {
		return CostMetadata{}, nil, fmt.Errorf("query allocation: %w", err)
	}
	defer rows.Close()

	var metadata CostMetadata
	var result []AllocationRow
	for rows.Next() {
		var item AllocationRow
		var allocationWeight decimal.Decimal
		var directCost decimal.Decimal
		var idleCost decimal.Decimal
		var networkCost decimal.Decimal
		var controlPlaneCost decimal.Decimal
		var systemNamespaceCost decimal.Decimal
		var allocatedCost decimal.Decimal
		if err := rows.Scan(
			&item.TenantID,
			&item.ClusterID,
			&item.NamespaceUID,
			&item.NamespaceName,
			&metadata.Currency,
			&metadata.ComputationVersion,
			&metadata.ComputedAt,
			&item.CPURequestCoreMilliseconds,
			&item.NetworkBytes,
			&allocationWeight,
			&directCost,
			&idleCost,
			&networkCost,
			&controlPlaneCost,
			&systemNamespaceCost,
			&allocatedCost,
		); err != nil {
			return CostMetadata{}, nil, fmt.Errorf("scan allocation row: %w", err)
		}
		item.AllocationWeight = allocationWeight.String()
		item.DirectCost = directCost.String()
		item.IdleCost = idleCost.String()
		item.NetworkCost = networkCost.String()
		item.ControlPlaneCost = controlPlaneCost.String()
		item.SystemNamespaceCost = systemNamespaceCost.String()
		item.AllocatedCost = allocatedCost.String()
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return CostMetadata{}, nil, fmt.Errorf("read allocation rows: %w", err)
	}
	return metadata, result, nil
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

func usageSQL(query AnalyticsQuery) (string, []any) {
	group := analyticsGroup(query.GroupBy, "cm.namespace_uid", "ns.namespace_name")
	where, args := analyticsMetricWhere(query, "cm")
	return fmt.Sprintf(`
SELECT
    cm.tenant_id,
    cm.cluster_id,
    %s AS namespace_uid,
    %s AS namespace_name,
    sum(toDecimal128(cm.cpu_usage_core_milliseconds, 9)) / 3600000 AS cpu_usage_core_hours,
    sum(toDecimal128(cm.cpu_request_core_milliseconds, 9)) / 3600000 AS cpu_request_core_hours,
    sum(toDecimal128(cm.cpu_limit_core_milliseconds, 9)) / 3600000 AS cpu_limit_core_hours,
    sum(toDecimal128(cm.memory_working_set_byte_seconds, 9)) / 3865470566400 AS memory_working_set_gib_hours,
    sum(toDecimal128(cm.memory_request_byte_seconds, 9)) / 3865470566400 AS memory_request_gib_hours,
    sum(toDecimal128(cm.memory_limit_byte_seconds, 9)) / 3865470566400 AS memory_limit_gib_hours,
    sum(cm.network_rx_bytes + cm.network_tx_bytes) AS network_bytes,
    sum(cm.filesystem_read_bytes + cm.filesystem_write_bytes) AS filesystem_bytes,
    sum(toDecimal128(cm.gpu_usage_milli_seconds, 9)) / 3600000 AS gpu_usage_milli_hours,
    sum(toDecimal128(cm.gpu_request_milli_seconds, 9)) / 3600000 AS gpu_request_milli_hours,
    sum(cm.oom_events) AS oom_events,
    sum(cm.cpu_throttled_periods) AS cpu_throttled_periods,
    sum(cm.sample_count) AS sample_count
FROM kube_cost.container_metrics_10s AS cm
LEFT JOIN kube_cost.current_namespace AS ns
    ON cm.tenant_id = ns.tenant_id
   AND cm.cluster_id = ns.cluster_id
   AND cm.namespace_uid = ns.namespace_uid
WHERE %s
GROUP BY cm.tenant_id, cm.cluster_id%s
ORDER BY cpu_request_core_hours DESC, cm.cluster_id, namespace_uid
LIMIT %d`, group.namespaceUIDSelect, group.namespaceNameSelect, where, group.groupBySuffix, normalizedAnalyticsLimit(query.Limit)), args
}

func costsSQL(query AnalyticsQuery) (string, []any) {
	group := analyticsGroup(query.GroupBy, "nc.namespace_uid", "nc.namespace_name")
	where, args := analyticsCostWhere(query, "nc")
	return fmt.Sprintf(`
SELECT
    nc.tenant_id,
    nc.cluster_id,
    %s AS namespace_uid,
    %s AS namespace_name,
    any(nc.currency) AS currency,
    anyLast(nc.computation_version) AS computation_version,
    max(nc.computed_at) AS computed_at,
    sum(nc.direct_cost) AS direct_cost,
    sum(nc.idle_cost) AS idle_cost,
    sum(nc.network_cost) AS network_cost,
    sum(nc.control_plane_cost) AS control_plane_cost,
    sum(nc.system_namespace_cost) AS system_namespace_cost,
    sum(nc.allocated_cost) AS allocated_cost
FROM kube_cost.current_namespace_cost_1h AS nc
WHERE %s
GROUP BY nc.tenant_id, nc.cluster_id%s
ORDER BY allocated_cost DESC, nc.cluster_id, namespace_uid
LIMIT %d`, group.namespaceUIDSelect, group.namespaceNameSelect, where, group.groupBySuffix, normalizedAnalyticsLimit(query.Limit)), args
}

func allocationSQL(query AnalyticsQuery) (string, []any) {
	group := analyticsGroup(query.GroupBy, "nc.namespace_uid", "nc.namespace_name")
	where, args := analyticsCostWhere(query, "nc")
	return fmt.Sprintf(`
SELECT
    nc.tenant_id,
    nc.cluster_id,
    %s AS namespace_uid,
    %s AS namespace_name,
    any(nc.currency) AS currency,
    anyLast(nc.computation_version) AS computation_version,
    max(nc.computed_at) AS computed_at,
    sum(nc.cpu_request_core_milliseconds) AS cpu_request_core_milliseconds,
    sum(nc.network_bytes) AS network_bytes,
    sum(nc.allocation_weight) AS allocation_weight,
    sum(nc.direct_cost) AS direct_cost,
    sum(nc.idle_cost) AS idle_cost,
    sum(nc.network_cost) AS network_cost,
    sum(nc.control_plane_cost) AS control_plane_cost,
    sum(nc.system_namespace_cost) AS system_namespace_cost,
    sum(nc.allocated_cost) AS allocated_cost
FROM kube_cost.current_namespace_cost_1h AS nc
WHERE %s
GROUP BY nc.tenant_id, nc.cluster_id%s
ORDER BY allocated_cost DESC, nc.cluster_id, namespace_uid
LIMIT %d`, group.namespaceUIDSelect, group.namespaceNameSelect, where, group.groupBySuffix, normalizedAnalyticsLimit(query.Limit)), args
}

type analyticsGroupSpec struct {
	namespaceUIDSelect  string
	namespaceNameSelect string
	groupBySuffix       string
}

func analyticsGroup(groupBy, namespaceUIDColumn, namespaceNameColumn string) analyticsGroupSpec {
	if groupBy == "cluster" {
		return analyticsGroupSpec{
			namespaceUIDSelect:  "''",
			namespaceNameSelect: "''",
			groupBySuffix:       "",
		}
	}
	return analyticsGroupSpec{
		namespaceUIDSelect:  namespaceUIDColumn,
		namespaceNameSelect: "if(empty(any(" + namespaceNameColumn + ")), " + namespaceUIDColumn + ", any(" + namespaceNameColumn + "))",
		groupBySuffix:       ", " + namespaceUIDColumn,
	}
}

func analyticsMetricWhere(query AnalyticsQuery, tableAlias string) (string, []any) {
	return analyticsWhere(query, tableAlias, "bucket_start")
}

func analyticsCostWhere(query AnalyticsQuery, tableAlias string) (string, []any) {
	return analyticsWhere(query, tableAlias, "bucket_start")
}

func analyticsWhere(query AnalyticsQuery, tableAlias, timeColumn string) (string, []any) {
	prefix := tableAlias + "."
	clauses := []string{
		prefix + "tenant_id = ?",
		prefix + timeColumn + " >= ?",
		prefix + timeColumn + " < ?",
	}
	args := []any{query.TenantID, query.Start, query.End}
	if query.ClusterID != "" {
		clauses = append(clauses, prefix+"cluster_id = ?")
		args = append(args, query.ClusterID)
	}
	return strings.Join(clauses, " AND "), args
}

func normalizedAnalyticsLimit(limit int) int {
	if limit <= 0 {
		return defaultAnalyticsLimit
	}
	if limit > maxAnalyticsLimit {
		return maxAnalyticsLimit
	}
	return limit
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
