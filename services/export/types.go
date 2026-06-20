package main

import "time"

const (
	tenantHeader        = "X-Kube-Cost-Tenant-ID"
	gatewaySecretHeader = "X-Kube-Cost-Gateway-Secret"
)

type ExportRequest struct {
	QueryType string `json:"query_type"`
	Format    string `json:"format"`
	ClusterID string `json:"cluster_id,omitempty"`
	Start     string `json:"start"`
	End       string `json:"end"`
	GroupBy   string `json:"group_by,omitempty"`
}

type ExportJob struct {
	ExportID  string         `json:"export_id"`
	TenantID  string         `json:"tenant_id"`
	Status    string         `json:"status"`
	Request   ExportSpec     `json:"request"`
	Manifest  ExportManifest `json:"manifest"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ExportSpec struct {
	QueryType string    `json:"query_type"`
	Format    string    `json:"format"`
	ClusterID string    `json:"cluster_id,omitempty"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	GroupBy   string    `json:"group_by"`
}

type ExportManifest struct {
	SchemaVersion string    `json:"schema_version"`
	ContentType   string    `json:"content_type"`
	ByteSize      int       `json:"byte_size"`
	SHA256        string    `json:"sha256"`
	Inline        bool      `json:"inline"`
	URI           string    `json:"uri,omitempty"`
	GeneratedAt   time.Time `json:"generated_at"`
}
