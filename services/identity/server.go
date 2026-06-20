package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kube-cost/kube-cost/internal/gatewayauth"
)

type API struct {
	now func() time.Time
}

func NewAPI(now func() time.Time) *API {
	return &API{now: now}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/identity/principal", a.principal)
	return mux
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) principal(w http.ResponseWriter, r *http.Request) {
	tenantID, principalID, ok := authenticatedPrincipal(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, PrincipalProfile{
		TenantID:    tenantID,
		PrincipalID: principalID,
		Source:      "gateway",
		SeenAt:      a.now().UTC(),
	})
}

func authenticatedPrincipal(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	if !trustedGateway(w, r) {
		return "", "", false
	}
	tenantID := strings.TrimSpace(r.Header.Get(tenantHeader))
	if tenantID == "" {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", tenantHeader+" is required")
		return "", "", false
	}
	principalID := strings.TrimSpace(r.Header.Get(principalHeader))
	if principalID == "" {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", principalHeader+" is required")
		return "", "", false
	}
	return tenantID, principalID, true
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
