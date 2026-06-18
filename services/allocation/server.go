package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type API struct {
	engine *Engine
}

func NewAPI(engine *Engine) *API {
	return &API{engine: engine}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/namespaces/cost", a.namespaceCost)
	return mux
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.engine.repository.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "clickhouse unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) namespaceCost(w http.ResponseWriter, r *http.Request) {
	start, err := parseTime(r.URL.Query().Get("start"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	end, err := parseTime(r.URL.Query().Get("end"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.engine.NamespaceCosts(r.Context(), Query{
		TenantID:  r.URL.Query().Get("tenant_id"),
		ClusterID: r.URL.Query().Get("cluster_id"),
		Start:     start,
		End:       end,
	})
	if err != nil {
		if errors.Is(err, ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("namespace allocation query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "namespace allocation query failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
