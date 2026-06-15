#!/bin/sh
set -eu

docker compose -f deploy/compose/docker-compose.yaml up -d --wait clickhouse
sh scripts/clickhouse-migrate.sh

CLICKHOUSE_INTEGRATION=1 \
CLICKHOUSE_ADDRESS="${CLICKHOUSE_ADDRESS:-localhost:9000}" \
go test -count=1 ./services/ingestion/persistence
