package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/shopspring/decimal"
)

type Repository interface {
	ImportCatalogPrices(context.Context, []CatalogPrice) error
	ImportBillingCharges(context.Context, []BillingCharge) error
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
