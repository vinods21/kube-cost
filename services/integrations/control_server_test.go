package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateIntegrationRequiresTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewControlAPI(NewIntegrationStore(), fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/integrations", bytes.NewBufferString(validIntegrationJSON()))
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestCreateValidateAndListIntegration(t *testing.T) {
	t.Parallel()
	api := NewControlAPI(NewIntegrationStore(), fixedNow)
	create := httptest.NewRequest(http.MethodPost, "/api/v1/integrations", bytes.NewBufferString(validIntegrationJSON()))
	create.Header.Set(tenantHeader, "tenant-a")
	create.Header.Set(principalHeader, "user-a")
	createResponse := httptest.NewRecorder()

	api.Routes().ServeHTTP(createResponse, create)

	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	var integration Integration
	if err := json.Unmarshal(createResponse.Body.Bytes(), &integration); err != nil {
		t.Fatal(err)
	}
	if integration.IntegrationID == "" ||
		integration.TenantID != "tenant-a" ||
		integration.Type != "billing" ||
		integration.Provider != "aws" ||
		integration.Status != "pending_validation" ||
		integration.CreatedBy != "user-a" {
		t.Fatalf("integration = %#v", integration)
	}

	validate := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/"+integration.IntegrationID+"/validate", nil)
	validate.Header.Set(tenantHeader, "tenant-a")
	validateResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(validateResponse, validate)
	if validateResponse.Code != http.StatusAccepted {
		t.Fatalf("validate status = %d, body = %s", validateResponse.Code, validateResponse.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/integrations", nil)
	list.Header.Set(tenantHeader, "tenant-a")
	listResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var result IntegrationsResult
	if err := json.Unmarshal(listResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "tenant-a" || result.ResultCount != 1 || result.Integrations[0].Status != "validated" {
		t.Fatalf("result = %#v", result)
	}
}

func TestIntegrationsAreTenantScoped(t *testing.T) {
	t.Parallel()
	store := NewIntegrationStore()
	store.Create("tenant-a", "user-a", IntegrationRequest{
		Name:     "billing",
		Type:     "billing",
		Provider: "aws",
	}, fixedNow())
	api := NewControlAPI(store, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/integrations", nil)
	request.Header.Set(tenantHeader, "tenant-b")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result IntegrationsResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TenantID != "tenant-b" || result.ResultCount != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCreateIntegrationRejectsInvalidType(t *testing.T) {
	t.Parallel()
	api := NewControlAPI(NewIntegrationStore(), fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/integrations", bytes.NewBufferString(`{
		"name": "bad",
		"type": "database",
		"provider": "aws"
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func validIntegrationJSON() string {
	return `{
		"name": "aws billing",
		"type": "billing",
		"provider": "aws",
		"account_id": "123456789012",
		"region": "us-east-1",
		"secret_ref": "aws-billing",
		"config": {"bucket": "cur-bucket"}
	}`
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
