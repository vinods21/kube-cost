package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ClickHouse ClickHouseConfig
}

func ConfigFromEnv() Config {
	return Config{
		ClickHouse: ClickHouseConfig{
			Address:  valueOrDefault("CLICKHOUSE_ADDRESS", "localhost:9000"),
			Database: valueOrDefault("CLICKHOUSE_DATABASE", "kube_cost"),
			Username: os.Getenv("CLICKHOUSE_USERNAME"),
			Password: os.Getenv("CLICKHOUSE_PASSWORD"),
			Secure:   boolValue("CLICKHOUSE_SECURE", false),
		},
	}
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
