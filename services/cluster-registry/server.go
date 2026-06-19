package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kube-cost/kube-cost/internal/gatewayauth"
)

const (
	tenantHeader        = "X-Kube-Cost-Tenant-ID"
	gatewaySecretHeader = "X-Kube-Cost-Gateway-Secret"
)

type EnrollmentTokenGenerator interface {
	NewToken() (string, error)
}

type API struct {
	repository Repository
	tokens     EnrollmentTokenGenerator
}

func NewAPI(repository Repository, tokens EnrollmentTokenGenerator) *API {
	return &API{repository: repository, tokens: tokens}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/v1/clusters", a.registerCluster)
	mux.HandleFunc("GET /api/v1/clusters", a.listClusters)
	mux.HandleFunc("GET /api/v1/clusters/{cluster_id}", a.getCluster)
	return mux
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) registerCluster(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	var request RegisterClusterRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("invalid JSON request: %v", err))
		return
	}
	if strings.TrimSpace(request.ClusterName) == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "cluster_name is required")
		return
	}
	token, err := a.tokens.NewToken()
	if err != nil {
		slog.Error("generate enrollment token failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "generate enrollment token failed")
		return
	}
	cluster, err := a.repository.Create(r.Context(), Cluster{
		TenantID:     tenantID,
		ClusterName:  strings.TrimSpace(request.ClusterName),
		Provider:     strings.TrimSpace(request.Provider),
		AccountID:    strings.TrimSpace(request.AccountID),
		Region:       strings.TrimSpace(request.Region),
		Capabilities: cleanStrings(request.Capabilities),
		Labels:       cleanLabels(request.Labels),
	})
	if err != nil {
		slog.Error("register cluster failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "register cluster failed")
		return
	}
	writeJSON(w, http.StatusCreated, RegisterClusterResponse{
		Cluster:         cluster,
		EnrollmentToken: token,
		TokenExpiresAt:  cluster.CreatedAt.Add(defaultEnrollmentTTL),
	})
}

func (a *API) listClusters(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	clusters, err := a.repository.List(r.Context(), tenantID)
	if err != nil {
		slog.Error("list clusters failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "list clusters failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string][]Cluster{"data": clusters})
}

func (a *API) getCluster(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	cluster, err := a.repository.Get(r.Context(), tenantID, r.PathValue("cluster_id"))
	if err != nil {
		if errors.Is(err, ErrClusterNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "cluster not found")
			return
		}
		slog.Error("get cluster failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "get cluster failed")
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

func authenticatedTenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !trustedGateway(w, r) {
		return "", false
	}
	tenantID := strings.TrimSpace(r.Header.Get(tenantHeader))
	if tenantID == "" {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", tenantHeader+" is required")
		return "", false
	}
	return tenantID, true
}

func trustedGateway(w http.ResponseWriter, r *http.Request) bool {
	expected := strings.TrimSpace(os.Getenv("TRUSTED_GATEWAY_SECRET"))
	if expected != "" && r.Header.Get(gatewaySecretHeader) != expected {
		writeProblem(w, http.StatusForbidden, "forbidden", gatewaySecretHeader+" is required")
		return false
	}
	signingKey := strings.TrimSpace(os.Getenv("TRUSTED_GATEWAY_SIGNING_KEY"))
	if err := gatewayauth.VerifyRequest(r, signingKey, time.Now().UTC(), 5*time.Minute); err != nil {
		writeProblem(w, http.StatusForbidden, "forbidden", err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "https://kube-cost.dev/problems/" + code,
		"title":  http.StatusText(status),
		"status": status,
		"code":   code,
		"detail": detail,
	})
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cleanLabels(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
