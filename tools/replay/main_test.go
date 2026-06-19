package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/proto"
)

func TestSummarizeArchivedBatch(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "batch.pb")
	batch := &agentv1.ObservationBatch{
		BatchId:       "batch-1",
		FirstSequence: 10,
		LastSequence:  11,
		Observations:  []*agentv1.Observation{{Sequence: 10}, {Sequence: 11}},
	}
	data, err := proto.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := summarize(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.BatchID != "batch-1" || summary.FirstSequence != 10 || summary.LastSequence != 11 || summary.ObservationCount != 2 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestSummarizeRejectsInvalidArchiveFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "batch.pb")
	if err := os.WriteFile(path, []byte("not protobuf"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := summarize(path)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("err = %v", err)
	}
}
