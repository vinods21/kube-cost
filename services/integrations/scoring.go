package main

import (
	"math"
	"sort"
	"time"
)

type Scorer struct{}

func (s Scorer) Score(snapshot Snapshot) Scores {
	claimsByPool := make(map[string][]NodeClaim)
	for _, claim := range snapshot.NodeClaims {
		claimsByPool[claim.NodePoolName] = append(claimsByPool[claim.NodePoolName], claim)
	}

	poolScores := make([]NodePoolScore, 0, len(snapshot.NodePools))
	for _, pool := range snapshot.NodePools {
		score := scoreNodePool(pool, claimsByPool[pool.Name])
		poolScores = append(poolScores, score)
	}
	sort.Slice(poolScores, func(i, j int) bool {
		return poolScores[i].NodePoolName < poolScores[j].NodePoolName
	})
	return Scores{
		ClusterID:            snapshot.ClusterID,
		GeneratedAt:          firstNonZero(snapshot.GeneratedAt, time.Now().UTC()),
		NodePools:            poolScores,
		BinPackingScore:      averagePoolScore(poolScores, func(score NodePoolScore) float64 { return score.BinPackingScore }),
		SpotSuitabilityScore: averagePoolScore(poolScores, func(score NodePoolScore) float64 { return score.SpotSuitabilityScore }),
		NodeUtilizationScore: averagePoolScore(poolScores, func(score NodePoolScore) float64 { return score.NodeUtilizationScore }),
	}
}

func scoreNodePool(pool NodePool, claims []NodeClaim) NodePoolScore {
	readyClaims := 0
	for _, claim := range claims {
		if claim.Ready {
			readyClaims++
		}
	}
	return NodePoolScore{
		NodePoolName:         pool.Name,
		NodeClassName:        pool.NodeClassName,
		NodeClaimCount:       len(claims),
		ReadyNodeClaimCount:  readyClaims,
		BinPackingScore:      binPackingScore(claims),
		SpotSuitabilityScore: spotSuitabilityScore(pool),
		NodeUtilizationScore: nodeUtilizationScore(claims),
	}
}

func binPackingScore(claims []NodeClaim) float64 {
	if len(claims) == 0 {
		return 0
	}
	var totalWeight float64
	for _, claim := range claims {
		cpu := ratio(claim.CPURequestedMillicores, claim.CPUCapacityMillicores)
		memory := ratio(claim.MemoryRequestedBytes, claim.MemoryCapacityBytes)
		totalWeight += clamp01((cpu + memory) / 2)
	}
	return score100(totalWeight / float64(len(claims)))
}

func nodeUtilizationScore(claims []NodeClaim) float64 {
	var count int
	var total float64
	for _, claim := range claims {
		if !claim.Ready {
			continue
		}
		cpu := claim.CPUUtilizationPercent / 100
		if cpu == 0 {
			cpu = ratio(claim.CPURequestedMillicores, claim.CPUCapacityMillicores)
		}
		memory := claim.MemoryUtilizationPercent / 100
		if memory == 0 {
			memory = ratio(claim.MemoryRequestedBytes, claim.MemoryCapacityBytes)
		}
		total += clamp01((cpu + memory) / 2)
		count++
	}
	if count == 0 {
		return 0
	}
	return score100(total / float64(count))
}

func spotSuitabilityScore(pool NodePool) float64 {
	var score float64
	if contains(pool.CapacityTypes, "spot") {
		score += 45
	}
	if contains(pool.CapacityTypes, "on-demand") && contains(pool.CapacityTypes, "spot") {
		score += 10
	}
	score += math.Min(float64(uniqueCount(pool.InstanceCategories))*8, 24)
	score += math.Min(float64(uniqueCount(pool.InstanceTypes))*4, 16)
	score += math.Min(float64(uniqueCount(pool.Zones))*5, 15)
	if pool.Consolidation {
		score += 5
	}
	return clamp(score, 0, 100)
}

func averagePoolScore(scores []NodePoolScore, value func(NodePoolScore) float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var total float64
	for _, score := range scores {
		total += value(score)
	}
	return roundScore(total / float64(len(scores)))
}

func ratio(numerator, denominator uint64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func score100(value float64) float64 {
	return roundScore(clamp01(value) * 100)
}

func clamp01(value float64) float64 {
	return clamp(value, 0, 1)
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func roundScore(value float64) float64 {
	return math.Round(value*100) / 100
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func uniqueCount(values []string) int {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	return len(seen)
}

func firstNonZero(left, right time.Time) time.Time {
	if !left.IsZero() {
		return left
	}
	return right
}
