package main

import "time"

const (
	defaultEnrollmentTTL = 15 * time.Minute
)

type Cluster struct {
	TenantID     string            `json:"tenant_id"`
	ClusterID    string            `json:"cluster_id"`
	ClusterName  string            `json:"cluster_name"`
	Provider     string            `json:"provider,omitempty"`
	AccountID    string            `json:"account_id,omitempty"`
	Region       string            `json:"region,omitempty"`
	Status       string            `json:"status"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type RegisterClusterRequest struct {
	ClusterName  string            `json:"cluster_name"`
	Provider     string            `json:"provider,omitempty"`
	AccountID    string            `json:"account_id,omitempty"`
	Region       string            `json:"region,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

type RegisterClusterResponse struct {
	Cluster         Cluster   `json:"cluster"`
	EnrollmentToken string    `json:"enrollment_token"`
	TokenExpiresAt  time.Time `json:"token_expires_at"`
}
