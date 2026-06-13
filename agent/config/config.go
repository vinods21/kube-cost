package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TenantID                string
	ClusterID               string
	AgentInstanceID         string
	AgentVersion            string
	IngestionEndpoint       string
	InsecureGRPC            bool
	TLSCAFile               string
	TLSCertFile             string
	TLSKeyFile              string
	TLSServerName           string
	LeaderElection          bool
	LeaderElectionID        string
	LeaderElectionNamespace string
	ResyncInterval          time.Duration
	QueueCapacity           int
	BatchSize               int
	MetricsAddress          string
	HealthProbeAddress      string
}

func FromEnv() (Config, error) {
	instanceID, err := instanceID()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		TenantID:                strings.TrimSpace(os.Getenv("TENANT_ID")),
		ClusterID:               strings.TrimSpace(os.Getenv("CLUSTER_ID")),
		AgentInstanceID:         valueOrDefault("AGENT_INSTANCE_ID", instanceID),
		AgentVersion:            valueOrDefault("AGENT_VERSION", "dev"),
		IngestionEndpoint:       strings.TrimSpace(os.Getenv("INGESTION_ENDPOINT")),
		InsecureGRPC:            boolValue("INSECURE_GRPC", false),
		TLSCAFile:               strings.TrimSpace(os.Getenv("TLS_CA_FILE")),
		TLSCertFile:             strings.TrimSpace(os.Getenv("TLS_CERT_FILE")),
		TLSKeyFile:              strings.TrimSpace(os.Getenv("TLS_KEY_FILE")),
		TLSServerName:           strings.TrimSpace(os.Getenv("TLS_SERVER_NAME")),
		LeaderElection:          boolValue("LEADER_ELECTION", true),
		LeaderElectionID:        valueOrDefault("LEADER_ELECTION_ID", "agent.cost.kube-cost.io"),
		LeaderElectionNamespace: strings.TrimSpace(os.Getenv("POD_NAMESPACE")),
		ResyncInterval:          durationValue("INVENTORY_RESYNC_INTERVAL", 30*time.Minute),
		QueueCapacity:           intValue("INVENTORY_QUEUE_CAPACITY", 10000),
		BatchSize:               intValue("INVENTORY_BATCH_SIZE", 200),
		MetricsAddress:          valueOrDefault("METRICS_ADDRESS", ":8080"),
		HealthProbeAddress:      valueOrDefault("HEALTH_PROBE_ADDRESS", ":8081"),
	}

	var validationErrors []error
	if cfg.TenantID == "" {
		validationErrors = append(validationErrors, errors.New("TENANT_ID is required"))
	}
	if cfg.ClusterID == "" {
		validationErrors = append(validationErrors, errors.New("CLUSTER_ID is required"))
	}
	if cfg.IngestionEndpoint == "" {
		validationErrors = append(validationErrors, errors.New("INGESTION_ENDPOINT is required"))
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		validationErrors = append(validationErrors, errors.New("TLS_CERT_FILE and TLS_KEY_FILE must be configured together"))
	}
	if cfg.QueueCapacity < 1 || cfg.BatchSize < 1 {
		validationErrors = append(validationErrors, errors.New("queue capacity and batch size must be positive"))
	}
	if cfg.BatchSize > cfg.QueueCapacity {
		validationErrors = append(validationErrors, errors.New("INVENTORY_BATCH_SIZE cannot exceed INVENTORY_QUEUE_CAPACITY"))
	}
	return cfg, errors.Join(validationErrors...)
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

func instanceID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("read hostname: %w", err)
	}
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate instance ID: %w", err)
	}
	return hostname + "-" + hex.EncodeToString(random), nil
}
