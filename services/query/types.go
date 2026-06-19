package main

import "time"

const (
	tenantHeader             = "X-Kube-Cost-Tenant-ID"
	defaultFreshnessWindow   = 10 * time.Minute
	defaultStaleStatus       = "stale"
	defaultFreshStatus       = "fresh"
	defaultEmptyStatus       = "empty"
	computationVersionDataQ1 = "data-quality-v1"
)

type DataQualityQuery struct {
	TenantID        string
	ClusterID       string
	FreshnessWindow time.Duration
}

type RecommendationQuery struct {
	TenantID              string
	ClusterID             string
	Status                string
	RecommendationType    string
	TargetKind            string
	TargetUID             string
	MinimumMonthlySavings string
	Limit                 int
}

type RecommendationResult struct {
	TenantID              string         `json:"tenant_id"`
	RecommendationID      string         `json:"recommendation_id"`
	ClusterID             string         `json:"cluster_id"`
	NamespaceUID          string         `json:"namespace_uid,omitempty"`
	TargetKind            string         `json:"target_kind"`
	TargetUID             string         `json:"target_uid"`
	RecommendationType    string         `json:"recommendation_type"`
	SafetyClass           string         `json:"safety_class"`
	Status                string         `json:"status"`
	AnalysisWindowStart   time.Time      `json:"analysis_window_start"`
	AnalysisWindowEnd     time.Time      `json:"analysis_window_end"`
	GeneratedAt           time.Time      `json:"generated_at"`
	ExpiresAt             time.Time      `json:"expires_at"`
	CurrentConfiguration  jsonRawMessage `json:"current_configuration"`
	ProposedConfiguration jsonRawMessage `json:"proposed_configuration"`
	Evidence              jsonRawMessage `json:"evidence"`
	Currency              string         `json:"currency"`
	MonthlyGrossSavings   string         `json:"monthly_gross_savings"`
	MonthlyNetSavings     string         `json:"monthly_net_savings"`
	Confidence            string         `json:"confidence"`
	RiskScore             string         `json:"risk_score"`
	PolicyVersion         string         `json:"policy_version,omitempty"`
	ModelVersion          string         `json:"model_version"`
	ComputationVersion    string         `json:"computation_version"`
	Version               uint64         `json:"version"`
}

type RecommendationListResult struct {
	TenantID        string                 `json:"tenant_id"`
	ClusterID       string                 `json:"cluster_id,omitempty"`
	GeneratedAt     time.Time              `json:"generated_at"`
	Recommendations []RecommendationResult `json:"recommendations"`
	ResultCount     int                    `json:"result_count"`
	Limit           int                    `json:"limit"`
}

type DataQualityResult struct {
	TenantID           string              `json:"tenant_id"`
	ClusterID          string              `json:"cluster_id,omitempty"`
	GeneratedAt        time.Time           `json:"generated_at"`
	DataThrough        *time.Time          `json:"data_through,omitempty"`
	ComputationVersion string              `json:"computation_version"`
	Signals            []DataQualitySignal `json:"signals"`
	Quality            QualitySummary      `json:"quality"`
}

type DataQualitySignal struct {
	Source                 string     `json:"source"`
	Grain                  string     `json:"grain"`
	ClusterID              string     `json:"cluster_id,omitempty"`
	RecordCount            uint64     `json:"record_count"`
	LatestBucketStart      *time.Time `json:"latest_bucket_start,omitempty"`
	LatestIngestedAt       *time.Time `json:"latest_ingested_at,omitempty"`
	FreshnessSeconds       *int64     `json:"freshness_seconds,omitempty"`
	Status                 string     `json:"status"`
	Warning                string     `json:"warning,omitempty"`
	ExpectedFreshnessLimit string     `json:"expected_freshness_limit"`
}

type QualitySummary struct {
	Status              string   `json:"status"`
	EstimatedPercent    float64  `json:"estimated_percent"`
	MissingScopes       []string `json:"missing_scopes,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
	FreshnessWindowSecs int64    `json:"freshness_window_seconds"`
}

type jsonRawMessage []byte

func (m jsonRawMessage) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	return m, nil
}

func (m *jsonRawMessage) UnmarshalJSON(data []byte) error {
	*m = append((*m)[0:0], data...)
	return nil
}
