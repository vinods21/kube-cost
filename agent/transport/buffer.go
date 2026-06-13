package transport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kube-cost/kube-cost/agent/inventory"
	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/proto"
)

var ErrClosed = errors.New("inventory buffer is closed")

type Buffer struct {
	mu        sync.Mutex
	entries   []*agentv1.Observation
	next      uint64
	persisted uint64
	capacity  int
	notify    chan struct{}
	closed    bool
}

func NewBuffer(capacity int) *Buffer {
	return &Buffer{
		next:     1,
		capacity: capacity,
		notify:   make(chan struct{}, 1),
	}
}

func (b *Buffer) Publish(ctx context.Context, event inventory.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	for {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return ErrClosed
		}
		if len(b.entries) < b.capacity {
			observation := proto.Clone(event.Observation).(*agentv1.Observation)
			observation.Sequence = b.next
			observation.EventId = eventID(event.Key, observation)
			b.next++
			b.entries = append(b.entries, observation)
			b.signal()
			b.mu.Unlock()
			return nil
		}
		b.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.notify:
		}
	}
}

func (b *Buffer) Batch(ctx context.Context, maximum int) ([]*agentv1.Observation, error) {
	for {
		b.mu.Lock()
		if len(b.entries) > 0 {
			count := maximum
			if count > len(b.entries) {
				count = len(b.entries)
			}
			for count > 1 && b.entries[count-1].Sequence != b.entries[0].Sequence+uint64(count-1) {
				count--
			}
			batch := make([]*agentv1.Observation, count)
			for index := range count {
				batch[index] = proto.Clone(b.entries[index]).(*agentv1.Observation)
			}
			b.mu.Unlock()
			return batch, nil
		}
		if b.closed {
			b.mu.Unlock()
			return nil, ErrClosed
		}
		b.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.notify:
		}
	}
}

func (b *Buffer) Acknowledge(persistedThrough uint64, terminal []uint64) {
	terminalSet := make(map[uint64]struct{}, len(terminal))
	for _, sequence := range terminal {
		terminalSet[sequence] = struct{}{}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	kept := b.entries[:0]
	for _, observation := range b.entries {
		_, rejected := terminalSet[observation.Sequence]
		if observation.Sequence <= persistedThrough || rejected {
			continue
		}
		kept = append(kept, observation)
	}
	b.entries = kept
	if persistedThrough > b.persisted {
		b.persisted = persistedThrough
	}
	b.signal()
}

func (b *Buffer) HighestSequence() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.next - 1
}

func (b *Buffer) PersistedThrough() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.persisted
}

func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

func (b *Buffer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.signal()
}

func (b *Buffer) signal() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

func eventID(key string, observation *agentv1.Observation) string {
	clone := proto.Clone(observation).(*agentv1.Observation)
	clone.Sequence = 0
	clone.EventId = ""
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		data = []byte(fmt.Sprintf("%s:%d:%s", key, observation.Sequence, time.Now().UTC()))
	}
	sum := sha256.Sum256(append([]byte(key+"\x00"), data...))
	return hex.EncodeToString(sum[:])
}
