param(
    [string]$User = "kube_cost",
    [string]$Password = "kube_cost"
)

$ErrorActionPreference = "Stop"

Get-ChildItem "deploy/clickhouse/migrations/*.sql" |
    Sort-Object Name |
    ForEach-Object {
        Write-Host "Applying $($_.FullName)"
        Get-Content -Raw $_.FullName |
            docker compose -f deploy/compose/docker-compose.yaml exec -T clickhouse clickhouse-client `
                --user $User `
                --password $Password `
                --multiquery
        if ($LASTEXITCODE -ne 0) {
            throw "Migration failed: $($_.Name)"
        }
    }
