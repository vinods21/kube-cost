package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	queryJobStatusQueued    = "queued"
	queryJobStatusRunning   = "running"
	queryJobStatusSucceeded = "succeeded"
	queryJobStatusFailed    = "failed"
	queryJobTimeout         = 2 * time.Minute
)

type QueryJobRequest struct {
	QueryType      string `json:"query_type"`
	ClusterID      string `json:"cluster_id,omitempty"`
	Start          string `json:"start"`
	End            string `json:"end"`
	GroupBy        string `json:"group_by,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	IncludeQuality bool   `json:"include_quality,omitempty"`
}

type QueryJobResult struct {
	QueryID    string              `json:"query_id"`
	TenantID   string              `json:"tenant_id"`
	QueryType  string              `json:"query_type"`
	Status     string              `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	StartedAt  *time.Time          `json:"started_at,omitempty"`
	FinishedAt *time.Time          `json:"finished_at,omitempty"`
	Query      AsyncAnalyticsQuery `json:"query"`
	Manifest   *QueryManifest      `json:"manifest,omitempty"`
	Error      string              `json:"error,omitempty"`
	Result     any                 `json:"result,omitempty"`
}

type AsyncAnalyticsQuery struct {
	ClusterID      string    `json:"cluster_id,omitempty"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	GroupBy        string    `json:"group_by"`
	Limit          int       `json:"limit"`
	IncludeQuality bool      `json:"include_quality,omitempty"`
}

type QueryManifest struct {
	ResultType  string    `json:"result_type"`
	RowCount    int       `json:"row_count"`
	GeneratedAt time.Time `json:"generated_at"`
	Inline      bool      `json:"inline"`
}

type queryJobStore struct {
	mu      sync.RWMutex
	maxJobs int
	order   []string
	jobs    map[string]QueryJobResult
}

func newQueryJobStore(maxJobs int) *queryJobStore {
	if maxJobs <= 0 {
		maxJobs = 1000
	}
	return &queryJobStore{maxJobs: maxJobs, jobs: make(map[string]QueryJobResult)}
}

func (s *queryJobStore) create(tenantID, queryType string, query AnalyticsQuery, now time.Time) QueryJobResult {
	job := QueryJobResult{
		QueryID:   newQueryID(),
		TenantID:  tenantID,
		QueryType: queryType,
		Status:    queryJobStatusQueued,
		CreatedAt: now,
		Query: AsyncAnalyticsQuery{
			ClusterID:      query.ClusterID,
			Start:          query.Start,
			End:            query.End,
			GroupBy:        query.GroupBy,
			Limit:          normalizedAnalyticsLimit(query.Limit),
			IncludeQuality: query.IncludeQuality,
		},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.QueryID] = job
	s.order = append(s.order, job.QueryID)
	s.evictLocked()
	return job
}

func (s *queryJobStore) start(tenantID, queryID string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[queryID]
	if !ok || job.TenantID != tenantID {
		return false
	}
	job.Status = queryJobStatusRunning
	startedAt := now
	job.StartedAt = &startedAt
	s.jobs[queryID] = job
	return true
}

func (s *queryJobStore) succeed(tenantID, queryID string, manifest QueryManifest, result any, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[queryID]
	if !ok || job.TenantID != tenantID {
		return false
	}
	job.Status = queryJobStatusSucceeded
	finishedAt := now
	job.FinishedAt = &finishedAt
	job.Manifest = &manifest
	job.Result = result
	s.jobs[queryID] = job
	return true
}

func (s *queryJobStore) fail(tenantID, queryID string, err error, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[queryID]
	if !ok || job.TenantID != tenantID {
		return false
	}
	job.Status = queryJobStatusFailed
	finishedAt := now
	job.FinishedAt = &finishedAt
	job.Error = err.Error()
	s.jobs[queryID] = job
	return true
}

func (s *queryJobStore) get(tenantID, queryID string) (QueryJobResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[queryID]
	if !ok || job.TenantID != tenantID {
		return QueryJobResult{}, false
	}
	return job, true
}

func (s *queryJobStore) evictLocked() {
	for len(s.order) > s.maxJobs {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.jobs, oldest)
	}
}

func newQueryID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("query-%d", time.Now().UnixNano())
	}
	return "query_" + hex.EncodeToString(data[:])
}

func (a *API) createQueryJob(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	var request QueryJobRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body must be valid query JSON")
		return
	}
	query, queryType, ok := asyncAnalyticsQuery(w, request, tenantID)
	if !ok {
		return
	}
	job := a.jobs.create(tenantID, queryType, query, a.now().UTC())
	go a.runQueryJob(job.QueryID, queryType, query)
	writeJSON(w, http.StatusAccepted, job)
}

func (a *API) queryJob(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	queryID := strings.TrimSpace(r.PathValue("query_id"))
	if queryID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "query_id is required")
		return
	}
	job, ok := a.jobs.get(tenantID, queryID)
	if !ok {
		writeProblem(w, http.StatusNotFound, "not_found", "query job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (a *API) runQueryJob(queryID, queryType string, query AnalyticsQuery) {
	a.jobs.start(query.TenantID, queryID, a.now().UTC())
	ctx, cancel := context.WithTimeout(context.Background(), queryJobTimeout)
	defer cancel()

	result, rowCount, err := a.executeQueryJob(ctx, queryType, query)
	if err != nil {
		slog.Error("async query job failed", "query_id", queryID, "query_type", queryType, "error", err)
		a.jobs.fail(query.TenantID, queryID, err, a.now().UTC())
		return
	}
	a.jobs.succeed(query.TenantID, queryID, QueryManifest{
		ResultType:  queryType,
		RowCount:    rowCount,
		GeneratedAt: a.now().UTC(),
		Inline:      true,
	}, result, a.now().UTC())
}

func (a *API) executeQueryJob(ctx context.Context, queryType string, query AnalyticsQuery) (any, int, error) {
	query.Offset = 0
	switch queryType {
	case usageCursorKind:
		rows, err := a.repository.Usage(ctx, query)
		if err != nil {
			return nil, 0, fmt.Errorf("usage query failed: %w", err)
		}
		rows, nextCursor := paginate(rows, query, usageCursorKind)
		result := UsageResult{
			TenantID:    query.TenantID,
			ClusterID:   query.ClusterID,
			Start:       query.Start,
			End:         query.End,
			GroupBy:     query.GroupBy,
			GeneratedAt: a.now().UTC(),
			Rows:        rows,
			ResultCount: len(rows),
			Limit:       normalizedAnalyticsLimit(query.Limit),
			NextCursor:  nextCursor,
		}
		if quality, failed := a.queryQuality(ctx, query); failed {
			return nil, 0, errors.New("quality query failed")
		} else {
			result.DataThrough = quality.dataThrough
			result.Quality = quality.summary
		}
		return result, len(rows), nil
	case costsCursorKind:
		metadata, rows, err := a.repository.Costs(ctx, query)
		if err != nil {
			return nil, 0, fmt.Errorf("cost query failed: %w", err)
		}
		rows, nextCursor := paginate(rows, query, costsCursorKind)
		result := CostResult{
			TenantID:           query.TenantID,
			ClusterID:          query.ClusterID,
			Start:              query.Start,
			End:                query.End,
			GroupBy:            query.GroupBy,
			GeneratedAt:        a.now().UTC(),
			Currency:           metadata.Currency,
			ComputationVersion: metadata.ComputationVersion,
			ComputedAt:         metadata.ComputedAt,
			Rows:               rows,
			ResultCount:        len(rows),
			Limit:              normalizedAnalyticsLimit(query.Limit),
			NextCursor:         nextCursor,
		}
		if quality, failed := a.queryQuality(ctx, query); failed {
			return nil, 0, errors.New("quality query failed")
		} else {
			result.DataThrough = quality.dataThrough
			result.Quality = quality.summary
		}
		return result, len(rows), nil
	case allocationCursorKind:
		metadata, rows, err := a.repository.Allocation(ctx, query)
		if err != nil {
			return nil, 0, fmt.Errorf("allocation query failed: %w", err)
		}
		rows, nextCursor := paginate(rows, query, allocationCursorKind)
		result := AllocationResult{
			TenantID:           query.TenantID,
			ClusterID:          query.ClusterID,
			Start:              query.Start,
			End:                query.End,
			GroupBy:            query.GroupBy,
			GeneratedAt:        a.now().UTC(),
			Currency:           metadata.Currency,
			ComputationVersion: metadata.ComputationVersion,
			ComputedAt:         metadata.ComputedAt,
			Rows:               rows,
			ResultCount:        len(rows),
			Limit:              normalizedAnalyticsLimit(query.Limit),
			NextCursor:         nextCursor,
		}
		if quality, failed := a.queryQuality(ctx, query); failed {
			return nil, 0, errors.New("quality query failed")
		} else {
			result.DataThrough = quality.dataThrough
			result.Quality = quality.summary
		}
		return result, len(rows), nil
	default:
		return nil, 0, fmt.Errorf("unsupported query_type %q", queryType)
	}
}

func asyncAnalyticsQuery(w http.ResponseWriter, request QueryJobRequest, tenantID string) (AnalyticsQuery, string, bool) {
	queryType := strings.TrimSpace(request.QueryType)
	if queryType != usageCursorKind && queryType != costsCursorKind && queryType != allocationCursorKind {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "query_type must be one of usage, costs, allocation")
		return AnalyticsQuery{}, "", false
	}
	start, err := parseRequiredTime(request.Start, "start")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return AnalyticsQuery{}, "", false
	}
	end, err := parseRequiredTime(request.End, "end")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return AnalyticsQuery{}, "", false
	}
	if !start.Before(end) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "start must be before end")
		return AnalyticsQuery{}, "", false
	}
	if !start.Truncate(time.Hour).Equal(start) || !end.Truncate(time.Hour).Equal(end) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "start and end must be aligned to whole hours")
		return AnalyticsQuery{}, "", false
	}
	groupBy := strings.TrimSpace(request.GroupBy)
	if groupBy == "" {
		groupBy = "namespace"
	}
	if !validAnalyticsGroupBy(groupBy) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "group_by must be one of namespace, cluster, team, project, environment, cost_center")
		return AnalyticsQuery{}, "", false
	}
	if request.Limit < 0 {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
		return AnalyticsQuery{}, "", false
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultAnalyticsLimit
	}
	return AnalyticsQuery{
		TenantID:       tenantID,
		ClusterID:      strings.TrimSpace(request.ClusterID),
		Start:          start,
		End:            end,
		GroupBy:        groupBy,
		Limit:          limit,
		IncludeQuality: request.IncludeQuality,
	}, queryType, true
}

func (a *API) queryQuality(ctx context.Context, query AnalyticsQuery) (analyticsQuality, bool) {
	if !query.IncludeQuality {
		return analyticsQuality{}, false
	}
	window := durationValue("QUERY_FRESHNESS_WINDOW", defaultFreshnessWindow)
	signals, err := a.repository.DataQuality(ctx, DataQualityQuery{
		TenantID:        query.TenantID,
		ClusterID:       query.ClusterID,
		FreshnessWindow: window,
	})
	if err != nil {
		return analyticsQuality{failed: true}, true
	}
	result := summarizeDataQuality(query.TenantID, query.ClusterID, a.now().UTC(), window, signals)
	return analyticsQuality{dataThrough: result.DataThrough, summary: &result.Quality}, false
}
