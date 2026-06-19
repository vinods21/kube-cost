package main

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestAllocateByCPURequestSplitsNodeCostByNamespaceRequests(t *testing.T) {
	t.Parallel()
	bucket := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	result := allocateByCPURequest([]NodeNamespaceRequest{
		{TenantID: "tenant", ClusterID: "cluster", NodeUID: "node-1", NamespaceUID: "apps", NamespaceName: "apps", BucketStart: bucket, CPURequestCoreMilliseconds: 750},
		{TenantID: "tenant", ClusterID: "cluster", NodeUID: "node-1", NamespaceUID: "platform", NamespaceName: "platform", BucketStart: bucket, CPURequestCoreMilliseconds: 250},
	}, 0.10)

	if len(result) != 2 {
		t.Fatalf("result len=%d, want 2", len(result))
	}
	assertCost(t, result[0], "apps", 0.75, 0.075)
	assertCost(t, result[1], "platform", 0.25, 0.025)
}

func TestAllocateByCPURequestAggregatesNamespaceAcrossNodes(t *testing.T) {
	t.Parallel()
	bucket := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	result := allocateByCPURequest([]NodeNamespaceRequest{
		{TenantID: "tenant", ClusterID: "cluster", NodeUID: "node-1", NamespaceUID: "apps", NamespaceName: "apps", BucketStart: bucket, CPURequestCoreMilliseconds: 500},
		{TenantID: "tenant", ClusterID: "cluster", NodeUID: "node-1", NamespaceUID: "platform", NamespaceName: "platform", BucketStart: bucket, CPURequestCoreMilliseconds: 500},
		{TenantID: "tenant", ClusterID: "cluster", NodeUID: "node-2", NamespaceUID: "apps", NamespaceName: "apps", BucketStart: bucket, CPURequestCoreMilliseconds: 1000},
	}, 0.10)

	if len(result) != 2 {
		t.Fatalf("result len=%d, want 2", len(result))
	}
	assertCost(t, result[0], "apps", 1.5, 0.15)
	assertCost(t, result[1], "platform", 0.5, 0.05)
}

func TestAllocateCostsComputesIdleNetworkControlPlaneAndSystemCosts(t *testing.T) {
	t.Parallel()
	bucket := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	result := allocateCosts([]NodeNamespaceRequest{
		{
			TenantID:                   "tenant",
			ClusterID:                  "cluster",
			NodeUID:                    "node-1",
			NamespaceUID:               "apps",
			NamespaceName:              "apps",
			BucketStart:                bucket,
			CPURequestCoreMilliseconds: 1_800_000,
			NetworkBytes:               1073741824,
			NodeAllocatableMillicores:  1000,
		},
		{
			TenantID:                   "tenant",
			ClusterID:                  "cluster",
			NodeUID:                    "node-1",
			NamespaceUID:               "kube-system",
			NamespaceName:              "kube-system",
			BucketStart:                bucket,
			CPURequestCoreMilliseconds: 900_000,
			NetworkBytes:               536870912,
			NodeAllocatableMillicores:  1000,
		},
	}, AllocationOptions{
		NodeHourlyCostUSD:         0.10,
		ControlPlaneHourlyCostUSD: 0.06,
		NetworkCostPerGiBUSD:      0.02,
	})

	if len(result) != 3 {
		t.Fatalf("result len=%d, want 3: %+v", len(result), result)
	}
	apps := costByNamespace(t, result, "apps")
	assertFloat(t, apps.DirectCost, 0.05)
	assertFloat(t, apps.NetworkCost, 0.02)
	assertFloat(t, apps.ControlPlaneCost, 0.04)
	assertFloat(t, apps.AllocatedCost, 0.11)

	system := costByNamespace(t, result, "kube-system")
	assertFloat(t, system.DirectCost, 0.025)
	assertFloat(t, system.NetworkCost, 0.01)
	assertFloat(t, system.ControlPlaneCost, 0.02)
	assertFloat(t, system.SystemNamespaceCost, 0.055)
	assertFloat(t, system.AllocatedCost, 0.055)

	idle := costByNamespace(t, result, idleNamespaceUID)
	assertFloat(t, idle.IdleCost, 0.025)
	assertFloat(t, idle.AllocatedCost, 0.025)
}

func TestNormalizeQueryDefaultsToLastCompleteHour(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 18, 10, 15, 0, 0, time.UTC)
	query, err := normalizeQuery(Query{TenantID: "tenant"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !query.Start.Equal(time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)) ||
		!query.End.Equal(time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("range=%s/%s", query.Start, query.End)
	}
}

func TestNormalizeQueryRequiresTenant(t *testing.T) {
	t.Parallel()
	_, err := normalizeQuery(Query{}, time.Now())
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("err=%v, want ErrInvalidQuery", err)
	}
}

func TestNormalizeQueryRequiresHourlyAlignment(t *testing.T) {
	t.Parallel()
	_, err := normalizeQuery(Query{
		TenantID: "tenant",
		Start:    time.Date(2026, 6, 18, 9, 15, 0, 0, time.UTC),
		End:      time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
	}, time.Now())
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("err=%v, want ErrInvalidQuery", err)
	}
}

func costByNamespace(t *testing.T, items []NamespaceCost, namespaceUID string) NamespaceCost {
	t.Helper()
	for _, item := range items {
		if item.NamespaceUID == namespaceUID {
			return item
		}
	}
	t.Fatalf("namespace %q not found in %+v", namespaceUID, items)
	return NamespaceCost{}
}

func assertFloat(t *testing.T, actual, expected float64) {
	t.Helper()
	if math.Abs(actual-expected) > 0.000001 {
		t.Fatalf("value=%f, want %f", actual, expected)
	}
}

func assertCost(t *testing.T, item NamespaceCost, namespace string, weight, cost float64) {
	t.Helper()
	if item.NamespaceUID != namespace {
		t.Fatalf("namespace=%s, want %s", item.NamespaceUID, namespace)
	}
	if math.Abs(item.AllocationWeight-weight) > 0.000001 {
		t.Fatalf("weight=%f, want %f", item.AllocationWeight, weight)
	}
	if math.Abs(item.AllocatedCost-cost) > 0.000001 {
		t.Fatalf("cost=%f, want %f", item.AllocatedCost, cost)
	}
	if item.Currency != defaultCurrency || item.AllocationMethod != allocationMethodCPU {
		t.Fatalf("metadata currency=%s method=%s", item.Currency, item.AllocationMethod)
	}
}

type fakeRepository struct {
	items []NamespaceCost
	err   error
}

func (r *fakeRepository) NamespaceCosts(context.Context, Query) ([]NamespaceCost, error) {
	return r.items, r.err
}

func (r *fakeRepository) Ping(context.Context) error {
	return r.err
}

func (r *fakeRepository) Close() error {
	return nil
}
