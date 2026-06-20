package main

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

const (
	tenantHeader        = "X-Kube-Cost-Tenant-ID"
	authorizationHeader = "Authorization"
	gatewaySecretHeader = "X-Kube-Cost-Gateway-Secret"
	defaultHTTPAddress  = ":8080"
	defaultGatewayID    = "gateway"
)

type Config struct {
	HTTPAddress         string
	TokenTenants        map[string]string
	QueryURL            *url.URL
	ClusterRegistryURL  *url.URL
	PricingURL          *url.URL
	WorkflowURL         *url.URL
	ExportURL           *url.URL
	BackendSharedSecret string
	BackendSigningKey   string
	GatewayIdentity     string
}

func ConfigFromEnv() (Config, error) {
	config := Config{
		HTTPAddress:         valueOrDefault(os.Getenv("GATEWAY_HTTP_ADDRESS"), defaultHTTPAddress),
		TokenTenants:        parseTokenTenants(os.Getenv("GATEWAY_TOKEN_TENANTS")),
		BackendSharedSecret: strings.TrimSpace(os.Getenv("GATEWAY_BACKEND_SHARED_SECRET")),
		BackendSigningKey:   strings.TrimSpace(os.Getenv("GATEWAY_BACKEND_SIGNING_KEY")),
		GatewayIdentity:     valueOrDefault(os.Getenv("GATEWAY_IDENTITY"), defaultGatewayID),
	}
	var err error
	if config.QueryURL, err = parseRequiredURL("QUERY_URL", os.Getenv("QUERY_URL")); err != nil {
		return Config{}, err
	}
	if config.ClusterRegistryURL, err = parseRequiredURL("CLUSTER_REGISTRY_URL", os.Getenv("CLUSTER_REGISTRY_URL")); err != nil {
		return Config{}, err
	}
	if config.PricingURL, err = parseRequiredURL("PRICING_URL", os.Getenv("PRICING_URL")); err != nil {
		return Config{}, err
	}
	if config.WorkflowURL, err = parseRequiredURL("WORKFLOW_URL", os.Getenv("WORKFLOW_URL")); err != nil {
		return Config{}, err
	}
	if config.ExportURL, err = parseRequiredURL("EXPORT_URL", os.Getenv("EXPORT_URL")); err != nil {
		return Config{}, err
	}
	if len(config.TokenTenants) == 0 {
		return Config{}, errors.New("GATEWAY_TOKEN_TENANTS must contain at least one token to tenant mapping")
	}
	return config, nil
}

func parseTokenTenants(value string) map[string]string {
	result := map[string]string{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		token, tenantID, ok := strings.Cut(item, ":")
		if !ok {
			token, tenantID, ok = strings.Cut(item, "=")
		}
		token = strings.TrimSpace(token)
		tenantID = strings.TrimSpace(tenantID)
		if ok && token != "" && tenantID != "" {
			result[token] = tenantID
		}
	}
	return result
}

func parseRequiredURL(name, value string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New(name + " is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New(name + " must be an absolute URL")
	}
	return parsed, nil
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
