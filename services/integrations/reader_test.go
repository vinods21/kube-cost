package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseNodePool(t *testing.T) {
	t.Parallel()
	pool := parseNodePool(unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "karpenter.sh/v1",
		"kind":       "NodePool",
		"metadata": map[string]any{
			"name": "general",
			"uid":  "pool-uid",
		},
		"spec": map[string]any{
			"weight": int64(10),
			"disruption": map[string]any{
				"consolidationPolicy": "WhenEmptyOrUnderutilized",
			},
			"template": map[string]any{
				"spec": map[string]any{
					"nodeClassRef": map[string]any{"name": "default"},
					"requirements": []any{
						map[string]any{"key": "karpenter.sh/capacity-type", "values": []any{"spot", "on-demand"}},
						map[string]any{"key": "karpenter.k8s.aws/instance-category", "values": []any{"c", "m"}},
						map[string]any{"key": "topology.kubernetes.io/zone", "values": []any{"us-east-1a"}},
					},
				},
			},
		},
	}})

	if pool.Name != "general" || pool.NodeClassName != "default" || !pool.Consolidation || pool.Weight != 10 {
		t.Fatalf("pool=%+v", pool)
	}
	if len(pool.CapacityTypes) != 2 || pool.CapacityTypes[0] != "on-demand" || pool.CapacityTypes[1] != "spot" {
		t.Fatalf("capacity types=%v", pool.CapacityTypes)
	}
}

func TestParseNodeClaim(t *testing.T) {
	t.Parallel()
	claim := parseNodeClaim(unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "karpenter.sh/v1",
		"kind":       "NodeClaim",
		"metadata": map[string]any{
			"name": "claim-1",
			"uid":  "claim-uid",
			"labels": map[string]any{
				"karpenter.sh/nodepool":            "general",
				"karpenter.sh/capacity-type":       "spot",
				"node.kubernetes.io/instance-type": "m7g.large",
				"topology.kubernetes.io/zone":      "us-east-1a",
				"kubernetes.io/arch":               "arm64",
			},
		},
		"status": map[string]any{
			"nodeName": "node-1",
			"capacity": map[string]any{
				"cpu":    "2",
				"memory": "8Gi",
			},
			"resources": map[string]any{
				"requests": map[string]any{
					"cpu":    "1000m",
					"memory": "4Gi",
				},
			},
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}}, nil)

	if claim.NodePoolName != "general" || claim.CapacityType != "spot" || !claim.Ready {
		t.Fatalf("claim=%+v", claim)
	}
	if claim.CPUCapacityMillicores != 2000 || claim.CPURequestedMillicores != 1000 {
		t.Fatalf("cpu capacity/request=%d/%d", claim.CPUCapacityMillicores, claim.CPURequestedMillicores)
	}
	if claim.MemoryCapacityBytes != 8*1024*1024*1024 || claim.MemoryRequestedBytes != 4*1024*1024*1024 {
		t.Fatalf("memory capacity/request=%d/%d", claim.MemoryCapacityBytes, claim.MemoryRequestedBytes)
	}
	if claim.CPUUtilizationPercent != 50 || claim.MemoryUtilizationPercent != 50 {
		t.Fatalf("utilization cpu/memory=%f/%f", claim.CPUUtilizationPercent, claim.MemoryUtilizationPercent)
	}
}

func TestParseNodeClass(t *testing.T) {
	t.Parallel()
	item := unstructured.Unstructured{}
	item.SetAPIVersion("karpenter.k8s.aws/v1")
	item.SetKind("EC2NodeClass")
	item.SetName("default")
	item.SetUID("class-uid")
	item.Object["spec"] = map[string]any{
		"amiFamily": "AL2023",
		"role":      "karpenter-node",
		"subnetSelectorTerms": []any{
			map[string]any{"tags": map[string]any{"karpenter.sh/discovery": "demo"}},
		},
	}

	class := parseNodeClass(item)

	if class.Name != "default" || class.APIVersion != "karpenter.k8s.aws/v1" || class.AMIFamily != "AL2023" {
		t.Fatalf("class=%+v", class)
	}
	if len(class.Subnets) != 1 {
		t.Fatalf("subnets=%v", class.Subnets)
	}
}

func TestParseQuantity(t *testing.T) {
	t.Parallel()
	if parseQuantity("250m") != 250 {
		t.Fatal("cpu millicores not parsed")
	}
	if parseQuantity("2") != 2000 {
		t.Fatal("cpu cores not parsed as millicores")
	}
	if parseQuantity("1Gi") != 1024*1024*1024 {
		t.Fatal("memory quantity not parsed")
	}
}
