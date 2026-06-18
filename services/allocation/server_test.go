package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNamespaceCostEndpointReturnsCosts(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{items: []NamespaceCost{{
		TenantID:                   "tenant",
		ClusterID:                  "cluster",
		NamespaceUID:               "apps",
		NamespaceName:              "apps",
		BucketStart:                "2026-06-18T09:00:00Z",
		CPURequestCoreMilliseconds: 1000,
		AllocationWeight:           1,
		AllocatedCost:              0.10,
	}}}
	api := NewAPI(NewEngine(repository, 0.10))
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/namespaces/cost?tenant_id=tenant&cluster_id=cluster&start=2026-06-18T09:00:00Z&end=2026-06-18T10:00:00Z",
		nil,
	)
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Currency != defaultCurrency || result.AllocationMethod != allocationMethodCPU {
		t.Fatalf("metadata currency=%s method=%s", result.Currency, result.AllocationMethod)
	}
	if len(result.Items) != 1 || result.Items[0].NamespaceUID != "apps" {
		t.Fatalf("items=%+v", result.Items)
	}
	if !result.Start.Equal(time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)) ||
		!result.End.Equal(time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("range=%s/%s", result.Start, result.End)
	}
}

func TestNamespaceCostEndpointRequiresTenant(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewEngine(&fakeRepository{}, 0.10))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/cost", nil)
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHealthEndpointChecksRepository(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewEngine(&fakeRepository{}, 0.10))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
