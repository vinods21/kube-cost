package main

import "time"

const tenantHeader = "X-Kube-Cost-Tenant-ID"

type Recommendation struct {
	TenantID              string    `json:"tenant_id"`
	RecommendationID      string    `json:"recommendation_id"`
	ClusterID             string    `json:"cluster_id"`
	NamespaceUID          string    `json:"namespace_uid,omitempty"`
	TargetKind            string    `json:"target_kind"`
	TargetUID             string    `json:"target_uid"`
	RecommendationType    string    `json:"recommendation_type"`
	SafetyClass           string    `json:"safety_class"`
	Status                string    `json:"status"`
	AnalysisWindowStart   time.Time `json:"analysis_window_start"`
	AnalysisWindowEnd     time.Time `json:"analysis_window_end"`
	GeneratedAt           time.Time `json:"generated_at"`
	ExpiresAt             time.Time `json:"expires_at"`
	CurrentConfiguration  string    `json:"current_configuration"`
	ProposedConfiguration string    `json:"proposed_configuration"`
	Evidence              string    `json:"evidence"`
	Currency              string    `json:"currency"`
	MonthlyGrossSavings   string    `json:"monthly_gross_savings"`
	MonthlyNetSavings     string    `json:"monthly_net_savings"`
	Confidence            string    `json:"confidence"`
	RiskScore             string    `json:"risk_score"`
	PolicyVersion         string    `json:"policy_version,omitempty"`
	ModelVersion          string    `json:"model_version"`
	ComputationVersion    string    `json:"computation_version"`
	Version               uint64    `json:"version"`
}

type CommandRequest struct {
	ActorID         string         `json:"actor_id"`
	Reason          string         `json:"reason"`
	ExpectedVersion uint64         `json:"expected_version"`
	Details         map[string]any `json:"details,omitempty"`
}

type ActionReference struct {
	TenantID         string    `json:"tenant_id"`
	RecommendationID string    `json:"recommendation_id"`
	ActionID         string    `json:"action_id"`
	Action           string    `json:"action"`
	Status           string    `json:"status"`
	OccurredAt       time.Time `json:"occurred_at"`
}

type WorkflowResult struct {
	Recommendation Recommendation  `json:"recommendation"`
	Action         ActionReference `json:"action"`
}

type WorkflowCommand struct {
	TenantID         string
	RecommendationID string
	Action           string
	NextStatus       string
	ActorID          string
	Reason           string
	ExpectedVersion  uint64
	Details          map[string]any
	OccurredAt       time.Time
}
