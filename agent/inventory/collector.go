package inventory

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Publisher interface {
	Publish(context.Context, Event) error
}

type change struct {
	kind      string
	object    any
	operation agentv1.InventoryOperation
}

type Collector struct {
	clusterID     string
	discovery     discovery.DiscoveryInterface
	factory       informers.SharedInformerFactory
	builder       *Builder
	cache         *Cache
	publisher     Publisher
	queue         workqueue.TypedRateLimitingInterface[*change]
	synced        atomic.Bool
	podContainers map[string]map[string]Event
}

func NewCollector(clusterID string, client kubernetes.Interface, discoveryClient discovery.DiscoveryInterface, resync time.Duration, publisher Publisher) *Collector {
	return &Collector{
		clusterID:     clusterID,
		discovery:     discoveryClient,
		factory:       informers.NewSharedInformerFactory(client, resync),
		builder:       NewBuilder(),
		cache:         NewCache(),
		publisher:     publisher,
		queue:         workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[*change]()),
		podContainers: make(map[string]map[string]Event),
	}
}

func (c *Collector) Start(ctx context.Context) error {
	nodes := c.factory.Core().V1().Nodes().Informer()
	namespaces := c.factory.Core().V1().Namespaces().Informer()
	deployments := c.factory.Apps().V1().Deployments().Informer()
	pods := c.factory.Core().V1().Pods().Informer()

	if _, err := nodes.AddEventHandler(c.handler("node")); err != nil {
		return fmt.Errorf("register node informer: %w", err)
	}
	if _, err := namespaces.AddEventHandler(c.handler("namespace")); err != nil {
		return fmt.Errorf("register namespace informer: %w", err)
	}
	if _, err := deployments.AddEventHandler(c.handler("deployment")); err != nil {
		return fmt.Errorf("register deployment informer: %w", err)
	}
	if _, err := pods.AddEventHandler(c.handler("pod")); err != nil {
		return fmt.Errorf("register pod informer: %w", err)
	}

	snapshotID := fmt.Sprintf("%s-%d", c.clusterID, time.Now().UnixNano())
	c.queue.Add(&change{
		kind:      "snapshot",
		object:    c.builder.Snapshot(snapshotID, agentv1.InventorySnapshotPhase_INVENTORY_SNAPSHOT_PHASE_STARTED),
		operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT,
	})
	c.factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), nodes.HasSynced, namespaces.HasSynced, deployments.HasSynced, pods.HasSynced) {
		return fmt.Errorf("inventory informer cache synchronization failed")
	}

	version, err := c.discovery.ServerVersion()
	if err != nil {
		return fmt.Errorf("discover Kubernetes version: %w", err)
	}
	systemNamespace, err := c.factory.Core().V1().Namespaces().Lister().Get("kube-system")
	if err != nil {
		return fmt.Errorf("read kube-system namespace: %w", err)
	}
	c.queue.Add(&change{kind: "cluster", object: c.builder.Cluster(c.clusterID, version.GitVersion, systemNamespace), operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT})
	c.queue.Add(&change{
		kind:      "snapshot",
		object:    c.builder.Snapshot(snapshotID, agentv1.InventorySnapshotPhase_INVENTORY_SNAPSHOT_PHASE_COMPLETED),
		operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT,
	})
	go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	c.synced.Store(true)

	<-ctx.Done()
	c.queue.ShutDown()
	return nil
}

func (c *Collector) Ready() bool {
	return c.synced.Load()
}

func (c *Collector) CacheSize() int {
	return c.cache.Len()
}

func (c *Collector) handler(kind string) cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(object any) {
			c.queue.Add(&change{kind: kind, object: object, operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT})
		},
		UpdateFunc: func(_, object any) {
			c.queue.Add(&change{kind: kind, object: object, operation: agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT})
		},
		DeleteFunc: func(object any) {
			if tombstone, ok := object.(cache.DeletedFinalStateUnknown); ok {
				object = tombstone.Obj
			}
			c.queue.Add(&change{kind: kind, object: object, operation: agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE})
		},
	}
}

func (c *Collector) runWorker(ctx context.Context) {
	for c.processNext(ctx) {
	}
}

func (c *Collector) processNext(ctx context.Context) bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	if err := c.process(ctx, item); err != nil {
		slog.Error("inventory change processing failed", "kind", item.kind, "error", err)
		c.queue.AddRateLimited(item)
		return true
	}
	c.queue.Forget(item)
	return true
}

func (c *Collector) process(ctx context.Context, item *change) error {
	events, err := c.events(item)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := c.publishDelta(ctx, event, item.operation); err != nil {
			return err
		}
	}
	if item.kind == "pod" {
		c.updatePodContainers(item, events)
	}
	return nil
}

func (c *Collector) events(item *change) ([]Event, error) {
	switch item.kind {
	case "cluster", "snapshot":
		event, ok := item.object.(Event)
		if !ok {
			return nil, fmt.Errorf("unexpected cluster object %T", item.object)
		}
		return []Event{event}, nil
	case "node":
		object, ok := item.object.(*corev1.Node)
		if !ok {
			return nil, fmt.Errorf("unexpected node object %T", item.object)
		}
		return []Event{c.builder.Node(object, item.operation)}, nil
	case "namespace":
		object, ok := item.object.(*corev1.Namespace)
		if !ok {
			return nil, fmt.Errorf("unexpected namespace object %T", item.object)
		}
		return []Event{c.builder.Namespace(object, item.operation)}, nil
	case "deployment":
		object, ok := item.object.(*appsv1.Deployment)
		if !ok {
			return nil, fmt.Errorf("unexpected deployment object %T", item.object)
		}
		return []Event{c.builder.Deployment(object, item.operation)}, nil
	case "pod":
		object, ok := item.object.(*corev1.Pod)
		if !ok {
			return nil, fmt.Errorf("unexpected pod object %T", item.object)
		}
		events := []Event{c.builder.Pod(object, item.operation)}
		events = append(events, c.builder.Containers(object, item.operation)...)
		if item.operation == agentv1.InventoryOperation_INVENTORY_OPERATION_UPSERT {
			current := make(map[string]struct{}, len(events))
			for _, event := range events[1:] {
				current[event.Key] = struct{}{}
			}
			for key, previous := range c.podContainers[string(object.UID)] {
				if _, exists := current[key]; exists {
					continue
				}
				events = append(events, asDelete(previous))
			}
		}
		return events, nil
	default:
		return nil, fmt.Errorf("unsupported inventory kind %q", item.kind)
	}
}

func (c *Collector) publishDelta(ctx context.Context, event Event, operation agentv1.InventoryOperation) error {
	if operation == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE || isDelete(event) {
		if !c.cache.Exists(event.Key) {
			return nil
		}
		if err := c.publisher.Publish(ctx, event); err != nil {
			return err
		}
		c.cache.Delete(event.Key)
		return nil
	}

	changed, fingerprint, err := c.cache.Changed(event)
	if err != nil || !changed {
		return err
	}
	if err := c.publisher.Publish(ctx, event); err != nil {
		return err
	}
	return c.cache.Commit(event.Key, fingerprint)
}

func (c *Collector) updatePodContainers(item *change, events []Event) {
	pod, ok := item.object.(*corev1.Pod)
	if !ok {
		return
	}
	if item.operation == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE {
		delete(c.podContainers, string(pod.UID))
		return
	}
	current := make(map[string]Event, len(events))
	for _, event := range events[1:] {
		if !isDelete(event) {
			current[event.Key] = event
		}
	}
	c.podContainers[string(pod.UID)] = current
}

func asDelete(event Event) Event {
	cloned, ok := event.Observation.GetPayload().(*agentv1.Observation_ContainerInventory)
	if !ok {
		return event
	}
	copy := proto.Clone(cloned.ContainerInventory).(*agentv1.ContainerInventory)
	copy.Record = record(agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE)
	event.Observation = &agentv1.Observation{
		ObservedAt:            event.Observation.ObservedAt,
		CollectedAt:           event.Observation.CollectedAt,
		SourceResourceVersion: event.Observation.SourceResourceVersion,
		Payload:               &agentv1.Observation_ContainerInventory{ContainerInventory: copy},
	}
	return event
}

func isDelete(event Event) bool {
	switch payload := event.Observation.GetPayload().(type) {
	case *agentv1.Observation_ClusterInventory:
		return payload.ClusterInventory.GetRecord().GetOperation() == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE
	case *agentv1.Observation_NodeInventory:
		return payload.NodeInventory.GetRecord().GetOperation() == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE
	case *agentv1.Observation_NamespaceInventory:
		return payload.NamespaceInventory.GetRecord().GetOperation() == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE
	case *agentv1.Observation_DeploymentInventory:
		return payload.DeploymentInventory.GetRecord().GetOperation() == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE
	case *agentv1.Observation_PodInventory:
		return payload.PodInventory.GetRecord().GetOperation() == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE
	case *agentv1.Observation_ContainerInventory:
		return payload.ContainerInventory.GetRecord().GetOperation() == agentv1.InventoryOperation_INVENTORY_OPERATION_DELETE
	default:
		return false
	}
}
