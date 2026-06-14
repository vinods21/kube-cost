package server

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"strings"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

const identityTrustDomain = "kube-cost"

type Authenticator interface {
	Authenticate(context.Context, *agentv1.AgentHello) error
}

type MTLSAuthenticator struct{}

func (MTLSAuthenticator) Authenticate(ctx context.Context, hello *agentv1.AgentHello) error {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return errors.New("transport peer information is missing")
	}
	tlsInfo, ok := peerInfo.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.PeerCertificates) == 0 {
		return errors.New("verified client certificate is required")
	}
	tenantID, clusterID, err := certificateIdentity(tlsInfo.State.PeerCertificates[0])
	if err != nil {
		return err
	}
	if tenantID != hello.GetTenantId() || clusterID != hello.GetClusterId() {
		return fmt.Errorf("certificate identity does not match hello tenant and cluster")
	}
	return nil
}

type InsecureAuthenticator struct{}

func (InsecureAuthenticator) Authenticate(_ context.Context, hello *agentv1.AgentHello) error {
	if strings.TrimSpace(hello.GetTenantId()) == "" || strings.TrimSpace(hello.GetClusterId()) == "" {
		return errors.New("tenant_id and cluster_id are required")
	}
	return nil
}

func certificateIdentity(certificate *x509.Certificate) (string, string, error) {
	for _, uri := range certificate.URIs {
		tenantID, clusterID, ok := parseIdentityURI(uri)
		if ok {
			return tenantID, clusterID, nil
		}
	}
	return "", "", fmt.Errorf("client certificate must contain a spiffe://%s/tenant/{tenant}/cluster/{cluster} URI SAN", identityTrustDomain)
}

func parseIdentityURI(uri *url.URL) (string, string, bool) {
	if uri == nil || uri.Scheme != "spiffe" || uri.Host != identityTrustDomain {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(uri.EscapedPath(), "/"), "/")
	if len(parts) != 4 || parts[0] != "tenant" || parts[2] != "cluster" {
		return "", "", false
	}
	tenantID, err := url.PathUnescape(parts[1])
	if err != nil || tenantID == "" {
		return "", "", false
	}
	clusterID, err := url.PathUnescape(parts[3])
	if err != nil || clusterID == "" {
		return "", "", false
	}
	return tenantID, clusterID, true
}
