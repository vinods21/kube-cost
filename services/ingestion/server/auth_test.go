package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"testing"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func TestMTLSAuthenticatorBindsCertificateToHello(t *testing.T) {
	t.Parallel()
	identity, err := url.Parse("spiffe://kube-cost/tenant/tenant-a/cluster/cluster-a")
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{URIs: []*url.URL{identity}}
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tlsState(certificate)},
	})
	authenticator := MTLSAuthenticator{}

	if err := authenticator.Authenticate(ctx, &agentv1.AgentHello{
		TenantId: "tenant-a", ClusterId: "cluster-a",
	}); err != nil {
		t.Fatalf("authenticate matching identity: %v", err)
	}
	if err := authenticator.Authenticate(ctx, &agentv1.AgentHello{
		TenantId: "tenant-b", ClusterId: "cluster-a",
	}); err == nil {
		t.Fatal("authenticate mismatched tenant succeeded")
	}
}

func TestMTLSAuthenticatorRejectsUnverifiedPeer(t *testing.T) {
	t.Parallel()
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{},
	})
	if err := (MTLSAuthenticator{}).Authenticate(ctx, &agentv1.AgentHello{}); err == nil {
		t.Fatal("authenticate unverified peer succeeded")
	}
}

func tlsState(certificate *x509.Certificate) tls.ConnectionState {
	return tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{certificate},
		VerifiedChains:   [][]*x509.Certificate{{certificate}},
	}
}
