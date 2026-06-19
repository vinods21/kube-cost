package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCatalogImportRequiresTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/prices/catalog", strings.NewReader(`{"prices":[]}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestCatalogImportPersistsTenantScopedPrices(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/prices/catalog", strings.NewReader(`{"prices":[{
		"provider":"AWS",
		"account_id":"123",
		"region":"us-east-1",
		"service":"EC2",
		"sku":"m7g.large",
		"resource_type":"instance",
		"unit":"hour",
		"currency":"usd",
		"unit_price":"0.077",
		"effective_start":"2026-06-01T00:00:00Z",
		"price_version":"aws-2026-06",
		"attributes":{"instance_family":"m7g"}
	}]}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(repository.prices) != 1 {
		t.Fatalf("prices len=%d", len(repository.prices))
	}
	price := repository.prices[0]
	if price.TenantID != "tenant-a" || price.Provider != "aws" || price.Currency != "USD" || price.PurchaseOption != "on_demand" {
		t.Fatalf("price = %#v", price)
	}
	var body ImportResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Imported != 1 || body.TenantID != "tenant-a" {
		t.Fatalf("body = %#v", body)
	}
}

func TestBillingImportPersistsTenantScopedCharges(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/billing/charges", strings.NewReader(`{"charges":[{
		"charge_id":"line-1",
		"provider":"aws",
		"account_id":"123",
		"billing_period_start":"2026-06-01T00:00:00Z",
		"billing_period_end":"2026-07-01T00:00:00Z",
		"usage_start":"2026-06-18T00:00:00Z",
		"usage_end":"2026-06-18T01:00:00Z",
		"service":"EC2",
		"sku":"m7g.large",
		"resource_id":"i-123",
		"cost_category":"compute",
		"currency":"USD",
		"list_cost":"1.00",
		"net_cost":"0.90",
		"amortized_cost":"0.95",
		"invoiced_cost":"0.90",
		"credits":"0.10",
		"taxes":"0.00",
		"invoice_id":"invoice-1"
	}]}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(repository.charges) != 1 {
		t.Fatalf("charges len=%d", len(repository.charges))
	}
	charge := repository.charges[0]
	if charge.TenantID != "tenant-a" || charge.Source != "import" || charge.NetCost != "0.90" {
		t.Fatalf("charge = %#v", charge)
	}
}

func TestCatalogImportRejectsInvalidDecimal(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/prices/catalog", strings.NewReader(`{"prices":[{
		"provider":"aws","region":"us-east-1","service":"EC2","sku":"sku","resource_type":"instance",
		"unit":"hour","currency":"USD","unit_price":"bad","effective_start":"2026-06-01T00:00:00Z","price_version":"v1"
	}]}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

type fakeRepository struct {
	prices  []CatalogPrice
	charges []BillingCharge
	pingErr error
}

func (r *fakeRepository) ImportCatalogPrices(_ context.Context, prices []CatalogPrice) error {
	r.prices = prices
	return nil
}

func (r *fakeRepository) ImportBillingCharges(_ context.Context, charges []BillingCharge) error {
	r.charges = charges
	return nil
}

func (r *fakeRepository) Ping(context.Context) error { return r.pingErr }

func (r *fakeRepository) Close() error { return nil }

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
