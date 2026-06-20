package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kube-cost/kube-cost/internal/gatewayauth"
)

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
	mux.HandleFunc("GET /api/v1/tenant", a.profile)
	mux.HandleFunc("GET /api/v1/tenant/members", a.listMembers)
	mux.HandleFunc("PUT /api/v1/tenant/members/{principal_id}", a.putMember)
	mux.HandleFunc("DELETE /api/v1/tenant/members/{principal_id}", a.deleteMember)
	return mux
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) profile(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, TenantProfile{
		TenantID: tenantID,
		Status:   "active",
		Source:   "gateway",
		SeenAt:   a.now().UTC(),
	})
}

func (a *API) listMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, MembersResult{
		TenantID: tenantID,
		Members:  a.store.Members(tenantID),
	})
}

func (a *API) putMember(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	principalID := strings.TrimSpace(r.PathValue("principal_id"))
	if principalID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "principal_id is required")
		return
	}
	request, ok := decodeMemberRequest(w, r)
	if !ok {
		return
	}
	role, ok := normalizeRole(w, request.Role)
	if !ok {
		return
	}
	member := a.store.UpsertMember(tenantID, principalID, role, strings.TrimSpace(request.DisplayName), a.now().UTC())
	writeJSON(w, http.StatusOK, member)
}

func (a *API) deleteMember(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	principalID := strings.TrimSpace(r.PathValue("principal_id"))
	if principalID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "principal_id is required")
		return
	}
	if !a.store.DeleteMember(tenantID, principalID) {
		writeProblem(w, http.StatusNotFound, "not_found", "tenant member not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeMemberRequest(w http.ResponseWriter, r *http.Request) (MemberRequest, bool) {
	defer r.Body.Close()
	var request MemberRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "request body is required")
			return MemberRequest{}, false
		}
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body must be valid member JSON")
		return MemberRequest{}, false
	}
	return request, true
}

func normalizeRole(w http.ResponseWriter, role string) (string, bool) {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "owner", "admin", "viewer":
		return role, true
	default:
		writeProblem(w, http.StatusBadRequest, "invalid_request", "role must be one of owner, admin, viewer")
		return "", false
	}
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
