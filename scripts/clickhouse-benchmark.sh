#!/bin/sh
set -eu

USER="${CLICKHOUSE_USER:-kube_cost}"
PASSWORD="${CLICKHOUSE_PASSWORD:-kube_cost}"
CLIENT="docker compose -f deploy/compose/docker-compose.yaml exec -T clickhouse clickhouse-client --user $USER --password $PASSWORD"

sh scripts/clickhouse-migrate.sh

echo "Resetting prior ClickHouse benchmark fixture"
$CLIENT --multiquery < tests/performance/clickhouse/reset.sql

echo "Loading ClickHouse benchmark fixture"
$CLIENT --multiquery < tests/performance/clickhouse/generate.sql

echo "Running ClickHouse benchmark queries"
$CLIENT --time --multiquery < tests/performance/clickhouse/queries.sql
