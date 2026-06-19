package main

import (
	"strings"
	"testing"
)

func TestBackfillInventorySQLRewritesNamespaceUIDAndBumpsVersion(t *testing.T) {
	t.Parallel()
	sql, args := backfillInventorySQL(inventoryTables[0], config{
		TenantID:  "tenant-a",
		ClusterID: "cluster-a",
	})
	for _, fragment := range []string{
		"INSERT INTO kube_cost.deployment",
		"ns.namespace_uid",
		"toUInt64(t.version + 1000000000000)",
		"INNER JOIN kube_cost.current_namespace AS ns",
		"t.namespace_uid = ns.namespace_name",
		"ns.namespace_uid != t.namespace_uid",
		"t.tenant_id = ?",
		"t.cluster_id = ?",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if len(args) != 2 || args[0] != "tenant-a" || args[1] != "cluster-a" {
		t.Fatalf("args = %#v", args)
	}
}

func TestCountMetricImpactSQLUsesNamespaceNameJoin(t *testing.T) {
	t.Parallel()
	sql, args := countMetricImpactSQL("container_metrics_10s", config{})
	for _, fragment := range []string{
		"FROM kube_cost.container_metrics_10s AS t",
		"INNER JOIN kube_cost.current_namespace AS ns",
		"t.namespace_uid = ns.namespace_name",
		"ns.namespace_uid != t.namespace_uid",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v", args)
	}
}

func TestSelectedTablesRejectsUnknownTable(t *testing.T) {
	t.Parallel()
	if _, err := selectedTables("namespace"); err == nil {
		t.Fatal("selectedTables should reject unsupported table")
	}
}

func TestParseTokenStyleEnvBool(t *testing.T) {
	t.Setenv("LINEAGE_NORMALIZER_TEST_BOOL", "yes")
	if !envBool("LINEAGE_NORMALIZER_TEST_BOOL", false) {
		t.Fatal("envBool should parse yes")
	}
}
