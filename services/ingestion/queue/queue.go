package queue

import (
	"context"
	"errors"
	"sync"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/proto"
)

var ErrFull = errors.New("ingestion queue is full")

type Batch struct {
	TenantID       string
	ClusterID      string
	AgentInstance  string
	SessionID      string
	ReceivedAt     time.Time
	ObservationSet *agentv1.ObservationBatch
}

type Queue struct {
	mu       sync.Mutex
	items    []*Batch
	capacity int
	ready    chan struct{}
}

func New(capacity int) *Queue {
	if capacity < 1 {
		capacity = 1
	}
	return &Queue{
		capacity: capacity,
		ready:    make(chan struct{}, 1),
	}
}

func (q *Queue) TryEnqueue(batch *Batch) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.capacity {
		return ErrFull
	}
	q.items = append(q.items, cloneBatch(batch))
	select {
	case q.ready <- struct{}{}:
	default:
	}
	return nil
}

func (q *Queue) Dequeue(ctx context.Context, max int) ([]*Batch, error) {
	if max < 1 {
		max = 1
	}
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			count := min(max, len(q.items))
			result := append([]*Batch(nil), q.items[:count]...)
			q.items = q.items[count:]
			if len(q.items) > 0 {
				select {
				case q.ready <- struct{}{}:
				default:
				}
			}
			q.mu.Unlock()
			return result, nil
		}
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-q.ready:
		}
	}
}

func (q *Queue) Depth() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (q *Queue) Capacity() int {
	return q.capacity
}

func cloneBatch(batch *Batch) *Batch {
	if batch == nil {
		return nil
	}
	cloned := *batch
	if batch.ObservationSet != nil {
		cloned.ObservationSet = proto.Clone(batch.ObservationSet).(*agentv1.ObservationBatch)
	}
	return &cloned
}
