param(
    [string]$User = "kube_cost",
    [string]$Password = "kube_cost"
)

$ErrorActionPreference = "Stop"

& "$PSScriptRoot/clickhouse-migrate.ps1" -User $User -Password $Password

Write-Host "Resetting prior ClickHouse benchmark fixture"
Get-Content -Raw "tests/performance/clickhouse/reset.sql" |
    docker compose -f deploy/compose/docker-compose.yaml exec -T clickhouse clickhouse-client `
        --user $User `
        --password $Password `
        --multiquery
if ($LASTEXITCODE -ne 0) {
    throw "Resetting benchmark fixture failed"
}

Write-Host "Loading ClickHouse benchmark fixture"
Get-Content -Raw "tests/performance/clickhouse/generate.sql" |
    docker compose -f deploy/compose/docker-compose.yaml exec -T clickhouse clickhouse-client `
        --user $User `
        --password $Password `
        --multiquery
if ($LASTEXITCODE -ne 0) {
    throw "Loading benchmark fixture failed"
}

Write-Host "Running ClickHouse benchmark queries"
Get-Content -Raw "tests/performance/clickhouse/queries.sql" |
    docker compose -f deploy/compose/docker-compose.yaml exec -T clickhouse clickhouse-client `
        --user $User `
        --password $Password `
        --time `
        --multiquery
if ($LASTEXITCODE -ne 0) {
    throw "Running benchmark queries failed"
}
