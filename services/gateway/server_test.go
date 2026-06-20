package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/kube-cost/kube-cost/internal/gatewayauth"
)

func TestGatewayInjectsTenantHeaderFromBearerToken(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(tenantHeader) != "tenant-a" {
			t.Fatalf("tenant header = %q", r.Header.Get(tenantHeader))
		}
		if r.Header.Get(authorizationHeader) != "" {
			t.Fatalf("authorization header should be stripped")
		}
		if r.Header.Get(gatewaySecretHeader) != "backend-secret" {
			t.Fatalf("gateway secret header = %q", r.Header.Get(gatewaySecretHeader))
		}
		if err := gatewayauth.VerifyRequest(r, "signing-key", fixedNow(), 5*time.Minute); err != nil {
			t.Fatalf("gateway signature invalid: %v", err)
		}
		if r.URL.Path != "/api/v1/usage" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]string{"upstream": "query"})
	}))
	defer upstream.Close()

	server := testGateway(t, upstream.URL, upstream.URL, upstream.URL, upstream.URL)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil)
	request.Header.Set(authorizationHeader, "Bearer token-a")
	request.Header.Set(tenantHeader, "forged-tenant")
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGatewayRejectsMissingBearerToken(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testGateway(t, upstream.URL, upstream.URL, upstream.URL, upstream.URL)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestGatewayRejectsUnknownToken(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	server := testGateway(t, upstream.URL, upstream.URL, upstream.URL, upstream.URL)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil)
	request.Header.Set(authorizationHeader, "Bearer unknown")
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestGatewayRoutesRecommendationCommandsToWorkflow(t *testing.T) {
	t.Parallel()
	query := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"upstream": "query"})
	}))
	defer query.Close()
	workflow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/recommendations/rec-1/approve" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]string{"upstream": "workflow"})
	}))
	defer workflow.Close()
	server := testGateway(t, query.URL, query.URL, query.URL, workflow.URL)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/rec-1/approve", nil)
	request.Header.Set(authorizationHeader, "Bearer token-a")
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["upstream"] != "workflow" {
		t.Fatalf("body = %#v", body)
	}
}

func TestGatewayRoutesEffectivePriceLookupToPricing(t *testing.T) {
	t.Parallel()
	query := httptest.NewServer(http.NotFoundHandler())
	defer query.Close()
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/prices/effective" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]string{"upstream": "pricing"})
	}))
	defer pricing.Close()
	server := testGateway(t, query.URL, query.URL, pricing.URL, query.URL)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/prices/effective?provider=aws&region=us-east-1&service=EC2&resource_type=instance&unit=hour", nil)
	request.Header.Set(authorizationHeader, "Bearer token-a")
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["upstream"] != "pricing" {
		t.Fatalf("body = %#v", body)
	}
}

func TestGatewayRoutesExportsToExportService(t *testing.T) {
	t.Parallel()
	query := httptest.NewServer(http.NotFoundHandler())
	defer query.Close()
	export := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/exports/export-1" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]string{"upstream": "export"})
	}))
	defer export.Close()
	server := testGatewayWithExport(t, query.URL, query.URL, query.URL, query.URL, export.URL)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/exports/export-1", nil)
	request.Header.Set(authorizationHeader, "Bearer token-a")
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["upstream"] != "export" {
		t.Fatalf("body = %#v", body)
	}
}

func TestParseTokenTenantsAcceptsColonAndEquals(t *testing.T) {
	t.Parallel()
	result := parseTokenTenants("token-a:tenant-a, token-b=tenant-b")
	if result["token-a"] != "tenant-a" || result["token-b"] != "tenant-b" {
		t.Fatalf("result = %#v", result)
	}
}

func testGateway(t *testing.T, queryURL, clusterRegistryURL, pricingURL, workflowURL string) *Server {
	return testGatewayWithExport(t, queryURL, clusterRegistryURL, pricingURL, workflowURL, queryURL)
}

func testGatewayWithExport(t *testing.T, queryURL, clusterRegistryURL, pricingURL, workflowURL, exportURL string) *Server {
	t.Helper()
	server, err := NewServer(Config{
		TokenTenants:        map[string]string{"token-a": "tenant-a"},
		BackendSharedSecret: "backend-secret",
		BackendSigningKey:   "signing-key",
		GatewayIdentity:     "gateway",
		QueryURL:            mustURL(t, queryURL),
		ClusterRegistryURL:  mustURL(t, clusterRegistryURL),
		PricingURL:          mustURL(t, pricingURL),
		WorkflowURL:         mustURL(t, workflowURL),
		ExportURL:           mustURL(t, exportURL),
	})
	if err != nil {
		t.Fatal(err)
	}
	server.now = fixedNow
	return server
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
