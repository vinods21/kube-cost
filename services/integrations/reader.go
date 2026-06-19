package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type KarpenterReader interface {
	Snapshot(context.Context) (Snapshot, error)
}

type DynamicReader struct {
	clusterID string
	dynamic   dynamic.Interface
	core      kubernetes.Interface
}

func NewDynamicReader(clusterID string, dynamicClient dynamic.Interface, coreClient kubernetes.Interface) *DynamicReader {
	return &DynamicReader{clusterID: clusterID, dynamic: dynamicClient, core: coreClient}
}

func (r *DynamicReader) Snapshot(ctx context.Context) (Snapshot, error) {
	nodePools, err := r.nodePools(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	nodeClasses, err := r.nodeClasses(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	nodeClaims, err := r.nodeClaims(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		ClusterID:   r.clusterID,
		GeneratedAt: metav1.Now().Time.UTC(),
		NodePools:   nodePools,
		NodeClasses: nodeClasses,
		NodeClaims:  nodeClaims,
	}, nil
}

func (r *DynamicReader) nodePools(ctx context.Context) ([]NodePool, error) {
	list, err := r.dynamic.Resource(schema.GroupVersionResource{
		Group:    "karpenter.sh",
		Version:  "v1",
		Resource: "nodepools",
	}).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Karpenter NodePools: %w", err)
	}
	result := make([]NodePool, 0, len(list.Items))
	for _, item := range list.Items {
		result = append(result, parseNodePool(item))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (r *DynamicReader) nodeClaims(ctx context.Context) ([]NodeClaim, error) {
	list, err := r.dynamic.Resource(schema.GroupVersionResource{
		Group:    "karpenter.sh",
		Version:  "v1",
		Resource: "nodeclaims",
	}).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Karpenter NodeClaims: %w", err)
	}
	nodeMetrics := r.nodeMetrics(ctx)
	result := make([]NodeClaim, 0, len(list.Items))
	for _, item := range list.Items {
		result = append(result, parseNodeClaim(item, nodeMetrics))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (r *DynamicReader) nodeClasses(ctx context.Context) ([]NodeClass, error) {
	var result []NodeClass
	for _, gvr := range []schema.GroupVersionResource{
		{Group: "karpenter.k8s.aws", Version: "v1", Resource: "ec2nodeclasses"},
		{Group: "karpenter.azure.com", Version: "v1", Resource: "aksnodeclasses"},
	} {
		list, err := r.dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			result = append(result, parseNodeClass(item))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (r *DynamicReader) nodeMetrics(ctx context.Context) map[string]nodeMetric {
	nodes, err := r.core.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	metrics := make(map[string]nodeMetric, len(nodes.Items))
	for _, node := range nodes.Items {
		metrics[node.Name] = nodeMetric{
			cpuCapacity:    quantityMilli(node.Status.Capacity[corev1.ResourceCPU]),
			memoryCapacity: quantityValue(node.Status.Capacity[corev1.ResourceMemory]),
		}
	}
	return metrics
}

type nodeMetric struct {
	cpuCapacity    uint64
	memoryCapacity uint64
}

func parseNodePool(item unstructured.Unstructured) NodePool {
	requirements, _, _ := unstructured.NestedSlice(item.Object, "spec", "template", "spec", "requirements")
	return NodePool{
		Name:               item.GetName(),
		UID:                string(item.GetUID()),
		NodeClassName:      firstString(nestedString(item.Object, "spec", "template", "spec", "nodeClassRef", "name"), nestedString(item.Object, "spec", "nodeClassRef", "name")),
		CapacityTypes:      requirementValues(requirements, "karpenter.sh/capacity-type"),
		InstanceCategories: requirementValues(requirements, "karpenter.k8s.aws/instance-category"),
		InstanceTypes:      requirementValues(requirements, "node.kubernetes.io/instance-type"),
		Zones:              requirementValues(requirements, "topology.kubernetes.io/zone"),
		Architectures:      requirementValues(requirements, "kubernetes.io/arch"),
		Consolidation:      consolidationEnabled(item.Object),
		Weight:             nestedInt64(item.Object, "spec", "weight"),
	}
}

func parseNodeClass(item unstructured.Unstructured) NodeClass {
	return NodeClass{
		Name:           item.GetName(),
		UID:            string(item.GetUID()),
		APIVersion:     item.GetAPIVersion(),
		Subnets:        selectorTerms(item.Object, "spec", "subnetSelectorTerms"),
		SecurityGroups: selectorTerms(item.Object, "spec", "securityGroupSelectorTerms"),
		AMIFamily:      nestedString(item.Object, "spec", "amiFamily"),
		Role:           nestedString(item.Object, "spec", "role"),
	}
}

func parseNodeClaim(item unstructured.Unstructured, nodeMetrics map[string]nodeMetric) NodeClaim {
	nodeName := firstString(nestedString(item.Object, "status", "nodeName"), nestedString(item.Object, "status", "node"))
	metric := nodeMetrics[nodeName]
	claim := NodeClaim{
		Name:                   item.GetName(),
		UID:                    string(item.GetUID()),
		NodePoolName:           firstString(label(item, "karpenter.sh/nodepool"), nestedString(item.Object, "spec", "nodePoolName")),
		NodeClassName:          firstString(nestedString(item.Object, "spec", "nodeClassRef", "name"), label(item, "karpenter.k8s.aws/ec2nodeclass")),
		NodeName:               nodeName,
		CapacityType:           label(item, "karpenter.sh/capacity-type"),
		InstanceType:           label(item, "node.kubernetes.io/instance-type"),
		Zone:                   label(item, "topology.kubernetes.io/zone"),
		Architecture:           label(item, "kubernetes.io/arch"),
		Ready:                  conditionTrue(item.Object, "Ready"),
		CPUCapacityMillicores:  firstUint64(quantityAt(item.Object, "status", "capacity", "cpu"), metric.cpuCapacity),
		MemoryCapacityBytes:    firstUint64(quantityAt(item.Object, "status", "capacity", "memory"), metric.memoryCapacity),
		CPURequestedMillicores: quantityAt(item.Object, "status", "resources", "requests", "cpu"),
		MemoryRequestedBytes:   quantityAt(item.Object, "status", "resources", "requests", "memory"),
	}
	claim.CPUUtilizationPercent = percent(claim.CPURequestedMillicores, claim.CPUCapacityMillicores)
	claim.MemoryUtilizationPercent = percent(claim.MemoryRequestedBytes, claim.MemoryCapacityBytes)
	return claim
}

func requirementValues(requirements []any, key string) []string {
	var result []string
	for _, entry := range requirements {
		requirement, ok := entry.(map[string]any)
		if !ok || fmt.Sprint(requirement["key"]) != key {
			continue
		}
		values, ok := requirement["values"].([]any)
		if !ok {
			continue
		}
		for _, value := range values {
			result = append(result, fmt.Sprint(value))
		}
	}
	sort.Strings(result)
	return result
}

func selectorTerms(object map[string]any, fields ...string) []string {
	terms, _, _ := unstructured.NestedSlice(object, fields...)
	var result []string
	for _, term := range terms {
		result = append(result, fmt.Sprint(term))
	}
	sort.Strings(result)
	return result
}

func consolidationEnabled(object map[string]any) bool {
	policy := nestedString(object, "spec", "disruption", "consolidationPolicy")
	return policy != "" && policy != "Never"
}

func conditionTrue(object map[string]any, conditionType string) bool {
	conditions, _, _ := unstructured.NestedSlice(object, "status", "conditions")
	for _, entry := range conditions {
		condition, ok := entry.(map[string]any)
		if ok && fmt.Sprint(condition["type"]) == conditionType && fmt.Sprint(condition["status"]) == "True" {
			return true
		}
	}
	return false
}

func label(item unstructured.Unstructured, key string) string {
	return item.GetLabels()[key]
}

func nestedString(object map[string]any, fields ...string) string {
	value, _, _ := unstructured.NestedString(object, fields...)
	return value
}

func nestedInt64(object map[string]any, fields ...string) int64 {
	value, _, _ := unstructured.NestedInt64(object, fields...)
	return value
}

func quantityAt(object map[string]any, fields ...string) uint64 {
	value, exists, _ := unstructured.NestedString(object, fields...)
	if exists {
		return parseQuantity(value)
	}
	raw, exists, _ := unstructured.NestedFieldNoCopy(object, fields...)
	if !exists {
		return 0
	}
	return parseQuantity(fmt.Sprint(raw))
}

func parseQuantity(value string) uint64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	quantity, err := resource.ParseQuantity(value)
	if err == nil {
		if strings.HasSuffix(value, "m") {
			return uint64(quantity.MilliValue())
		}
		if strings.Contains(value, "i") || strings.Contains(value, "E") || strings.Contains(value, "G") || strings.Contains(value, "M") || strings.Contains(value, "K") {
			return uint64(quantity.Value())
		}
		return uint64(quantity.MilliValue())
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func quantityMilli(quantity resource.Quantity) uint64 {
	if quantity.IsZero() {
		return 0
	}
	return uint64(quantity.MilliValue())
}

func quantityValue(quantity resource.Quantity) uint64 {
	if quantity.IsZero() {
		return 0
	}
	return uint64(quantity.Value())
}

func percent(numerator, denominator uint64) float64 {
	if denominator == 0 {
		return 0
	}
	return roundScore((float64(numerator) / float64(denominator)) * 100)
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstUint64(values ...uint64) uint64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
