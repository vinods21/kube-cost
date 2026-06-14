param(
    [string]$ClusterName = "kube-cost"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot

try {
$requiredCommands = @("docker", "kind", "helm", "kubectl")
foreach ($command in $requiredCommands) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw "Required command '$command' was not found in PATH"
    }
}

docker info | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "Docker daemon is not available"
}

try {
    docker compose -f deploy/compose/docker-compose.yaml up --detach --wait
    if ($LASTEXITCODE -ne 0) {
        throw "Starting Docker Compose dependencies failed"
    }
    & "$PSScriptRoot/clickhouse-migrate.ps1"

    $clusters = @(kind get clusters)
    if ($LASTEXITCODE -ne 0) {
        throw "Listing Kind clusters failed"
    }
    if ($clusters -notcontains $ClusterName) {
        kind create cluster --config deploy/kind/cluster.yaml
        if ($LASTEXITCODE -ne 0) {
            throw "Creating Kind cluster failed"
        }
    }

    kubectl config use-context "kind-$ClusterName" | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Selecting Kind kubeconfig context failed"
    }
    & "$PSScriptRoot/helm-install.ps1" -ClusterName $ClusterName

    Write-Host ""
    Write-Host "Local development environment is ready."
    Write-Host "Grafana:    http://localhost:3000  (admin/admin)"
    Write-Host "ClickHouse: http://localhost:8123"
    Write-Host "Kubernetes: context kind-$ClusterName"
}
catch {
    [Console]::Error.WriteLine("Development environment startup failed: $($_.Exception.Message)")
    & "$PSScriptRoot/dev-down.ps1" -ClusterName $ClusterName
    throw
}
}
finally {
    Pop-Location
}
