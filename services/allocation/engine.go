package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrInvalidQuery = errors.New("invalid allocation query")

type CostRepository interface {
	NamespaceCosts(context.Context, Query) ([]NamespaceCost, error)
	Ping(context.Context) error
	Close() error
}

type Engine struct {
	repository CostRepository
	options    AllocationOptions
}

func NewEngine(repository CostRepository, nodeHourlyCostUSD float64) *Engine {
	return NewEngineWithOptions(repository, AllocationOptions{NodeHourlyCostUSD: nodeHourlyCostUSD})
}

func NewEngineWithOptions(repository CostRepository, options AllocationOptions) *Engine {
	options = normalizeAllocationOptions(options)
	return &Engine{
		repository: repository,
		options:    options,
	}
}

func (e *Engine) NamespaceCosts(ctx context.Context, query Query) (Result, error) {
	normalized, err := normalizeQuery(query, time.Now().UTC())
	if err != nil {
		return Result{}, err
	}
	items, err := e.repository.NamespaceCosts(ctx, normalized)
	if err != nil {
		return Result{}, err
	}
	for index := range items {
		items[index].Currency = defaultCurrency
		items[index].AllocationMethod = allocationMethodCPU
		items[index].ComputationVersion = computationVersionV1
	}
	return Result{
		Currency:                  defaultCurrency,
		AllocationMethod:          allocationMethodCPU,
		NodeHourlyCostUSD:         e.options.NodeHourlyCostUSD,
		ControlPlaneHourlyCostUSD: e.options.ControlPlaneHourlyCostUSD,
		NetworkCostPerGiBUSD:      e.options.NetworkCostPerGiBUSD,
		Start:                     normalized.Start,
		End:                       normalized.End,
		Items:                     items,
	}, nil
}

func normalizeQuery(query Query, now time.Time) (Query, error) {
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.ClusterID = strings.TrimSpace(query.ClusterID)
	if query.TenantID == "" {
		return Query{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidQuery)
	}
	if query.End.IsZero() {
		query.End = now.Truncate(time.Hour)
	}
	if query.Start.IsZero() {
		query.Start = query.End.Add(-time.Hour)
	}
	query.Start = query.Start.UTC()
	query.End = query.End.UTC()
	if !query.Start.Before(query.End) {
		return Query{}, fmt.Errorf("%w: start must be before end", ErrInvalidQuery)
	}
	if !query.Start.Equal(query.Start.Truncate(time.Hour)) || !query.End.Equal(query.End.Truncate(time.Hour)) {
		return Query{}, fmt.Errorf("%w: start and end must be aligned to whole hours", ErrInvalidQuery)
	}
	return query, nil
}

func allocateByCPURequest(requests []NodeNamespaceRequest, nodeHourlyCostUSD float64) []NamespaceCost {
	return allocateCosts(requests, AllocationOptions{NodeHourlyCostUSD: nodeHourlyCostUSD})
}

func allocateCosts(requests []NodeNamespaceRequest, options AllocationOptions) []NamespaceCost {
	options = normalizeAllocationOptions(options)
	nodeTotals := make(map[string]uint64)
	nodeAllocatable := make(map[string]uint64)
	clusterTotals := make(map[string]uint64)
	for _, request := range requests {
		if request.NodeUID == "" || request.CPURequestCoreMilliseconds == 0 {
			continue
		}
		nodeKey := nodeBucketKey(request)
		clusterKey := clusterBucketKey(request)
		nodeTotals[nodeKey] += request.CPURequestCoreMilliseconds
		clusterTotals[clusterKey] += request.CPURequestCoreMilliseconds
		allocatable := request.NodeAllocatableMillicores * 3600
		if allocatable > nodeAllocatable[nodeKey] {
			nodeAllocatable[nodeKey] = allocatable
		}
	}

	byNamespace := make(map[string]*NamespaceCost)
	for _, request := range requests {
		nodeKey := nodeBucketKey(request)
		total := nodeTotals[nodeKey]
		denominator := maxUint64(total, nodeAllocatable[nodeKey])
		if denominator == 0 || request.CPURequestCoreMilliseconds == 0 {
			continue
		}
		key := namespaceBucketKey(request)
		item := byNamespace[key]
		if item == nil {
			bucketStart := request.BucketStart.UTC().Format(time.RFC3339)
			item = &NamespaceCost{
				TenantID:      request.TenantID,
				ClusterID:     request.ClusterID,
				NamespaceUID:  request.NamespaceUID,
				NamespaceName: request.NamespaceName,
				BucketStart:   bucketStart,
			}
			byNamespace[key] = item
		}
		clusterTotal := clusterTotals[clusterBucketKey(request)]
		nodeWeight := float64(request.CPURequestCoreMilliseconds) / float64(denominator)
		controlPlaneWeight := 0.0
		if clusterTotal > 0 {
			controlPlaneWeight = float64(request.CPURequestCoreMilliseconds) / float64(clusterTotal)
		}
		networkCost := bytesToGiB(request.NetworkBytes) * options.NetworkCostPerGiBUSD
		controlPlaneCost := options.ControlPlaneHourlyCostUSD * controlPlaneWeight
		directCost := options.NodeHourlyCostUSD * nodeWeight
		item.CPURequestCoreMilliseconds += request.CPURequestCoreMilliseconds
		item.NetworkBytes += request.NetworkBytes
		item.AllocationWeight += nodeWeight
		item.DirectCost += directCost
		item.NetworkCost += networkCost
		item.ControlPlaneCost += controlPlaneCost
		item.AllocatedCost += directCost + networkCost + controlPlaneCost
		if isSystemNamespace(request.NamespaceName, request.NamespaceUID) {
			item.SystemNamespaceCost += directCost + networkCost + controlPlaneCost
		}
	}
	for nodeKey, total := range nodeTotals {
		denominator := maxUint64(total, nodeAllocatable[nodeKey])
		if denominator == 0 || denominator <= total {
			continue
		}
		request := firstNodeRequest(nodeKey, requests)
		key := idleNamespaceBucketKey(request)
		item := byNamespace[key]
		if item == nil {
			item = &NamespaceCost{
				TenantID:      request.TenantID,
				ClusterID:     request.ClusterID,
				NamespaceUID:  idleNamespaceUID,
				NamespaceName: idleNamespaceName,
				BucketStart:   request.BucketStart.UTC().Format(time.RFC3339),
			}
			byNamespace[key] = item
		}
		idleWeight := float64(denominator-total) / float64(denominator)
		idleCost := options.NodeHourlyCostUSD * idleWeight
		item.IdleCost += idleCost
		item.AllocatedCost += idleCost
	}

	result := make([]NamespaceCost, 0, len(byNamespace))
	for _, item := range byNamespace {
		item.Currency = defaultCurrency
		item.AllocationMethod = allocationMethodCPU
		item.ComputationVersion = computationVersionV1
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].BucketStart != result[j].BucketStart {
			return result[i].BucketStart < result[j].BucketStart
		}
		if result[i].ClusterID != result[j].ClusterID {
			return result[i].ClusterID < result[j].ClusterID
		}
		return result[i].NamespaceUID < result[j].NamespaceUID
	})
	return result
}

func normalizeAllocationOptions(options AllocationOptions) AllocationOptions {
	if options.NodeHourlyCostUSD <= 0 {
		options.NodeHourlyCostUSD = defaultNodeHourlyCostUSD
	}
	if options.ControlPlaneHourlyCostUSD < 0 {
		options.ControlPlaneHourlyCostUSD = defaultControlPlaneHourlyCostUSD
	}
	if options.NetworkCostPerGiBUSD < 0 {
		options.NetworkCostPerGiBUSD = defaultNetworkCostPerGiBUSD
	}
	return options
}

func isSystemNamespace(namespaceName, namespaceUID string) bool {
	switch namespaceName {
	case "kube-system", "kube-public", "kube-node-lease":
		return true
	}
	switch namespaceUID {
	case "kube-system", "kube-public", "kube-node-lease":
		return true
	}
	return false
}

func bytesToGiB(bytes uint64) float64 {
	return float64(bytes) / 1073741824
}

func maxUint64(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}

func firstNodeRequest(nodeKey string, requests []NodeNamespaceRequest) NodeNamespaceRequest {
	for _, request := range requests {
		if nodeBucketKey(request) == nodeKey {
			return request
		}
	}
	return NodeNamespaceRequest{}
}

func nodeBucketKey(request NodeNamespaceRequest) string {
	return request.TenantID + "\x00" + request.ClusterID + "\x00" + request.NodeUID + "\x00" + request.BucketStart.UTC().Format(time.RFC3339)
}

func namespaceBucketKey(request NodeNamespaceRequest) string {
	return request.TenantID + "\x00" + request.ClusterID + "\x00" + request.NamespaceUID + "\x00" + request.BucketStart.UTC().Format(time.RFC3339)
}

func idleNamespaceBucketKey(request NodeNamespaceRequest) string {
	return request.TenantID + "\x00" + request.ClusterID + "\x00" + idleNamespaceUID + "\x00" + request.BucketStart.UTC().Format(time.RFC3339)
}

func clusterBucketKey(request NodeNamespaceRequest) string {
	return request.TenantID + "\x00" + request.ClusterID + "\x00" + request.BucketStart.UTC().Format(time.RFC3339)
}
