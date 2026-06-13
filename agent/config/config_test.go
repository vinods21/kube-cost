package config

import "testing"

func TestFromEnvRequiresIdentityAndEndpoint(t *testing.T) {
	t.Setenv("TENANT_ID", "")
	t.Setenv("CLUSTER_ID", "")
	t.Setenv("INGESTION_ENDPOINT", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected required configuration error")
	}
}

func TestFromEnvLoadsInventorySettings(t *testing.T) {
	t.Setenv("TENANT_ID", "tenant")
	t.Setenv("CLUSTER_ID", "cluster")
	t.Setenv("INGESTION_ENDPOINT", "ingestion:443")
	t.Setenv("INVENTORY_QUEUE_CAPACITY", "100")
	t.Setenv("INVENTORY_BATCH_SIZE", "25")
	t.Setenv("INSECURE_GRPC", "true")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.QueueCapacity != 100 || cfg.BatchSize != 25 || !cfg.InsecureGRPC {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
}

func TestFromEnvRequiresCertificatePair(t *testing.T) {
	t.Setenv("TENANT_ID", "tenant")
	t.Setenv("CLUSTER_ID", "cluster")
	t.Setenv("INGESTION_ENDPOINT", "ingestion:443")
	t.Setenv("TLS_CERT_FILE", "client.crt")
	t.Setenv("TLS_KEY_FILE", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected certificate pair validation error")
	}
}
