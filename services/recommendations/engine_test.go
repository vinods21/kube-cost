package main

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestEngineGeneratesCPUAndMemoryRightsizingRecommendation(t *testing.T) {
	t.Parallel()
	end := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	samples := make([]Sample, 0, 100)
	for index := 1; index <= 100; index++ {
		memoryBytes := uint64(index) * 1024 * 1024
		samples = append(samples, sample(end.Add(time.Duration(index-100)*time.Hour), uint64(index), memoryBytes, 500, 512*1024*1024))
	}
	engine := NewEngine(&fakeRepository{samples: samples}, EngineConfig{
		AnalysisWindow:        30 * 24 * time.Hour,
		CPURequestHeadroom:    1.15,
		MemoryRequestHeadroom: 1.20,
		CPULimitMultiplier:    2,
		MemoryLimitMultiplier: 1.5,
		CPUCoreHourUSD:        0.03,
		MemoryGiBHourUSD:      0.004,
		MinimumSampleCount:    10,
		MinCPUMillicores:      10,
		MinMemoryBytes:        64 * 1024 * 1024,
	})

	result, err := engine.Recommendations(context.Background(), Query{TenantID: "tenant", End: end})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("recommendations=%d, want 1", len(result))
	}
	recommendation := result[0]
	if recommendation.CPUUsageP95Millicores != 95 {
		t.Fatalf("cpu p95=%d", recommendation.CPUUsageP95Millicores)
	}
	if recommendation.RecommendedCPURequestMillicores != 110 {
		t.Fatalf("recommended cpu request=%d", recommendation.RecommendedCPURequestMillicores)
	}
	if recommendation.MemoryWorkingSetP99Bytes != 99*1024*1024 {
		t.Fatalf("memory p99=%d", recommendation.MemoryWorkingSetP99Bytes)
	}
	if recommendation.RecommendedMemoryRequestBytes != 124570829 {
		t.Fatalf("recommended memory request=%d", recommendation.RecommendedMemoryRequestBytes)
	}
	if recommendation.RecommendedCPULimitMillicores != 220 {
		t.Fatalf("recommended cpu limit=%d", recommendation.RecommendedCPULimitMillicores)
	}
	if recommendation.RecommendedMemoryLimitBytes != 186856244 {
		t.Fatalf("recommended memory limit=%d", recommendation.RecommendedMemoryLimitBytes)
	}
	if recommendation.EstimatedMonthlySavingsUSD <= 0 {
		t.Fatalf("estimated savings=%f", recommendation.EstimatedMonthlySavingsUSD)
	}
	if recommendation.ComputationVersion != computationVersionV1 {
		t.Fatalf("version=%s", recommendation.ComputationVersion)
	}
}

func TestEngineSuppressesRecommendationWithoutSavings(t *testing.T) {
	t.Parallel()
	end := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	samples := []Sample{
		sample(end.Add(-3*time.Hour), 400, 500*1024*1024, 100, 128*1024*1024),
		sample(end.Add(-2*time.Hour), 450, 512*1024*1024, 100, 128*1024*1024),
		sample(end.Add(-time.Hour), 500, 600*1024*1024, 100, 128*1024*1024),
	}
	engine := NewEngine(&fakeRepository{samples: samples}, EngineConfig{MinimumSampleCount: 1})

	result, err := engine.Recommendations(context.Background(), Query{TenantID: "tenant", End: end})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("recommendations=%d, want 0", len(result))
	}
}

func TestEngineSuppressesRecommendationWithoutCurrentRequests(t *testing.T) {
	t.Parallel()
	end := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	engine := NewEngine(&fakeRepository{samples: []Sample{{
		TenantID:              "tenant",
		ClusterID:             "cluster",
		ScopeType:             "container",
		ScopeID:               "pod-1/app",
		BucketStart:           end.Add(-time.Hour),
		BucketSeconds:         3600,
		CPUUsageCoreMS:        100 * 3600,
		MemoryWorkingSetBytes: 128 * 1024 * 1024 * 3600,
	}}}, EngineConfig{MinimumSampleCount: 1})

	result, err := engine.Recommendations(context.Background(), Query{TenantID: "tenant", End: end})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("recommendations=%d, want 0", len(result))
	}
}

func TestNormalizeQueryRequiresTenant(t *testing.T) {
	t.Parallel()
	_, err := normalizeQuery(Query{}, time.Now(), defaultAnalysisWindow)
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("err=%v, want ErrInvalidQuery", err)
	}
}

func TestPercentileUsesNearestRank(t *testing.T) {
	t.Parallel()
	if got := percentile([]uint64{1, 2, 3, 4, 5}, 0.95); got != 5 {
		t.Fatalf("p95=%d, want 5", got)
	}
	if got := percentile([]uint64{5, 1, 3, 2, 4}, 0.50); got != 3 {
		t.Fatalf("p50=%d, want 3", got)
	}
}

func TestEstimatedMonthlySavingsUsesRequestDeltas(t *testing.T) {
	t.Parallel()
	engine := NewEngine(&fakeRepository{}, EngineConfig{
		CPUCoreHourUSD:   0.03,
		MemoryGiBHourUSD: 0.004,
	})
	savings := engine.estimatedMonthlySavings(500, 250, 2*1024*1024*1024, 1024*1024*1024)
	expected := (0.25 * 0.03 * defaultMonthlyHours) + (1 * 0.004 * defaultMonthlyHours)
	if math.Abs(savings-expected) > 0.000001 {
		t.Fatalf("savings=%f, want %f", savings, expected)
	}
}

func sample(bucket time.Time, cpuMillicores, memoryBytes, requestMillicores, requestMemoryBytes uint64) Sample {
	const seconds = 3600
	return Sample{
		TenantID:              "tenant",
		ClusterID:             "cluster",
		ScopeType:             "container",
		ScopeID:               "pod-1/app",
		BucketStart:           bucket,
		BucketSeconds:         seconds,
		CPUUsageCoreMS:        cpuMillicores * seconds,
		CPURequestCoreMS:      requestMillicores * seconds,
		CPULimitCoreMS:        requestMillicores * 2 * seconds,
		MemoryWorkingSetBytes: memoryBytes * seconds,
		MemoryRequestBytes:    requestMemoryBytes * seconds,
		MemoryLimitBytes:      requestMemoryBytes * 2 * seconds,
	}
}

type fakeRepository struct {
	samples []Sample
	err     error
}

func (r *fakeRepository) Samples(context.Context, Query, time.Duration) ([]Sample, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.samples, nil
}

func (r *fakeRepository) Ping(context.Context) error {
	return r.err
}

func (r *fakeRepository) Close() error {
	return nil
}
