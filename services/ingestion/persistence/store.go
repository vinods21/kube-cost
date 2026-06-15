package persistence

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type Insert struct {
	Table   string
	Columns []string
	Rows    [][]any
}

type Store interface {
	Insert(context.Context, Insert) error
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

type ClickHouseStore struct {
	connection clickhouse.Conn
}

func OpenClickHouse(config ClickHouseConfig) (*ClickHouseStore, error) {
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
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	}
	if config.Secure {
		options.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	connection, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("open ClickHouse connection: %w", err)
	}
	return &ClickHouseStore{connection: connection}, nil
}

func (s *ClickHouseStore) Insert(ctx context.Context, insert Insert) error {
	if len(insert.Rows) == 0 {
		return nil
	}
	query := fmt.Sprintf(
		"INSERT INTO kube_cost.%s (%s)",
		insert.Table,
		joinColumns(insert.Columns),
	)
	batch, err := s.connection.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare %s batch: %w", insert.Table, err)
	}
	for _, row := range insert.Rows {
		if len(row) != len(insert.Columns) {
			return fmt.Errorf("%s row has %d values for %d columns", insert.Table, len(row), len(insert.Columns))
		}
		if err := batch.Append(row...); err != nil {
			return fmt.Errorf("append %s row: %w", insert.Table, err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send %s batch: %w", insert.Table, err)
	}
	return nil
}

func (s *ClickHouseStore) Ping(ctx context.Context) error {
	return s.connection.Ping(ctx)
}

func (s *ClickHouseStore) Close() error {
	return s.connection.Close()
}

func joinColumns(columns []string) string {
	result := ""
	for index, column := range columns {
		if index > 0 {
			result += ", "
		}
		result += column
	}
	return result
}
