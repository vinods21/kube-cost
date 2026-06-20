package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var ErrVersionExists = errors.New("policy version already exists")

type Store struct {
	mu       sync.RWMutex
	families map[string]map[string]*policyFamilyState
}

type policyFamilyState struct {
	activeVersion string
	versions      map[string]PolicyVersion
}

func NewStore() *Store {
	return &Store{families: make(map[string]map[string]*policyFamilyState)}
}

func (s *Store) CreateVersion(tenantID, family, createdBy string, request VersionRequest, now time.Time) (PolicyVersion, error) {
	version := request.Version
	if version == "" {
		version = newPolicyVersionID()
	}
	policy := PolicyVersion{
		TenantID:       tenantID,
		Family:         family,
		Version:        version,
		Description:    request.Description,
		Status:         "draft",
		EffectiveStart: mustPolicyTime(request.EffectiveStart, now),
		Rules:          cloneRawMessage(request.Rules),
		CreatedBy:      createdBy,
		CreatedAt:      now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tenantFamilies := s.families[tenantID]
	if tenantFamilies == nil {
		tenantFamilies = make(map[string]*policyFamilyState)
		s.families[tenantID] = tenantFamilies
	}
	state := tenantFamilies[family]
	if state == nil {
		state = &policyFamilyState{versions: make(map[string]PolicyVersion)}
		tenantFamilies[family] = state
	}
	if _, ok := state.versions[version]; ok {
		return PolicyVersion{}, ErrVersionExists
	}
	state.versions[version] = policy
	return clonePolicy(policy), nil
}

func (s *Store) ActivateVersion(tenantID, family, version, actor string, now time.Time) (PolicyVersion, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.families[tenantID][family]
	if state == nil {
		return PolicyVersion{}, false
	}
	policy, ok := state.versions[version]
	if !ok {
		return PolicyVersion{}, false
	}
	for currentVersion, current := range state.versions {
		if current.Status == "active" {
			current.Status = "inactive"
			state.versions[currentVersion] = current
		}
	}
	policy.Status = "active"
	policy.ActivatedBy = actor
	policy.ActivatedAt = &now
	state.activeVersion = version
	state.versions[version] = policy
	return clonePolicy(policy), true
}

func (s *Store) Families(tenantID string) []PolicyFamily {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tenantFamilies := s.families[tenantID]
	result := make([]PolicyFamily, 0, len(tenantFamilies))
	for family, state := range tenantFamilies {
		versions := make([]PolicyVersion, 0, len(state.versions))
		for _, version := range state.versions {
			versions = append(versions, clonePolicy(version))
		}
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].CreatedAt.Before(versions[j].CreatedAt)
		})
		result = append(result, PolicyFamily{
			TenantID:      tenantID,
			Family:        family,
			ActiveVersion: state.activeVersion,
			Versions:      versions,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Family < result[j].Family
	})
	return result
}

func newPolicyVersionID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("policy-%d", time.Now().UnixNano())
	}
	return "policy_" + hex.EncodeToString(data[:])
}

func mustPolicyTime(value string, fallback time.Time) time.Time {
	if value == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return fallback
	}
	return parsed.UTC()
}

func clonePolicy(policy PolicyVersion) PolicyVersion {
	policy.Rules = cloneRawMessage(policy.Rules)
	if policy.ActivatedAt != nil {
		activatedAt := *policy.ActivatedAt
		policy.ActivatedAt = &activatedAt
	}
	return policy
}

func cloneRawMessage(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	return append([]byte(nil), value...)
}
