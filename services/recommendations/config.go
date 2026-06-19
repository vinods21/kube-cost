package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TenantID   string
	ClusterID  string
	ClickHouse ClickHouseConfig
	Engine     EngineConfig
}

func ConfigFromEnv() (Config, error) {
	config := Config{
		TenantID:  strings.TrimSpace(os.Getenv("TENANT_ID")),
		ClusterID: strings.TrimSpace(os.Getenv("CLUSTER_ID")),
		ClickHouse: ClickHouseConfig{
			Address:  valueOrDefault("CLICKHOUSE_ADDRESS", "localhost:9000"),
			Database: valueOrDefault("CLICKHOUSE_DATABASE", "kube_cost"),
			Username: os.Getenv("CLICKHOUSE_USERNAME"),
			Password: os.Getenv("CLICKHOUSE_PASSWORD"),
			Secure:   boolValue("CLICKHOUSE_SECURE", false),
		},
		Engine: EngineConfig{
			AnalysisWindow:        durationValue("OPTIMIZATION_ANALYSIS_WINDOW", defaultAnalysisWindow),
			CPURequestHeadroom:    floatValue("OPTIMIZATION_CPU_REQUEST_HEADROOM", defaultCPURequestHeadroom),
			MemoryRequestHeadroom: floatValue("OPTIMIZATION_MEMORY_REQUEST_HEADROOM", defaultMemoryRequestHeadroom),
			CPULimitMultiplier:    floatValue("OPTIMIZATION_CPU_LIMIT_MULTIPLIER", defaultCPULimitMultiplier),
			MemoryLimitMultiplier: floatValue("OPTIMIZATION_MEMORY_LIMIT_MULTIPLIER", defaultMemoryLimitMultiplier),
			CPUCoreHourUSD:        floatValue("OPTIMIZATION_CPU_CORE_HOUR_USD", defaultCPUCoreHourUSD),
			MemoryGiBHourUSD:      floatValue("OPTIMIZATION_MEMORY_GIB_HOUR_USD", defaultMemoryGiBHourUSD),
			MinimumSampleCount:    intValue("OPTIMIZATION_MINIMUM_SAMPLE_COUNT", 24),
			MinCPUMillicores:      uintValue("OPTIMIZATION_MIN_CPU_MILLICORES", defaultMinCPUMillicores),
			MinMemoryBytes:        uintValue("OPTIMIZATION_MIN_MEMORY_BYTES", defaultMinMemoryBytes),
		},
	}
	if config.TenantID == "" {
		return Config{}, fmt.Errorf("TENANT_ID is required")
	}
	return config, nil
}

func valueOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func boolValue(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func floatValue(name string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func intValue(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func uintValue(name string, fallback uint64) uint64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationValue(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
