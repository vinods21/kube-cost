package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/proto"
)

func TestFileArchiverWritesDeterministicBatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archiver, err := NewFileArchiver(root)
	if err != nil {
		t.Fatal(err)
	}
	batch := &agentv1.ObservationBatch{
		BatchId:       "batch/one",
		FirstSequence: 1,
		LastSequence:  1,
		Observations: []*agentv1.Observation{{
			Sequence: 1,
			EventId:  "event-1",
			Payload: &agentv1.Observation_ClusterInventory{ClusterInventory: &agentv1.ClusterInventory{
				Record: &agentv1.InventoryRecord{Operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT},
			}},
		}},
	}
	record := ArchiveRecord{
		TenantID:   "tenant/a",
		ClusterID:  "cluster/a",
		ReceivedAt: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
		Batch:      batch,
	}

	if err := archiver.Archive(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "tenant_a", "cluster_a", "2026", "06", "19", "00000000000000000001-00000000000000000001-batch_one.pb")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded agentv1.ObservationBatch
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetBatchId() != "batch/one" || decoded.GetFirstSequence() != 1 {
		t.Fatalf("decoded batch = %#v", &decoded)
	}
}

func TestSafePathElement(t *testing.T) {
	t.Parallel()
	if got := safePathElement("a/b\\c:d"); got != "a_b_c_d" {
		t.Fatalf("safePathElement=%q", got)
	}
}
