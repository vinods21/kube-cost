package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	ErrRecommendationNotFound = errors.New("recommendation not found")
	ErrVersionConflict        = errors.New("recommendation version conflict")
	ErrInvalidTransition      = errors.New("invalid recommendation transition")
)

type Repository interface {
	ApplyCommand(context.Context, WorkflowCommand) (WorkflowResult, error)
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
		Compression:     &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
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

func (r *ClickHouseRepository) ApplyCommand(ctx context.Context, command WorkflowCommand) (WorkflowResult, error) {
	current, err := r.recommendation(ctx, command.TenantID, command.RecommendationID)
	if err != nil {
		return WorkflowResult{}, err
	}
	if command.ExpectedVersion != 0 && command.ExpectedVersion != current.Version {
		return WorkflowResult{}, ErrVersionConflict
	}
	if !transitionAllowed(current.Status, command.NextStatus) {
		return WorkflowResult{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, current.Status, command.NextStatus)
	}

	updated := current
	updated.Status = command.NextStatus
	updated.Version = uint64(command.OccurredAt.UnixNano())
	action := ActionReference{
		TenantID:         command.TenantID,
		RecommendationID: command.RecommendationID,
		ActionID:         uuid.NewString(),
		Action:           command.Action,
		Status:           "recorded",
		OccurredAt:       command.OccurredAt,
	}
	if err := r.insertRecommendation(ctx, updated); err != nil {
		return WorkflowResult{}, err
	}
	if err := r.insertAction(ctx, command, action); err != nil {
		return WorkflowResult{}, err
	}
	return WorkflowResult{Recommendation: updated, Action: action}, nil
}

func (r *ClickHouseRepository) recommendation(ctx context.Context, tenantID, recommendationID string) (Recommendation, error) {
	rows, err := r.connection.Query(ctx, recommendationSelectSQL, tenantID, recommendationID)
	if err != nil {
		return Recommendation{}, fmt.Errorf("query recommendation: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Recommendation{}, fmt.Errorf("read recommendation row: %w", err)
		}
		return Recommendation{}, ErrRecommendationNotFound
	}
	recommendation, err := scanRecommendation(rows)
	if err != nil {
		return Recommendation{}, err
	}
	return recommendation, nil
}

func (r *ClickHouseRepository) insertRecommendation(ctx context.Context, recommendation Recommendation) error {
	batch, err := r.connection.PrepareBatch(ctx, "INSERT INTO kube_cost.recommendation ("+joinColumns(recommendationColumns)+")")
	if err != nil {
		return fmt.Errorf("prepare recommendation batch: %w", err)
	}
	if err := batch.Append(recommendationRow(recommendation)...); err != nil {
		return fmt.Errorf("append recommendation row: %w", err)
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send recommendation batch: %w", err)
	}
	return nil
}

func (r *ClickHouseRepository) insertAction(ctx context.Context, command WorkflowCommand, action ActionReference) error {
	details, err := json.Marshal(command.Details)
	if err != nil {
		return fmt.Errorf("encode action details: %w", err)
	}
	batch, err := r.connection.PrepareBatch(ctx, "INSERT INTO kube_cost.recommendation_action ("+joinColumns(actionColumns)+")")
	if err != nil {
		return fmt.Errorf("prepare recommendation action batch: %w", err)
	}
	if err := batch.Append(
		action.TenantID,
		action.RecommendationID,
		uuid.MustParse(action.ActionID),
		action.Action,
		"user",
		command.ActorID,
		command.Reason,
		action.OccurredAt,
		"",
		action.Status,
		string(details),
	); err != nil {
		return fmt.Errorf("append recommendation action row: %w", err)
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send recommendation action batch: %w", err)
	}
	return nil
}

func (r *ClickHouseRepository) Ping(ctx context.Context) error {
	return r.connection.Ping(ctx)
}

func (r *ClickHouseRepository) Close() error {
	return r.connection.Close()
}

func transitionAllowed(current, next string) bool {
	switch next {
	case "approved":
		return current == "open" || current == "acknowledged"
	case "rejected", "suppressed":
		return current == "open" || current == "acknowledged" || current == "approved"
	case "executing":
		return current == "approved"
	default:
		return false
	}
}

func scanRecommendation(scanner interface{ Scan(dest ...any) error }) (Recommendation, error) {
	var recommendation Recommendation
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
		&recommendation.CurrentConfiguration,
		&recommendation.ProposedConfiguration,
		&recommendation.Evidence,
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
		return Recommendation{}, fmt.Errorf("scan recommendation row: %w", err)
	}
	recommendation.MonthlyGrossSavings = monthlyGrossSavings.String()
	recommendation.MonthlyNetSavings = monthlyNetSavings.String()
	recommendation.Confidence = confidence.String()
	recommendation.RiskScore = riskScore.String()
	return recommendation, nil
}

func recommendationRow(recommendation Recommendation) []any {
	return []any{
		recommendation.TenantID,
		recommendation.RecommendationID,
		recommendation.ClusterID,
		recommendation.NamespaceUID,
		recommendation.TargetKind,
		recommendation.TargetUID,
		recommendation.RecommendationType,
		recommendation.SafetyClass,
		recommendation.Status,
		recommendation.AnalysisWindowStart,
		recommendation.AnalysisWindowEnd,
		recommendation.GeneratedAt,
		recommendation.ExpiresAt,
		recommendation.CurrentConfiguration,
		recommendation.ProposedConfiguration,
		recommendation.Evidence,
		recommendation.Currency,
		decimal.RequireFromString(recommendation.MonthlyGrossSavings),
		decimal.RequireFromString(recommendation.MonthlyNetSavings),
		decimal.RequireFromString(recommendation.Confidence),
		decimal.RequireFromString(recommendation.RiskScore),
		recommendation.PolicyVersion,
		recommendation.ModelVersion,
		recommendation.ComputationVersion,
		recommendation.Version,
	}
}

func joinColumns(columns []string) string {
	result := ""
	for index, column := range columns {
		if index > 0 {
			result += ", "
		}
		result += column
	}
	return result
}

var recommendationColumns = []string{
	"tenant_id", "recommendation_id", "cluster_id", "namespace_uid", "target_kind", "target_uid",
	"recommendation_type", "safety_class", "status", "analysis_window_start", "analysis_window_end",
	"generated_at", "expires_at", "current_configuration", "proposed_configuration", "evidence",
	"currency", "monthly_gross_savings", "monthly_net_savings", "confidence", "risk_score",
	"policy_version", "model_version", "computation_version", "version",
}

var actionColumns = []string{
	"tenant_id", "recommendation_id", "action_id", "action", "actor_type", "actor_id",
	"reason", "occurred_at", "execution_id", "result", "details",
}

const recommendationSelectSQL = `
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
WHERE tenant_id = ? AND recommendation_id = ?
ORDER BY generated_at DESC
LIMIT 1`
