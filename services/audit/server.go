package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
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
	mux.HandleFunc("POST /api/v1/audit/events", a.appendEvent)
	mux.HandleFunc("GET /api/v1/audit/events", a.listEvents)
	return mux
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) appendEvent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	request, ok := decodeEventRequest(w, r)
	if !ok {
		return
	}
	event, ok := normalizeEvent(w, request)
	if !ok {
		return
	}
	writeJSON(w, http.StatusAccepted, a.store.Append(tenantID, event, a.now().UTC()))
}

func (a *API) listEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	filter, ok := eventFilter(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, EventsResult{
		TenantID: tenantID,
		Events:   a.store.List(tenantID, filter),
		Limit:    filter.Limit,
	})
}

func decodeEventRequest(w http.ResponseWriter, r *http.Request) (EventRequest, bool) {
	defer r.Body.Close()
	var request EventRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "request body is required")
			return EventRequest{}, false
		}
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body must be valid audit event JSON")
		return EventRequest{}, false
	}
	return request, true
}

func normalizeEvent(w http.ResponseWriter, request EventRequest) (EventRequest, bool) {
	request.ActorID = strings.TrimSpace(request.ActorID)
	request.Action = strings.TrimSpace(request.Action)
	request.ResourceType = strings.TrimSpace(request.ResourceType)
	request.ResourceID = strings.TrimSpace(request.ResourceID)
	request.Outcome = strings.ToLower(strings.TrimSpace(request.Outcome))
	if request.ActorID == "" || request.Action == "" || request.ResourceType == "" || request.ResourceID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "actor_id, action, resource_type, and resource_id are required")
		return EventRequest{}, false
	}
	switch request.Outcome {
	case "succeeded", "failed", "denied":
	default:
		writeProblem(w, http.StatusBadRequest, "invalid_request", "outcome must be one of succeeded, failed, denied")
		return EventRequest{}, false
	}
	if len(request.Details) > 0 && !json.Valid(request.Details) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "details must be valid JSON")
		return EventRequest{}, false
	}
	request.Details = cloneRawMessage(request.Details)
	return request, true
}

func eventFilter(w http.ResponseWriter, r *http.Request) (EventFilter, bool) {
	limit := 100
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return EventFilter{}, false
		}
		limit = parsed
	}
	if limit > 500 {
		limit = 500
	}
	return EventFilter{
		ActorID:      strings.TrimSpace(r.URL.Query().Get("actor_id")),
		ResourceType: strings.TrimSpace(r.URL.Query().Get("resource_type")),
		ResourceID:   strings.TrimSpace(r.URL.Query().Get("resource_id")),
		Limit:        limit,
	}, true
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
