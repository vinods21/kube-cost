package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestQueryManifestIncludesDeterministicInlineMetadata(t *testing.T) {
	t.Parallel()
	generatedAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	result := UsageResult{
		TenantID:    "tenant-a",
		Start:       time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
		GroupBy:     "namespace",
		GeneratedAt: generatedAt,
		Rows:        []UsageRow{{TenantID: "tenant-a", ClusterID: "cluster-a", GroupKey: "namespace", GroupValue: "apps"}},
		ResultCount: 1,
		Limit:       100,
	}

	manifest, err := queryManifest(usageCursorKind, 1, result, generatedAt)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if manifest.ResultType != usageCursorKind ||
		manifest.SchemaVersion != "query-result-v1" ||
		manifest.ContentType != "application/json" ||
		manifest.RowCount != 1 ||
		manifest.ByteSize != len(data) ||
		manifest.SHA256 != hex.EncodeToString(sum[:]) ||
		!manifest.GeneratedAt.Equal(generatedAt) ||
		!manifest.Inline {
		t.Fatalf("manifest = %#v", manifest)
	}
}
