package main

import (
	"math"
	"testing"
	"time"
)

func TestScorerProducesPoolAndFleetScores(t *testing.T) {
	t.Parallel()
	snapshot := Snapshot{
		ClusterID:   "cluster",
		GeneratedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		NodePools: []NodePool{{
			Name:               "general",
			NodeClassName:      "default",
			CapacityTypes:      []string{"spot", "on-demand"},
			InstanceCategories: []string{"c", "m", "r"},
			InstanceTypes:      []string{"c7g.large", "m7g.large"},
			Zones:              []string{"us-east-1a", "us-east-1b"},
			Consolidation:      true,
		}},
		NodeClaims: []NodeClaim{{
			Name:                     "claim-1",
			NodePoolName:             "general",
			Ready:                    true,
			CPUCapacityMillicores:    4000,
			CPURequestedMillicores:   2000,
			MemoryCapacityBytes:      8 * 1024 * 1024 * 1024,
			MemoryRequestedBytes:     6 * 1024 * 1024 * 1024,
			CPUUtilizationPercent:    60,
			MemoryUtilizationPercent: 70,
		}},
	}

	scores := Scorer{}.Score(snapshot)

	if scores.ClusterID != "cluster" || len(scores.NodePools) != 1 {
		t.Fatalf("scores=%+v", scores)
	}
	pool := scores.NodePools[0]
	assertScore(t, pool.BinPackingScore, 62.5)
	assertScore(t, pool.NodeUtilizationScore, 65)
	assertScore(t, pool.SpotSuitabilityScore, 100)
	assertScore(t, scores.BinPackingScore, 62.5)
}

func TestSpotSuitabilityWithoutSpotIsLow(t *testing.T) {
	t.Parallel()
	score := spotSuitabilityScore(NodePool{
		CapacityTypes: []string{"on-demand"},
		Zones:         []string{"us-east-1a"},
	})
	if score >= 50 {
		t.Fatalf("spot suitability=%f, want low score", score)
	}
}

func assertScore(t *testing.T, actual, expected float64) {
	t.Helper()
	if math.Abs(actual-expected) > 0.001 {
		t.Fatalf("score=%f, want %f", actual, expected)
	}
}
