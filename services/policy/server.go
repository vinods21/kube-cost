package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kube-cost/kube-cost/internal/gatewayauth"
)

var policyNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

type API struct {
	store *Store
	now   func() time.Time
}

func NewAPI(store *Store, now func() time.Time) *API {
	return &API{store: store, now: now}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/policies", a.listPolicies)
	mux.HandleFunc("POST /api/v1/policies/{family}/versions", a.createVersion)
	mux.HandleFunc("POST /api/v1/policies/{family}/versions/{version}/activate", a.activateVersion)
	return mux
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) listPolicies(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := authenticatedActor(w, r)
	if !ok {
		return
	}
	families := a.store.Families(tenantID)
	writeJSON(w, http.StatusOK, PolicyFamiliesResult{
		TenantID:    tenantID,
		Families:    families,
		ResultCount: len(families),
	})
}

func (a *API) createVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, actor, ok := authenticatedActor(w, r)
	if !ok {
		return
	}
	family, ok := pathPolicyName(w, r.PathValue("family"), "family")
	if !ok {
		return
	}
	request, ok := decodeVersionRequest(w, r)
	if !ok {
		return
	}
	request, ok = normalizeVersionRequest(w, request)
	if !ok {
		return
	}
	version, err := a.store.CreateVersion(tenantID, family, actor, request, a.now().UTC())
	if errors.Is(err, ErrVersionExists) {
		writeProblem(w, http.StatusConflict, "conflict", "policy version already exists")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "policy_failed", "create policy version failed")
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

func (a *API) activateVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, actor, ok := authenticatedActor(w, r)
	if !ok {
		return
	}
	family, ok := pathPolicyName(w, r.PathValue("family"), "family")
	if !ok {
		return
	}
	version, ok := pathPolicyName(w, r.PathValue("version"), "version")
	if !ok {
		return
	}
	policy, found := a.store.ActivateVersion(tenantID, family, version, actor, a.now().UTC())
	if !found {
		writeProblem(w, http.StatusNotFound, "not_found", "policy version not found")
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func decodeVersionRequest(w http.ResponseWriter, r *http.Request) (VersionRequest, bool) {
	defer r.Body.Close()
	var request VersionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "request body is required")
			return VersionRequest{}, false
		}
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body must be valid policy JSON")
		return VersionRequest{}, false
	}
	return request, true
}

func normalizeVersionRequest(w http.ResponseWriter, request VersionRequest) (VersionRequest, bool) {
	request.Version = strings.TrimSpace(request.Version)
	if request.Version != "" && !policyNamePattern.MatchString(request.Version) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "version must be lowercase letters, numbers, dots, underscores, or hyphens")
		return VersionRequest{}, false
	}
	request.Description = strings.TrimSpace(request.Description)
	request.EffectiveStart = strings.TrimSpace(request.EffectiveStart)
	if request.EffectiveStart != "" {
		if _, err := time.Parse(time.RFC3339, request.EffectiveStart); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "effective_start must be an RFC3339 timestamp")
			return VersionRequest{}, false
		}
	}
	if len(request.Rules) == 0 || !json.Valid(request.Rules) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "rules must be valid JSON")
		return VersionRequest{}, false
	}
	request.Rules = cloneRawMessage(request.Rules)
	return request, true
}

func pathPolicyName(w http.ResponseWriter, value, name string) (string, bool) {
	value = strings.TrimSpace(value)
	if !policyNamePattern.MatchString(value) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", name+" must be lowercase letters, numbers, dots, underscores, or hyphens")
		return "", false
	}
	return value, true
}

func authenticatedActor(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	if !trustedGateway(w, r) {
		return "", "", false
	}
	tenantID := strings.TrimSpace(r.Header.Get(tenantHeader))
	if tenantID == "" {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", tenantHeader+" is required")
		return "", "", false
	}
	return tenantID, strings.TrimSpace(r.Header.Get(principalHeader)), true
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
