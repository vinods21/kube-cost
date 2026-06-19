package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

const versionBump uint64 = 1_000_000_000_000

var inventoryTables = []lineageTable{
	{
		Name: "deployment",
		Columns: []string{
			"tenant_id", "cluster_id", "namespace_uid", "deployment_uid",
			"namespace_name", "deployment_name", "desired_replicas", "available_replicas",
			"strategy", "team", "project", "environment", "cost_center", "labels",
			"operation", "valid_from", "valid_to", "observed_at", "event_id", "version",
		},
	},
	{
		Name: "pod",
		Columns: []string{
			"tenant_id", "cluster_id", "namespace_uid", "deployment_uid", "pod_uid",
			"node_uid", "namespace_name", "deployment_name", "pod_name", "phase",
			"qos_class", "owner_kind", "owner_uid", "scheduled_at", "started_at",
			"finished_at", "labels", "operation", "valid_from", "valid_to",
			"observed_at", "event_id", "version",
		},
	},
	{
		Name: "container",
		Columns: []string{
			"tenant_id", "cluster_id", "namespace_uid", "deployment_uid", "pod_uid",
			"owner_kind", "owner_uid", "container_name", "container_id", "image", "image_id",
			"restart_count", "cpu_request_millicores", "cpu_limit_millicores",
			"memory_request_bytes", "memory_limit_bytes", "gpu_request_milli", "operation",
			"valid_from", "valid_to", "observed_at", "event_id", "version",
		},
	},
}

type lineageTable struct {
	Name    string
	Columns []string
}

type config struct {
	Address   string
	Database  string
	Username  string
	Password  string
	Secure    bool
	TenantID  string
	ClusterID string
	Table     string
	Apply     bool
}

type report struct {
	Applied             bool                 `json:"applied"`
	InventoryBackfills  []inventoryBackfill  `json:"inventory_backfills"`
	MetricReplayImpacts []metricReplayImpact `json:"metric_replay_impacts"`
}

type inventoryBackfill struct {
	Table       string `json:"table"`
	MatchedRows uint64 `json:"matched_rows"`
	SQL         string `json:"sql"`
}

type metricReplayImpact struct {
	Table       string `json:"table"`
	MatchedRows uint64 `json:"matched_rows"`
	Note        string `json:"note"`
}

func main() {
	cfg := parseFlags()
	ctx := context.Background()
	conn, err := openClickHouse(cfg)
	if err != nil {
		slog.Error("open ClickHouse", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	result, err := run(ctx, conn, cfg)
	if err != nil {
		slog.Error("normalize lineage", "error", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		slog.Error("write report", "error", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.Address, "clickhouse-address", envDefault("CLICKHOUSE_ADDRESS", "localhost:9000"), "ClickHouse native address")
	flag.StringVar(&cfg.Database, "clickhouse-database", envDefault("CLICKHOUSE_DATABASE", "kube_cost"), "ClickHouse database")
	flag.StringVar(&cfg.Username, "clickhouse-username", os.Getenv("CLICKHOUSE_USERNAME"), "ClickHouse username")
	flag.StringVar(&cfg.Password, "clickhouse-password", os.Getenv("CLICKHOUSE_PASSWORD"), "ClickHouse password")
	flag.BoolVar(&cfg.Secure, "clickhouse-secure", envBool("CLICKHOUSE_SECURE", false), "Use TLS for ClickHouse")
	flag.StringVar(&cfg.TenantID, "tenant-id", "", "Optional tenant filter")
	flag.StringVar(&cfg.ClusterID, "cluster-id", "", "Optional cluster filter")
	flag.StringVar(&cfg.Table, "table", "all", "Inventory table to normalize: all, deployment, pod, or container")
	flag.BoolVar(&cfg.Apply, "apply", false, "Apply replacement inserts. Default is dry-run report only")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, conn clickhouse.Conn, cfg config) (report, error) {
	tables, err := selectedTables(cfg.Table)
	if err != nil {
		return report{}, err
	}
	result := report{Applied: cfg.Apply}
	for _, table := range tables {
		countSQL, countArgs := countInventorySQL(table.Name, cfg)
		count, err := countRows(ctx, conn, countSQL, countArgs...)
		if err != nil {
			return report{}, fmt.Errorf("count %s: %w", table.Name, err)
		}
		applySQL, applyArgs := backfillInventorySQL(table, cfg)
		result.InventoryBackfills = append(result.InventoryBackfills, inventoryBackfill{
			Table:       table.Name,
			MatchedRows: count,
			SQL:         applySQL,
		})
		if cfg.Apply && count > 0 {
			if err := conn.Exec(ctx, applySQL, applyArgs...); err != nil {
				return report{}, fmt.Errorf("backfill %s: %w", table.Name, err)
			}
		}
	}
	for _, table := range []string{"container_metrics_10s", "namespace_cost_1h"} {
		countSQL, countArgs := countMetricImpactSQL(table, cfg)
		count, err := countRows(ctx, conn, countSQL, countArgs...)
		if err != nil {
			return report{}, fmt.Errorf("count %s impact: %w", table, err)
		}
		result.MetricReplayImpacts = append(result.MetricReplayImpacts, metricReplayImpact{
			Table:       table,
			MatchedRows: count,
			Note:        "immutable fact rows are not rewritten; replay raw archived batches or rebuild derived facts after inventory normalization",
		})
	}
	return result, nil
}

func countRows(ctx context.Context, conn clickhouse.Conn, sql string, args ...any) (uint64, error) {
	var count uint64
	if err := conn.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func selectedTables(name string) ([]lineageTable, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "all" {
		return inventoryTables, nil
	}
	for _, table := range inventoryTables {
		if table.Name == name {
			return []lineageTable{table}, nil
		}
	}
	return nil, fmt.Errorf("unsupported table %q", name)
}

func countInventorySQL(table string, cfg config) (string, []any) {
	where, args := namespaceNameWhere("t", cfg)
	return fmt.Sprintf(`
SELECT count()
FROM kube_cost.%s AS t
INNER JOIN kube_cost.current_namespace AS ns
    ON t.tenant_id = ns.tenant_id
   AND t.cluster_id = ns.cluster_id
   AND t.namespace_uid = ns.namespace_name
WHERE %s`, table, where), args
}

func backfillInventorySQL(table lineageTable, cfg config) (string, []any) {
	where, args := namespaceNameWhere("t", cfg)
	selects := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		switch column {
		case "namespace_uid":
			selects = append(selects, "ns.namespace_uid")
		case "version":
			selects = append(selects, fmt.Sprintf("toUInt64(t.version + %d)", versionBump))
		default:
			selects = append(selects, "t."+column)
		}
	}
	return fmt.Sprintf(`
INSERT INTO kube_cost.%s (%s)
SELECT
    %s
FROM kube_cost.%s AS t
INNER JOIN kube_cost.current_namespace AS ns
    ON t.tenant_id = ns.tenant_id
   AND t.cluster_id = ns.cluster_id
   AND t.namespace_uid = ns.namespace_name
WHERE %s`, table.Name, strings.Join(table.Columns, ", "), strings.Join(selects, ",\n    "), table.Name, where), args
}

func countMetricImpactSQL(table string, cfg config) (string, []any) {
	where, args := namespaceNameWhere("t", cfg)
	return fmt.Sprintf(`
SELECT count()
FROM kube_cost.%s AS t
INNER JOIN kube_cost.current_namespace AS ns
    ON t.tenant_id = ns.tenant_id
   AND t.cluster_id = ns.cluster_id
   AND t.namespace_uid = ns.namespace_name
WHERE %s`, table, where), args
}

func namespaceNameWhere(alias string, cfg config) (string, []any) {
	prefix := alias + "."
	clauses := []string{"ns.namespace_uid != " + prefix + "namespace_uid"}
	args := []any{}
	if strings.TrimSpace(cfg.TenantID) != "" {
		clauses = append(clauses, prefix+"tenant_id = ?")
		args = append(args, strings.TrimSpace(cfg.TenantID))
	}
	if strings.TrimSpace(cfg.ClusterID) != "" {
		clauses = append(clauses, prefix+"cluster_id = ?")
		args = append(args, strings.TrimSpace(cfg.ClusterID))
	}
	return strings.Join(clauses, " AND "), args
}

func openClickHouse(cfg config) (clickhouse.Conn, error) {
	options := &clickhouse.Options{
		Addr: []string{cfg.Address},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    2,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	}
	if cfg.Secure {
		options.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return clickhouse.Open(options)
}

func envDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}
