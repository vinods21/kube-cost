package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	defaultRecommendationTTL = 30 * 24 * time.Hour
	recommendationType       = "rightsizing"
	recommendationSafety     = "review_required"
	recommendationStatus     = "open"
	recommendationCurrency   = "USD"
	recommendationConfidence = 0.70
	recommendationRiskScore  = 0.30
)

var recommendationColumns = []string{
	"tenant_id",
	"recommendation_id",
	"cluster_id",
	"namespace_uid",
	"target_kind",
	"target_uid",
	"recommendation_type",
	"safety_class",
	"status",
	"analysis_window_start",
	"analysis_window_end",
	"generated_at",
	"expires_at",
	"current_configuration",
	"proposed_configuration",
	"evidence",
	"currency",
	"monthly_gross_savings",
	"monthly_net_savings",
	"confidence",
	"risk_score",
	"policy_version",
	"model_version",
	"computation_version",
	"version",
}

type recommendationFact struct {
	TenantID             string
	RecommendationID     string
	ClusterID            string
	NamespaceUID         string
	TargetKind           string
	TargetUID            string
	RecommendationType   string
	SafetyClass          string
	Status               string
	AnalysisWindowStart  time.Time
	AnalysisWindowEnd    time.Time
	GeneratedAt          time.Time
	ExpiresAt            time.Time
	CurrentConfiguration string
	ProposedConfig       string
	Evidence             string
	Currency             string
	MonthlyGrossSavings  decimal.Decimal
	MonthlyNetSavings    decimal.Decimal
	Confidence           decimal.Decimal
	RiskScore            decimal.Decimal
	PolicyVersion        string
	ModelVersion         string
	ComputationVersion   string
	Version              uint64
}

func recommendationFacts(recommendations []Recommendation, generatedAt time.Time) ([]recommendationFact, error) {
	generatedAt = generatedAt.UTC()
	facts := make([]recommendationFact, 0, len(recommendations))
	for _, recommendation := range recommendations {
		currentConfiguration, err := json.Marshal(map[string]uint64{
			"cpu_request_millicores": recommendation.CurrentCPURequestMillicores,
			"cpu_limit_millicores":   recommendation.CurrentCPULimitMillicores,
			"memory_request_bytes":   recommendation.CurrentMemoryRequestBytes,
			"memory_limit_bytes":     recommendation.CurrentMemoryLimitBytes,
		})
		if err != nil {
			return nil, fmt.Errorf("encode current recommendation configuration: %w", err)
		}
		proposedConfiguration, err := json.Marshal(map[string]uint64{
			"cpu_request_millicores": recommendation.RecommendedCPURequestMillicores,
			"cpu_limit_millicores":   recommendation.RecommendedCPULimitMillicores,
			"memory_request_bytes":   recommendation.RecommendedMemoryRequestBytes,
			"memory_limit_bytes":     recommendation.RecommendedMemoryLimitBytes,
		})
		if err != nil {
			return nil, fmt.Errorf("encode proposed recommendation configuration: %w", err)
		}
		evidence, err := json.Marshal(map[string]any{
			"cpu_usage_p95_millicores":     recommendation.CPUUsageP95Millicores,
			"memory_working_set_p99_bytes": recommendation.MemoryWorkingSetP99Bytes,
			"sample_count":                 recommendation.SampleCount,
		})
		if err != nil {
			return nil, fmt.Errorf("encode recommendation evidence: %w", err)
		}

		savings := decimal.NewFromFloat(recommendation.EstimatedMonthlySavingsUSD)
		facts = append(facts, recommendationFact{
			TenantID:             recommendation.TenantID,
			RecommendationID:     recommendationID(recommendation),
			ClusterID:            recommendation.ClusterID,
			TargetKind:           recommendation.ScopeType,
			TargetUID:            recommendation.ScopeID,
			RecommendationType:   recommendationType,
			SafetyClass:          recommendationSafety,
			Status:               recommendationStatus,
			AnalysisWindowStart:  recommendation.AnalysisWindowStart.UTC(),
			AnalysisWindowEnd:    recommendation.AnalysisWindowEnd.UTC(),
			GeneratedAt:          generatedAt,
			ExpiresAt:            generatedAt.Add(defaultRecommendationTTL),
			CurrentConfiguration: string(currentConfiguration),
			ProposedConfig:       string(proposedConfiguration),
			Evidence:             string(evidence),
			Currency:             recommendationCurrency,
			MonthlyGrossSavings:  savings,
			MonthlyNetSavings:    savings,
			Confidence:           decimal.NewFromFloat(recommendationConfidence),
			RiskScore:            decimal.NewFromFloat(recommendationRiskScore),
			ModelVersion:         computationVersionV1,
			ComputationVersion:   recommendation.ComputationVersion,
			Version:              uint64(generatedAt.UnixNano()),
		})
	}
	return facts, nil
}

func recommendationID(recommendation Recommendation) string {
	key := strings.Join([]string{
		recommendation.TenantID,
		recommendation.ClusterID,
		recommendation.ScopeType,
		recommendation.ScopeID,
		recommendation.AnalysisWindowStart.UTC().Format(time.RFC3339Nano),
		recommendation.AnalysisWindowEnd.UTC().Format(time.RFC3339Nano),
		recommendation.ComputationVersion,
	}, "\x00")
	return uuid.NewHash(sha1.New(), uuid.NameSpaceOID, []byte(key), 5).String()
}

func (fact recommendationFact) row() []any {
	return []any{
		fact.TenantID,
		fact.RecommendationID,
		fact.ClusterID,
		fact.NamespaceUID,
		fact.TargetKind,
		fact.TargetUID,
		fact.RecommendationType,
		fact.SafetyClass,
		fact.Status,
		fact.AnalysisWindowStart,
		fact.AnalysisWindowEnd,
		fact.GeneratedAt,
		fact.ExpiresAt,
		fact.CurrentConfiguration,
		fact.ProposedConfig,
		fact.Evidence,
		fact.Currency,
		fact.MonthlyGrossSavings,
		fact.MonthlyNetSavings,
		fact.Confidence,
		fact.RiskScore,
		fact.PolicyVersion,
		fact.ModelVersion,
		fact.ComputationVersion,
		fact.Version,
	}
}
