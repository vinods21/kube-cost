package main

import (
	"encoding/json"
	"time"
)

const (
	tenantHeader        = "X-Kube-Cost-Tenant-ID"
	gatewaySecretHeader = "X-Kube-Cost-Gateway-Secret"
)

type EventRequest struct {
	ActorID      string          `json:"actor_id"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Outcome      string          `json:"outcome"`
	Details      json.RawMessage `json:"details,omitempty"`
}

type Event struct {
	AuditID      string          `json:"audit_id"`
	TenantID     string          `json:"tenant_id"`
	ActorID      string          `json:"actor_id"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Outcome      string          `json:"outcome"`
	Details      json.RawMessage `json:"details,omitempty"`
	OccurredAt   time.Time       `json:"occurred_at"`
}

type EventFilter struct {
	ActorID      string
	ResourceType string
	ResourceID   string
	Limit        int
}

type EventsResult struct {
	TenantID string  `json:"tenant_id"`
	Events   []Event `json:"events"`
	Limit    int     `json:"limit"`
}
