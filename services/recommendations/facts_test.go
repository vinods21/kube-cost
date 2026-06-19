package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRecommendationFactsMapToClickHouseRows(t *testing.T) {
	t.Parallel()
	analysisStart := time.Date(2026, 5, 19, 10, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60))
	analysisEnd := time.Date(2026, 6, 18, 10, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60))
	generatedAt := time.Date(2026, 6, 18, 11, 30, 0, 123456789, time.FixedZone("IST", 5*60*60+30*60))
	recommendation := Recommendation{
		TenantID:                        "tenant-a",
		ClusterID:                       "cluster-a",
		ScopeType:                       "container",
		ScopeID:                         "pod-uid/container-a",
		AnalysisWindowStart:             analysisStart,
		AnalysisWindowEnd:               analysisEnd,
		CPUUsageP95Millicores:           95,
		MemoryWorkingSetP99Bytes:        99 * 1024 * 1024,
		CurrentCPURequestMillicores:     500,
		RecommendedCPURequestMillicores: 110,
		CurrentMemoryRequestBytes:       512 * 1024 * 1024,
		RecommendedMemoryRequestBytes:   128 * 1024 * 1024,
		CurrentCPULimitMillicores:       1000,
		RecommendedCPULimitMillicores:   220,
		CurrentMemoryLimitBytes:         1024 * 1024 * 1024,
		RecommendedMemoryLimitBytes:     192 * 1024 * 1024,
		EstimatedMonthlySavingsUSD:      8.125,
		SampleCount:                     720,
		ComputationVersion:              computationVersionV1,
	}

	facts, err := recommendationFacts([]Recommendation{recommendation}, generatedAt)
	if err != nil {
		t.Fatalf("recommendationFacts returned error: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("facts len=%d, want 1", len(facts))
	}
	fact := facts[0]
	if fact.RecommendationID == "" {
		t.Fatal("recommendation id should be set")
	}
	if fact.RecommendationID != recommendationID(recommendation) {
		t.Fatalf("recommendation id=%s, want %s", fact.RecommendationID, recommendationID(recommendation))
	}
	if fact.TargetKind != "container" || fact.TargetUID != "pod-uid/container-a" {
		t.Fatalf("target=%s/%s", fact.TargetKind, fact.TargetUID)
	}
	if fact.RecommendationType != "rightsizing" || fact.SafetyClass != "review_required" || fact.Status != "open" {
		t.Fatalf("workflow defaults=%s/%s/%s", fact.RecommendationType, fact.SafetyClass, fact.Status)
	}
	if fact.AnalysisWindowStart.Location() != time.UTC || fact.GeneratedAt.Location() != time.UTC {
		t.Fatalf("timestamps should be normalized to UTC")
	}
	if !fact.ExpiresAt.Equal(fact.GeneratedAt.Add(defaultRecommendationTTL)) {
		t.Fatalf("expires_at=%s generated_at=%s", fact.ExpiresAt, fact.GeneratedAt)
	}
	if fact.MonthlyGrossSavings.String() != "8.125" || fact.MonthlyNetSavings.String() != "8.125" {
		t.Fatalf("savings gross=%s net=%s", fact.MonthlyGrossSavings, fact.MonthlyNetSavings)
	}
	if fact.Confidence.String() != "0.7" || fact.RiskScore.String() != "0.3" {
		t.Fatalf("confidence/risk=%s/%s", fact.Confidence, fact.RiskScore)
	}
	if fact.ModelVersion != computationVersionV1 || fact.ComputationVersion != computationVersionV1 {
		t.Fatalf("versions=%s/%s", fact.ModelVersion, fact.ComputationVersion)
	}
	if fact.Version != uint64(fact.GeneratedAt.UnixNano()) {
		t.Fatalf("version=%d generated_at=%d", fact.Version, fact.GeneratedAt.UnixNano())
	}
	if len(fact.row()) != len(recommendationColumns) {
		t.Fatalf("row has %d values for %d columns", len(fact.row()), len(recommendationColumns))
	}

	var proposed map[string]uint64
	if err := json.Unmarshal([]byte(fact.ProposedConfig), &proposed); err != nil {
		t.Fatalf("decode proposed config: %v", err)
	}
	if proposed["cpu_request_millicores"] != 110 || proposed["memory_limit_bytes"] != 192*1024*1024 {
		t.Fatalf("proposed config=%v", proposed)
	}

	var evidence map[string]uint64
	if err := json.Unmarshal([]byte(fact.Evidence), &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if evidence["sample_count"] != 720 || evidence["cpu_usage_p95_millicores"] != 95 {
		t.Fatalf("evidence=%v", evidence)
	}
}

func TestRecommendationIDChangesWithComputationVersion(t *testing.T) {
	t.Parallel()
	base := Recommendation{
		TenantID:            "tenant",
		ClusterID:           "cluster",
		ScopeType:           "container",
		ScopeID:             "pod/container",
		AnalysisWindowStart: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		AnalysisWindowEnd:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		ComputationVersion:  computationVersionV1,
	}
	changed := base
	changed.ComputationVersion = "optimization-v2"
	if recommendationID(base) == recommendationID(changed) {
		t.Fatal("recommendation id should change with computation version")
	}
}
