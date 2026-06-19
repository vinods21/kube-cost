package main

import (
	"strings"
	"testing"
	"time"
)

func TestNamespaceCostSQLUsesMetricsAndNodeInventory(t *testing.T) {
	t.Parallel()
	query := Query{
		TenantID:  "tenant",
		ClusterID: "cluster",
		Start:     time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
	}
	sql, args := namespaceCostSQL(query, AllocationOptions{
		NodeHourlyCostUSD:         0.10,
		ControlPlaneHourlyCostUSD: 0.05,
		NetworkCostPerGiBUSD:      0.01,
	})

	for _, fragment := range []string{
		"kube_cost.container_metrics_10s",
		"kube_cost.current_node",
		"kube_cost.current_namespace",
		"cpu_request_core_milliseconds",
		"network_rx_bytes + network_tx_bytes",
		"allocatable_cpu_millicores",
		"'__idle__'",
		"toStartOfHour(bucket_start)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if len(args) != 8 {
		t.Fatalf("args len=%d, want 8: %#v", len(args), args)
	}
}

func TestMetricWhereOmitsClusterWhenUnspecified(t *testing.T) {
	t.Parallel()
	where, args := metricWhere(Query{
		TenantID: "tenant",
		Start:    time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
	})
	if strings.Contains(where, "cluster_id = ?") {
		t.Fatalf("where should not require cluster: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("args len=%d, want 3", len(args))
	}
}
