package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type API struct {
	repository Repository
	now        func() time.Time
}

func NewAPI(repository Repository, now func() time.Time) *API {
	return &API{repository: repository, now: now}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/data-quality", a.dataQuality)
	mux.HandleFunc("GET /api/v1/usage", a.usage)
	mux.HandleFunc("GET /api/v1/costs", a.costs)
	mux.HandleFunc("GET /api/v1/allocation", a.allocation)
	mux.HandleFunc("GET /api/v1/recommendations", a.recommendations)
	mux.HandleFunc("GET /api/v1/recommendations/{recommendation_id}", a.recommendation)
	return mux
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.repository.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "dependency_unavailable", "clickhouse unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) dataQuality(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	window := durationValue("QUERY_FRESHNESS_WINDOW", defaultFreshnessWindow)
	signals, err := a.repository.DataQuality(r.Context(), DataQualityQuery{
		TenantID:        tenantID,
		ClusterID:       strings.TrimSpace(r.URL.Query().Get("cluster_id")),
		FreshnessWindow: window,
	})
	if err != nil {
		slog.Error("data quality query failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "query_failed", "data quality query failed")
		return
	}
	writeJSON(w, http.StatusOK, summarizeDataQuality(tenantID, strings.TrimSpace(r.URL.Query().Get("cluster_id")), a.now().UTC(), window, signals))
}

func (a *API) usage(w http.ResponseWriter, r *http.Request) {
	query, ok := a.analyticsQuery(w, r, usageCursorKind)
	if !ok {
		return
	}
	rows, err := a.repository.Usage(r.Context(), query)
	if err != nil {
		slog.Error("usage query failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "query_failed", "usage query failed")
		return
	}
	rows, nextCursor := paginate(rows, query, usageCursorKind)
	quality := a.analyticsQuality(w, r, query)
	if quality.failed {
		return
	}
	writeJSON(w, http.StatusOK, UsageResult{
		TenantID:    query.TenantID,
		ClusterID:   query.ClusterID,
		Start:       query.Start,
		End:         query.End,
		GroupBy:     query.GroupBy,
		GeneratedAt: a.now().UTC(),
		DataThrough: quality.dataThrough,
		Quality:     quality.summary,
		Rows:        rows,
		ResultCount: len(rows),
		Limit:       normalizedAnalyticsLimit(query.Limit),
		NextCursor:  nextCursor,
	})
}

func (a *API) costs(w http.ResponseWriter, r *http.Request) {
	query, ok := a.analyticsQuery(w, r, costsCursorKind)
	if !ok {
		return
	}
	metadata, rows, err := a.repository.Costs(r.Context(), query)
	if err != nil {
		slog.Error("cost query failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "query_failed", "cost query failed")
		return
	}
	rows, nextCursor := paginate(rows, query, costsCursorKind)
	quality := a.analyticsQuality(w, r, query)
	if quality.failed {
		return
	}
	writeJSON(w, http.StatusOK, CostResult{
		TenantID:           query.TenantID,
		ClusterID:          query.ClusterID,
		Start:              query.Start,
		End:                query.End,
		GroupBy:            query.GroupBy,
		GeneratedAt:        a.now().UTC(),
		DataThrough:        quality.dataThrough,
		Quality:            quality.summary,
		Currency:           metadata.Currency,
		ComputationVersion: metadata.ComputationVersion,
		ComputedAt:         metadata.ComputedAt,
		Rows:               rows,
		ResultCount:        len(rows),
		Limit:              normalizedAnalyticsLimit(query.Limit),
		NextCursor:         nextCursor,
	})
}

func (a *API) allocation(w http.ResponseWriter, r *http.Request) {
	query, ok := a.analyticsQuery(w, r, allocationCursorKind)
	if !ok {
		return
	}
	metadata, rows, err := a.repository.Allocation(r.Context(), query)
	if err != nil {
		slog.Error("allocation query failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "query_failed", "allocation query failed")
		return
	}
	rows, nextCursor := paginate(rows, query, allocationCursorKind)
	quality := a.analyticsQuality(w, r, query)
	if quality.failed {
		return
	}
	writeJSON(w, http.StatusOK, AllocationResult{
		TenantID:           query.TenantID,
		ClusterID:          query.ClusterID,
		Start:              query.Start,
		End:                query.End,
		GroupBy:            query.GroupBy,
		GeneratedAt:        a.now().UTC(),
		DataThrough:        quality.dataThrough,
		Quality:            quality.summary,
		Currency:           metadata.Currency,
		ComputationVersion: metadata.ComputationVersion,
		ComputedAt:         metadata.ComputedAt,
		Rows:               rows,
		ResultCount:        len(rows),
		Limit:              normalizedAnalyticsLimit(query.Limit),
		NextCursor:         nextCursor,
	})
}

func (a *API) recommendations(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	query, ok := recommendationQueryFromRequest(w, r, tenantID)
	if !ok {
		return
	}
	recommendations, err := a.repository.Recommendations(r.Context(), query)
	if err != nil {
		slog.Error("recommendations query failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "query_failed", "recommendations query failed")
		return
	}
	writeJSON(w, http.StatusOK, RecommendationListResult{
		TenantID:        tenantID,
		ClusterID:       query.ClusterID,
		GeneratedAt:     a.now().UTC(),
		Recommendations: recommendations,
		ResultCount:     len(recommendations),
		Limit:           normalizedRecommendationLimit(query.Limit),
	})
}

func (a *API) recommendation(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	recommendationID := strings.TrimSpace(r.PathValue("recommendation_id"))
	if recommendationID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "recommendation_id is required")
		return
	}
	recommendation, err := a.repository.Recommendation(r.Context(), tenantID, recommendationID)
	if err != nil {
		if errors.Is(err, ErrRecommendationNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "recommendation not found")
			return
		}
		slog.Error("recommendation query failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "query_failed", "recommendation query failed")
		return
	}
	writeJSON(w, http.StatusOK, recommendation)
}

func (a *API) analyticsQuery(w http.ResponseWriter, r *http.Request, cursorKind string) (AnalyticsQuery, bool) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return AnalyticsQuery{}, false
	}
	values := r.URL.Query()
	start, err := parseRequiredTime(values.Get("start"), "start")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return AnalyticsQuery{}, false
	}
	end, err := parseRequiredTime(values.Get("end"), "end")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return AnalyticsQuery{}, false
	}
	if !start.Before(end) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "start must be before end")
		return AnalyticsQuery{}, false
	}
	if !start.Truncate(time.Hour).Equal(start) || !end.Truncate(time.Hour).Equal(end) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "start and end must be aligned to whole hours")
		return AnalyticsQuery{}, false
	}
	groupBy := strings.TrimSpace(values.Get("group_by"))
	if groupBy == "" {
		groupBy = "namespace"
	}
	if groupBy != "namespace" && groupBy != "cluster" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "group_by must be namespace or cluster")
		return AnalyticsQuery{}, false
	}
	limit := defaultAnalyticsLimit
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return AnalyticsQuery{}, false
		}
		limit = parsed
	}
	offset := 0
	if rawCursor := strings.TrimSpace(values.Get("cursor")); rawCursor != "" {
		cursor, err := decodeAnalyticsCursor(rawCursor)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "cursor is invalid")
			return AnalyticsQuery{}, false
		}
		if cursor.TenantID != tenantID ||
			cursor.Kind != cursorKind ||
			cursor.ClusterID != strings.TrimSpace(values.Get("cluster_id")) ||
			cursor.Start != start.Format(time.RFC3339) ||
			cursor.End != end.Format(time.RFC3339) ||
			cursor.GroupBy != groupBy {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "cursor does not match query")
			return AnalyticsQuery{}, false
		}
		offset = cursor.Offset
	}
	return AnalyticsQuery{
		TenantID:       tenantID,
		ClusterID:      strings.TrimSpace(values.Get("cluster_id")),
		Start:          start,
		End:            end,
		GroupBy:        groupBy,
		Limit:          limit,
		Offset:         offset,
		IncludeQuality: boolQuery(values.Get("include_quality")),
	}, true
}

type analyticsQuality struct {
	dataThrough *time.Time
	summary     *QualitySummary
	failed      bool
}

func (a *API) analyticsQuality(w http.ResponseWriter, r *http.Request, query AnalyticsQuery) analyticsQuality {
	if !query.IncludeQuality {
		return analyticsQuality{}
	}
	window := durationValue("QUERY_FRESHNESS_WINDOW", defaultFreshnessWindow)
	signals, err := a.repository.DataQuality(r.Context(), DataQualityQuery{
		TenantID:        query.TenantID,
		ClusterID:       query.ClusterID,
		FreshnessWindow: window,
	})
	if err != nil {
		slog.Error("analytics quality query failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "query_failed", "quality query failed")
		return analyticsQuality{failed: true}
	}
	result := summarizeDataQuality(query.TenantID, query.ClusterID, a.now().UTC(), window, signals)
	return analyticsQuality{dataThrough: result.DataThrough, summary: &result.Quality}
}

func boolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

const (
	usageCursorKind      = "usage"
	costsCursorKind      = "costs"
	allocationCursorKind = "allocation"
)

type analyticsCursor struct {
	Kind      string `json:"kind"`
	TenantID  string `json:"tenant_id"`
	ClusterID string `json:"cluster_id,omitempty"`
	Start     string `json:"start"`
	End       string `json:"end"`
	GroupBy   string `json:"group_by"`
	Offset    int    `json:"offset"`
}

func paginate[T any](rows []T, query AnalyticsQuery, kind string) ([]T, string) {
	limit := normalizedAnalyticsLimit(query.Limit)
	if len(rows) <= limit {
		return rows, ""
	}
	return rows[:limit], encodeAnalyticsCursor(analyticsCursor{
		Kind:      kind,
		TenantID:  query.TenantID,
		ClusterID: query.ClusterID,
		Start:     query.Start.Format(time.RFC3339),
		End:       query.End.Format(time.RFC3339),
		GroupBy:   query.GroupBy,
		Offset:    query.Offset + limit,
	})
}

func encodeAnalyticsCursor(cursor analyticsCursor) string {
	data, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeAnalyticsCursor(value string) (analyticsCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return analyticsCursor{}, err
	}
	var cursor analyticsCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return analyticsCursor{}, err
	}
	if cursor.Offset < 0 {
		return analyticsCursor{}, errors.New("negative cursor offset")
	}
	return cursor, nil
}

func parseRequiredTime(value, name string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New(name + " is required")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New(name + " must be an RFC3339 timestamp")
	}
	return parsed.UTC(), nil
}

func summarizeDataQuality(tenantID, clusterID string, now time.Time, window time.Duration, signals []DataQualitySignal) DataQualityResult {
	result := DataQualityResult{
		TenantID:           tenantID,
		ClusterID:          clusterID,
		GeneratedAt:        now,
		ComputationVersion: computationVersionDataQ1,
		Signals:            make([]DataQualitySignal, 0, len(signals)),
		Quality: QualitySummary{
			Status:              defaultFreshStatus,
			EstimatedPercent:    100,
			FreshnessWindowSecs: int64(window.Seconds()),
		},
	}
	if len(signals) == 0 {
		result.Quality.Status = defaultEmptyStatus
		result.Quality.EstimatedPercent = 0
		result.Quality.MissingScopes = []string{"container_metrics_10s", "node_metrics_10s"}
		result.Quality.Warnings = []string{"no metric facts found for tenant scope"}
		return result
	}
	seen := make(map[string]struct{}, len(signals))
	for _, signal := range signals {
		signal.ExpectedFreshnessLimit = window.String()
		seen[signal.Source] = struct{}{}
		if signal.RecordCount == 0 || signal.LatestBucketStart == nil {
			signal.Status = defaultEmptyStatus
			signal.Warning = "no metric facts found"
			result.Quality.Status = defaultStaleStatus
			result.Quality.Warnings = append(result.Quality.Warnings, signal.Source+" has no metric facts")
		} else {
			freshness := int64(now.Sub(*signal.LatestBucketStart).Seconds())
			if freshness < 0 {
				freshness = 0
			}
			signal.FreshnessSeconds = &freshness
			if time.Duration(freshness)*time.Second > window {
				signal.Status = defaultStaleStatus
				signal.Warning = "latest metric bucket is outside freshness window"
				result.Quality.Status = defaultStaleStatus
				result.Quality.Warnings = append(result.Quality.Warnings, signal.Source+" is stale")
			} else {
				signal.Status = defaultFreshStatus
			}
			if result.DataThrough == nil || signal.LatestBucketStart.Before(*result.DataThrough) {
				dataThrough := *signal.LatestBucketStart
				result.DataThrough = &dataThrough
			}
		}
		result.Signals = append(result.Signals, signal)
	}
	for _, source := range []string{"container_metrics_10s", "node_metrics_10s"} {
		if _, ok := seen[source]; ok {
			continue
		}
		result.Quality.Status = defaultStaleStatus
		result.Quality.EstimatedPercent = 50
		result.Quality.MissingScopes = append(result.Quality.MissingScopes, source)
		result.Quality.Warnings = append(result.Quality.Warnings, source+" has no metric facts")
	}
	return result
}

func recommendationQueryFromRequest(w http.ResponseWriter, r *http.Request, tenantID string) (RecommendationQuery, bool) {
	values := r.URL.Query()
	limit := 100
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return RecommendationQuery{}, false
		}
		limit = parsed
	}
	minimumSavings := strings.TrimSpace(values.Get("min_monthly_savings"))
	if minimumSavings != "" {
		if _, err := decimal.NewFromString(minimumSavings); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "min_monthly_savings must be a decimal string")
			return RecommendationQuery{}, false
		}
	}
	return RecommendationQuery{
		TenantID:              tenantID,
		ClusterID:             strings.TrimSpace(values.Get("cluster_id")),
		Status:                strings.TrimSpace(values.Get("status")),
		RecommendationType:    strings.TrimSpace(values.Get("type")),
		TargetKind:            strings.TrimSpace(values.Get("target_kind")),
		TargetUID:             strings.TrimSpace(values.Get("target_uid")),
		MinimumMonthlySavings: minimumSavings,
		Limit:                 limit,
	}, true
}

func normalizedRecommendationLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func authenticatedTenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !trustedGateway(w, r) {
		return "", false
	}
	tenantID := strings.TrimSpace(r.Header.Get(tenantHeader))
	if tenantID == "" {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", tenantHeader+" is required")
		return "", false
	}
	return tenantID, true
}

func trustedGateway(w http.ResponseWriter, r *http.Request) bool {
	expected := strings.TrimSpace(os.Getenv("TRUSTED_GATEWAY_SECRET"))
	if expected == "" {
		return true
	}
	if r.Header.Get(gatewaySecretHeader) != expected {
		writeProblem(w, http.StatusForbidden, "forbidden", gatewaySecretHeader+" is required")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "https://kube-cost.dev/problems/" + code,
		"title":  http.StatusText(status),
		"status": status,
		"code":   code,
		"detail": detail,
	})
}
