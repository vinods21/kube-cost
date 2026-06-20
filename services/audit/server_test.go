package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAppendAuditEventRequiresTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(10), fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", bytes.NewBufferString(validEventJSON()))
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestAppendAndListAuditEvents(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(10), fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", bytes.NewBufferString(validEventJSON()))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("append status = %d, body = %s", response.Code, response.Body.String())
	}
	var event Event
	if err := json.Unmarshal(response.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.AuditID == "" ||
		event.TenantID != "tenant-a" ||
		event.ActorID != "user-1" ||
		event.Action != "cluster.create" ||
		event.Outcome != "succeeded" {
		t.Fatalf("event = %#v", event)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?resource_type=cluster&resource_id=cluster-a", nil)
	list.Header.Set(tenantHeader, "tenant-a")
	listResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var result EventsResult
	if err := json.Unmarshal(listResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "tenant-a" || result.Limit != 100 || len(result.Events) != 1 || result.Events[0].AuditID != event.AuditID {
		t.Fatalf("result = %#v", result)
	}
}

func TestAuditEventsAreTenantScoped(t *testing.T) {
	t.Parallel()
	store := NewStore(10)
	store.Append("tenant-a", EventRequest{
		ActorID:      "user-1",
		Action:       "cluster.create",
		ResourceType: "cluster",
		ResourceID:   "cluster-a",
		Outcome:      "succeeded",
	}, fixedNow())
	api := NewAPI(store, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
	request.Header.Set(tenantHeader, "tenant-b")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result EventsResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "tenant-b" || len(result.Events) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestAppendAuditEventRejectsInvalidRequest(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(10), fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", bytes.NewBufferString(`{
		"actor_id": "user-1",
		"action": "cluster.create",
		"resource_type": "cluster",
		"resource_id": "cluster-a",
		"outcome": "unknown"
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestAuditEventLimitIsCapped(t *testing.T) {
	t.Parallel()
	filter, ok := eventFilter(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?limit=999", nil))
	if !ok {
		t.Fatal("filter should be valid")
	}
	if filter.Limit != 500 {
		t.Fatalf("limit = %d, want 500", filter.Limit)
	}
}

func validEventJSON() string {
	return `{
		"actor_id": "user-1",
		"action": "cluster.create",
		"resource_type": "cluster",
		"resource_id": "cluster-a",
		"outcome": "succeeded",
		"details": {"source": "test"}
	}`
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
