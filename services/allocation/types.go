package main

import "time"

const (
	defaultNodeHourlyCostUSD         = 0.10
	defaultControlPlaneHourlyCostUSD = 0.05
	defaultNetworkCostPerGiBUSD      = 0.01
	defaultCurrency                  = "USD"
	allocationMethodCPU              = "cpu_request_allocatable"
	computationVersionV1             = "allocation-v1-cpu-request-overhead-idle"
	idleNamespaceUID                 = "__idle__"
	idleNamespaceName                = "__idle__"
)

type Query struct {
	TenantID  string
	ClusterID string
	Start     time.Time
	End       time.Time
}

type Result struct {
	Currency                  string          `json:"currency"`
	AllocationMethod          string          `json:"allocation_method"`
	NodeHourlyCostUSD         float64         `json:"node_hourly_cost_usd"`
	ControlPlaneHourlyCostUSD float64         `json:"control_plane_hourly_cost_usd"`
	NetworkCostPerGiBUSD      float64         `json:"network_cost_per_gib_usd"`
	Start                     time.Time       `json:"start"`
	End                       time.Time       `json:"end"`
	Items                     []NamespaceCost `json:"items"`
}

type NamespaceCost struct {
	TenantID                   string  `json:"tenant_id"`
	ClusterID                  string  `json:"cluster_id"`
	NamespaceUID               string  `json:"namespace_uid"`
	NamespaceName              string  `json:"namespace_name"`
	BucketStart                string  `json:"bucket_start"`
	CPURequestCoreMilliseconds uint64  `json:"cpu_request_core_milliseconds"`
	NetworkBytes               uint64  `json:"network_bytes"`
	AllocationWeight           float64 `json:"allocation_weight"`
	DirectCost                 float64 `json:"direct_cost"`
	IdleCost                   float64 `json:"idle_cost"`
	NetworkCost                float64 `json:"network_cost"`
	ControlPlaneCost           float64 `json:"control_plane_cost"`
	SystemNamespaceCost        float64 `json:"system_namespace_cost"`
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
	NetworkBytes               uint64
	NodeAllocatableMillicores  uint64
}

type AllocationOptions struct {
	NodeHourlyCostUSD         float64
	ControlPlaneHourlyCostUSD float64
	NetworkCostPerGiBUSD      float64
}
