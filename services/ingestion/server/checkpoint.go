package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CheckpointStore interface {
	Load(context.Context, string, string) (uint64, error)
	Save(context.Context, string, string, uint64) error
}

type MemoryCheckpointStore struct{}

func (MemoryCheckpointStore) Load(context.Context, string, string) (uint64, error) { return 0, nil }

func (MemoryCheckpointStore) Save(context.Context, string, string, uint64) error { return nil }

type FileCheckpointStore struct {
	root string
}

type checkpointFile struct {
	TenantID         string `json:"tenant_id"`
	ClusterID        string `json:"cluster_id"`
	PersistedThrough uint64 `json:"persisted_through_sequence"`
}

func NewFileCheckpointStore(root string) (*FileCheckpointStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("checkpoint root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create checkpoint root: %w", err)
	}
	return &FileCheckpointStore{root: root}, nil
}

func (s *FileCheckpointStore) Load(ctx context.Context, tenantID, clusterID string) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	data, err := os.ReadFile(s.path(tenantID, clusterID))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read checkpoint: %w", err)
	}
	var checkpoint checkpointFile
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return 0, fmt.Errorf("decode checkpoint: %w", err)
	}
	return checkpoint.PersistedThrough, nil
}

func (s *FileCheckpointStore) Save(ctx context.Context, tenantID, clusterID string, sequence uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := s.path(tenantID, clusterID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create checkpoint partition: %w", err)
	}
	data, err := json.Marshal(checkpointFile{
		TenantID:         tenantID,
		ClusterID:        clusterID,
		PersistedThrough: sequence,
	})
	if err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("replace checkpoint: %w", err)
	}
	return nil
}

func (s *FileCheckpointStore) path(tenantID, clusterID string) string {
	return filepath.Join(s.root, safePathElement(tenantID), safePathElement(clusterID), "checkpoint.json")
}
