package main

import "testing"

func TestTransitionAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current string
		next    string
		allowed bool
	}{
		{current: "open", next: "approved", allowed: true},
		{current: "acknowledged", next: "approved", allowed: true},
		{current: "approved", next: "executing", allowed: true},
		{current: "approved", next: "rejected", allowed: true},
		{current: "approved", next: "suppressed", allowed: true},
		{current: "open", next: "executing", allowed: false},
		{current: "rejected", next: "approved", allowed: false},
		{current: "open", next: "applied", allowed: false},
	}
	for _, test := range tests {
		if got := transitionAllowed(test.current, test.next); got != test.allowed {
			t.Fatalf("transitionAllowed(%q, %q)=%v, want %v", test.current, test.next, got, test.allowed)
		}
	}
}

func TestRecommendationRowMatchesColumns(t *testing.T) {
	t.Parallel()
	recommendation := Recommendation{
		TenantID:              "tenant-a",
		RecommendationID:      "rec-1",
		ClusterID:             "cluster-a",
		TargetKind:            "container",
		TargetUID:             "pod/container",
		RecommendationType:    "rightsizing",
		SafetyClass:           "review_required",
		Status:                "approved",
		CurrentConfiguration:  "{}",
		ProposedConfiguration: "{}",
		Evidence:              "{}",
		Currency:              "USD",
		MonthlyGrossSavings:   "1.25",
		MonthlyNetSavings:     "1.25",
		Confidence:            "0.7",
		RiskScore:             "0.3",
		ModelVersion:          "optimization-v1",
		ComputationVersion:    "optimization-v1",
		Version:               2,
	}
	if len(recommendationRow(recommendation)) != len(recommendationColumns) {
		t.Fatalf("row len=%d columns=%d", len(recommendationRow(recommendation)), len(recommendationColumns))
	}
	if joinColumns(actionColumns) == "" {
		t.Fatal("action columns should not be empty")
	}
}
