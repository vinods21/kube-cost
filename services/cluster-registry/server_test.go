package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterClusterUsesAuthenticatedTenant(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewMemoryRepository(), fixedToken("token"))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{
		"cluster_name": "prod-usw2",
		"provider": "aws",
		"capabilities": ["metrics", "metrics", "inventory"],
		"labels": {" team ": " platform "}
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body RegisterClusterResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Cluster.TenantID != "tenant-a" || body.Cluster.ClusterName != "prod-usw2" {
		t.Fatalf("cluster = %#v", body.Cluster)
	}
	if body.EnrollmentToken != "token" || body.TokenExpiresAt.IsZero() {
		t.Fatalf("enrollment response = %#v", body)
	}
	if got := body.Cluster.Capabilities; len(got) != 2 || got[0] != "metrics" || got[1] != "inventory" {
		t.Fatalf("capabilities = %#v", got)
	}
	if body.Cluster.Labels["team"] != "platform" {
		t.Fatalf("labels = %#v", body.Cluster.Labels)
	}
}

func TestRegisterClusterRejectsTenantInBody(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewMemoryRepository(), fixedToken("token"))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{
		"tenant_id": "spoofed",
		"cluster_name": "prod"
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %q", response.Header().Get("Content-Type"))
	}
}

func TestClusterAPIsRequireTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewMemoryRepository(), fixedToken("token"))
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestListAndGetClustersAreTenantScoped(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewMemoryRepository(), fixedToken("token"))
	register(t, api, "tenant-a", "cluster-a")
	register(t, api, "tenant-b", "cluster-b")

	list := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	listRequest.Header.Set(tenantHeader, "tenant-a")
	api.Routes().ServeHTTP(list, listRequest)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}
	var listed struct {
		Data []Cluster `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].ClusterName != "cluster-a" {
		t.Fatalf("listed clusters = %#v", listed.Data)
	}

	get := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+listed.Data[0].ClusterID, nil)
	getRequest.Header.Set(tenantHeader, "tenant-a")
	api.Routes().ServeHTTP(get, getRequest)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", get.Code, get.Body.String())
	}

	crossTenant := httptest.NewRecorder()
	crossTenantRequest := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+listed.Data[0].ClusterID, nil)
	crossTenantRequest.Header.Set(tenantHeader, "tenant-b")
	api.Routes().ServeHTTP(crossTenant, crossTenantRequest)
	if crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant status = %d, want 404", crossTenant.Code)
	}
}

func register(t *testing.T, api *API, tenantID, clusterName string) RegisterClusterResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", strings.NewReader(`{"cluster_name":"`+clusterName+`"}`))
	request.Header.Set(tenantHeader, tenantID)
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", response.Code, response.Body.String())
	}
	var body RegisterClusterResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

type fixedToken string

func (t fixedToken) NewToken() (string, error) {
	return string(t), nil
}
