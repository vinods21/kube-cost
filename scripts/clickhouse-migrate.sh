#!/bin/sh
set -eu

USER="${CLICKHOUSE_USER:-kube_cost}"
PASSWORD="${CLICKHOUSE_PASSWORD:-kube_cost}"

for migration in deploy/clickhouse/migrations/*.sql; do
  echo "Applying $migration"
  docker compose -f deploy/compose/docker-compose.yaml exec -T clickhouse clickhouse-client \
    --user "$USER" \
    --password "$PASSWORD" \
    --multiquery < "$migration"
done
