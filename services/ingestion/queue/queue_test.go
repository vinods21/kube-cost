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
