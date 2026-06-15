package persistence

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"github.com/kube-cost/kube-cost/services/ingestion/queue"
)

func TestClickHouseInventoryLifecycle(t *testing.T) {
	if os.Getenv("CLICKHOUSE_INTEGRATION") != "1" {
		t.Skip("set CLICKHOUSE_INTEGRATION=1 to run ClickHouse integration tests")
	}
	address := os.Getenv("CLICKHOUSE_ADDRESS")
	if address == "" {
		address = "localhost:9000"
	}
	store, err := OpenClickHouse(ClickHouseConfig{
		Address:  address,
		Database: "kube_cost",
		Username: envOr("CLICKHOUSE_USERNAME", "kube_cost"),
		Password: envOr("CLICKHOUSE_PASSWORD", "kube_cost"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	tenantID := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = store.connection.Exec(context.Background(),
			"ALTER TABLE kube_cost.namespace DELETE WHERE tenant_id = ? SETTINGS mutations_sync = 2",
			tenantID,
		)
	})
	repository := NewRepository(store)
	now := time.Now().UTC().Truncate(time.Millisecond)

	upsert := inventoryBatch(now,
		namespaceObservation(1, "integration-upsert", "before", agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT, now),
	)
	upsert.TenantID = tenantID
	if err := repository.Persist(ctx, []*queue.Batch{upsert}); err != nil {
		t.Fatal(err)
	}
	assertCurrentNamespace(t, ctx, store, tenantID, "before", 1)

	update := inventoryBatch(now,
		namespaceObservation(2, "integration-update", "after", agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT, now.Add(time.Second)),
	)
	update.TenantID = tenantID
	if err := repository.Persist(ctx, []*queue.Batch{update}); err != nil {
		t.Fatal(err)
	}
	assertCurrentNamespace(t, ctx, store, tenantID, "after", 1)

	if err := repository.Persist(ctx, []*queue.Batch{update}); err != nil {
		t.Fatal(err)
	}
	var updateRows uint64
	if err := store.connection.QueryRow(ctx,
		"SELECT count() FROM kube_cost.namespace FINAL WHERE tenant_id = ? AND event_id = ?",
		tenantID, eventUUID("integration-update"),
	).Scan(&updateRows); err != nil {
		t.Fatal(err)
	}
	if updateRows != 1 {
		t.Fatalf("idempotent update rows = %d, want 1", updateRows)
	}

	deleteBatch := inventoryBatch(now,
		namespaceObservation(3, "integration-delete", "after", agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE, now.Add(2*time.Second)),
	)
	deleteBatch.TenantID = tenantID
	if err := repository.Persist(ctx, []*queue.Batch{deleteBatch}); err != nil {
		t.Fatal(err)
	}
	assertCurrentNamespace(t, ctx, store, tenantID, "", 0)
}

func TestClickHousePersistsAllInventoryEntities(t *testing.T) {
	if os.Getenv("CLICKHOUSE_INTEGRATION") != "1" {
		t.Skip("set CLICKHOUSE_INTEGRATION=1 to run ClickHouse integration tests")
	}
	store, err := OpenClickHouse(ClickHouseConfig{
		Address:  envOr("CLICKHOUSE_ADDRESS", "localhost:9000"),
		Database: "kube_cost",
		Username: envOr("CLICKHOUSE_USERNAME", "kube_cost"),
		Password: envOr("CLICKHOUSE_PASSWORD", "kube_cost"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tenantID := fmt.Sprintf("integration-all-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		for _, table := range []string{"cluster", "node", "namespace", "deployment", "pod", "container"} {
			_ = store.connection.Exec(context.Background(),
				fmt.Sprintf("ALTER TABLE kube_cost.%s DELETE WHERE tenant_id = ? SETTINGS mutations_sync = 2", table),
				tenantID,
			)
		}
	})
	now := time.Now().UTC().Truncate(time.Millisecond)
	record := &agentv1.InventoryRecord{Operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT}
	metadata := &agentv1.ObjectMetadata{Uid: "uid-1", Name: "name", Namespace: "namespace"}
	batch := inventoryBatch(now,
		observation(1, "all-cluster", now, &agentv1.Observation_ClusterInventory{ClusterInventory: &agentv1.ClusterInventory{Record: record, Name: "cluster"}}),
		observation(2, "all-node", now, &agentv1.Observation_NodeInventory{NodeInventory: &agentv1.NodeInventory{Record: record, Metadata: metadata}}),
		observation(3, "all-namespace", now, &agentv1.Observation_NamespaceInventory{NamespaceInventory: &agentv1.NamespaceInventory{Record: record, Metadata: metadata}}),
		observation(4, "all-deployment", now, &agentv1.Observation_DeploymentInventory{DeploymentInventory: &agentv1.DeploymentInventory{Record: record, Metadata: metadata}}),
		observation(5, "all-pod", now, &agentv1.Observation_PodInventory{PodInventory: &agentv1.PodInventory{Record: record, Metadata: metadata}}),
		observation(6, "all-container", now, &agentv1.Observation_ContainerInventory{ContainerInventory: &agentv1.ContainerInventory{Record: record, PodUid: "pod-1", ContainerName: "main"}}),
	)
	batch.TenantID = tenantID
	if err := NewRepository(store).Persist(ctx, []*queue.Batch{batch}); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"current_cluster", "current_node", "current_namespace", "current_deployment", "current_pod", "current_container"} {
		var count uint64
		if err := store.connection.QueryRow(ctx,
			fmt.Sprintf("SELECT count() FROM kube_cost.%s WHERE tenant_id = ?", view),
			tenantID,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s count = %d, want 1", view, count)
		}
	}
}

func assertCurrentNamespace(t *testing.T, ctx context.Context, store *ClickHouseStore, tenantID, expectedName string, expectedCount uint64) {
	t.Helper()
	var count uint64
	var name string
	row := store.connection.QueryRow(ctx,
		"SELECT count(), any(namespace_name) FROM kube_cost.current_namespace WHERE tenant_id = ?",
		tenantID,
	)
	if err := row.Scan(&count, &name); err != nil {
		t.Fatal(err)
	}
	if count != expectedCount || (expectedCount > 0 && name != expectedName) {
		t.Fatalf("current namespace count=%d name=%q, want %d %q", count, name, expectedCount, expectedName)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
