package inventory

import (
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestBuilderMapsRequiredInventory(t *testing.T) {
	t.Parallel()
	builder := NewBuilder()
	now := metav1.NewTime(time.Unix(100, 0))

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			UID:             types.UID("node-1"),
			Name:            "node-1",
			ResourceVersion: "10",
			Labels: map[string]string{
				"node.kubernetes.io/instance-type": "m7i.large",
				"topology.kubernetes.io/region":    "us-east-1",
				"topology.kubernetes.io/zone":      "us-east-1a",
			},
		},
		Spec: corev1.NodeSpec{ProviderID: "aws:///us-east-1a/i-1"},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1900m"),
				corev1.ResourceMemory: resource.MustParse("7Gi"),
			},
		},
	}
	nodeEvent := builder.Node(node, agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT)
	nodeInventory := nodeEvent.Observation.GetNodeInventory()
	if nodeInventory.InstanceType != "m7i.large" || nodeInventory.GetCapacity().GetCpuMillicores() != 2000 {
		t.Fatalf("unexpected node inventory: %+v", nodeInventory)
	}

	replicas := int32(3)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID("deployment-1"), Name: "api", Namespace: "apps"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
		},
	}
	if got := builder.Deployment(deployment, agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT).Observation.GetDeploymentInventory().DesiredReplicas; got != 3 {
		t.Fatalf("desired replicas=%d", got)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID("pod-1"),
			Name:      "api-1",
			Namespace: "apps",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{{
				Name:  "app",
				Image: "api:v1",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")},
				},
			}},
		},
		Status: corev1.PodStatus{
			StartTime: &now,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "app",
				ContainerID:  "containerd://one",
				RestartCount: 2,
			}},
		},
	}
	containers := builder.Containers(pod, agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT)
	if len(containers) != 1 {
		t.Fatalf("containers=%d", len(containers))
	}
	container := containers[0].Observation.GetContainerInventory()
	if container.ContainerId != "containerd://one" || container.GetRequests().GetCpuMillicores() != 250 {
		t.Fatalf("unexpected container inventory: %+v", container)
	}
}

func TestMetadataDoesNotCollectAnnotations(t *testing.T) {
	t.Parallel()
	meta := metadata(metav1.ObjectMeta{
		UID:         types.UID("one"),
		Name:        "one",
		Annotations: map[string]string{"secret.example/token": "sensitive"},
	})
	if len(meta.Annotations) != 0 {
		t.Fatal("annotations must not be collected by Agent V1")
	}
}
