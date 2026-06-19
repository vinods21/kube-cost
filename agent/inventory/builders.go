package inventory

import (
	"fmt"
	"sort"
	"strings"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/types/known/timestamppb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Builder struct {
	now func() time.Time
}

func NewBuilder() *Builder {
	return &Builder{now: time.Now}
}

func (b *Builder) Cluster(clusterID, kubernetesVersion string, namespace *corev1.Namespace) Event {
	inventory := &agentv1.ClusterInventory{
		Record:            record(agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT),
		ClusterUid:        string(namespace.UID),
		Name:              clusterID,
		KubernetesVersion: kubernetesVersion,
	}
	return Event{
		Key:         "cluster/" + clusterID,
		Observation: withPayload(b.observation(namespace.ResourceVersion, namespace.CreationTimestamp.Time), &agentv1.Observation_ClusterInventory{ClusterInventory: inventory}),
	}
}

func (b *Builder) Snapshot(snapshotID string, phase agentv1.InventorySnapshotPhase) Event {
	now := b.now()
	marker := &agentv1.InventorySnapshotMarker{
		SnapshotId: snapshotID,
		Phase:      phase,
		ResourceKinds: []string{
			"Cluster",
			"Node",
			"Namespace",
			"Deployment",
			"Pod",
			"Container",
		},
	}
	if phase == agentv1.InventorySnapshotPhase_INVENTORY_SNAPSHOT_PHASE_STARTED {
		marker.StartedAt = timestamppb.New(now)
	}
	if phase == agentv1.InventorySnapshotPhase_INVENTORY_SNAPSHOT_PHASE_COMPLETED {
		marker.CompletedAt = timestamppb.New(now)
	}
	return Event{
		Key:         fmt.Sprintf("snapshot/%s/%s", snapshotID, phase),
		Observation: withPayload(b.observation("", now), &agentv1.Observation_InventorySnapshotMarker{InventorySnapshotMarker: marker}),
	}
}

func (b *Builder) Node(node *corev1.Node, operation agentv1.InventoryOperation) Event {
	inventory := &agentv1.NodeInventory{
		Record:                record(operation),
		Metadata:              metadata(node.ObjectMeta),
		ProviderId:            node.Spec.ProviderID,
		InstanceType:          firstNonEmpty(node.Labels["node.kubernetes.io/instance-type"], node.Labels["beta.kubernetes.io/instance-type"]),
		Architecture:          node.Status.NodeInfo.Architecture,
		OperatingSystem:       node.Status.NodeInfo.OperatingSystem,
		Region:                firstNonEmpty(node.Labels["topology.kubernetes.io/region"], node.Labels["failure-domain.beta.kubernetes.io/region"]),
		Zone:                  firstNonEmpty(node.Labels["topology.kubernetes.io/zone"], node.Labels["failure-domain.beta.kubernetes.io/zone"]),
		CapacityType:          firstNonEmpty(node.Labels["karpenter.sh/capacity-type"], node.Labels["eks.amazonaws.com/capacityType"]),
		Capacity:              resourceValues(node.Status.Capacity),
		Allocatable:           resourceValues(node.Status.Allocatable),
		Schedulable:           !node.Spec.Unschedulable,
		Taints:                taints(node.Spec.Taints),
		KarpenterNodeClaimUid: node.Labels["karpenter.sh/nodeclaim"],
		KarpenterNodePool:     node.Labels["karpenter.sh/nodepool"],
	}
	return Event{
		Key:         "node/" + string(node.UID),
		Observation: withPayload(b.observation(node.ResourceVersion, b.now()), &agentv1.Observation_NodeInventory{NodeInventory: inventory}),
	}
}

func (b *Builder) Namespace(namespace *corev1.Namespace, operation agentv1.InventoryOperation) Event {
	inventory := &agentv1.NamespaceInventory{
		Record:   record(operation),
		Metadata: metadata(namespace.ObjectMeta),
		Phase:    string(namespace.Status.Phase),
	}
	return Event{
		Key:         "namespace/" + string(namespace.UID),
		Observation: withPayload(b.observation(namespace.ResourceVersion, b.now()), &agentv1.Observation_NamespaceInventory{NamespaceInventory: inventory}),
	}
}

func (b *Builder) Deployment(deployment *appsv1.Deployment, namespaceUID string, operation agentv1.InventoryOperation) Event {
	inventory := &agentv1.DeploymentInventory{
		Record:            record(operation),
		Metadata:          metadata(deployment.ObjectMeta),
		DesiredReplicas:   valueOrZero(deployment.Spec.Replicas),
		AvailableReplicas: deployment.Status.AvailableReplicas,
		Selector:          metav1.FormatLabelSelector(deployment.Spec.Selector),
		Strategy:          string(deployment.Spec.Strategy.Type),
		NamespaceUid:      namespaceUID,
	}
	return Event{
		Key:         "deployment/" + string(deployment.UID),
		Observation: withPayload(b.observation(deployment.ResourceVersion, b.now()), &agentv1.Observation_DeploymentInventory{DeploymentInventory: inventory}),
	}
}

func (b *Builder) Pod(pod *corev1.Pod, namespaceUID string, operation agentv1.InventoryOperation) Event {
	workloadKind, workloadUID := workloadOwner(pod.OwnerReferences)
	inventory := &agentv1.PodInventory{
		Record:             record(operation),
		Metadata:           metadata(pod.ObjectMeta),
		NamespaceUid:       namespaceUID,
		NodeName:           pod.Spec.NodeName,
		Phase:              string(pod.Status.Phase),
		QosClass:           string(pod.Status.QOSClass),
		ServiceAccountName: pod.Spec.ServiceAccountName,
		PriorityClassName:  pod.Spec.PriorityClassName,
		WorkloadKind:       workloadKind,
		WorkloadUid:        workloadUID,
		ScheduledAt:        conditionTime(pod.Status.Conditions, corev1.PodScheduled),
		StartedAt:          timestamp(pod.Status.StartTime),
	}
	return Event{
		Key:         "pod/" + string(pod.UID),
		Observation: withPayload(b.observation(pod.ResourceVersion, b.now()), &agentv1.Observation_PodInventory{PodInventory: inventory}),
	}
}

func (b *Builder) Containers(pod *corev1.Pod, namespaceUID string, operation agentv1.InventoryOperation) []Event {
	statuses := make(map[string]corev1.ContainerStatus, len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	for _, status := range append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...) {
		statuses[status.Name] = status
	}

	workloadKind, workloadUID := workloadOwner(pod.OwnerReferences)
	events := make([]Event, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	for _, container := range pod.Spec.InitContainers {
		events = append(events, b.container(pod, container, statuses[container.Name], namespaceUID, workloadKind, workloadUID, true, operation))
	}
	for _, container := range pod.Spec.Containers {
		events = append(events, b.container(pod, container, statuses[container.Name], namespaceUID, workloadKind, workloadUID, false, operation))
	}
	return events
}

func (b *Builder) container(
	pod *corev1.Pod,
	container corev1.Container,
	status corev1.ContainerStatus,
	namespaceUID string,
	workloadKind string,
	workloadUID string,
	init bool,
	operation agentv1.InventoryOperation,
) Event {
	startedAt, finishedAt := containerTimes(status)
	inventory := &agentv1.ContainerInventory{
		Record:        record(operation),
		PodUid:        string(pod.UID),
		Namespace:     pod.Namespace,
		NamespaceUid:  namespaceUID,
		PodName:       pod.Name,
		ContainerName: container.Name,
		ContainerId:   status.ContainerID,
		Image:         container.Image,
		ImageId:       status.ImageID,
		Requests:      resourceValues(container.Resources.Requests),
		Limits:        resourceValues(container.Resources.Limits),
		RestartCount:  status.RestartCount,
		InitContainer: init,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		WorkloadKind:  workloadKind,
		WorkloadUid:   workloadUID,
	}
	kind := "container"
	if init {
		kind = "init-container"
	}
	return Event{
		Key:         fmt.Sprintf("%s/%s/%s", kind, pod.UID, container.Name),
		Observation: withPayload(b.observation(pod.ResourceVersion, b.now()), &agentv1.Observation_ContainerInventory{ContainerInventory: inventory}),
	}
}

func (b *Builder) observation(resourceVersion string, observedAt time.Time) *agentv1.Observation {
	now := b.now()
	if observedAt.IsZero() {
		observedAt = now
	}
	return &agentv1.Observation{
		ObservedAt:            timestamppb.New(observedAt),
		CollectedAt:           timestamppb.New(now),
		SourceResourceVersion: resourceVersion,
	}
}

func withPayload(observation *agentv1.Observation, payload any) *agentv1.Observation {
	switch value := payload.(type) {
	case *agentv1.Observation_ClusterInventory:
		observation.Payload = value
	case *agentv1.Observation_NodeInventory:
		observation.Payload = value
	case *agentv1.Observation_NamespaceInventory:
		observation.Payload = value
	case *agentv1.Observation_DeploymentInventory:
		observation.Payload = value
	case *agentv1.Observation_PodInventory:
		observation.Payload = value
	case *agentv1.Observation_ContainerInventory:
		observation.Payload = value
	case *agentv1.Observation_InventorySnapshotMarker:
		observation.Payload = value
	default:
		panic(fmt.Sprintf("unsupported inventory payload %T", payload))
	}
	return observation
}

func record(operation agentv1.InventoryOperation) *agentv1.InventoryRecord {
	return &agentv1.InventoryRecord{Operation: operation}
}

func metadata(meta metav1.ObjectMeta) *agentv1.ObjectMetadata {
	owners := make([]*agentv1.ObjectReference, 0, len(meta.OwnerReferences))
	for _, owner := range meta.OwnerReferences {
		owners = append(owners, &agentv1.ObjectReference{
			ApiVersion: owner.APIVersion,
			Kind:       owner.Kind,
			Name:       owner.Name,
			Uid:        string(owner.UID),
		})
	}
	return &agentv1.ObjectMetadata{
		Uid:             string(meta.UID),
		Name:            meta.Name,
		Namespace:       meta.Namespace,
		Labels:          cloneMap(meta.Labels),
		CreatedAt:       timestamppb.New(meta.CreationTimestamp.Time),
		DeletedAt:       timestamp(meta.DeletionTimestamp),
		OwnerReferences: owners,
	}
}

func resourceValues(resources corev1.ResourceList) *agentv1.ResourceValues {
	values := &agentv1.ResourceValues{ExtendedResources: make(map[string]string)}
	if quantity, exists := resources[corev1.ResourceCPU]; exists {
		value := quantity.MilliValue()
		values.CpuMillicores = &value
	}
	if quantity, exists := resources[corev1.ResourceMemory]; exists {
		value := quantity.Value()
		values.MemoryBytes = &value
	}
	if quantity, exists := resources[corev1.ResourceEphemeralStorage]; exists {
		value := quantity.Value()
		values.EphemeralStorageBytes = &value
	}
	var gpu int64
	var hasGPU bool
	for name, quantity := range resources {
		switch name {
		case corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceEphemeralStorage, corev1.ResourceStorage:
			continue
		}
		if strings.Contains(strings.ToLower(string(name)), "gpu") {
			gpu += quantity.Value()
			hasGPU = true
		}
		values.ExtendedResources[string(name)] = quantity.String()
	}
	if hasGPU {
		values.GpuCount = &gpu
	}
	return values
}

func taints(values []corev1.Taint) []string {
	result := make([]string, 0, len(values))
	for _, taint := range values {
		result = append(result, fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect))
	}
	sort.Strings(result)
	return result
}

func workloadOwner(owners []metav1.OwnerReference) (string, string) {
	for _, owner := range owners {
		if owner.Controller != nil && *owner.Controller {
			return owner.Kind, string(owner.UID)
		}
	}
	if len(owners) > 0 {
		return owners[0].Kind, string(owners[0].UID)
	}
	return "", ""
}

func conditionTime(conditions []corev1.PodCondition, conditionType corev1.PodConditionType) *timestamppb.Timestamp {
	for _, condition := range conditions {
		if condition.Type == conditionType && condition.Status == corev1.ConditionTrue {
			return timestamppb.New(condition.LastTransitionTime.Time)
		}
	}
	return nil
}

func containerTimes(status corev1.ContainerStatus) (*timestamppb.Timestamp, *timestamppb.Timestamp) {
	if status.State.Running != nil {
		return timestamppb.New(status.State.Running.StartedAt.Time), nil
	}
	if status.State.Terminated != nil {
		return timestamppb.New(status.State.Terminated.StartedAt.Time), timestamppb.New(status.State.Terminated.FinishedAt.Time)
	}
	if status.LastTerminationState.Terminated != nil {
		return timestamppb.New(status.LastTerminationState.Terminated.StartedAt.Time), timestamppb.New(status.LastTerminationState.Terminated.FinishedAt.Time)
	}
	return nil, nil
}

func timestamp(value *metav1.Time) *timestamppb.Timestamp {
	if value == nil || value.IsZero() {
		return nil
	}
	return timestamppb.New(value.Time)
}

func cloneMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func valueOrZero(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}
