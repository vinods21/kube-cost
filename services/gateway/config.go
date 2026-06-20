package main

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

const (
	tenantHeader        = "X-Kube-Cost-Tenant-ID"
	principalHeader     = "X-Kube-Cost-Principal-ID"
	authorizationHeader = "Authorization"
	gatewaySecretHeader = "X-Kube-Cost-Gateway-Secret"
	defaultHTTPAddress  = ":8080"
	defaultGatewayID    = "gateway"
)

type Config struct {
	HTTPAddress         string
	TokenTenants        map[string]string
	TokenPrincipals     map[string]string
	QueryURL            *url.URL
	ClusterRegistryURL  *url.URL
	PricingURL          *url.URL
	WorkflowURL         *url.URL
	ExportURL           *url.URL
	TenantURL           *url.URL
	AuditURL            *url.URL
	IdentityURL         *url.URL
	PolicyURL           *url.URL
	IntegrationsURL     *url.URL
	BackendSharedSecret string
	BackendSigningKey   string
	GatewayIdentity     string
}

func ConfigFromEnv() (Config, error) {
	config := Config{
		HTTPAddress:         valueOrDefault(os.Getenv("GATEWAY_HTTP_ADDRESS"), defaultHTTPAddress),
		TokenTenants:        parseTokenTenants(os.Getenv("GATEWAY_TOKEN_TENANTS")),
		TokenPrincipals:     parseTokenTenants(os.Getenv("GATEWAY_TOKEN_PRINCIPALS")),
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
	if config.TenantURL, err = parseRequiredURL("TENANT_URL", os.Getenv("TENANT_URL")); err != nil {
		return Config{}, err
	}
	if config.AuditURL, err = parseRequiredURL("AUDIT_URL", os.Getenv("AUDIT_URL")); err != nil {
		return Config{}, err
	}
	if config.IdentityURL, err = parseRequiredURL("IDENTITY_URL", os.Getenv("IDENTITY_URL")); err != nil {
		return Config{}, err
	}
	if config.PolicyURL, err = parseRequiredURL("POLICY_URL", os.Getenv("POLICY_URL")); err != nil {
		return Config{}, err
	}
	if config.IntegrationsURL, err = parseRequiredURL("INTEGRATIONS_URL", os.Getenv("INTEGRATIONS_URL")); err != nil {
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
