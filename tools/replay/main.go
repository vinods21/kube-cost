package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/proto"
)

type batchSummary struct {
	Path             string `json:"path"`
	BatchID          string `json:"batch_id"`
	FirstSequence    uint64 `json:"first_sequence"`
	LastSequence     uint64 `json:"last_sequence"`
	ObservationCount int    `json:"observation_count"`
}

func main() {
	root := flag.String("archive-dir", "", "raw archive root to scan")
	flag.Parse()
	if *root == "" {
		fmt.Fprintln(os.Stderr, "-archive-dir is required")
		os.Exit(2)
	}
	if err := emitSummaries(*root, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func emitSummaries(root string, output *os.File) error {
	encoder := json.NewEncoder(output)
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".pb" {
			return nil
		}
		summary, err := summarize(path)
		if err != nil {
			return err
		}
		return encoder.Encode(summary)
	})
}

func summarize(path string) (batchSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return batchSummary{}, fmt.Errorf("read %s: %w", path, err)
	}
	var batch agentv1.ObservationBatch
	if err := proto.Unmarshal(data, &batch); err != nil {
		return batchSummary{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return batchSummary{
		Path:             path,
		BatchID:          batch.GetBatchId(),
		FirstSequence:    batch.GetFirstSequence(),
		LastSequence:     batch.GetLastSequence(),
		ObservationCount: len(batch.GetObservations()),
	}, nil
}
