package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kube-cost/kube-cost/internal/gatewayauth"
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

func TestDataQualityRequiresTrustedGatewayWhenConfigured(t *testing.T) {
	t.Setenv("TRUSTED_GATEWAY_SECRET", "backend-secret")
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/data-quality", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestDataQualityAcceptsTrustedGatewayWhenConfigured(t *testing.T) {
	t.Setenv("TRUSTED_GATEWAY_SECRET", "backend-secret")
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/data-quality", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	request.Header.Set(gatewaySecretHeader, "backend-secret")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestDataQualityRequiresTrustedGatewaySignatureWhenConfigured(t *testing.T) {
	t.Setenv("TRUSTED_GATEWAY_SIGNING_KEY", "signing-key")
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/data-quality", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestDataQualityAcceptsTrustedGatewaySignatureWhenConfigured(t *testing.T) {
	t.Setenv("TRUSTED_GATEWAY_SIGNING_KEY", "signing-key")
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/data-quality", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	gatewayauth.SignRequest(request, "gateway", "signing-key", time.Now().UTC())
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
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

func TestUsageReturnsTenantScopedAnalytics(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{usageRows: []UsageRow{{
		TenantID:            "tenant-a",
		ClusterID:           "cluster-a",
		NamespaceUID:        "namespace-a",
		NamespaceName:       "apps",
		CPURequestCoreHours: "1.5",
		NetworkBytes:        1024,
	}}}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage?cluster_id=cluster-a&start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&group_by=namespace&limit=25", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.analyticsQuery.TenantID != "tenant-a" ||
		repository.analyticsQuery.ClusterID != "cluster-a" ||
		repository.analyticsQuery.GroupBy != "namespace" ||
		repository.analyticsQuery.Limit != 25 {
		t.Fatalf("query = %#v", repository.analyticsQuery)
	}
	var body UsageResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ResultCount != 1 || body.Limit != 25 || body.Rows[0].NamespaceName != "apps" {
		t.Fatalf("body = %#v", body)
	}
}

func TestUsageReturnsAndAcceptsOpaqueCursor(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{usageRows: []UsageRow{
		{TenantID: "tenant-a", ClusterID: "cluster-a", NamespaceUID: "namespace-a"},
		{TenantID: "tenant-a", ClusterID: "cluster-a", NamespaceUID: "namespace-b"},
	}}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage?start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&limit=1", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var firstPage UsageResult
	if err := json.Unmarshal(response.Body.Bytes(), &firstPage); err != nil {
		t.Fatal(err)
	}
	if firstPage.NextCursor == "" || firstPage.ResultCount != 1 {
		t.Fatalf("first page = %#v", firstPage)
	}

	repository.usageRows = []UsageRow{{TenantID: "tenant-a", ClusterID: "cluster-a", NamespaceUID: "namespace-b"}}
	nextRequest := httptest.NewRequest(http.MethodGet, "/api/v1/usage?start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&limit=1&cursor="+firstPage.NextCursor, nil)
	nextRequest.Header.Set(tenantHeader, "tenant-a")
	nextResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(nextResponse, nextRequest)

	if nextResponse.Code != http.StatusOK {
		t.Fatalf("next status = %d, body = %s", nextResponse.Code, nextResponse.Body.String())
	}
	if repository.analyticsQuery.Offset != 1 {
		t.Fatalf("offset = %d, want 1", repository.analyticsQuery.Offset)
	}
}

func TestAnalyticsRejectsMismatchedCursor(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	cursor := encodeAnalyticsCursor(analyticsCursor{
		Kind:     costsCursorKind,
		TenantID: "tenant-a",
		Start:    "2026-06-19T10:00:00Z",
		End:      "2026-06-19T12:00:00Z",
		GroupBy:  "namespace",
		Offset:   1,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage?start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&cursor="+cursor, nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestUsageCanIncludeQualitySummary(t *testing.T) {
	t.Parallel()
	latest := fixedNow().Add(-2 * time.Minute)
	repository := &fakeRepository{
		usageRows: []UsageRow{{TenantID: "tenant-a", ClusterID: "cluster-a"}},
		signals: []DataQualitySignal{
			{Source: "container_metrics_10s", Grain: "10s", ClusterID: "cluster-a", RecordCount: 10, LatestBucketStart: &latest, LatestIngestedAt: &latest},
			{Source: "node_metrics_10s", Grain: "10s", ClusterID: "cluster-a", RecordCount: 2, LatestBucketStart: &latest, LatestIngestedAt: &latest},
		},
	}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage?cluster_id=cluster-a&start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&include_quality=true", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body UsageResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Quality == nil || body.Quality.Status != defaultFreshStatus || body.DataThrough == nil {
		t.Fatalf("body = %#v", body)
	}
}

func TestCostsCapsLimitAndDefaultsGroupBy(t *testing.T) {
	t.Parallel()
	computedAt := fixedNow().Add(-time.Minute)
	repository := &fakeRepository{
		costMetadata: CostMetadata{Currency: "USD", ComputationVersion: "allocation-v1", ComputedAt: computedAt},
		costRows:     []CostRow{{TenantID: "tenant-a", ClusterID: "cluster-a", AllocatedCost: "1.25"}},
	}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/costs?start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&limit=1000", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.analyticsQuery.GroupBy != "namespace" || repository.analyticsQuery.Limit != 1000 {
		t.Fatalf("query = %#v", repository.analyticsQuery)
	}
	var body CostResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Currency != "USD" || body.ComputationVersion != "allocation-v1" || body.Limit != maxAnalyticsLimit {
		t.Fatalf("body = %#v", body)
	}
}

func TestAllocationSupportsClusterGrouping(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{
		costMetadata:   CostMetadata{Currency: "USD", ComputationVersion: "allocation-v1"},
		allocationRows: []AllocationRow{{TenantID: "tenant-a", ClusterID: "cluster-a", AllocatedCost: "2.50"}},
	}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/allocation?start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&group_by=cluster", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.analyticsQuery.GroupBy != "cluster" {
		t.Fatalf("query = %#v", repository.analyticsQuery)
	}
	var body AllocationResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ResultCount != 1 || body.Rows[0].AllocatedCost != "2.50" {
		t.Fatalf("body = %#v", body)
	}
}

func TestUsageSupportsPromotedDimensionGrouping(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{usageRows: []UsageRow{{
		TenantID:            "tenant-a",
		ClusterID:           "cluster-a",
		GroupKey:            "team",
		GroupValue:          "platform",
		CPURequestCoreHours: "1.5",
	}}}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage?start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&group_by=team", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.analyticsQuery.GroupBy != "team" {
		t.Fatalf("query = %#v", repository.analyticsQuery)
	}
	var body UsageResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ResultCount != 1 || body.Rows[0].GroupKey != "team" || body.Rows[0].GroupValue != "platform" {
		t.Fatalf("body = %#v", body)
	}
}

func TestAnalyticsRejectsInvalidRange(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage?start=2026-06-19T10:30:00Z&end=2026-06-19T12:00:00Z", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestAnalyticsRejectsInvalidGroupBy(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/costs?start=2026-06-19T10:00:00Z&end=2026-06-19T12:00:00Z&group_by=pod", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestAsyncQueryJobCompletesAndReturnsManifest(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{usageRows: []UsageRow{{
		TenantID:            "tenant-a",
		ClusterID:           "cluster-a",
		GroupKey:            "team",
		GroupValue:          "platform",
		CPURequestCoreHours: "1.5",
	}}}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/queries", bytes.NewBufferString(`{
		"query_type": "usage",
		"cluster_id": "cluster-a",
		"start": "2026-06-19T10:00:00Z",
		"end": "2026-06-19T12:00:00Z",
		"group_by": "team",
		"limit": 25
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var created QueryJobResult
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.QueryID == "" || created.Status != queryJobStatusQueued || created.Query.GroupBy != "team" {
		t.Fatalf("created = %#v", created)
	}

	var completed QueryJobResult
	for i := 0; i < 50; i++ {
		getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/queries/"+created.QueryID, nil)
		getRequest.Header.Set(tenantHeader, "tenant-a")
		getResponse := httptest.NewRecorder()
		api.Routes().ServeHTTP(getResponse, getRequest)
		if getResponse.Code != http.StatusOK {
			t.Fatalf("get status = %d, body = %s", getResponse.Code, getResponse.Body.String())
		}
		if err := json.Unmarshal(getResponse.Body.Bytes(), &completed); err != nil {
			t.Fatal(err)
		}
		if completed.Status == queryJobStatusSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed.Status != queryJobStatusSucceeded || completed.Manifest == nil || completed.Manifest.RowCount != 1 {
		t.Fatalf("completed = %#v", completed)
	}
	if completed.Manifest.SchemaVersion != "query-result-v1" ||
		completed.Manifest.ContentType != "application/json" ||
		completed.Manifest.ByteSize <= 0 ||
		completed.Manifest.SHA256 == "" {
		t.Fatalf("manifest = %#v", completed.Manifest)
	}
	if completed.Result == nil {
		t.Fatalf("result should be included: %#v", completed)
	}
}

func TestAsyncQueryJobIsTenantScoped(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{usageRows: []UsageRow{{TenantID: "tenant-a"}}}, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/queries", bytes.NewBufferString(`{
		"query_type": "usage",
		"start": "2026-06-19T10:00:00Z",
		"end": "2026-06-19T12:00:00Z"
	}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var created QueryJobResult
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/queries/"+created.QueryID, nil)
	getRequest.Header.Set(tenantHeader, "tenant-b")
	getResponse := httptest.NewRecorder()
	api.Routes().ServeHTTP(getResponse, getRequest)

	if getResponse.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", getResponse.Code)
	}
}

func TestAsyncQueryJobRejectsUnsupportedType(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/queries", bytes.NewBufferString(`{
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

func TestRecommendationsReturnsTenantScopedResults(t *testing.T) {
	t.Parallel()
	recommendation := RecommendationResult{
		TenantID:              "tenant-a",
		RecommendationID:      "rec-1",
		ClusterID:             "cluster-a",
		TargetKind:            "container",
		TargetUID:             "pod/container",
		RecommendationType:    "rightsizing",
		SafetyClass:           "review_required",
		Status:                "open",
		AnalysisWindowStart:   fixedNow().Add(-30 * 24 * time.Hour),
		AnalysisWindowEnd:     fixedNow(),
		GeneratedAt:           fixedNow(),
		ExpiresAt:             fixedNow().Add(30 * 24 * time.Hour),
		CurrentConfiguration:  jsonRawMessage(`{"cpu_request_millicores":500}`),
		ProposedConfiguration: jsonRawMessage(`{"cpu_request_millicores":100}`),
		Evidence:              jsonRawMessage(`{"sample_count":720}`),
		Currency:              "USD",
		MonthlyGrossSavings:   "7.25",
		MonthlyNetSavings:     "7.25",
		Confidence:            "0.7",
		RiskScore:             "0.3",
		ModelVersion:          "optimization-v1",
		ComputationVersion:    "optimization-v1",
		Version:               1,
	}
	repository := &fakeRepository{recommendations: []RecommendationResult{recommendation}}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations?cluster_id=cluster-a&status=open&type=rightsizing&target_kind=container&target_uid=pod/container&min_monthly_savings=5.00&limit=25", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.recommendationQuery.TenantID != "tenant-a" ||
		repository.recommendationQuery.ClusterID != "cluster-a" ||
		repository.recommendationQuery.Status != "open" ||
		repository.recommendationQuery.RecommendationType != "rightsizing" ||
		repository.recommendationQuery.TargetKind != "container" ||
		repository.recommendationQuery.TargetUID != "pod/container" ||
		repository.recommendationQuery.MinimumMonthlySavings != "5.00" ||
		repository.recommendationQuery.Limit != 25 {
		t.Fatalf("query = %#v", repository.recommendationQuery)
	}
	var body RecommendationListResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TenantID != "tenant-a" || body.ResultCount != 1 || body.Limit != 25 {
		t.Fatalf("body = %#v", body)
	}
	if len(body.Recommendations) != 1 || string(body.Recommendations[0].Evidence) != `{"sample_count":720}` {
		t.Fatalf("recommendations = %#v", body.Recommendations)
	}
}

func TestRecommendationsRejectsInvalidMinimumSavings(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations?min_monthly_savings=abc", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestRecommendationDetailReturnsTenantScopedResult(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{recommendation: RecommendationResult{RecommendationID: "rec-1", TenantID: "tenant-a"}}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations/rec-1", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.recommendationTenantID != "tenant-a" || repository.recommendationID != "rec-1" {
		t.Fatalf("lookup = %s/%s", repository.recommendationTenantID, repository.recommendationID)
	}
	var body RecommendationResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RecommendationID != "rec-1" {
		t.Fatalf("body = %#v", body)
	}
}

func TestRecommendationDetailReturnsNotFound(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{recommendationErr: ErrRecommendationNotFound}, fixedNow)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations/missing", nil)
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
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
	signals                []DataQualitySignal
	query                  DataQualityQuery
	analyticsQuery         AnalyticsQuery
	usageRows              []UsageRow
	costMetadata           CostMetadata
	costRows               []CostRow
	allocationRows         []AllocationRow
	recommendations        []RecommendationResult
	recommendationQuery    RecommendationQuery
	recommendation         RecommendationResult
	recommendationTenantID string
	recommendationID       string
	recommendationErr      error
	pingErr                error
}

func (r *fakeRepository) DataQuality(_ context.Context, query DataQualityQuery) ([]DataQualitySignal, error) {
	r.query = query
	return r.signals, nil
}

func (r *fakeRepository) Usage(_ context.Context, query AnalyticsQuery) ([]UsageRow, error) {
	r.analyticsQuery = query
	return r.usageRows, nil
}

func (r *fakeRepository) Costs(_ context.Context, query AnalyticsQuery) (CostMetadata, []CostRow, error) {
	r.analyticsQuery = query
	return r.costMetadata, r.costRows, nil
}

func (r *fakeRepository) Allocation(_ context.Context, query AnalyticsQuery) (CostMetadata, []AllocationRow, error) {
	r.analyticsQuery = query
	return r.costMetadata, r.allocationRows, nil
}

func (r *fakeRepository) Recommendations(_ context.Context, query RecommendationQuery) ([]RecommendationResult, error) {
	r.recommendationQuery = query
	return r.recommendations, nil
}

func (r *fakeRepository) Recommendation(_ context.Context, tenantID, recommendationID string) (RecommendationResult, error) {
	r.recommendationTenantID = tenantID
	r.recommendationID = recommendationID
	return r.recommendation, r.recommendationErr
}

func (r *fakeRepository) Ping(context.Context) error {
	return r.pingErr
}

func (r *fakeRepository) Close() error { return nil }

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
