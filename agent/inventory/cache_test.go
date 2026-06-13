package inventory

import (
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCacheSuppressesSemanticDuplicates(t *testing.T) {
	t.Parallel()
	cache := NewCache()
	event := Event{
		Key: "namespace/one",
		Observation: &agentv1.Observation{
			ObservedAt:            timestamppb.Now(),
			CollectedAt:           timestamppb.Now(),
			SourceResourceVersion: "1",
			Payload: &agentv1.Observation_NamespaceInventory{
				NamespaceInventory: &agentv1.NamespaceInventory{
					Record:   record(agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT),
					Metadata: &agentv1.ObjectMetadata{Uid: "one", Name: "one"},
				},
			},
		},
	}

	changed, fingerprint, err := cache.Changed(event)
	if err != nil || !changed {
		t.Fatalf("first event changed=%v err=%v", changed, err)
	}
	if err := cache.Commit(event.Key, fingerprint); err != nil {
		t.Fatal(err)
	}

	event.Observation.ObservedAt = timestamppb.New(time.Now().Add(time.Minute))
	event.Observation.CollectedAt = timestamppb.Now()
	event.Observation.SourceResourceVersion = "2"
	changed, _, err = cache.Changed(event)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("timestamp and resource-version-only change must be suppressed")
	}

	event.Observation.GetNamespaceInventory().Metadata.Name = "renamed"
	changed, _, err = cache.Changed(event)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("semantic inventory change was suppressed")
	}
}
