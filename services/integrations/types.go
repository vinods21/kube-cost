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

type IntegrationRequest struct {
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Provider  string          `json:"provider"`
	AccountID string          `json:"account_id,omitempty"`
	Region    string          `json:"region,omitempty"`
	SecretRef string          `json:"secret_ref,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
}

type Integration struct {
	IntegrationID string          `json:"integration_id"`
	TenantID      string          `json:"tenant_id"`
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Provider      string          `json:"provider"`
	AccountID     string          `json:"account_id,omitempty"`
	Region        string          `json:"region,omitempty"`
	SecretRef     string          `json:"secret_ref,omitempty"`
	Config        json.RawMessage `json:"config,omitempty"`
	Status        string          `json:"status"`
	CreatedBy     string          `json:"created_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	LastValidated *time.Time      `json:"last_validated_at,omitempty"`
}

type IntegrationsResult struct {
	TenantID     string        `json:"tenant_id"`
	Integrations []Integration `json:"integrations"`
	ResultCount  int           `json:"result_count"`
}

type ValidationResult struct {
	TenantID      string    `json:"tenant_id"`
	IntegrationID string    `json:"integration_id"`
	Status        string    `json:"status"`
	ValidatedAt   time.Time `json:"validated_at"`
	Message       string    `json:"message"`
}

type Snapshot struct {
	ClusterID   string      `json:"cluster_id"`
	GeneratedAt time.Time   `json:"generated_at"`
	NodePools   []NodePool  `json:"node_pools"`
	NodeClasses []NodeClass `json:"node_classes"`
	NodeClaims  []NodeClaim `json:"node_claims"`
}

type NodePool struct {
	Name               string   `json:"name"`
	UID                string   `json:"uid"`
	NodeClassName      string   `json:"node_class_name"`
	CapacityTypes      []string `json:"capacity_types"`
	InstanceCategories []string `json:"instance_categories"`
	InstanceTypes      []string `json:"instance_types"`
	Zones              []string `json:"zones"`
	Architectures      []string `json:"architectures"`
	Consolidation      bool     `json:"consolidation"`
	Weight             int64    `json:"weight"`
}

type NodeClass struct {
	Name           string   `json:"name"`
	UID            string   `json:"uid"`
	APIVersion     string   `json:"api_version"`
	Subnets        []string `json:"subnets"`
	SecurityGroups []string `json:"security_groups"`
	AMIFamily      string   `json:"ami_family"`
	Role           string   `json:"role"`
}

type NodeClaim struct {
	Name                     string  `json:"name"`
	UID                      string  `json:"uid"`
	NodePoolName             string  `json:"node_pool_name"`
	NodeClassName            string  `json:"node_class_name"`
	NodeName                 string  `json:"node_name"`
	CapacityType             string  `json:"capacity_type"`
	InstanceType             string  `json:"instance_type"`
	Zone                     string  `json:"zone"`
	Architecture             string  `json:"architecture"`
	Ready                    bool    `json:"ready"`
	CPUCapacityMillicores    uint64  `json:"cpu_capacity_millicores"`
	MemoryCapacityBytes      uint64  `json:"memory_capacity_bytes"`
	CPURequestedMillicores   uint64  `json:"cpu_requested_millicores"`
	MemoryRequestedBytes     uint64  `json:"memory_requested_bytes"`
	CPUUtilizationPercent    float64 `json:"cpu_utilization_percent"`
	MemoryUtilizationPercent float64 `json:"memory_utilization_percent"`
}

type Scores struct {
	ClusterID            string          `json:"cluster_id"`
	GeneratedAt          time.Time       `json:"generated_at"`
	NodePools            []NodePoolScore `json:"node_pools"`
	BinPackingScore      float64         `json:"bin_packing_score"`
	SpotSuitabilityScore float64         `json:"spot_suitability_score"`
	NodeUtilizationScore float64         `json:"node_utilization_score"`
}

type NodePoolScore struct {
	NodePoolName         string  `json:"node_pool_name"`
	NodeClassName        string  `json:"node_class_name"`
	NodeClaimCount       int     `json:"node_claim_count"`
	ReadyNodeClaimCount  int     `json:"ready_node_claim_count"`
	BinPackingScore      float64 `json:"bin_packing_score"`
	SpotSuitabilityScore float64 `json:"spot_suitability_score"`
	NodeUtilizationScore float64 `json:"node_utilization_score"`
}
