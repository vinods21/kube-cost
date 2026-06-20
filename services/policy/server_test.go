package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListPoliciesRequiresTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(), fixedNow)
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestCreateActivateAndListPolicyVersion(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(), fixedNow)
	create := httptest.NewRequest(http.MethodPost, "/api/v1/policies/allocation/versions", bytes.NewBufferString(`{
		"version": "v1",
		"description": "default allocation policy",
		"effective_start": "2026-06-19T00:00:00Z",
		"rules": {"idle": "separate"}
	}`))
	create.Header.Set(tenantHeader, "tenant-a")
	create.Header.Set(principalHeader, "user-a")
	createResponse := httptest.NewRecorder()

	api.Routes().ServeHTTP(createResponse, create)

	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	var version PolicyVersion
	if err := json.Unmarshal(createResponse.Body.Bytes(), &version); err != nil {
		t.Fatal(err)
	}
	if version.TenantID != "tenant-a" ||
		version.Family != "allocation" ||
		version.Version != "v1" ||
		version.Status != "draft" ||
		version.CreatedBy != "user-a" {
		t.Fatalf("version = %#v", version)
	}

	activate := httptest.NewRequest(http.MethodPost, "/api/v1/policies/allocation/versions/v1/activate", nil)
	activate.Header.Set(tenantHeader, "tenant-a")
	activate.Header.Set(principalHeader, "user-a")
	activateResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(activateResponse, activate)
	if activateResponse.Code != http.StatusOK {
		t.Fatalf("activate status = %d, body = %s", activateResponse.Code, activateResponse.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil)
	list.Header.Set(tenantHeader, "tenant-a")
	listResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var result PolicyFamiliesResult
	if err := json.Unmarshal(listResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "tenant-a" ||
		result.ResultCount != 1 ||
		result.Families[0].ActiveVersion != "v1" ||
		result.Families[0].Versions[0].Status != "active" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPoliciesAreTenantScoped(t *testing.T) {
	t.Parallel()
	store := NewStore()
	_, err := store.CreateVersion("tenant-a", "allocation", "user-a", VersionRequest{
		Version: "v1",
		Rules:   json.RawMessage(`{"idle":"separate"}`),
	}, fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	api := NewAPI(store, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil)
	request.Header.Set(tenantHeader, "tenant-b")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result PolicyFamiliesResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "tenant-b" || result.ResultCount != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCreatePolicyRejectsInvalidRules(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(), fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/policies/allocation/versions", bytes.NewBufferString(`{
		"version": "v1"
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestCreatePolicyRejectsDuplicateVersion(t *testing.T) {
	t.Parallel()
	store := NewStore()
	_, err := store.CreateVersion("tenant-a", "allocation", "user-a", VersionRequest{
		Version: "v1",
		Rules:   json.RawMessage(`{"idle":"separate"}`),
	}, fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	api := NewAPI(store, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/policies/allocation/versions", bytes.NewBufferString(`{
		"version": "v1",
		"rules": {"idle": "shared"}
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", response.Code)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
