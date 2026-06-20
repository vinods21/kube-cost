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
	mux.HandleFunc("POST /api/v1/exports", a.createExport)
	mux.HandleFunc("GET /api/v1/exports/{export_id}", a.getExport)
	return mux
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) createExport(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	request, ok := decodeExportRequest(w, r)
	if !ok {
		return
	}
	spec, ok := exportSpec(w, request)
	if !ok {
		return
	}
	job, err := a.store.Create(tenantID, spec, a.now().UTC())
	if err != nil {
		slog.Error("create export job failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "export_failed", "create export job failed")
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (a *API) getExport(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	exportID := strings.TrimSpace(r.PathValue("export_id"))
	if exportID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "export_id is required")
		return
	}
	job, ok := a.store.Get(tenantID, exportID)
	if !ok {
		writeProblem(w, http.StatusNotFound, "not_found", "export job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func decodeExportRequest(w http.ResponseWriter, r *http.Request) (ExportRequest, bool) {
	defer r.Body.Close()
	var request ExportRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "request body is required")
			return ExportRequest{}, false
		}
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body must be valid export JSON")
		return ExportRequest{}, false
	}
	return request, true
}

func exportSpec(w http.ResponseWriter, request ExportRequest) (ExportSpec, bool) {
	queryType := strings.TrimSpace(request.QueryType)
	switch queryType {
	case "usage", "costs", "allocation":
	default:
		writeProblem(w, http.StatusBadRequest, "invalid_request", "query_type must be one of usage, costs, allocation")
		return ExportSpec{}, false
	}
	format := strings.ToLower(strings.TrimSpace(request.Format))
	if format == "" {
		format = "json"
	}
	switch format {
	case "json", "csv", "parquet":
	default:
		writeProblem(w, http.StatusBadRequest, "invalid_request", "format must be one of json, csv, parquet")
		return ExportSpec{}, false
	}
	start, err := parseRequiredTime(request.Start, "start")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return ExportSpec{}, false
	}
	end, err := parseRequiredTime(request.End, "end")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return ExportSpec{}, false
	}
	if !start.Before(end) {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "start must be before end")
		return ExportSpec{}, false
	}
	groupBy := strings.TrimSpace(request.GroupBy)
	if groupBy == "" {
		groupBy = "namespace"
	}
	return ExportSpec{
		QueryType: queryType,
		Format:    format,
		ClusterID: strings.TrimSpace(request.ClusterID),
		Start:     start,
		End:       end,
		GroupBy:   groupBy,
	}, true
}

func parseRequiredTime(value, name string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New(name + " is required")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New(name + " must be an RFC3339 timestamp")
	}
	return parsed.UTC(), nil
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
