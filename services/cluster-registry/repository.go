package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var ErrClusterNotFound = errors.New("cluster not found")

type Repository interface {
	Create(context.Context, Cluster) (Cluster, error)
	List(context.Context, string) ([]Cluster, error)
	Get(context.Context, string, string) (Cluster, error)
}

type MemoryRepository struct {
	mu       sync.Mutex
	clusters map[string]Cluster
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{clusters: make(map[string]Cluster)}
}

func (r *MemoryRepository) Create(_ context.Context, cluster Cluster) (Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if cluster.ClusterID == "" {
		clusterID, err := newID("clu")
		if err != nil {
			return Cluster{}, err
		}
		cluster.ClusterID = clusterID
	}
	cluster.Status = "pending_enrollment"
	cluster.CreatedAt = now
	cluster.UpdatedAt = now
	cluster.Labels = cloneStringMap(cluster.Labels)
	cluster.Capabilities = cloneStrings(cluster.Capabilities)
	r.clusters[clusterKey(cluster.TenantID, cluster.ClusterID)] = cluster
	return cluster, nil
}

func (r *MemoryRepository) List(_ context.Context, tenantID string) ([]Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Cluster, 0)
	for _, cluster := range r.clusters {
		if cluster.TenantID == tenantID {
			result = append(result, cloneCluster(cluster))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (r *MemoryRepository) Get(_ context.Context, tenantID, clusterID string) (Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cluster, ok := r.clusters[clusterKey(tenantID, clusterID)]
	if !ok {
		return Cluster{}, ErrClusterNotFound
	}
	return cloneCluster(cluster), nil
}

func clusterKey(tenantID, clusterID string) string {
	return tenantID + "\x00" + clusterID
}

func newID(prefix string) (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(random), nil
}

func cloneCluster(cluster Cluster) Cluster {
	cluster.Labels = cloneStringMap(cluster.Labels)
	cluster.Capabilities = cloneStrings(cluster.Capabilities)
	return cluster
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneStrings(source []string) []string {
	if len(source) == 0 {
		return nil
	}
	return append([]string(nil), source...)
}

type TokenGenerator struct{}

func (TokenGenerator) NewToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate enrollment token: %w", err)
	}
	return hex.EncodeToString(random), nil
}
