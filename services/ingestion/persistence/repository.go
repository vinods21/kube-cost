package persistence

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"github.com/kube-cost/kube-cost/services/ingestion/queue"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var ErrInvalidInventory = errors.New("invalid inventory observation")

type Repository struct {
	store Store
}

type mappedRow struct {
	table   string
	columns []string
	values  []any
}

func NewRepository(store Store) *Repository {
	return &Repository{store: store}
}

func (r *Repository) Persist(ctx context.Context, batches []*queue.Batch) error {
	grouped := make(map[string]*Insert)
	order := make([]string, 0, 6)
	for _, batch := range batches {
		if batch == nil || batch.ObservationSet == nil {
			continue
		}
		for _, observation := range batch.ObservationSet.GetObservations() {
			row, ok, err := mapObservation(batch, observation)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			insert := grouped[row.table]
			if insert == nil {
				insert = &Insert{Table: row.table, Columns: row.columns}
				grouped[row.table] = insert
				order = append(order, row.table)
			}
			insert.Rows = append(insert.Rows, row.values)
		}
	}
	sort.Strings(order)
	for _, table := range order {
		if err := r.store.Insert(ctx, *grouped[table]); err != nil {
			return err
		}
	}
	return nil
}

func mapObservation(batch *queue.Batch, observation *agentv1.Observation) (mappedRow, bool, error) {
	if observation == nil {
		return mappedRow{}, false, fmt.Errorf("%w: observation is nil", ErrInvalidInventory)
	}
	observedAt := batch.ReceivedAt.UTC()
	if timestampValid(observation.GetObservedAt()) {
		observedAt = observation.GetObservedAt().AsTime().UTC()
	}
	base := rowBase{
		tenantID:   batch.TenantID,
		clusterID:  batch.ClusterID,
		observedAt: observedAt,
		eventID:    eventUUID(observation.GetEventId()),
		version:    observation.GetSequence(),
	}
	if base.tenantID == "" || base.clusterID == "" || observation.GetEventId() == "" || base.version == 0 {
		return mappedRow{}, false, fmt.Errorf("%w: tenant, cluster, event ID, and sequence are required", ErrInvalidInventory)
	}

	switch payload := observation.GetPayload().(type) {
	case *agentv1.Observation_ClusterInventory:
		return mapCluster(base, payload.ClusterInventory)
	case *agentv1.Observation_NodeInventory:
		return mapNode(base, payload.NodeInventory)
	case *agentv1.Observation_NamespaceInventory:
		return mapNamespace(base, payload.NamespaceInventory)
	case *agentv1.Observation_DeploymentInventory:
		return mapDeployment(base, payload.DeploymentInventory)
	case *agentv1.Observation_PodInventory:
		return mapPod(base, payload.PodInventory)
	case *agentv1.Observation_ContainerInventory:
		return mapContainer(base, payload.ContainerInventory)
	default:
		return mappedRow{}, false, nil
	}
}

type rowBase struct {
	tenantID   string
	clusterID  string
	observedAt time.Time
	eventID    uuid.UUID
	version    uint64
}

func eventUUID(eventID string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(eventID))
}

func mapCluster(base rowBase, inventory *agentv1.ClusterInventory) (mappedRow, bool, error) {
	operation, err := operationName(inventory.GetRecord())
	if err != nil {
		return mappedRow{}, false, err
	}
	return mappedRow{
		table: "cluster",
		columns: []string{
			"tenant_id", "cluster_id", "cluster_name", "provider", "account_id",
			"region", "kubernetes_version", "labels", "operation", "valid_from",
			"valid_to", "observed_at", "event_id", "version",
		},
		values: []any{
			base.tenantID, base.clusterID, inventory.GetName(), inventory.GetProvider(),
			inventory.GetAccountId(), inventory.GetRegion(), inventory.GetKubernetesVersion(),
			cloneMap(inventory.GetLabels()), operation, base.observedAt, nil, base.observedAt,
			base.eventID, base.version,
		},
	}, true, nil
}

func mapNode(base rowBase, inventory *agentv1.NodeInventory) (mappedRow, bool, error) {
	operation, err := operationName(inventory.GetRecord())
	if err != nil {
		return mappedRow{}, false, err
	}
	metadata := inventory.GetMetadata()
	if metadata.GetUid() == "" {
		return mappedRow{}, false, fmt.Errorf("%w: node UID is required", ErrInvalidInventory)
	}
	return mappedRow{
		table: "node",
		columns: []string{
			"tenant_id", "cluster_id", "node_uid", "node_name", "provider_id",
			"instance_type", "architecture", "operating_system", "region", "zone",
			"purchase_option", "capacity_cpu_millicores", "allocatable_cpu_millicores",
			"capacity_memory_bytes", "allocatable_memory_bytes", "capacity_gpu_milli",
			"labels", "operation", "valid_from", "valid_to", "observed_at", "event_id", "version",
		},
		values: []any{
			base.tenantID, base.clusterID, metadata.GetUid(), metadata.GetName(),
			inventory.GetProviderId(), inventory.GetInstanceType(), inventory.GetArchitecture(),
			inventory.GetOperatingSystem(), inventory.GetRegion(), inventory.GetZone(),
			inventory.GetCapacityType(), nonNegative(inventory.GetCapacity().GetCpuMillicores()),
			nonNegative(inventory.GetAllocatable().GetCpuMillicores()),
			nonNegative(inventory.GetCapacity().GetMemoryBytes()),
			nonNegative(inventory.GetAllocatable().GetMemoryBytes()),
			gpuMilli(inventory.GetCapacity().GetGpuCount()), cloneMap(metadata.GetLabels()),
			operation, base.observedAt, nil, base.observedAt, base.eventID, base.version,
		},
	}, true, nil
}

func mapNamespace(base rowBase, inventory *agentv1.NamespaceInventory) (mappedRow, bool, error) {
	operation, err := operationName(inventory.GetRecord())
	if err != nil {
		return mappedRow{}, false, err
	}
	metadata := inventory.GetMetadata()
	if metadata.GetUid() == "" {
		return mappedRow{}, false, fmt.Errorf("%w: namespace UID is required", ErrInvalidInventory)
	}
	dimensions := promotedDimensions(metadata.GetLabels())
	return mappedRow{
		table: "namespace",
		columns: []string{
			"tenant_id", "cluster_id", "namespace_uid", "namespace_name", "phase",
			"team", "project", "environment", "cost_center", "labels", "operation",
			"valid_from", "valid_to", "observed_at", "event_id", "version",
		},
		values: []any{
			base.tenantID, base.clusterID, metadata.GetUid(), metadata.GetName(),
			inventory.GetPhase(), dimensions.team, dimensions.project, dimensions.environment,
			dimensions.costCenter, cloneMap(metadata.GetLabels()), operation, base.observedAt,
			nil, base.observedAt, base.eventID, base.version,
		},
	}, true, nil
}

func mapDeployment(base rowBase, inventory *agentv1.DeploymentInventory) (mappedRow, bool, error) {
	operation, err := operationName(inventory.GetRecord())
	if err != nil {
		return mappedRow{}, false, err
	}
	metadata := inventory.GetMetadata()
	if metadata.GetUid() == "" {
		return mappedRow{}, false, fmt.Errorf("%w: deployment UID is required", ErrInvalidInventory)
	}
	dimensions := promotedDimensions(metadata.GetLabels())
	return mappedRow{
		table: "deployment",
		columns: []string{
			"tenant_id", "cluster_id", "namespace_uid", "deployment_uid",
			"namespace_name", "deployment_name", "desired_replicas", "available_replicas",
			"strategy", "team", "project", "environment", "cost_center", "labels",
			"operation", "valid_from", "valid_to", "observed_at", "event_id", "version",
		},
		values: []any{
			base.tenantID, base.clusterID, metadata.GetNamespace(), metadata.GetUid(),
			metadata.GetNamespace(), metadata.GetName(), nonNegative32(inventory.GetDesiredReplicas()),
			nonNegative32(inventory.GetAvailableReplicas()), inventory.GetStrategy(),
			dimensions.team, dimensions.project, dimensions.environment, dimensions.costCenter,
			cloneMap(metadata.GetLabels()), operation, base.observedAt, nil, base.observedAt,
			base.eventID, base.version,
		},
	}, true, nil
}

func mapPod(base rowBase, inventory *agentv1.PodInventory) (mappedRow, bool, error) {
	operation, err := operationName(inventory.GetRecord())
	if err != nil {
		return mappedRow{}, false, err
	}
	metadata := inventory.GetMetadata()
	if metadata.GetUid() == "" {
		return mappedRow{}, false, fmt.Errorf("%w: pod UID is required", ErrInvalidInventory)
	}
	return mappedRow{
		table: "pod",
		columns: []string{
			"tenant_id", "cluster_id", "namespace_uid", "deployment_uid", "pod_uid",
			"node_uid", "namespace_name", "deployment_name", "pod_name", "phase",
			"qos_class", "owner_kind", "owner_uid", "scheduled_at", "started_at",
			"finished_at", "labels", "operation", "valid_from", "valid_to",
			"observed_at", "event_id", "version",
		},
		values: []any{
			base.tenantID, base.clusterID, metadata.GetNamespace(), inventory.GetWorkloadUid(),
			metadata.GetUid(), inventory.GetNodeUid(), metadata.GetNamespace(), "",
			metadata.GetName(), inventory.GetPhase(), inventory.GetQosClass(),
			inventory.GetWorkloadKind(), inventory.GetWorkloadUid(), nullableTime(inventory.GetScheduledAt()),
			nullableTime(inventory.GetStartedAt()), nullableTime(metadata.GetDeletedAt()),
			cloneMap(metadata.GetLabels()), operation, base.observedAt, nil, base.observedAt,
			base.eventID, base.version,
		},
	}, true, nil
}

func mapContainer(base rowBase, inventory *agentv1.ContainerInventory) (mappedRow, bool, error) {
	operation, err := operationName(inventory.GetRecord())
	if err != nil {
		return mappedRow{}, false, err
	}
	if inventory.GetPodUid() == "" || inventory.GetContainerName() == "" {
		return mappedRow{}, false, fmt.Errorf("%w: container pod UID and name are required", ErrInvalidInventory)
	}
	return mappedRow{
		table: "container",
		columns: []string{
			"tenant_id", "cluster_id", "namespace_uid", "deployment_uid", "pod_uid",
			"container_name", "container_id", "image", "image_id", "restart_count",
			"cpu_request_millicores", "cpu_limit_millicores", "memory_request_bytes",
			"memory_limit_bytes", "gpu_request_milli", "operation", "valid_from",
			"valid_to", "observed_at", "event_id", "version",
		},
		values: []any{
			base.tenantID, base.clusterID, inventory.GetNamespace(), "", inventory.GetPodUid(),
			inventory.GetContainerName(), inventory.GetContainerId(), inventory.GetImage(),
			inventory.GetImageId(), nonNegative32(inventory.GetRestartCount()),
			nonNegative(inventory.GetRequests().GetCpuMillicores()),
			nonNegative(inventory.GetLimits().GetCpuMillicores()),
			nonNegative(inventory.GetRequests().GetMemoryBytes()),
			nonNegative(inventory.GetLimits().GetMemoryBytes()),
			gpuMilli(inventory.GetRequests().GetGpuCount()), operation, base.observedAt,
			nil, base.observedAt, base.eventID, base.version,
		},
	}, true, nil
}

func operationName(record *agentv1.InventoryRecord) (string, error) {
	switch record.GetOperation() {
	case agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT:
		return "upsert", nil
	case agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE:
		return "delete", nil
	default:
		return "", fmt.Errorf("%w: inventory operation is required", ErrInvalidInventory)
	}
}

type dimensions struct {
	team        string
	project     string
	environment string
	costCenter  string
}

func promotedDimensions(labels map[string]string) dimensions {
	return dimensions{
		team:        firstLabel(labels, "team", "app.kubernetes.io/team"),
		project:     firstLabel(labels, "project", "app.kubernetes.io/part-of"),
		environment: firstLabel(labels, "environment", "env"),
		costCenter:  firstLabel(labels, "cost-center", "cost_center"),
	}
}

func firstLabel(labels map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	return ""
}

func cloneMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func nullableTime(timestamp *timestamppb.Timestamp) any {
	if !timestampValid(timestamp) {
		return nil
	}
	return timestamp.AsTime().UTC()
}

func timestampValid(timestamp *timestamppb.Timestamp) bool {
	return timestamp != nil && timestamp.IsValid()
}

func nonNegative(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func nonNegative32(value int32) uint32 {
	if value <= 0 {
		return 0
	}
	return uint32(value)
}

func gpuMilli(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxUint64/1000 {
		return math.MaxUint64
	}
	return uint64(value) * 1000
}
