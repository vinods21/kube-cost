package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var ErrInvalidQuery = errors.New("invalid optimization query")

type MetricsRepository interface {
	Samples(context.Context, Query, time.Duration) ([]Sample, error)
	Ping(context.Context) error
	Close() error
}

type Engine struct {
	repository MetricsRepository
	config     EngineConfig
}

func NewEngine(repository MetricsRepository, config EngineConfig) *Engine {
	return &Engine{
		repository: repository,
		config:     normalizeConfig(config),
	}
}

func (e *Engine) Recommendations(ctx context.Context, query Query) ([]Recommendation, error) {
	normalized, err := normalizeQuery(query, time.Now().UTC(), e.config.AnalysisWindow)
	if err != nil {
		return nil, err
	}
	samples, err := e.repository.Samples(ctx, normalized, e.config.AnalysisWindow)
	if err != nil {
		return nil, err
	}
	return e.Generate(samples, normalized), nil
}

func (e *Engine) Generate(samples []Sample, query Query) []Recommendation {
	byTarget := make(map[string][]Sample)
	for _, sample := range samples {
		if sample.BucketSeconds == 0 || sample.ScopeID == "" {
			continue
		}
		key := targetKey(sample)
		byTarget[key] = append(byTarget[key], sample)
	}

	recommendations := make([]Recommendation, 0, len(byTarget))
	for _, targetSamples := range byTarget {
		sort.Slice(targetSamples, func(i, j int) bool {
			return targetSamples[i].BucketStart.Before(targetSamples[j].BucketStart)
		})
		recommendation, ok := e.recommend(targetSamples, query)
		if ok {
			recommendations = append(recommendations, recommendation)
		}
	}
	sort.Slice(recommendations, func(i, j int) bool {
		if recommendations[i].EstimatedMonthlySavingsUSD != recommendations[j].EstimatedMonthlySavingsUSD {
			return recommendations[i].EstimatedMonthlySavingsUSD > recommendations[j].EstimatedMonthlySavingsUSD
		}
		if recommendations[i].ClusterID != recommendations[j].ClusterID {
			return recommendations[i].ClusterID < recommendations[j].ClusterID
		}
		return recommendations[i].ScopeID < recommendations[j].ScopeID
	})
	return recommendations
}

func (e *Engine) recommend(samples []Sample, query Query) (Recommendation, bool) {
	if len(samples) < e.config.MinimumSampleCount {
		return Recommendation{}, false
	}
	cpuUsage := make([]uint64, 0, len(samples))
	memoryUsage := make([]uint64, 0, len(samples))
	for _, sample := range samples {
		cpuUsage = append(cpuUsage, averageMillicores(sample.CPUUsageCoreMS, sample.BucketSeconds))
		memoryUsage = append(memoryUsage, averageBytes(sample.MemoryWorkingSetBytes, sample.BucketSeconds))
	}

	cpuP95 := percentile(cpuUsage, 0.95)
	memoryP99 := percentile(memoryUsage, 0.99)
	currentCPURequest, currentMemoryRequest, currentCPULimit, currentMemoryLimit := currentConfiguration(samples)
	if currentCPURequest == 0 || currentMemoryRequest == 0 || (cpuP95 == 0 && memoryP99 == 0) {
		return Recommendation{}, false
	}
	recommendedCPURequest := maxUint64(e.config.MinCPUMillicores, ceilUint64(float64(cpuP95)*e.config.CPURequestHeadroom))
	recommendedMemoryRequest := maxUint64(e.config.MinMemoryBytes, ceilUint64(float64(memoryP99)*e.config.MemoryRequestHeadroom))
	recommendedCPULimit := maxUint64(recommendedCPURequest, ceilUint64(float64(recommendedCPURequest)*e.config.CPULimitMultiplier))
	recommendedMemoryLimit := maxUint64(recommendedMemoryRequest, ceilUint64(float64(recommendedMemoryRequest)*e.config.MemoryLimitMultiplier))
	savings := e.estimatedMonthlySavings(currentCPURequest, recommendedCPURequest, currentMemoryRequest, recommendedMemoryRequest)
	if savings <= 0 {
		return Recommendation{}, false
	}

	first := samples[0]
	return Recommendation{
		TenantID:                        first.TenantID,
		ClusterID:                       first.ClusterID,
		ScopeType:                       first.ScopeType,
		ScopeID:                         first.ScopeID,
		AnalysisWindowStart:             query.End.Add(-e.config.AnalysisWindow),
		AnalysisWindowEnd:               query.End,
		CPUUsageP95Millicores:           cpuP95,
		MemoryWorkingSetP99Bytes:        memoryP99,
		CurrentCPURequestMillicores:     currentCPURequest,
		RecommendedCPURequestMillicores: recommendedCPURequest,
		CurrentMemoryRequestBytes:       currentMemoryRequest,
		RecommendedMemoryRequestBytes:   recommendedMemoryRequest,
		CurrentCPULimitMillicores:       currentCPULimit,
		RecommendedCPULimitMillicores:   recommendedCPULimit,
		CurrentMemoryLimitBytes:         currentMemoryLimit,
		RecommendedMemoryLimitBytes:     recommendedMemoryLimit,
		EstimatedMonthlySavingsUSD:      savings,
		SampleCount:                     len(samples),
		ComputationVersion:              computationVersionV1,
	}, true
}

func (e *Engine) estimatedMonthlySavings(currentCPU, recommendedCPU, currentMemory, recommendedMemory uint64) float64 {
	cpuDeltaMillicores := positiveDelta(currentCPU, recommendedCPU)
	memoryDeltaBytes := positiveDelta(currentMemory, recommendedMemory)
	cpuSavings := (float64(cpuDeltaMillicores) / 1000) * e.config.CPUCoreHourUSD * defaultMonthlyHours
	memorySavings := (float64(memoryDeltaBytes) / 1073741824) * e.config.MemoryGiBHourUSD * defaultMonthlyHours
	return roundMoney(cpuSavings + memorySavings)
}

func normalizeQuery(query Query, now time.Time, window time.Duration) (Query, error) {
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.ClusterID = strings.TrimSpace(query.ClusterID)
	if query.TenantID == "" {
		return Query{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidQuery)
	}
	if query.End.IsZero() {
		query.End = now.Truncate(time.Hour)
	}
	query.End = query.End.UTC()
	if window <= 0 {
		return Query{}, fmt.Errorf("%w: analysis window must be positive", ErrInvalidQuery)
	}
	return query, nil
}

func normalizeConfig(config EngineConfig) EngineConfig {
	if config.AnalysisWindow <= 0 {
		config.AnalysisWindow = defaultAnalysisWindow
	}
	if config.CPURequestHeadroom <= 0 {
		config.CPURequestHeadroom = defaultCPURequestHeadroom
	}
	if config.MemoryRequestHeadroom <= 0 {
		config.MemoryRequestHeadroom = defaultMemoryRequestHeadroom
	}
	if config.CPULimitMultiplier <= 0 {
		config.CPULimitMultiplier = defaultCPULimitMultiplier
	}
	if config.MemoryLimitMultiplier <= 0 {
		config.MemoryLimitMultiplier = defaultMemoryLimitMultiplier
	}
	if config.MinCPUMillicores == 0 {
		config.MinCPUMillicores = defaultMinCPUMillicores
	}
	if config.MinMemoryBytes == 0 {
		config.MinMemoryBytes = defaultMinMemoryBytes
	}
	if config.CPUCoreHourUSD <= 0 {
		config.CPUCoreHourUSD = defaultCPUCoreHourUSD
	}
	if config.MemoryGiBHourUSD <= 0 {
		config.MemoryGiBHourUSD = defaultMemoryGiBHourUSD
	}
	if config.MinimumSampleCount <= 0 {
		config.MinimumSampleCount = 24
	}
	return config
}

func currentConfiguration(samples []Sample) (cpuRequest, memoryRequest, cpuLimit, memoryLimit uint64) {
	for index := len(samples) - 1; index >= 0; index-- {
		sample := samples[index]
		if sample.BucketSeconds == 0 {
			continue
		}
		if cpuRequest == 0 {
			cpuRequest = averageMillicores(sample.CPURequestCoreMS, sample.BucketSeconds)
		}
		if memoryRequest == 0 {
			memoryRequest = averageBytes(sample.MemoryRequestBytes, sample.BucketSeconds)
		}
		if cpuLimit == 0 {
			cpuLimit = averageMillicores(sample.CPULimitCoreMS, sample.BucketSeconds)
		}
		if memoryLimit == 0 {
			memoryLimit = averageBytes(sample.MemoryLimitBytes, sample.BucketSeconds)
		}
		if cpuRequest > 0 && memoryRequest > 0 && cpuLimit > 0 && memoryLimit > 0 {
			break
		}
	}
	return cpuRequest, memoryRequest, cpuLimit, memoryLimit
}

func averageMillicores(coreMilliseconds, seconds uint64) uint64 {
	if seconds == 0 {
		return 0
	}
	return ceilDiv(coreMilliseconds, seconds)
}

func averageBytes(byteSeconds, seconds uint64) uint64 {
	if seconds == 0 {
		return 0
	}
	return ceilDiv(byteSeconds, seconds)
}

func percentile(values []uint64, quantile float64) uint64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]uint64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func targetKey(sample Sample) string {
	return sample.TenantID + "\x00" + sample.ClusterID + "\x00" + sample.ScopeType + "\x00" + sample.ScopeID
}

func ceilDiv(value, divisor uint64) uint64 {
	if value == 0 {
		return 0
	}
	return ((value - 1) / divisor) + 1
}

func ceilUint64(value float64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(math.Ceil(value))
}

func maxUint64(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}

func positiveDelta(current, recommended uint64) uint64 {
	if current <= recommended {
		return 0
	}
	return current - recommended
}

func roundMoney(value float64) float64 {
	return math.Round(value*1000000) / 1000000
}
