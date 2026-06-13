param(
    [string]$ClusterName = "kube-cost"
)

$ErrorActionPreference = "Continue"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot

try {
if (Get-Command kind -ErrorAction SilentlyContinue) {
    $clusters = @(kind get clusters 2>$null)
    if ($clusters -contains $ClusterName) {
        if ((Get-Command helm -ErrorAction SilentlyContinue) -and (Get-Command kubectl -ErrorAction SilentlyContinue)) {
            kubectl config use-context "kind-$ClusterName" | Out-Null
            & "$PSScriptRoot/helm-uninstall.ps1"
        }
        kind delete cluster --name $ClusterName
    }
}

if (Get-Command docker -ErrorAction SilentlyContinue) {
    docker compose -f deploy/compose/docker-compose.yaml down --volumes --remove-orphans
}
}
finally {
    Pop-Location
}
