package main

import (
	"strings"
	"testing"
	"time"
)

func TestCatalogPriceRowMatchesColumns(t *testing.T) {
	t.Parallel()
	row, err := catalogPriceRow(CatalogPrice{
		CatalogPriceInput: CatalogPriceInput{
			Provider:       "aws",
			Region:         "us-east-1",
			Service:        "EC2",
			SKU:            "m7g.large",
			ResourceType:   "instance",
			PurchaseOption: "on_demand",
			Unit:           "hour",
			Currency:       "USD",
			UnitPrice:      "0.077",
			EffectiveStart: fixedNow(),
			Source:         "import",
			PriceVersion:   "v1",
		},
		TenantID:   "tenant-a",
		IngestedAt: fixedNow(),
		Version:    uint64(fixedNow().UnixNano()),
	})
	if err != nil {
		t.Fatalf("catalogPriceRow returned error: %v", err)
	}
	if len(row) != len(catalogPriceColumns) {
		t.Fatalf("row len=%d columns=%d", len(row), len(catalogPriceColumns))
	}
}

func TestBillingChargeRowMatchesColumns(t *testing.T) {
	t.Parallel()
	row, err := billingChargeRow(BillingCharge{
		BillingChargeInput: BillingChargeInput{
			ChargeID:           "charge-1",
			Provider:           "aws",
			AccountID:          "123",
			BillingPeriodStart: fixedNow(),
			BillingPeriodEnd:   fixedNow().Add(30 * 24 * time.Hour),
			UsageStart:         fixedNow(),
			UsageEnd:           fixedNow().Add(time.Hour),
			Service:            "EC2",
			SKU:                "m7g.large",
			ResourceID:         "i-123",
			CostCategory:       "compute",
			Currency:           "USD",
			ListCost:           "1.00",
			NetCost:            "0.90",
			AmortizedCost:      "0.95",
			InvoicedCost:       "0.90",
			Credits:            "0.10",
			Taxes:              "0.00",
			InvoiceID:          "invoice-1",
			Source:             "import",
		},
		TenantID:   "tenant-a",
		IngestedAt: fixedNow(),
		Version:    uint64(fixedNow().UnixNano()),
	})
	if err != nil {
		t.Fatalf("billingChargeRow returned error: %v", err)
	}
	if len(row) != len(billingChargeColumns) {
		t.Fatalf("row len=%d columns=%d", len(row), len(billingChargeColumns))
	}
}

func TestBillingChargeRowRejectsInvalidDecimal(t *testing.T) {
	t.Parallel()
	_, err := billingChargeRow(BillingCharge{
		BillingChargeInput: BillingChargeInput{
			ListCost:      "bad",
			NetCost:       "0",
			AmortizedCost: "0",
			InvoicedCost:  "0",
			Credits:       "0",
			Taxes:         "0",
		},
	})
	if err == nil {
		t.Fatal("billingChargeRow should reject invalid decimal")
	}
}

func TestEffectiveCatalogPriceSQLMatchesSpecificThenWildcardRows(t *testing.T) {
	t.Parallel()
	sql, args := effectiveCatalogPriceSQL(EffectivePriceQuery{
		TenantID:       "tenant-a",
		Provider:       "aws",
		AccountID:      "123",
		Region:         "us-east-1",
		Service:        "EC2",
		SKU:            "m7g.large",
		ResourceType:   "instance",
		PurchaseOption: "on_demand",
		Unit:           "hour",
		At:             time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
	})
	for _, fragment := range []string{
		"FROM kube_cost.catalog_price_interval FINAL",
		"tenant_id = ?",
		"effective_start <= ?",
		"(effective_end IS NULL OR effective_end > ?)",
		"(account_id = ? OR account_id = '')",
		"(sku = ? OR sku = '')",
		"if(account_id = ?, 1, 0) + if(sku = ?, 1, 0) DESC",
		"LIMIT 1",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL missing %q:\n%s", fragment, sql)
		}
	}
	if len(args) != 13 {
		t.Fatalf("args len=%d, want 13: %#v", len(args), args)
	}
}
