package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/proto"
)

type ArchiveRecord struct {
	TenantID      string
	ClusterID     string
	AgentInstance string
	SessionID     string
	ReceivedAt    time.Time
	Batch         *agentv1.ObservationBatch
}

type Archiver interface {
	Archive(context.Context, ArchiveRecord) error
}

type NoopArchiver struct{}

func (NoopArchiver) Archive(context.Context, ArchiveRecord) error { return nil }

type FileArchiver struct {
	root string
}

func NewFileArchiver(root string) (*FileArchiver, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("archive root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create archive root: %w", err)
	}
	return &FileArchiver{root: root}, nil
}

func (a *FileArchiver) Archive(ctx context.Context, record ArchiveRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.Batch == nil {
		return fmt.Errorf("archive batch is required")
	}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(record.Batch)
	if err != nil {
		return fmt.Errorf("marshal archive batch: %w", err)
	}
	path := a.path(record)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create archive partition: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write archive batch: %w", err)
	}
	return nil
}

func (a *FileArchiver) path(record ArchiveRecord) string {
	received := record.ReceivedAt.UTC()
	fileName := fmt.Sprintf(
		"%020d-%020d-%s.pb",
		record.Batch.GetFirstSequence(),
		record.Batch.GetLastSequence(),
		safePathElement(record.Batch.GetBatchId()),
	)
	return filepath.Join(
		a.root,
		safePathElement(record.TenantID),
		safePathElement(record.ClusterID),
		received.Format("2006"),
		received.Format("01"),
		received.Format("02"),
		fileName,
	)
}

func safePathElement(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "\x00", "_")
	return replacer.Replace(value)
}
