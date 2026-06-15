package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"github.com/kube-cost/kube-cost/services/ingestion/queue"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRepositoryMapsInventoryUpsertsUpdatesAndDeletes(t *testing.T) {
	t.Parallel()
	store := &recordingStore{}
	repository := NewRepository(store)
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	batch := inventoryBatch(now,
		namespaceObservation(1, "event-1", "old-name", agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT, now),
		namespaceObservation(2, "event-2", "new-name", agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT, now.Add(time.Minute)),
		namespaceObservation(3, "event-3", "new-name", agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE, now.Add(2*time.Minute)),
	)

	if err := repository.Persist(context.Background(), []*queue.Batch{batch}); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if len(store.inserts) != 1 {
		t.Fatalf("insert count = %d, want 1", len(store.inserts))
	}
	insert := store.inserts[0]
	if insert.Table != "namespace" || len(insert.Rows) != 3 {
		t.Fatalf("insert = %#v", insert)
	}
	operationIndex := columnIndex(t, insert.Columns, "operation")
	nameIndex := columnIndex(t, insert.Columns, "namespace_name")
	if insert.Rows[0][operationIndex] != "upsert" ||
		insert.Rows[1][nameIndex] != "new-name" ||
		insert.Rows[2][operationIndex] != "delete" {
		t.Fatalf("unexpected persisted operations: %#v", insert.Rows)
	}
}

func TestRepositoryGroupsAllInventoryTablesAndSkipsNonInventory(t *testing.T) {
	t.Parallel()
	store := &recordingStore{}
	repository := NewRepository(store)
	now := time.Now().UTC()
	record := &agentv1.InventoryRecord{Operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT}
	metadata := &agentv1.ObjectMetadata{Uid: "uid-1", Name: "name", Namespace: "ns"}
	observations := []*agentv1.Observation{
		observation(1, "cluster", now, &agentv1.Observation_ClusterInventory{ClusterInventory: &agentv1.ClusterInventory{Record: record}}),
		observation(2, "node", now, &agentv1.Observation_NodeInventory{NodeInventory: &agentv1.NodeInventory{Record: record, Metadata: metadata}}),
		observation(3, "namespace", now, &agentv1.Observation_NamespaceInventory{NamespaceInventory: &agentv1.NamespaceInventory{Record: record, Metadata: metadata}}),
		observation(4, "deployment", now, &agentv1.Observation_DeploymentInventory{DeploymentInventory: &agentv1.DeploymentInventory{Record: record, Metadata: metadata}}),
		observation(5, "pod", now, &agentv1.Observation_PodInventory{PodInventory: &agentv1.PodInventory{Record: record, Metadata: metadata}}),
		observation(6, "container", now, &agentv1.Observation_ContainerInventory{ContainerInventory: &agentv1.ContainerInventory{Record: record, PodUid: "pod-1", ContainerName: "main"}}),
		observation(7, "marker", now, &agentv1.Observation_InventorySnapshotMarker{InventorySnapshotMarker: &agentv1.InventorySnapshotMarker{SnapshotId: "snapshot"}}),
	}

	if err := repository.Persist(context.Background(), []*queue.Batch{inventoryBatch(now, observations...)}); err != nil {
		t.Fatal(err)
	}
	if len(store.inserts) != 6 {
		t.Fatalf("insert groups = %d, want 6", len(store.inserts))
	}
}

func TestRepositoryRejectsUnspecifiedOperation(t *testing.T) {
	t.Parallel()
	repository := NewRepository(&recordingStore{})
	now := time.Now().UTC()
	item := namespaceObservation(1, "event", "namespace", agentv1.InventoryOperation_INVENTORY_OPERATION_UNSPECIFIED, now)
	err := repository.Persist(context.Background(), []*queue.Batch{inventoryBatch(now, item)})
	if !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("persist error = %v, want ErrInvalidInventory", err)
	}
}

func TestWorkerRetriesFailedLease(t *testing.T) {
	t.Parallel()
	store := &recordingStore{failures: 1}
	batches := queue.New(1)
	now := time.Now().UTC()
	if err := batches.TryEnqueue(inventoryBatch(now,
		namespaceObservation(1, "event", "namespace", agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT, now),
	)); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(WorkerConfig{
		BatchSize:    1,
		RetryInitial: time.Millisecond,
		RetryMaximum: time.Millisecond,
	}, batches, NewRepository(store))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()

	deadline := time.Now().Add(500 * time.Millisecond)
	for store.callCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if store.callCount() < 2 {
		t.Fatalf("store calls = %d, want retry", store.callCount())
	}
	for (batches.Depth() != 0 || batches.InFlight() != 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("worker stopped: %v", err)
	}
	if batches.Depth() != 0 || batches.InFlight() != 0 {
		t.Fatalf("queue depth=%d in-flight=%d after success", batches.Depth(), batches.InFlight())
	}
}

type recordingStore struct {
	mu       sync.Mutex
	inserts  []Insert
	failures int
	calls    int
}

func (s *recordingStore) Insert(_ context.Context, insert Insert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.failures > 0 {
		s.failures--
		return errors.New("temporary ClickHouse error")
	}
	s.inserts = append(s.inserts, insert)
	return nil
}

func (s *recordingStore) Ping(context.Context) error { return nil }
func (s *recordingStore) Close() error               { return nil }

func (s *recordingStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func inventoryBatch(receivedAt time.Time, observations ...*agentv1.Observation) *queue.Batch {
	return &queue.Batch{
		TenantID:   "tenant-a",
		ClusterID:  "cluster-a",
		ReceivedAt: receivedAt,
		ObservationSet: &agentv1.ObservationBatch{
			BatchId:      "batch",
			Observations: observations,
		},
	}
}

func namespaceObservation(sequence uint64, eventID, name string, operation agentv1.InventoryOperation, at time.Time) *agentv1.Observation {
	return observation(sequence, eventID, at, &agentv1.Observation_NamespaceInventory{
		NamespaceInventory: &agentv1.NamespaceInventory{
			Record: &agentv1.InventoryRecord{Operation: operation},
			Metadata: &agentv1.ObjectMetadata{
				Uid:       "namespace-uid",
				Name:      name,
				Labels:    map[string]string{"team": "platform"},
				CreatedAt: timestamppb.New(at.Add(-time.Hour)),
			},
			Phase: "Active",
		},
	})
}

func observation(sequence uint64, eventID string, at time.Time, payload any) *agentv1.Observation {
	result := &agentv1.Observation{
		Sequence:   sequence,
		EventId:    eventID,
		ObservedAt: timestamppb.New(at),
	}
	switch value := payload.(type) {
	case *agentv1.Observation_ClusterInventory:
		result.Payload = value
	case *agentv1.Observation_NodeInventory:
		result.Payload = value
	case *agentv1.Observation_NamespaceInventory:
		result.Payload = value
	case *agentv1.Observation_DeploymentInventory:
		result.Payload = value
	case *agentv1.Observation_PodInventory:
		result.Payload = value
	case *agentv1.Observation_ContainerInventory:
		result.Payload = value
	case *agentv1.Observation_InventorySnapshotMarker:
		result.Payload = value
	default:
		panic("unsupported test payload")
	}
	return result
}

func columnIndex(t *testing.T, columns []string, name string) int {
	t.Helper()
	for index, column := range columns {
		if column == name {
			return index
		}
	}
	t.Fatalf("column %q not found", name)
	return -1
}
