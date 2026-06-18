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
