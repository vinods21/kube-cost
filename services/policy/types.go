package main

import (
	"encoding/json"
	"time"
)

const (
	tenantHeader        = "X-Kube-Cost-Tenant-ID"
	principalHeader     = "X-Kube-Cost-Principal-ID"
	gatewaySecretHeader = "X-Kube-Cost-Gateway-Secret"
)

type VersionRequest struct {
	Version        string          `json:"version,omitempty"`
	Description    string          `json:"description,omitempty"`
	EffectiveStart string          `json:"effective_start,omitempty"`
	Rules          json.RawMessage `json:"rules"`
}

type PolicyVersion struct {
	TenantID       string          `json:"tenant_id"`
	Family         string          `json:"family"`
	Version        string          `json:"version"`
	Description    string          `json:"description,omitempty"`
	Status         string          `json:"status"`
	EffectiveStart time.Time       `json:"effective_start"`
	Rules          json.RawMessage `json:"rules"`
	CreatedBy      string          `json:"created_by,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	ActivatedBy    string          `json:"activated_by,omitempty"`
	ActivatedAt    *time.Time      `json:"activated_at,omitempty"`
}

type PolicyFamily struct {
	TenantID      string          `json:"tenant_id"`
	Family        string          `json:"family"`
	ActiveVersion string          `json:"active_version,omitempty"`
	Versions      []PolicyVersion `json:"versions"`
}

type PolicyFamiliesResult struct {
	TenantID    string         `json:"tenant_id"`
	Families    []PolicyFamily `json:"families"`
	ResultCount int            `json:"result_count"`
}
