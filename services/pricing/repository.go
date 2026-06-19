package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/shopspring/decimal"
)

var ErrEffectivePriceNotFound = errors.New("effective price not found")

type Repository interface {
	ImportCatalogPrices(context.Context, []CatalogPrice) error
	ImportBillingCharges(context.Context, []BillingCharge) error
	EffectiveCatalogPrice(context.Context, EffectivePriceQuery) (EffectivePriceResult, error)
	Ping(context.Context) error
	Close() error
}

type ClickHouseConfig struct {
	Address      string
	Database     string
	Username     string
	Password     string
	Secure       bool
	DialTimeout  time.Duration
	MaxOpenConns int
	MaxIdleConns int
}

type ClickHouseRepository struct {
	connection clickhouse.Conn
}

func OpenRepository(config ClickHouseConfig) (*ClickHouseRepository, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("ClickHouse address is required")
	}
	if config.Database == "" {
		config.Database = "kube_cost"
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = 5 * time.Second
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 10
	}
	if config.MaxIdleConns <= 0 {
		config.MaxIdleConns = 5
	}
	options := &clickhouse.Options{
		Addr: []string{config.Address},
		Auth: clickhouse.Auth{
			Database: config.Database,
			Username: config.Username,
			Password: config.Password,
		},
		DialTimeout:     config.DialTimeout,
		MaxOpenConns:    config.MaxOpenConns,
		MaxIdleConns:    config.MaxIdleConns,
		ConnMaxLifetime: time.Hour,
		Compression:     &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	}
	if config.Secure {
		options.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	connection, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("open ClickHouse connection: %w", err)
	}
	return &ClickHouseRepository{connection: connection}, nil
}

func (r *ClickHouseRepository) ImportCatalogPrices(ctx context.Context, prices []CatalogPrice) error {
	if len(prices) == 0 {
		return nil
	}
	batch, err := r.connection.PrepareBatch(ctx, "INSERT INTO kube_cost.catalog_price_interval ("+joinColumns(catalogPriceColumns)+")")
	if err != nil {
		return fmt.Errorf("prepare catalog price batch: %w", err)
	}
	for _, price := range prices {
		row, err := catalogPriceRow(price)
		if err != nil {
			return err
		}
		if err := batch.Append(row...); err != nil {
			return fmt.Errorf("append catalog price row: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send catalog price batch: %w", err)
	}
	return nil
}

func (r *ClickHouseRepository) ImportBillingCharges(ctx context.Context, charges []BillingCharge) error {
	if len(charges) == 0 {
		return nil
	}
	batch, err := r.connection.PrepareBatch(ctx, "INSERT INTO kube_cost.billing_charge ("+joinColumns(billingChargeColumns)+")")
	if err != nil {
		return fmt.Errorf("prepare billing charge batch: %w", err)
	}
	for _, charge := range charges {
		row, err := billingChargeRow(charge)
		if err != nil {
			return err
		}
		if err := batch.Append(row...); err != nil {
			return fmt.Errorf("append billing charge row: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send billing charge batch: %w", err)
	}
	return nil
}

func (r *ClickHouseRepository) EffectiveCatalogPrice(ctx context.Context, query EffectivePriceQuery) (EffectivePriceResult, error) {
	sql, args := effectiveCatalogPriceSQL(query)
	rows, err := r.connection.Query(ctx, sql, args...)
	if err != nil {
		return EffectivePriceResult{}, fmt.Errorf("query effective catalog price: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return EffectivePriceResult{}, fmt.Errorf("read effective catalog price: %w", err)
		}
		return EffectivePriceResult{}, ErrEffectivePriceNotFound
	}
	var result EffectivePriceResult
	var unitPrice decimal.Decimal
	var attributes string
	if err := rows.Scan(
		&result.TenantID,
		&result.Provider,
		&result.AccountID,
		&result.Region,
		&result.Service,
		&result.SKU,
		&result.ResourceType,
		&result.PurchaseOption,
		&result.Unit,
		&result.Currency,
		&unitPrice,
		&result.EffectiveStart,
		&result.EffectiveEnd,
		&result.Source,
		&result.PriceVersion,
		&attributes,
	); err != nil {
		return EffectivePriceResult{}, fmt.Errorf("scan effective catalog price: %w", err)
	}
	if strings.TrimSpace(attributes) != "" {
		_ = json.Unmarshal([]byte(attributes), &result.Attributes)
	}
	result.UnitPrice = unitPrice.String()
	result.MatchedAt = query.At
	return result, nil
}

func (r *ClickHouseRepository) Ping(ctx context.Context) error {
	return r.connection.Ping(ctx)
}

func (r *ClickHouseRepository) Close() error {
	return r.connection.Close()
}

func catalogPriceRow(price CatalogPrice) ([]any, error) {
	unitPrice, err := decimal.NewFromString(price.UnitPrice)
	if err != nil {
		return nil, fmt.Errorf("invalid unit_price: %w", err)
	}
	attributes, err := json.Marshal(price.Attributes)
	if err != nil {
		return nil, fmt.Errorf("encode catalog price attributes: %w", err)
	}
	return []any{
		price.TenantID, price.Provider, price.AccountID, price.Region, price.Service, price.SKU,
		price.ResourceType, price.PurchaseOption, price.Unit, price.Currency, unitPrice,
		price.EffectiveStart, price.EffectiveEnd, price.Source, price.PriceVersion,
		string(attributes), price.IngestedAt, price.Version,
	}, nil
}

func billingChargeRow(charge BillingCharge) ([]any, error) {
	listCost, err := decimal.NewFromString(charge.ListCost)
	if err != nil {
		return nil, fmt.Errorf("invalid list_cost: %w", err)
	}
	netCost, err := decimal.NewFromString(charge.NetCost)
	if err != nil {
		return nil, fmt.Errorf("invalid net_cost: %w", err)
	}
	amortizedCost, err := decimal.NewFromString(charge.AmortizedCost)
	if err != nil {
		return nil, fmt.Errorf("invalid amortized_cost: %w", err)
	}
	invoicedCost, err := decimal.NewFromString(charge.InvoicedCost)
	if err != nil {
		return nil, fmt.Errorf("invalid invoiced_cost: %w", err)
	}
	credits, err := decimal.NewFromString(charge.Credits)
	if err != nil {
		return nil, fmt.Errorf("invalid credits: %w", err)
	}
	taxes, err := decimal.NewFromString(charge.Taxes)
	if err != nil {
		return nil, fmt.Errorf("invalid taxes: %w", err)
	}
	attributes, err := json.Marshal(charge.Attributes)
	if err != nil {
		return nil, fmt.Errorf("encode billing charge attributes: %w", err)
	}
	return []any{
		charge.TenantID, charge.ChargeID, charge.Provider, charge.AccountID,
		charge.BillingPeriodStart, charge.BillingPeriodEnd, charge.UsageStart, charge.UsageEnd,
		charge.Service, charge.SKU, charge.ResourceID, charge.CostCategory, charge.Currency,
		listCost, netCost, amortizedCost, invoicedCost, credits, taxes,
		charge.InvoiceID, charge.Source, string(attributes), charge.IngestedAt, charge.Version,
	}, nil
}

func effectiveCatalogPriceSQL(query EffectivePriceQuery) (string, []any) {
	sql := `
SELECT
    tenant_id,
    provider,
    account_id,
    region,
    service,
    sku,
    resource_type,
    purchase_option,
    unit,
    currency,
    unit_price,
    effective_start,
    effective_end,
    source,
    price_version,
    attributes
FROM kube_cost.catalog_price_interval FINAL
WHERE tenant_id = ?
  AND provider = ?
  AND region = ?
  AND service = ?
  AND resource_type = ?
  AND purchase_option = ?
  AND unit = ?
  AND effective_start <= ?
  AND (effective_end IS NULL OR effective_end > ?)
  AND (account_id = ? OR account_id = '')
  AND (sku = ? OR sku = '')
ORDER BY
    if(account_id = ?, 1, 0) + if(sku = ?, 1, 0) DESC,
    effective_start DESC,
    version DESC
LIMIT 1`
	return sql, []any{
		query.TenantID,
		query.Provider,
		query.Region,
		query.Service,
		query.ResourceType,
		query.PurchaseOption,
		query.Unit,
		query.At,
		query.At,
		query.AccountID,
		query.SKU,
		query.AccountID,
		query.SKU,
	}
}

func joinColumns(columns []string) string {
	return strings.Join(columns, ", ")
}

var catalogPriceColumns = []string{
	"tenant_id", "provider", "account_id", "region", "service", "sku", "resource_type",
	"purchase_option", "unit", "currency", "unit_price", "effective_start", "effective_end",
	"source", "price_version", "attributes", "ingested_at", "version",
}

var billingChargeColumns = []string{
	"tenant_id", "charge_id", "provider", "account_id", "billing_period_start",
	"billing_period_end", "usage_start", "usage_end", "service", "sku", "resource_id",
	"cost_category", "currency", "list_cost", "net_cost", "amortized_cost", "invoiced_cost",
	"credits", "taxes", "invoice_id", "source", "attributes", "ingested_at", "version",
}
