package main

import (
	"strings"
	"testing"
	"time"
)

func TestSamplesSQLReadsHourlyContainerMetrics(t *testing.T) {
	t.Parallel()
	sql, args := samplesSQL(Query{
		TenantID:  "tenant",
		ClusterID: "cluster",
		End:       time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
	}, 30*24*time.Hour)

	for _, fragment := range []string{
		"kube_cost.scope_metrics_1h",
		"scope_type = 'container'",
		"cpu_usage_core_milliseconds",
		"memory_working_set_byte_seconds",
		"bucket_start >= ?",
		"bucket_start < ?",
		"cluster_id = ?",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if len(args) != 4 {
		t.Fatalf("args len=%d, want 4", len(args))
	}
}

func TestSampleWhereOmitsClusterWhenUnspecified(t *testing.T) {
	t.Parallel()
	where, args := sampleWhere(Query{
		TenantID: "tenant",
		End:      time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
	}, 30*24*time.Hour)
	if strings.Contains(where, "cluster_id = ?") {
		t.Fatalf("where should not include cluster filter: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("args len=%d, want 3", len(args))
	}
}
