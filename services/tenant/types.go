package main

import "time"

const (
	tenantHeader        = "X-Kube-Cost-Tenant-ID"
	gatewaySecretHeader = "X-Kube-Cost-Gateway-Secret"
)

type TenantProfile struct {
	TenantID string    `json:"tenant_id"`
	Status   string    `json:"status"`
	Source   string    `json:"source"`
	SeenAt   time.Time `json:"seen_at"`
}

type MemberRequest struct {
	Role        string `json:"role"`
	DisplayName string `json:"display_name,omitempty"`
}

type Member struct {
	TenantID    string    `json:"tenant_id"`
	PrincipalID string    `json:"principal_id"`
	Role        string    `json:"role"`
	DisplayName string    `json:"display_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MembersResult struct {
	TenantID string   `json:"tenant_id"`
	Members  []Member `json:"members"`
}
