package inventory

import (
	"context"
	"sync"
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []Event
	notify chan struct{}
}

func newRecordingPublisher() *recordingPublisher {
	return &recordingPublisher{notify: make(chan struct{}, 100)}
}

func (p *recordingPublisher) Publish(_ context.Context, event Event) error {
	p.mu.Lock()
	p.events = append(p.events, event)
	p.mu.Unlock()
	select {
	case p.notify <- struct{}{}:
	default:
	}
	return nil
}

func (p *recordingPublisher) payloads() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make(map[string]int)
	for _, event := range p.events {
		switch event.Observation.Payload.(type) {
		case *agentv1.Observation_ClusterInventory:
			result["cluster"]++
		case *agentv1.Observation_NodeInventory:
			result["node"]++
		case *agentv1.Observation_NamespaceInventory:
			result["namespace"]++
		case *agentv1.Observation_DeploymentInventory:
			result["deployment"]++
		case *agentv1.Observation_PodInventory:
			result["pod"]++
		case *agentv1.Observation_ContainerInventory:
			result["container"]++
		case *agentv1.Observation_InventorySnapshotMarker:
			switch event.Observation.GetInventorySnapshotMarker().Phase {
			case agentv1.InventorySnapshotPhase_INVENTORY_SNAPSHOT_PHASE_STARTED:
				result["snapshot_started"]++
			case agentv1.InventorySnapshotPhase_INVENTORY_SNAPSHOT_PHASE_COMPLETED:
				result["snapshot_completed"]++
			}
		}
	}
	return result
}

func TestCollectorPublishesInitialInformerInventory(t *testing.T) {
	t.Parallel()
	client := kubernetesfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{UID: types.UID("system"), Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{UID: types.UID("apps"), Name: "apps"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{UID: types.UID("node"), Name: "node"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{UID: types.UID("deployment"), Name: "api", Namespace: "apps"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{UID: types.UID("pod"), Name: "api-1", Namespace: "apps"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "api:v1"}}},
		},
	)
	discovery := client.Discovery().(*fake.FakeDiscovery)
	discovery.FakedServerVersion = &version.Info{GitVersion: "v1.33.0"}
	publisher := newRecordingPublisher()
	collector := NewCollector("cluster-1", client, discovery, 0, publisher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- collector.Start(ctx) }()

	deadline := time.After(5 * time.Second)
	for {
		payloads := publisher.payloads()
		if payloads["cluster"] == 1 &&
			payloads["node"] == 1 &&
			payloads["namespace"] == 2 &&
			payloads["deployment"] == 1 &&
			payloads["pod"] == 1 &&
			payloads["container"] == 1 &&
			payloads["snapshot_started"] == 1 &&
			payloads["snapshot_completed"] == 1 {
			break
		}
		select {
		case <-publisher.notify:
		case err := <-errCh:
			t.Fatalf("collector stopped: %v", err)
		case <-deadline:
			t.Fatalf("timed out waiting for inventory: %#v", payloads)
		}
	}
	if !collector.Ready() {
		t.Fatal("collector did not become ready")
	}
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.events[0].Observation.GetInventorySnapshotMarker().GetPhase() != agentv1.InventorySnapshotPhase_INVENTORY_SNAPSHOT_PHASE_STARTED {
		t.Fatal("initial inventory did not begin with a snapshot marker")
	}
	if publisher.events[len(publisher.events)-1].Observation.GetInventorySnapshotMarker().GetPhase() != agentv1.InventorySnapshotPhase_INVENTORY_SNAPSHOT_PHASE_COMPLETED {
		t.Fatal("initial inventory did not end with a snapshot completion marker")
	}
	for _, event := range publisher.events {
		if pod := event.Observation.GetPodInventory(); pod != nil && pod.GetNamespaceUid() != "apps" {
			t.Fatalf("pod namespace_uid = %q, want apps", pod.GetNamespaceUid())
		}
		if container := event.Observation.GetContainerInventory(); container != nil && container.GetNamespaceUid() != "apps" {
			t.Fatalf("container namespace_uid = %q, want apps", container.GetNamespaceUid())
		}
	}
}

func TestCollectorEmitsRemovedContainerTombstone(t *testing.T) {
	t.Parallel()
	publisher := newRecordingPublisher()
	collector := &Collector{
		builder:       NewBuilder(),
		cache:         NewCache(),
		publisher:     publisher,
		podContainers: make(map[string]map[string]Event),
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID("pod"), Name: "pod", Namespace: "apps"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "one"},
			{Name: "two"},
		}},
	}
	if err := collector.process(context.Background(), &change{
		kind: "pod", object: pod, operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT,
	}); err != nil {
		t.Fatal(err)
	}
	pod = pod.DeepCopy()
	pod.ResourceVersion = "2"
	pod.Spec.Containers = pod.Spec.Containers[:1]
	if err := collector.process(context.Background(), &change{
		kind: "pod", object: pod, operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT,
	}); err != nil {
		t.Fatal(err)
	}

	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	var deleted bool
	for _, event := range publisher.events {
		container := event.Observation.GetContainerInventory()
		if container != nil &&
			container.ContainerName == "two" &&
			container.GetRecord().GetOperation() == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE {
			deleted = true
		}
	}
	if !deleted {
		t.Fatal("removed container tombstone was not published")
	}
}
