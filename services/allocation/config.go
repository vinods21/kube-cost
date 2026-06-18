package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddress       string
	ClickHouse        ClickHouseConfig
	NodeHourlyCostUSD float64
}

func ConfigFromEnv() (Config, error) {
	nodeCost := floatValue("ALLOCATION_NODE_HOURLY_COST_USD", defaultNodeHourlyCostUSD)
	config := Config{
		HTTPAddress: valueOrDefault("HTTP_ADDRESS", ":8080"),
		ClickHouse: ClickHouseConfig{
			Address:  valueOrDefault("CLICKHOUSE_ADDRESS", "localhost:9000"),
			Database: valueOrDefault("CLICKHOUSE_DATABASE", "kube_cost"),
			Username: os.Getenv("CLICKHOUSE_USERNAME"),
			Password: os.Getenv("CLICKHOUSE_PASSWORD"),
			Secure:   boolValue("CLICKHOUSE_SECURE", false),
		},
		NodeHourlyCostUSD: nodeCost,
	}
	if config.NodeHourlyCostUSD <= 0 {
		return Config{}, fmt.Errorf("ALLOCATION_NODE_HOURLY_COST_USD must be positive")
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

func parseTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q as RFC3339: %w", value, err)
	}
	return parsed, nil
}
