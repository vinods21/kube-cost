package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateExportReturnsTenantScopedJob(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(10), fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/exports", bytes.NewBufferString(`{
		"query_type": "usage",
		"format": "csv",
		"cluster_id": "cluster-a",
		"start": "2026-06-19T10:00:00Z",
		"end": "2026-06-19T12:00:00Z",
		"group_by": "team"
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var job ExportJob
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.ExportID == "" ||
		job.TenantID != "tenant-a" ||
		job.Request.QueryType != "usage" ||
		job.Request.Format != "csv" ||
		job.Manifest.ContentType != "text/csv" ||
		job.Manifest.SHA256 == "" ||
		!job.Manifest.Inline {
		t.Fatalf("job = %#v", job)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+job.ExportID, nil)
	getRequest.Header.Set(tenantHeader, "tenant-a")
	getResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
}

func TestExportJobsAreTenantScoped(t *testing.T) {
	t.Parallel()
	store := NewStore(10)
	job, err := store.Create("tenant-a", ExportSpec{
		QueryType: "usage",
		Format:    "json",
		Start:     time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
		GroupBy:   "namespace",
	}, fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	api := NewAPI(store, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/exports/"+job.ExportID, nil)
	request.Header.Set(tenantHeader, "tenant-b")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestCreateExportRejectsInvalidRequest(t *testing.T) {
	t.Parallel()
	api := NewAPI(NewStore(10), fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/exports", bytes.NewBufferString(`{
		"query_type": "recommendations",
		"start": "2026-06-19T10:00:00Z",
		"end": "2026-06-19T12:00:00Z"
	}`))
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
