$ErrorActionPreference = "Stop"

$env:GOCACHE = Join-Path (Get-Location) ".cache\go-build"
$env:GOTMPDIR = Join-Path (Get-Location) ".cache\gotmp"
$env:GOMODCACHE = Join-Path (Get-Location) ".cache\go-mod"

go test ./...

helm template kube-cost deploy/helm/kube-cost-platform | Out-Null
helm template kube-cost deploy/helm/kube-cost-platform `
    --set gateway.tokenTenants=token-a:tenant-a `
    --set ingress.enabled=true `
    --set ingress.host=kube-cost.example.com `
    --set ingestion.insecure=true `
    --set ingestion.rawArchive.enabled=true `
    --set ingestion.sequenceCheckpoint.enabled=true `
    --set podDisruptionBudget.enabled=true `
    --set topologySpread.enabled=true `
    --set networkPolicy.enabled=true | Out-Null
