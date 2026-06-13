param(
    [string]$Namespace = "kube-cost-system"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot

try {
helm uninstall kube-cost-agent --namespace $Namespace --ignore-not-found
if ($LASTEXITCODE -ne 0) { throw "Uninstalling agent Helm release failed" }
helm uninstall kube-cost-platform --namespace $Namespace --ignore-not-found
if ($LASTEXITCODE -ne 0) { throw "Uninstalling platform Helm release failed" }
kubectl delete namespace $Namespace --ignore-not-found --wait=false
if ($LASTEXITCODE -ne 0) { throw "Deleting development namespace failed" }
}
finally {
    Pop-Location
}
