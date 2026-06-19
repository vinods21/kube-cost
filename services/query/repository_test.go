package main

import (
	"strings"
	"testing"
)

func TestRecommendationsSQLFiltersTenantAndRecommendationFields(t *testing.T) {
	t.Parallel()
	sql, args, err := recommendationsSQL(RecommendationQuery{
		TenantID:              "tenant-a",
		ClusterID:             "cluster-a",
		Status:                "open",
		RecommendationType:    "rightsizing",
		TargetKind:            "container",
		TargetUID:             "pod/container",
		MinimumMonthlySavings: "5.00",
		Limit:                 25,
	})
	if err != nil {
		t.Fatalf("recommendationsSQL returned error: %v", err)
	}

	for _, fragment := range []string{
		"FROM kube_cost.recommendation FINAL",
		"tenant_id = ?",
		"cluster_id = ?",
		"status = ?",
		"recommendation_type = ?",
		"target_kind = ?",
		"target_uid = ?",
		"monthly_net_savings >= ?",
		"ORDER BY monthly_net_savings DESC, generated_at DESC, recommendation_id",
		"LIMIT 25",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if len(args) != 7 {
		t.Fatalf("args len=%d, want 7", len(args))
	}
}

func TestRecommendationsSQLCapsLimit(t *testing.T) {
	t.Parallel()
	sql, args, err := recommendationsSQL(RecommendationQuery{TenantID: "tenant-a", Limit: 1000})
	if err != nil {
		t.Fatalf("recommendationsSQL returned error: %v", err)
	}
	if !strings.Contains(sql, "LIMIT 500") {
		t.Fatalf("SQL should cap limit at 500:\n%s", sql)
	}
	if len(args) != 1 || args[0] != "tenant-a" {
		t.Fatalf("args = %#v", args)
	}
}

func TestRecommendationsSQLRejectsInvalidMinimumSavings(t *testing.T) {
	t.Parallel()
	if _, _, err := recommendationsSQL(RecommendationQuery{TenantID: "tenant-a", MinimumMonthlySavings: "bad"}); err == nil {
		t.Fatal("recommendationsSQL should reject invalid minimum savings")
	}
}
