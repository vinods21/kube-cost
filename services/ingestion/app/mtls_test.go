package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	"github.com/kube-cost/kube-cost/services/ingestion/queue"
	ingestion "github.com/kube-cost/kube-cost/services/ingestion/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/test/bufconn"
)

func TestMTLSStreamAcceptsBoundAgentIdentity(t *testing.T) {
	t.Parallel()
	ca, caKey := createCertificateAuthority(t)
	serverCertificate := createCertificate(t, ca, caKey, certificateOptions{
		commonName: "ingestion.test",
		dnsNames:   []string{"ingestion.test"},
		usage:      x509.ExtKeyUsageServerAuth,
	})
	identity, err := url.Parse("spiffe://kube-cost/tenant/tenant-a/cluster/cluster-a")
	if err != nil {
		t.Fatal(err)
	}
	clientCertificate := createCertificate(t, ca, caKey, certificateOptions{
		commonName: "agent-a",
		uris:       []*url.URL{identity},
		usage:      x509.ExtKeyUsageClientAuth,
	})

	directory := t.TempDir()
	serverCertFile, serverKeyFile := writeCertificate(t, directory, "server", serverCertificate)
	caFile := filepath.Join(directory, "ca.crt")
	if err := os.WriteFile(caFile, ca.certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	serverTransport, err := serverCredentials(Config{
		TLSCertFile:  serverCertFile,
		TLSKeyFile:   serverKeyFile,
		ClientCAFile: caFile,
	})
	if err != nil {
		t.Fatalf("server credentials: %v", err)
	}

	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer(grpc.Creds(serverTransport))
	agentv1.RegisterAgentIngestionServiceServer(grpcServer, ingestion.New(
		ingestion.Config{MaxBatchRecords: 10, MaxBatchBytes: 1 << 20},
		queue.New(10),
		ingestion.MTLSAuthenticator{},
	))
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(grpcServer.Stop)

	roots := x509.NewCertPool()
	roots.AddCert(ca.certificate)
	clientTLS, err := tls.X509KeyPair(clientCertificate.certPEM, clientCertificate.keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := grpc.NewClient("passthrough:///ingestion.test",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			MinVersion:   tls.VersionTLS12,
			RootCAs:      roots,
			Certificates: []tls.Certificate{clientTLS},
			ServerName:   "ingestion.test",
		})),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := agentv1.NewAgentIngestionServiceClient(connection).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&agentv1.AgentToIngestion{
		Frame: &agentv1.AgentToIngestion_Hello{Hello: &agentv1.AgentHello{
			TenantId:        "tenant-a",
			ClusterId:       "cluster-a",
			AgentInstanceId: "agent-a",
			SupportedProtocolVersions: []*agentv1.ProtocolVersion{
				{Major: 1},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv()
	if err != nil {
		t.Fatalf("receive server hello: %v", err)
	}
	if frame.GetHello() == nil {
		t.Fatalf("first server frame = %#v, want ServerHello", frame)
	}
}

type testCA struct {
	certificate *x509.Certificate
	certPEM     []byte
}

type testCertificate struct {
	certPEM []byte
	keyPEM  []byte
}

type certificateOptions struct {
	commonName string
	dnsNames   []string
	uris       []*url.URL
	usage      x509.ExtKeyUsage
}

func createCertificateAuthority(t *testing.T) (testCA, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testCA{
		certificate: certificate,
		certPEM:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}, key
}

func createCertificate(t *testing.T, ca testCA, caKey *rsa.PrivateKey, options certificateOptions) testCertificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: options.commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{options.usage},
		DNSNames:     options.dnsNames,
		URIs:         options.uris,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.certificate, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return testCertificate{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM: pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		}),
	}
}

func writeCertificate(t *testing.T, directory, name string, certificate testCertificate) (string, string) {
	t.Helper()
	certFile := filepath.Join(directory, name+".crt")
	keyFile := filepath.Join(directory, name+".key")
	if err := os.WriteFile(certFile, certificate.certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, certificate.keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
