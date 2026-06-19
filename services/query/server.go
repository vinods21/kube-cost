package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
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

func authenticatedTenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	tenantID := strings.TrimSpace(r.Header.Get(tenantHeader))
	if tenantID == "" {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", tenantHeader+" is required")
		return "", false
	}
	return tenantID, true
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
