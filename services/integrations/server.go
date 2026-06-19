package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type API struct {
	reader KarpenterReader
	scorer Scorer
}

func NewAPI(reader KarpenterReader, scorer Scorer) *API {
	return &API{reader: reader, scorer: scorer}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/karpenter/snapshot", a.snapshot)
	mux.HandleFunc("GET /api/v1/karpenter/scores", a.scores)
	return mux
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) snapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.reader.Snapshot(r.Context())
	if err != nil {
		slog.Error("read Karpenter snapshot failed", "error", err)
		writeError(w, http.StatusInternalServerError, "read Karpenter snapshot failed")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (a *API) scores(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.reader.Snapshot(r.Context())
	if err != nil {
		slog.Error("read Karpenter snapshot failed", "error", err)
		writeError(w, http.StatusInternalServerError, "read Karpenter snapshot failed")
		return
	}
	writeJSON(w, http.StatusOK, a.scorer.Score(snapshot))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
