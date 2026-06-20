package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

type IntegrationStore struct {
	mu           sync.RWMutex
	integrations map[string]map[string]Integration
}

func NewIntegrationStore() *IntegrationStore {
	return &IntegrationStore{integrations: make(map[string]map[string]Integration)}
}

func (s *IntegrationStore) Create(tenantID, actor string, request IntegrationRequest, now time.Time) Integration {
	integration := Integration{
		IntegrationID: newIntegrationID(),
		TenantID:      tenantID,
		Name:          request.Name,
		Type:          request.Type,
		Provider:      request.Provider,
		AccountID:     request.AccountID,
		Region:        request.Region,
		SecretRef:     request.SecretRef,
		Config:        cloneRawMessage(request.Config),
		Status:        "pending_validation",
		CreatedBy:     actor,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tenantIntegrations := s.integrations[tenantID]
	if tenantIntegrations == nil {
		tenantIntegrations = make(map[string]Integration)
		s.integrations[tenantID] = tenantIntegrations
	}
	tenantIntegrations[integration.IntegrationID] = integration
	return cloneIntegration(integration)
}

func (s *IntegrationStore) List(tenantID string) []Integration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tenantIntegrations := s.integrations[tenantID]
	result := make([]Integration, 0, len(tenantIntegrations))
	for _, integration := range tenantIntegrations {
		result = append(result, cloneIntegration(integration))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

func (s *IntegrationStore) Validate(tenantID, integrationID string, now time.Time) (Integration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tenantIntegrations := s.integrations[tenantID]
	if tenantIntegrations == nil {
		return Integration{}, false
	}
	integration, ok := tenantIntegrations[integrationID]
	if !ok {
		return Integration{}, false
	}
	integration.Status = "validated"
	integration.LastValidated = &now
	integration.UpdatedAt = now
	tenantIntegrations[integrationID] = integration
	return cloneIntegration(integration), true
}

func newIntegrationID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("integration-%d", time.Now().UnixNano())
	}
	return "integration_" + hex.EncodeToString(data[:])
}

func cloneIntegration(integration Integration) Integration {
	integration.Config = cloneRawMessage(integration.Config)
	if integration.LastValidated != nil {
		lastValidated := *integration.LastValidated
		integration.LastValidated = &lastValidated
	}
	return integration
}

func cloneRawMessage(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	return append([]byte(nil), value...)
}
