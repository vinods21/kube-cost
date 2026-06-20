package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type Store struct {
	mu        sync.RWMutex
	maxEvents int
	events    []Event
}

func NewStore(maxEvents int) *Store {
	if maxEvents <= 0 {
		maxEvents = 1000
	}
	return &Store{maxEvents: maxEvents}
}

func (s *Store) Append(tenantID string, request EventRequest, now time.Time) Event {
	event := Event{
		AuditID:      newAuditID(),
		TenantID:     tenantID,
		ActorID:      request.ActorID,
		Action:       request.Action,
		ResourceType: request.ResourceType,
		ResourceID:   request.ResourceID,
		Outcome:      request.Outcome,
		Details:      cloneRawMessage(request.Details),
		OccurredAt:   now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if len(s.events) > s.maxEvents {
		s.events = append([]Event(nil), s.events[len(s.events)-s.maxEvents:]...)
	}
	return event
}

func (s *Store) List(tenantID string, filter EventFilter) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	result := make([]Event, 0, limit)
	for i := len(s.events) - 1; i >= 0 && len(result) < limit; i-- {
		event := s.events[i]
		if event.TenantID != tenantID ||
			(filter.ActorID != "" && event.ActorID != filter.ActorID) ||
			(filter.ResourceType != "" && event.ResourceType != filter.ResourceType) ||
			(filter.ResourceID != "" && event.ResourceID != filter.ResourceID) {
			continue
		}
		event.Details = cloneRawMessage(event.Details)
		result = append(result, event)
	}
	return result
}

func newAuditID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("audit-%d", time.Now().UnixNano())
	}
	return "audit_" + hex.EncodeToString(data[:])
}

func cloneRawMessage(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	return append([]byte(nil), value...)
}
