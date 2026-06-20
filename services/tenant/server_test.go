package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTenantProfileRequiresTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(), fixedNow)
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tenant", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestTenantProfileReturnsGatewayTenant(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(), fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tenant", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var profile TenantProfile
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.TenantID != "tenant-a" || profile.Status != "active" || profile.Source != "gateway" {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestTenantMembersCanBeUpsertedListedAndDeleted(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(), fixedNow)
	put := httptest.NewRequest(http.MethodPut, "/api/v1/tenant/members/user-1", bytes.NewBufferString(`{
		"role": "admin",
		"display_name": "User One"
	}`))
	put.Header.Set(tenantHeader, "tenant-a")
	putResponse := httptest.NewRecorder()

	api.Routes().ServeHTTP(putResponse, put)

	if putResponse.Code != http.StatusOK {
		t.Fatalf("put status = %d, body = %s", putResponse.Code, putResponse.Body.String())
	}
	var member Member
	if err := json.Unmarshal(putResponse.Body.Bytes(), &member); err != nil {
		t.Fatal(err)
	}
	if member.TenantID != "tenant-a" || member.PrincipalID != "user-1" || member.Role != "admin" {
		t.Fatalf("member = %#v", member)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/members", nil)
	list.Header.Set(tenantHeader, "tenant-a")
	listResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var result MembersResult
	if err := json.Unmarshal(listResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "tenant-a" || len(result.Members) != 1 || result.Members[0].PrincipalID != "user-1" {
		t.Fatalf("result = %#v", result)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/tenant/members/user-1", nil)
	deleteRequest.Header.Set(tenantHeader, "tenant-a")
	deleteResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteResponse.Code, deleteResponse.Body.String())
	}
}

func TestTenantMembersAreTenantScoped(t *testing.T) {
	t.Parallel()
	store := NewStore()
	store.UpsertMember("tenant-a", "user-1", "viewer", "", fixedNow())
	api := NewAPI(store, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/members", nil)
	request.Header.Set(tenantHeader, "tenant-b")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result MembersResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "tenant-b" || len(result.Members) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestTenantMemberRejectsInvalidRole(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(), fixedNow)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/tenant/members/user-1", bytes.NewBufferString(`{"role":"auditor"}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
