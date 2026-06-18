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
	repository        CostRepository
	nodeHourlyCostUSD float64
}

func NewEngine(repository CostRepository, nodeHourlyCostUSD float64) *Engine {
	if nodeHourlyCostUSD <= 0 {
		nodeHourlyCostUSD = defaultNodeHourlyCostUSD
	}
	return &Engine{
		repository:        repository,
		nodeHourlyCostUSD: nodeHourlyCostUSD,
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
		Currency:          defaultCurrency,
		AllocationMethod:  allocationMethodCPU,
		NodeHourlyCostUSD: e.nodeHourlyCostUSD,
		Start:             normalized.Start,
		End:               normalized.End,
		Items:             items,
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
	if nodeHourlyCostUSD <= 0 {
		nodeHourlyCostUSD = defaultNodeHourlyCostUSD
	}
	nodeTotals := make(map[string]uint64)
	for _, request := range requests {
		if request.NodeUID == "" || request.CPURequestCoreMilliseconds == 0 {
			continue
		}
		nodeTotals[nodeBucketKey(request)] += request.CPURequestCoreMilliseconds
	}

	byNamespace := make(map[string]*NamespaceCost)
	for _, request := range requests {
		total := nodeTotals[nodeBucketKey(request)]
		if total == 0 || request.CPURequestCoreMilliseconds == 0 {
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
		weight := float64(request.CPURequestCoreMilliseconds) / float64(total)
		item.CPURequestCoreMilliseconds += request.CPURequestCoreMilliseconds
		item.AllocationWeight += weight
		item.AllocatedCost += nodeHourlyCostUSD * weight
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

func nodeBucketKey(request NodeNamespaceRequest) string {
	return request.TenantID + "\x00" + request.ClusterID + "\x00" + request.NodeUID + "\x00" + request.BucketStart.UTC().Format(time.RFC3339)
}

func namespaceBucketKey(request NodeNamespaceRequest) string {
	return request.TenantID + "\x00" + request.ClusterID + "\x00" + request.NamespaceUID + "\x00" + request.BucketStart.UTC().Format(time.RFC3339)
}
