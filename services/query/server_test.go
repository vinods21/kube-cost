package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDataQualityRequiresTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/data-quality", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %q", response.Header().Get("Content-Type"))
	}
}

func TestDataQualityReturnsFreshSignals(t *testing.T) {
	t.Parallel()
	latest := fixedNow().Add(-2 * time.Minute)
	repository := &fakeRepository{signals: []DataQualitySignal{
		{Source: "container_metrics_10s", Grain: "10s", ClusterID: "cluster-a", RecordCount: 10, LatestBucketStart: &latest, LatestIngestedAt: &latest},
		{Source: "node_metrics_10s", Grain: "10s", ClusterID: "cluster-a", RecordCount: 2, LatestBucketStart: &latest, LatestIngestedAt: &latest},
	}}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/data-quality?cluster_id=cluster-a", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.query.TenantID != "tenant-a" || repository.query.ClusterID != "cluster-a" {
		t.Fatalf("query = %#v", repository.query)
	}
	var body DataQualityResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Quality.Status != defaultFreshStatus || body.DataThrough == nil || len(body.Signals) != 2 {
		t.Fatalf("body = %#v", body)
	}
}

func TestDataQualityMarksStaleAndMissingSources(t *testing.T) {
	t.Parallel()
	stale := fixedNow().Add(-time.Hour)
	api := NewAPI(&fakeRepository{signals: []DataQualitySignal{
		{Source: "container_metrics_10s", Grain: "10s", ClusterID: "cluster-a", RecordCount: 10, LatestBucketStart: &stale, LatestIngestedAt: &stale},
	}}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/data-quality", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body DataQualityResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Quality.Status != defaultStaleStatus || len(body.Quality.MissingScopes) != 1 || len(body.Quality.Warnings) < 2 {
		t.Fatalf("quality = %#v", body.Quality)
	}
}

func TestHealthChecksRepository(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{pingErr: errors.New("down")}, fixedNow)
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}

type fakeRepository struct {
	signals []DataQualitySignal
	query   DataQualityQuery
	pingErr error
}

func (r *fakeRepository) DataQuality(_ context.Context, query DataQualityQuery) ([]DataQualitySignal, error) {
	r.query = query
	return r.signals, nil
}

func (r *fakeRepository) Ping(context.Context) error {
	return r.pingErr
}

func (r *fakeRepository) Close() error { return nil }

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
