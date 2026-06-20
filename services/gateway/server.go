package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/kube-cost/kube-cost/internal/gatewayauth"
)

type Server struct {
	tokenTenants    map[string]string
	query           http.Handler
	clusterRegistry http.Handler
	pricing         http.Handler
	workflow        http.Handler
	export          http.Handler
	tenant          http.Handler
	audit           http.Handler
	backendSecret   string
	signingKey      string
	identity        string
	now             func() time.Time
}

func NewServer(config Config) (*Server, error) {
	if len(config.TokenTenants) == 0 {
		return nil, errors.New("token tenant mappings are required")
	}
	return &Server{
		tokenTenants:    config.TokenTenants,
		query:           proxy(config.QueryURL),
		clusterRegistry: proxy(config.ClusterRegistryURL),
		pricing:         proxy(config.PricingURL),
		workflow:        proxy(config.WorkflowURL),
		export:          proxy(config.ExportURL),
		tenant:          proxy(config.TenantURL),
		audit:           proxy(config.AuditURL),
		backendSecret:   config.BackendSharedSecret,
		signingKey:      config.BackendSigningKey,
		identity:        config.GatewayIdentity,
		now:             time.Now,
	}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("/", s.authenticate(http.HandlerFunc(s.route)))
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := s.tenantFromRequest(w, r)
		if !ok {
			return
		}
		r.Header.Del(tenantHeader)
		r.Header.Del(authorizationHeader)
		r.Header.Del(gatewaySecretHeader)
		r.Header.Set(tenantHeader, tenantID)
		if s.backendSecret != "" {
			r.Header.Set(gatewaySecretHeader, s.backendSecret)
		}
		gatewayauth.SignRequest(r, s.identity, s.signingKey, s.now().UTC())
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tenantFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	auth := strings.TrimSpace(r.Header.Get(authorizationHeader))
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Bearer token is required")
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	tenantID := s.tokenTenants[token]
	if tenantID == "" {
		writeProblem(w, http.StatusForbidden, "forbidden", "token is not mapped to a tenant")
		return "", false
	}
	return tenantID, true
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/v1/clusters" || strings.HasPrefix(path, "/api/v1/clusters/"):
		s.clusterRegistry.ServeHTTP(w, r)
	case path == "/api/v1/prices/catalog" || path == "/api/v1/prices/effective" || path == "/api/v1/billing/charges":
		s.pricing.ServeHTTP(w, r)
	case path == "/api/v1/recommendations" || isRecommendationRead(r):
		s.query.ServeHTTP(w, r)
	case isRecommendationCommand(r):
		s.workflow.ServeHTTP(w, r)
	case path == "/api/v1/data-quality" || path == "/api/v1/usage" || path == "/api/v1/costs" || path == "/api/v1/allocation":
		s.query.ServeHTTP(w, r)
	case path == "/api/v1/exports" || strings.HasPrefix(path, "/api/v1/exports/"):
		s.export.ServeHTTP(w, r)
	case path == "/api/v1/tenant" || strings.HasPrefix(path, "/api/v1/tenant/"):
		s.tenant.ServeHTTP(w, r)
	case path == "/api/v1/audit/events":
		s.audit.ServeHTTP(w, r)
	default:
		writeProblem(w, http.StatusNotFound, "not_found", "route not found")
	}
}

func isRecommendationRead(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/recommendations/")
}

func isRecommendationCommand(r *http.Request) bool {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/v1/recommendations/") {
		return false
	}
	action := recommendationAction(r.URL.Path)
	switch action {
	case "approve", "reject", "suppress", "execute":
		return true
	default:
		return false
	}
}

func recommendationAction(path string) string {
	path = strings.TrimSuffix(path, "/")
	index := strings.LastIndex(path, "/")
	if index == -1 {
		return ""
	}
	return path[index+1:]
}

func proxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		originalDirector(request)
		request.Host = target.Host
	}
	return proxy
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
