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

type ControlAPI struct {
	store *IntegrationStore
	now   func() time.Time
}

func NewControlAPI(store *IntegrationStore, now func() time.Time) *ControlAPI {
	return &ControlAPI{store: store, now: now}
}

func (a *ControlAPI) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/v1/integrations", a.createIntegration)
	mux.HandleFunc("GET /api/v1/integrations", a.listIntegrations)
	mux.HandleFunc("POST /api/v1/integrations/{integration_id}/validate", a.validateIntegration)
	return mux
}

func (a *ControlAPI) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *ControlAPI) createIntegration(w http.ResponseWriter, r *http.Request) {
	tenantID, actor, ok := authenticatedActor(w, r)
	if !ok {
		return
	}
	request, ok := decodeIntegrationRequest(w, r)
	if !ok {
		return
	}
	request, ok = normalizeIntegrationRequest(w, request)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, a.store.Create(tenantID, actor, request, a.now().UTC()))
}

func (a *ControlAPI) listIntegrations(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := authenticatedActor(w, r)
	if !ok {
		return
	}
	integrations := a.store.List(tenantID)
	writeJSON(w, http.StatusOK, IntegrationsResult{
		TenantID:     tenantID,
		Integrations: integrations,
		ResultCount:  len(integrations),
	})
}

func (a *ControlAPI) validateIntegration(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := authenticatedActor(w, r)
	if !ok {
		return
	}
	integrationID := strings.TrimSpace(r.PathValue("integration_id"))
	if integrationID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "integration_id is required")
		return
	}
	integration, found := a.store.Validate(tenantID, integrationID, a.now().UTC())
	if !found {
		writeProblem(w, http.StatusNotFound, "not_found", "integration not found")
		return
	}
	writeJSON(w, http.StatusAccepted, ValidationResult{
		TenantID:      tenantID,
		IntegrationID: integration.IntegrationID,
		Status:        integration.Status,
		ValidatedAt:   *integration.LastValidated,
		Message:       "bootstrap validation recorded",
	})
}

func decodeIntegrationRequest(w http.ResponseWriter, r *http.Request) (IntegrationRequest, bool) {
	defer r.Body.Close()
	var request IntegrationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "request body is required")
			return IntegrationRequest{}, false
		}
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body must be valid integration JSON")
		return IntegrationRequest{}, false
	}
	return request, true
}

func normalizeIntegrationRequest(w http.ResponseWriter, request IntegrationRequest) (IntegrationRequest, bool) {
	request.Name = strings.TrimSpace(request.Name)
	request.Type = strings.ToLower(strings.TrimSpace(request.Type))
	request.Provider = strings.ToLower(strings.TrimSpace(request.Provider))
	request.AccountID = strings.TrimSpace(request.AccountID)
	request.Region = strings.TrimSpace(request.Region)
	request.SecretRef = strings.TrimSpace(request.SecretRef)
	if request.Name == "" || request.Provider == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "name and provider are required")
		return IntegrationRequest{}, false
	}
	switch request.Type {
	case "cloud", "billing", "notification":
	default:
		writeProblem(w, http.StatusBadRequest, "invalid_request", "type must be one of cloud, billing, notification")
		return IntegrationRequest{}, false
	}
	if len(request.Config) > 0 && !json.Valid(request.Config) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "config must be valid JSON")
		return IntegrationRequest{}, false
	}
	request.Config = cloneRawMessage(request.Config)
	return request, true
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
