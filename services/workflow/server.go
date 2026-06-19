package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

type API struct {
	repository Repository
	now        func() time.Time
}

func NewAPI(repository Repository, now func() time.Time) *API {
	return &API{repository: repository, now: now}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/v1/recommendations/{recommendation_id}/approve", a.approve)
	mux.HandleFunc("POST /api/v1/recommendations/{recommendation_id}/reject", a.reject)
	mux.HandleFunc("POST /api/v1/recommendations/{recommendation_id}/suppress", a.suppress)
	mux.HandleFunc("POST /api/v1/recommendations/{recommendation_id}/execute", a.execute)
	return mux
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.repository.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "dependency_unavailable", "clickhouse unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) approve(w http.ResponseWriter, r *http.Request) {
	a.command(w, r, "approve", "approved")
}

func (a *API) reject(w http.ResponseWriter, r *http.Request) {
	a.command(w, r, "reject", "rejected")
}

func (a *API) suppress(w http.ResponseWriter, r *http.Request) {
	a.command(w, r, "suppress", "suppressed")
}

func (a *API) execute(w http.ResponseWriter, r *http.Request) {
	a.command(w, r, "request_execution", "executing")
}

func (a *API) command(w http.ResponseWriter, r *http.Request, action, nextStatus string) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	recommendationID := strings.TrimSpace(r.PathValue("recommendation_id"))
	if recommendationID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "recommendation_id is required")
		return
	}
	request, ok := decodeCommandRequest(w, r)
	if !ok {
		return
	}
	result, err := a.repository.ApplyCommand(r.Context(), WorkflowCommand{
		TenantID:         tenantID,
		RecommendationID: recommendationID,
		Action:           action,
		NextStatus:       nextStatus,
		ActorID:          request.ActorID,
		Reason:           request.Reason,
		ExpectedVersion:  request.ExpectedVersion,
		Details:          request.Details,
		OccurredAt:       a.now().UTC(),
	})
	if err != nil {
		writeCommandError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeCommandRequest(w http.ResponseWriter, r *http.Request) (CommandRequest, bool) {
	defer r.Body.Close()
	var request CommandRequest
	if r.Body == http.NoBody {
		return request, true
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return request, true
		}
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return CommandRequest{}, false
	}
	request.ActorID = strings.TrimSpace(request.ActorID)
	request.Reason = strings.TrimSpace(request.Reason)
	return request, true
}

func writeCommandError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRecommendationNotFound):
		writeProblem(w, http.StatusNotFound, "not_found", "recommendation not found")
	case errors.Is(err, ErrVersionConflict):
		writeProblem(w, http.StatusConflict, "version_conflict", "recommendation version does not match expected_version")
	case errors.Is(err, ErrInvalidTransition):
		writeProblem(w, http.StatusConflict, "invalid_transition", "recommendation status does not allow this transition")
	default:
		slog.Error("recommendation workflow command failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "command_failed", "recommendation workflow command failed")
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
	if expected == "" {
		return true
	}
	if r.Header.Get(gatewaySecretHeader) != expected {
		writeProblem(w, http.StatusForbidden, "forbidden", gatewaySecretHeader+" is required")
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
