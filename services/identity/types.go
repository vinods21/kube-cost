package main

import "time"

const (
	tenantHeader        = "X-Kube-Cost-Tenant-ID"
	principalHeader     = "X-Kube-Cost-Principal-ID"
	gatewaySecretHeader = "X-Kube-Cost-Gateway-Secret"
)

type PrincipalProfile struct {
	TenantID    string    `json:"tenant_id"`
	PrincipalID string    `json:"principal_id"`
	Source      string    `json:"source"`
	SeenAt      time.Time `json:"seen_at"`
}
