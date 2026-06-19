package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestScoresEndpoint(t *testing.T) {
	t.Parallel()
	api := NewAPI(fakeKarpenterReader{snapshot: Snapshot{
		ClusterID:   "cluster",
		GeneratedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		NodePools: []NodePool{{
			Name:          "general",
			CapacityTypes: []string{"spot"},
			Zones:         []string{"a", "b"},
		}},
		NodeClaims: []NodeClaim{{
			NodePoolName:           "general",
			Ready:                  true,
			CPUCapacityMillicores:  1000,
			CPURequestedMillicores: 500,
			MemoryCapacityBytes:    1024,
			MemoryRequestedBytes:   512,
		}},
	}}, Scorer{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/karpenter/scores", nil)

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var scores Scores
	if err := json.Unmarshal(response.Body.Bytes(), &scores); err != nil {
		t.Fatal(err)
	}
	if scores.ClusterID != "cluster" || len(scores.NodePools) != 1 || scores.BinPackingScore != 50 {
		t.Fatalf("scores=%+v", scores)
	}
}

func TestSnapshotEndpoint(t *testing.T) {
	t.Parallel()
	api := NewAPI(fakeKarpenterReader{snapshot: Snapshot{ClusterID: "cluster"}}, Scorer{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/karpenter/snapshot", nil)

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestScoresEndpointReportsReaderError(t *testing.T) {
	t.Parallel()
	api := NewAPI(fakeKarpenterReader{err: errors.New("boom")}, Scorer{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/karpenter/scores", nil)

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeKarpenterReader struct {
	snapshot Snapshot
	err      error
}

func (r fakeKarpenterReader) Snapshot(context.Context) (Snapshot, error) {
	if r.err != nil {
		return Snapshot{}, r.err
	}
	return r.snapshot, nil
}
