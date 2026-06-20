package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPrincipalRequiresTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/identity/principal", nil)
	request.Header.Set(principalHeader, "user-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestPrincipalRequiresPrincipalHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/identity/principal", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestPrincipalReturnsGatewayIdentity(t *testing.T) {
	t.Parallel()
	api := NewAPI(fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/identity/principal", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	request.Header.Set(principalHeader, "user-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var profile PrincipalProfile
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.TenantID != "tenant-a" || profile.PrincipalID != "user-a" || profile.Source != "gateway" {
		t.Fatalf("profile = %#v", profile)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
