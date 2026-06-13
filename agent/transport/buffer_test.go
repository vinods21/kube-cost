package transport

import (
	"context"
	"testing"

	"github.com/kube-cost/kube-cost/agent/inventory"
	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
)

func TestBufferSequencesAndAcknowledges(t *testing.T) {
	t.Parallel()
	buffer := NewBuffer(10)
	for _, uid := range []string{"one", "two", "three"} {
		err := buffer.Publish(context.Background(), inventory.Event{
			Key: "namespace/" + uid,
			Observation: &agentv1.Observation{
				Payload: &agentv1.Observation_NamespaceInventory{
					NamespaceInventory: &agentv1.NamespaceInventory{
						Record:   &agentv1.InventoryRecord{Operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT},
						Metadata: &agentv1.ObjectMetadata{Uid: uid},
					},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	batch, err := buffer.Batch(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 3 || batch[0].Sequence != 1 || batch[2].Sequence != 3 {
		t.Fatalf("unexpected batch: %+v", batch)
	}
	if batch[0].EventId == "" || batch[0].EventId == batch[1].EventId {
		t.Fatal("event IDs must be non-empty and distinct")
	}

	buffer.Acknowledge(1, []uint64{2})
	if buffer.PersistedThrough() != 1 {
		t.Fatalf("persisted sequence=%d", buffer.PersistedThrough())
	}
	batch, err = buffer.Batch(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || batch[0].Sequence != 3 {
		t.Fatalf("unexpected retained batch: %+v", batch)
	}
}
