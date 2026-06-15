$ErrorActionPreference = "Stop"

docker compose -f deploy/compose/docker-compose.yaml up -d --wait clickhouse
if ($LASTEXITCODE -ne 0) {
    throw "Starting ClickHouse failed"
}

& "$PSScriptRoot/clickhouse-migrate.ps1"

$env:CLICKHOUSE_INTEGRATION = "1"
if (-not $env:CLICKHOUSE_ADDRESS) {
    $env:CLICKHOUSE_ADDRESS = "localhost:9000"
}
go test -count=1 ./services/ingestion/persistence
if ($LASTEXITCODE -ne 0) {
    throw "ClickHouse integration tests failed"
}
