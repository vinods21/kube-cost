package main

import "time"

const (
	defaultNodeHourlyCostUSD = 0.10
	defaultCurrency          = "USD"
	allocationMethodCPU      = "cpu_request"
	computationVersionV1     = "allocation-v1-cpu-request-static-node"
)

type Query struct {
	TenantID  string
	ClusterID string
	Start     time.Time
	End       time.Time
}

type Result struct {
	Currency          string          `json:"currency"`
	AllocationMethod  string          `json:"allocation_method"`
	NodeHourlyCostUSD float64         `json:"node_hourly_cost_usd"`
	Start             time.Time       `json:"start"`
	End               time.Time       `json:"end"`
	Items             []NamespaceCost `json:"items"`
}

type NamespaceCost struct {
	TenantID                   string  `json:"tenant_id"`
	ClusterID                  string  `json:"cluster_id"`
	NamespaceUID               string  `json:"namespace_uid"`
	NamespaceName              string  `json:"namespace_name"`
	BucketStart                string  `json:"bucket_start"`
	CPURequestCoreMilliseconds uint64  `json:"cpu_request_core_milliseconds"`
	AllocationWeight           float64 `json:"allocation_weight"`
	AllocatedCost              float64 `json:"allocated_cost"`
	Currency                   string  `json:"currency"`
	AllocationMethod           string  `json:"allocation_method"`
	ComputationVersion         string  `json:"computation_version"`
}

type NodeNamespaceRequest struct {
	TenantID                   string
	ClusterID                  string
	NodeUID                    string
	NamespaceUID               string
	NamespaceName              string
	BucketStart                time.Time
	CPURequestCoreMilliseconds uint64
}
