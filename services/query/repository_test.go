package main

import (
	"strings"
	"testing"
	"time"
)

func TestUsageSQLFiltersTenantRangeAndCluster(t *testing.T) {
	t.Parallel()
	sql, args := usageSQL(AnalyticsQuery{
		TenantID:  "tenant-a",
		ClusterID: "cluster-a",
		Start:     time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
		GroupBy:   "namespace",
		Limit:     25,
	})
	for _, fragment := range []string{
		"FROM kube_cost.container_metrics_10s AS cm",
		"LEFT JOIN kube_cost.current_namespace AS ns",
		"cm.tenant_id = ?",
		"cm.bucket_start >= ?",
		"cm.bucket_start < ?",
		"cm.cluster_id = ?",
		"GROUP BY cm.tenant_id, cm.cluster_id, cm.namespace_uid",
		"LIMIT 25",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if len(args) != 4 {
		t.Fatalf("args len=%d, want 4", len(args))
	}
}

func TestCostsSQLUsesCurrentNamespaceCostAndCapsLimit(t *testing.T) {
	t.Parallel()
	sql, args := costsSQL(AnalyticsQuery{
		TenantID: "tenant-a",
		Start:    time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
		GroupBy:  "namespace",
		Limit:    1000,
	})
	for _, fragment := range []string{
		"FROM kube_cost.current_namespace_cost_1h AS nc",
		"nc.tenant_id = ?",
		"nc.bucket_start >= ?",
		"nc.bucket_start < ?",
		"sum(nc.allocated_cost) AS allocated_cost",
		"GROUP BY nc.tenant_id, nc.cluster_id, nc.namespace_uid",
		"LIMIT 500",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if len(args) != 3 {
		t.Fatalf("args len=%d, want 3", len(args))
	}
}

func TestAllocationSQLSupportsClusterGrouping(t *testing.T) {
	t.Parallel()
	sql, args := allocationSQL(AnalyticsQuery{
		TenantID: "tenant-a",
		Start:    time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
		GroupBy:  "cluster",
	})
	for _, fragment := range []string{
		"FROM kube_cost.current_namespace_cost_1h AS nc",
		"'' AS namespace_uid",
		"'' AS namespace_name",
		"sum(nc.allocation_weight) AS allocation_weight",
		"GROUP BY nc.tenant_id, nc.cluster_id",
		"LIMIT 100",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if strings.Contains(sql, "GROUP BY nc.tenant_id, nc.cluster_id, nc.namespace_uid") {
		t.Fatalf("cluster grouping should not include namespace_uid:\n%s", sql)
	}
	if len(args) != 3 {
		t.Fatalf("args len=%d, want 3", len(args))
	}
}

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
