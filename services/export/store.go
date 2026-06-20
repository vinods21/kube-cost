package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Store struct {
	mu      sync.RWMutex
	maxJobs int
	order   []string
	jobs    map[string]ExportJob
}

func NewStore(maxJobs int) *Store {
	if maxJobs <= 0 {
		maxJobs = 1000
	}
	return &Store{maxJobs: maxJobs, jobs: make(map[string]ExportJob)}
}

func (s *Store) Create(tenantID string, spec ExportSpec, now time.Time) (ExportJob, error) {
	job := ExportJob{
		ExportID:  newExportID(),
		TenantID:  tenantID,
		Status:    "succeeded",
		Request:   spec,
		CreatedAt: now,
		UpdatedAt: now,
	}
	manifest, err := exportManifest(job, now)
	if err != nil {
		return ExportJob{}, err
	}
	job.Manifest = manifest

	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ExportID] = job
	s.order = append(s.order, job.ExportID)
	s.evictLocked()
	return job, nil
}

func (s *Store) Get(tenantID, exportID string) (ExportJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[exportID]
	if !ok || job.TenantID != tenantID {
		return ExportJob{}, false
	}
	return job, true
}

func (s *Store) evictLocked() {
	for len(s.order) > s.maxJobs {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.jobs, oldest)
	}
}

func newExportID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("export-%d", time.Now().UnixNano())
	}
	return "export_" + hex.EncodeToString(data[:])
}

func exportManifest(job ExportJob, generatedAt time.Time) (ExportManifest, error) {
	payload := struct {
		ExportID string     `json:"export_id"`
		TenantID string     `json:"tenant_id"`
		Request  ExportSpec `json:"request"`
	}{
		ExportID: job.ExportID,
		TenantID: job.TenantID,
		Request:  job.Request,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ExportManifest{}, fmt.Errorf("encode export manifest payload: %w", err)
	}
	sum := sha256.Sum256(data)
	return ExportManifest{
		SchemaVersion: "export-manifest-v1",
		ContentType:   contentType(job.Request.Format),
		ByteSize:      len(data),
		SHA256:        hex.EncodeToString(sum[:]),
		Inline:        true,
		GeneratedAt:   generatedAt,
	}, nil
}

func contentType(format string) string {
	switch format {
	case "csv":
		return "text/csv"
	case "parquet":
		return "application/vnd.apache.parquet"
	default:
		return "application/json"
	}
}
