package main

import "time"

const tenantHeader = "X-Kube-Cost-Tenant-ID"

type CatalogPriceInput struct {
	Provider       string         `json:"provider"`
	AccountID      string         `json:"account_id"`
	Region         string         `json:"region"`
	Service        string         `json:"service"`
	SKU            string         `json:"sku"`
	ResourceType   string         `json:"resource_type"`
	PurchaseOption string         `json:"purchase_option"`
	Unit           string         `json:"unit"`
	Currency       string         `json:"currency"`
	UnitPrice      string         `json:"unit_price"`
	EffectiveStart time.Time      `json:"effective_start"`
	EffectiveEnd   *time.Time     `json:"effective_end,omitempty"`
	Source         string         `json:"source"`
	PriceVersion   string         `json:"price_version"`
	Attributes     map[string]any `json:"attributes,omitempty"`
}

type BillingChargeInput struct {
	ChargeID           string         `json:"charge_id"`
	Provider           string         `json:"provider"`
	AccountID          string         `json:"account_id"`
	BillingPeriodStart time.Time      `json:"billing_period_start"`
	BillingPeriodEnd   time.Time      `json:"billing_period_end"`
	UsageStart         time.Time      `json:"usage_start"`
	UsageEnd           time.Time      `json:"usage_end"`
	Service            string         `json:"service"`
	SKU                string         `json:"sku"`
	ResourceID         string         `json:"resource_id"`
	CostCategory       string         `json:"cost_category"`
	Currency           string         `json:"currency"`
	ListCost           string         `json:"list_cost"`
	NetCost            string         `json:"net_cost"`
	AmortizedCost      string         `json:"amortized_cost"`
	InvoicedCost       string         `json:"invoiced_cost"`
	Credits            string         `json:"credits"`
	Taxes              string         `json:"taxes"`
	InvoiceID          string         `json:"invoice_id"`
	Source             string         `json:"source"`
	Attributes         map[string]any `json:"attributes,omitempty"`
}

type CatalogImportRequest struct {
	Prices []CatalogPriceInput `json:"prices"`
}

type BillingImportRequest struct {
	Charges []BillingChargeInput `json:"charges"`
}

type ImportResponse struct {
	TenantID   string    `json:"tenant_id"`
	Imported   int       `json:"imported"`
	IngestedAt time.Time `json:"ingested_at"`
}

type CatalogPrice struct {
	CatalogPriceInput
	TenantID   string
	IngestedAt time.Time
	Version    uint64
}

type BillingCharge struct {
	BillingChargeInput
	TenantID   string
	IngestedAt time.Time
	Version    uint64
}
