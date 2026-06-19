package main

import "time"

const (
	defaultAnalysisWindow        = 30 * 24 * time.Hour
	defaultCPURequestHeadroom    = 1.15
	defaultMemoryRequestHeadroom = 1.20
	defaultCPULimitMultiplier    = 2.0
	defaultMemoryLimitMultiplier = 1.5
	defaultMinCPUMillicores      = uint64(10)
	defaultMinMemoryBytes        = uint64(64 * 1024 * 1024)
	defaultMonthlyHours          = 730.0
	defaultCPUCoreHourUSD        = 0.03
	defaultMemoryGiBHourUSD      = 0.004
	computationVersionV1         = "optimization-v1-cpu-p95-memory-p99"
)

type Query struct {
	TenantID  string
	ClusterID string
	End       time.Time
}

type Sample struct {
	TenantID              string
	ClusterID             string
	ScopeType             string
	ScopeID               string
	BucketStart           time.Time
	BucketSeconds         uint64
	CPUUsageCoreMS        uint64
	CPURequestCoreMS      uint64
	CPULimitCoreMS        uint64
	MemoryWorkingSetBytes uint64
	MemoryRequestBytes    uint64
	MemoryLimitBytes      uint64
}

type Recommendation struct {
	TenantID                        string    `json:"tenant_id"`
	ClusterID                       string    `json:"cluster_id"`
	ScopeType                       string    `json:"scope_type"`
	ScopeID                         string    `json:"scope_id"`
	AnalysisWindowStart             time.Time `json:"analysis_window_start"`
	AnalysisWindowEnd               time.Time `json:"analysis_window_end"`
	CPUUsageP95Millicores           uint64    `json:"cpu_usage_p95_millicores"`
	MemoryWorkingSetP99Bytes        uint64    `json:"memory_working_set_p99_bytes"`
	CurrentCPURequestMillicores     uint64    `json:"current_cpu_request_millicores"`
	RecommendedCPURequestMillicores uint64    `json:"recommended_cpu_request_millicores"`
	CurrentMemoryRequestBytes       uint64    `json:"current_memory_request_bytes"`
	RecommendedMemoryRequestBytes   uint64    `json:"recommended_memory_request_bytes"`
	CurrentCPULimitMillicores       uint64    `json:"current_cpu_limit_millicores"`
	RecommendedCPULimitMillicores   uint64    `json:"recommended_cpu_limit_millicores"`
	CurrentMemoryLimitBytes         uint64    `json:"current_memory_limit_bytes"`
	RecommendedMemoryLimitBytes     uint64    `json:"recommended_memory_limit_bytes"`
	EstimatedMonthlySavingsUSD      float64   `json:"estimated_monthly_savings_usd"`
	SampleCount                     int       `json:"sample_count"`
	ComputationVersion              string    `json:"computation_version"`
}

type EngineConfig struct {
	AnalysisWindow        time.Duration
	CPURequestHeadroom    float64
	MemoryRequestHeadroom float64
	CPULimitMultiplier    float64
	MemoryLimitMultiplier float64
	MinCPUMillicores      uint64
	MinMemoryBytes        uint64
	CPUCoreHourUSD        float64
	MemoryGiBHourUSD      float64
	MinimumSampleCount    int
}
