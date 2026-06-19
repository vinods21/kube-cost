package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
)

func TestQueueCapacityAndDequeue(t *testing.T) {
	t.Parallel()
	q := New(1)
	original := &Batch{
		TenantID:   "tenant-a",
		ClusterID:  "cluster-a",
		ReceivedAt: time.Now(),
		ObservationSet: &agentv1.ObservationBatch{
			BatchId: "batch-1",
		},
	}
	if err := q.TryEnqueue(original); err != nil {
		t.Fatalf("enqueue first batch: %v", err)
	}
	original.ObservationSet.BatchId = "mutated"
	if err := q.TryEnqueue(&Batch{}); !errors.Is(err, ErrFull) {
		t.Fatalf("enqueue full queue error = %v, want ErrFull", err)
	}

	items, err := q.Dequeue(context.Background(), 10)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if len(items) != 1 || items[0].ObservationSet.GetBatchId() != "batch-1" {
		t.Fatalf("dequeued batch = %#v, want immutable batch-1", items)
	}
	if q.Depth() != 0 {
		t.Fatalf("queue depth = %d, want 0", q.Depth())
	}
}

func TestDequeueHonorsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := New(1).Dequeue(ctx, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("dequeue error = %v, want deadline exceeded", err)
	}
}

func TestLeaseRetryPreservesCapacityAndOrder(t *testing.T) {
	t.Parallel()
	q := New(2)
	if err := q.TryEnqueue(&Batch{TenantID: "first"}); err != nil {
		t.Fatal(err)
	}
	lease, err := q.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if q.InFlight() != 1 {
		t.Fatalf("in-flight = %d, want 1", q.InFlight())
	}
	if err := q.TryEnqueue(&Batch{TenantID: "second"}); err != nil {
		t.Fatal(err)
	}
	if err := q.TryEnqueue(&Batch{TenantID: "third"}); !errors.Is(err, ErrFull) {
		t.Fatalf("enqueue while leased error = %v, want ErrFull", err)
	}

	lease.Retry()
	if q.InFlight() != 0 || q.Depth() != 2 {
		t.Fatalf("after retry depth=%d in-flight=%d, want 2 and 0", q.Depth(), q.InFlight())
	}
	items, err := q.Dequeue(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].TenantID != "first" || items[1].TenantID != "second" {
		t.Fatalf("retry order = %q, %q", items[0].TenantID, items[1].TenantID)
	}
}

func TestLeaseCommitReleasesCapacity(t *testing.T) {
	t.Parallel()
	q := New(1)
	if err := q.TryEnqueue(&Batch{TenantID: "first"}); err != nil {
		t.Fatal(err)
	}
	lease, err := q.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	lease.Commit()
	if err := q.TryEnqueue(&Batch{TenantID: "second"}); err != nil {
		t.Fatalf("enqueue after commit: %v", err)
	}
}

func TestLeaseCommitSignalsPersistenceNotification(t *testing.T) {
	t.Parallel()
	q := New(1)
	batch := &Batch{TenantID: "first"}
	batch.EnablePersistenceNotification()
	if err := q.TryEnqueue(batch); err != nil {
		t.Fatal(err)
	}
	lease, err := q.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-batch.Persisted():
		t.Fatal("batch was marked persisted before lease commit")
	default:
	}
	lease.Commit()
	select {
	case <-batch.Persisted():
	case <-time.After(time.Second):
		t.Fatal("batch was not marked persisted after lease commit")
	}
}
