package main

import "time"

const (
	tenantHeader             = "X-Kube-Cost-Tenant-ID"
	defaultFreshnessWindow   = 10 * time.Minute
	defaultStaleStatus       = "stale"
	defaultFreshStatus       = "fresh"
	defaultEmptyStatus       = "empty"
	computationVersionDataQ1 = "data-quality-v1"
)

type DataQualityQuery struct {
	TenantID        string
	ClusterID       string
	FreshnessWindow time.Duration
}

type DataQualityResult struct {
	TenantID           string              `json:"tenant_id"`
	ClusterID          string              `json:"cluster_id,omitempty"`
	GeneratedAt        time.Time           `json:"generated_at"`
	DataThrough        *time.Time          `json:"data_through,omitempty"`
	ComputationVersion string              `json:"computation_version"`
	Signals            []DataQualitySignal `json:"signals"`
	Quality            QualitySummary      `json:"quality"`
}

type DataQualitySignal struct {
	Source                 string     `json:"source"`
	Grain                  string     `json:"grain"`
	ClusterID              string     `json:"cluster_id,omitempty"`
	RecordCount            uint64     `json:"record_count"`
	LatestBucketStart      *time.Time `json:"latest_bucket_start,omitempty"`
	LatestIngestedAt       *time.Time `json:"latest_ingested_at,omitempty"`
	FreshnessSeconds       *int64     `json:"freshness_seconds,omitempty"`
	Status                 string     `json:"status"`
	Warning                string     `json:"warning,omitempty"`
	ExpectedFreshnessLimit string     `json:"expected_freshness_limit"`
}

type QualitySummary struct {
	Status              string   `json:"status"`
	EstimatedPercent    float64  `json:"estimated_percent"`
	MissingScopes       []string `json:"missing_scopes,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
	FreshnessWindowSecs int64    `json:"freshness_window_seconds"`
}
