package main

import (
	"encoding/json"
	"testing"
	"time"
)

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

func TestExecutionRequestForRecommendation(t *testing.T) {
	t.Parallel()
	requestedAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	recommendation := Recommendation{
		TenantID:              "tenant-a",
		RecommendationID:      "rec-1",
		ClusterID:             "cluster-a",
		NamespaceUID:          "namespace-a",
		TargetKind:            "container",
		TargetUID:             "pod/container",
		RecommendationType:    "rightsizing",
		SafetyClass:           "review_required",
		PolicyVersion:         "policy-v1",
		CurrentConfiguration:  `{"cpu_request_millicores":500}`,
		ProposedConfiguration: `{"cpu_request_millicores":250}`,
		Evidence:              `{"sample_count":720}`,
	}
	action := ActionReference{ActionID: "action-1"}

	request := executionRequestFor(recommendation, action, requestedAt)

	if request.ExecutionID == "" ||
		request.ActionID != "action-1" ||
		request.TargetUID != "pod/container" ||
		request.Status != "pending_executor" ||
		!request.RequestedAt.Equal(requestedAt) {
		t.Fatalf("request = %#v", request)
	}
}

func TestActionDetailsIncludesExecutionRequest(t *testing.T) {
	t.Parallel()
	requestedAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	execution := &ExecutionRequest{
		ExecutionID:      "exec-1",
		TenantID:         "tenant-a",
		RecommendationID: "rec-1",
		ActionID:         "action-1",
		RequestedAt:      requestedAt,
		Status:           "pending_executor",
	}

	payload, err := actionDetails(WorkflowCommand{Details: map[string]any{"ticket": "INC-1"}}, execution)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["ticket"] != "INC-1" {
		t.Fatalf("payload = %#v", decoded)
	}
	embedded, ok := decoded["execution_request"].(map[string]any)
	if !ok || embedded["execution_id"] != "exec-1" || embedded["status"] != "pending_executor" {
		t.Fatalf("payload = %#v", decoded)
	}
}
