package gatewayauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSignAndVerifyRequest(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage?cluster_id=a", nil)
	request.Header.Set(TenantHeader, "tenant-a")

	SignRequest(request, "public-gateway", "signing-key", now)

	if err := VerifyRequest(request, "signing-key", now.Add(time.Minute), 5*time.Minute); err != nil {
		t.Fatalf("VerifyRequest returned error: %v", err)
	}
}

func TestVerifyRejectsTamperedRequest(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil)
	request.Header.Set(TenantHeader, "tenant-a")
	SignRequest(request, "public-gateway", "signing-key", now)
	request.URL.RawQuery = "cluster_id=other"

	if err := VerifyRequest(request, "signing-key", now, 5*time.Minute); err != ErrInvalidSignature {
		t.Fatalf("VerifyRequest error = %v, want ErrInvalidSignature", err)
	}
}

func TestVerifyRejectsExpiredSignature(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil)
	request.Header.Set(TenantHeader, "tenant-a")
	SignRequest(request, "public-gateway", "signing-key", now)

	if err := VerifyRequest(request, "signing-key", now.Add(10*time.Minute), time.Minute); err != ErrExpiredSignature {
		t.Fatalf("VerifyRequest error = %v, want ErrExpiredSignature", err)
	}
}
