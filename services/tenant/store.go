package main

import (
	"sort"
	"sync"
	"time"
)

type Store struct {
	mu      sync.RWMutex
	members map[string]map[string]Member
}

func NewStore() *Store {
	return &Store{members: make(map[string]map[string]Member)}
}

func (s *Store) UpsertMember(tenantID, principalID, role, displayName string, now time.Time) Member {
	s.mu.Lock()
	defer s.mu.Unlock()
	tenantMembers := s.members[tenantID]
	if tenantMembers == nil {
		tenantMembers = make(map[string]Member)
		s.members[tenantID] = tenantMembers
	}
	member := tenantMembers[principalID]
	if member.CreatedAt.IsZero() {
		member.CreatedAt = now
	}
	member.TenantID = tenantID
	member.PrincipalID = principalID
	member.Role = role
	member.DisplayName = displayName
	member.UpdatedAt = now
	tenantMembers[principalID] = member
	return member
}

func (s *Store) Members(tenantID string) []Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tenantMembers := s.members[tenantID]
	members := make([]Member, 0, len(tenantMembers))
	for _, member := range tenantMembers {
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool {
		return members[i].PrincipalID < members[j].PrincipalID
	})
	return members
}

func (s *Store) DeleteMember(tenantID, principalID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	tenantMembers := s.members[tenantID]
	if tenantMembers == nil {
		return false
	}
	if _, ok := tenantMembers[principalID]; !ok {
		return false
	}
	delete(tenantMembers, principalID)
	return true
}
