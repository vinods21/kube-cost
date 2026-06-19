package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const maxImportBatch = 1000

type API struct {
	repository Repository
	now        func() time.Time
}

func NewAPI(repository Repository, now func() time.Time) *API {
	return &API{repository: repository, now: now}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/v1/prices/catalog", a.importCatalogPrices)
	mux.HandleFunc("POST /api/v1/billing/charges", a.importBillingCharges)
	return mux
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.repository.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "dependency_unavailable", "clickhouse unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) importCatalogPrices(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	var request CatalogImportRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if len(request.Prices) == 0 || len(request.Prices) > maxImportBatch {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "prices must contain 1 to 1000 items")
		return
	}
	ingestedAt := a.now().UTC()
	prices := make([]CatalogPrice, 0, len(request.Prices))
	for _, input := range request.Prices {
		if err := validateCatalogPrice(input); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		prices = append(prices, CatalogPrice{
			CatalogPriceInput: normalizeCatalogPrice(input),
			TenantID:          tenantID,
			IngestedAt:        ingestedAt,
			Version:           uint64(ingestedAt.UnixNano()),
		})
	}
	if err := a.repository.ImportCatalogPrices(r.Context(), prices); err != nil {
		slog.Error("catalog price import failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "import_failed", "catalog price import failed")
		return
	}
	writeJSON(w, http.StatusAccepted, ImportResponse{TenantID: tenantID, Imported: len(prices), IngestedAt: ingestedAt})
}

func (a *API) importBillingCharges(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := authenticatedTenant(w, r)
	if !ok {
		return
	}
	var request BillingImportRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if len(request.Charges) == 0 || len(request.Charges) > maxImportBatch {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "charges must contain 1 to 1000 items")
		return
	}
	ingestedAt := a.now().UTC()
	charges := make([]BillingCharge, 0, len(request.Charges))
	for _, input := range request.Charges {
		if err := validateBillingCharge(input); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		charges = append(charges, BillingCharge{
			BillingChargeInput: normalizeBillingCharge(input),
			TenantID:           tenantID,
			IngestedAt:         ingestedAt,
			Version:            uint64(ingestedAt.UnixNano()),
		})
	}
	if err := a.repository.ImportBillingCharges(r.Context(), charges); err != nil {
		slog.Error("billing charge import failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "import_failed", "billing charge import failed")
		return
	}
	writeJSON(w, http.StatusAccepted, ImportResponse{TenantID: tenantID, Imported: len(charges), IngestedAt: ingestedAt})
}

func validateCatalogPrice(price CatalogPriceInput) error {
	for name, value := range map[string]string{
		"provider":      price.Provider,
		"region":        price.Region,
		"service":       price.Service,
		"sku":           price.SKU,
		"resource_type": price.ResourceType,
		"unit":          price.Unit,
		"currency":      price.Currency,
		"unit_price":    price.UnitPrice,
		"price_version": price.PriceVersion,
	} {
		if strings.TrimSpace(value) == "" {
			return fieldError(name + " is required")
		}
	}
	if len(strings.TrimSpace(price.Currency)) != 3 {
		return fieldError("currency must be a 3-letter code")
	}
	if _, err := decimal.NewFromString(price.UnitPrice); err != nil {
		return fieldError("unit_price must be a decimal string")
	}
	if price.EffectiveStart.IsZero() {
		return fieldError("effective_start is required")
	}
	if price.EffectiveEnd != nil && !price.EffectiveEnd.After(price.EffectiveStart) {
		return fieldError("effective_end must be after effective_start")
	}
	return nil
}

func validateBillingCharge(charge BillingChargeInput) error {
	for name, value := range map[string]string{
		"charge_id":      charge.ChargeID,
		"provider":       charge.Provider,
		"account_id":     charge.AccountID,
		"service":        charge.Service,
		"cost_category":  charge.CostCategory,
		"currency":       charge.Currency,
		"list_cost":      charge.ListCost,
		"net_cost":       charge.NetCost,
		"amortized_cost": charge.AmortizedCost,
		"invoiced_cost":  charge.InvoicedCost,
		"credits":        charge.Credits,
		"taxes":          charge.Taxes,
		"invoice_id":     charge.InvoiceID,
	} {
		if strings.TrimSpace(value) == "" {
			return fieldError(name + " is required")
		}
	}
	if len(strings.TrimSpace(charge.Currency)) != 3 {
		return fieldError("currency must be a 3-letter code")
	}
	for name, value := range map[string]string{
		"list_cost": charge.ListCost, "net_cost": charge.NetCost, "amortized_cost": charge.AmortizedCost,
		"invoiced_cost": charge.InvoicedCost, "credits": charge.Credits, "taxes": charge.Taxes,
	} {
		if _, err := decimal.NewFromString(value); err != nil {
			return fieldError(name + " must be a decimal string")
		}
	}
	if charge.BillingPeriodStart.IsZero() || charge.BillingPeriodEnd.IsZero() || !charge.BillingPeriodEnd.After(charge.BillingPeriodStart) {
		return fieldError("billing period timestamps are required and must be ordered")
	}
	if charge.UsageStart.IsZero() || charge.UsageEnd.IsZero() || !charge.UsageEnd.After(charge.UsageStart) {
		return fieldError("usage timestamps are required and must be ordered")
	}
	return nil
}

func normalizeCatalogPrice(price CatalogPriceInput) CatalogPriceInput {
	price.Provider = strings.ToLower(strings.TrimSpace(price.Provider))
	price.AccountID = strings.TrimSpace(price.AccountID)
	price.Region = strings.TrimSpace(price.Region)
	price.Service = strings.TrimSpace(price.Service)
	price.SKU = strings.TrimSpace(price.SKU)
	price.ResourceType = strings.TrimSpace(price.ResourceType)
	price.PurchaseOption = valueOrDefaultString(price.PurchaseOption, "on_demand")
	price.Unit = strings.TrimSpace(price.Unit)
	price.Currency = strings.ToUpper(strings.TrimSpace(price.Currency))
	price.UnitPrice = strings.TrimSpace(price.UnitPrice)
	price.Source = valueOrDefaultString(price.Source, "import")
	price.PriceVersion = strings.TrimSpace(price.PriceVersion)
	price.EffectiveStart = price.EffectiveStart.UTC()
	if price.EffectiveEnd != nil {
		end := price.EffectiveEnd.UTC()
		price.EffectiveEnd = &end
	}
	return price
}

func normalizeBillingCharge(charge BillingChargeInput) BillingChargeInput {
	charge.Provider = strings.ToLower(strings.TrimSpace(charge.Provider))
	charge.AccountID = strings.TrimSpace(charge.AccountID)
	charge.Service = strings.TrimSpace(charge.Service)
	charge.SKU = strings.TrimSpace(charge.SKU)
	charge.ResourceID = strings.TrimSpace(charge.ResourceID)
	charge.CostCategory = strings.TrimSpace(charge.CostCategory)
	charge.Currency = strings.ToUpper(strings.TrimSpace(charge.Currency))
	charge.Source = valueOrDefaultString(charge.Source, "import")
	charge.BillingPeriodStart = charge.BillingPeriodStart.UTC()
	charge.BillingPeriodEnd = charge.BillingPeriodEnd.UTC()
	charge.UsageStart = charge.UsageStart.UTC()
	charge.UsageEnd = charge.UsageEnd.UTC()
	return charge
}

func valueOrDefaultString(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

type fieldError string

func (e fieldError) Error() string { return string(e) }

func authenticatedTenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	tenantID := strings.TrimSpace(r.Header.Get(tenantHeader))
	if tenantID == "" {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", tenantHeader+" is required")
		return "", false
	}
	return tenantID, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return false
	}
	return true
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
