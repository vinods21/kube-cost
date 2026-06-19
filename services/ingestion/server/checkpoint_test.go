package server

import (
	"context"
	"testing"
)

func TestFileCheckpointStoreRoundTripsSequence(t *testing.T) {
	t.Parallel()
	store, err := NewFileCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(context.Background(), "tenant/a", "cluster/a"); err != nil || got != 0 {
		t.Fatalf("initial load sequence=%d err=%v", got, err)
	}
	if err := store.Save(context.Background(), "tenant/a", "cluster/a", 42); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(context.Background(), "tenant/a", "cluster/a"); err != nil || got != 42 {
		t.Fatalf("load sequence=%d err=%v", got, err)
	}
}
