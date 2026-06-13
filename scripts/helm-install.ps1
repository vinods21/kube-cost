param(
    [string]$ClusterName = "kube-cost",
    [string]$Namespace = "kube-cost-system"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot

function Assert-LastExitCode {
    param([string]$Operation)
    if ($LASTEXITCODE -ne 0) {
        throw "$Operation failed with exit code $LASTEXITCODE"
    }
}

try {
$services = @(
    "gateway",
    "identity",
    "tenant",
    "cluster-registry",
    "policy",
    "integrations",
    "pricing",
    "ingestion",
    "allocation",
    "recommendations",
    "query",
    "workflow",
    "export",
    "audit"
)

foreach ($service in $services) {
    docker build `
        --build-arg "TARGET=./services/$service" `
        --tag "kube-cost/${service}:dev" `
        --file build/Dockerfile `
        .
    Assert-LastExitCode "Building image kube-cost/${service}:dev"
}

$images = @()
foreach ($service in $services) {
    $images += "kube-cost/${service}:dev"
}

$deployables = @(
    @{ Target = "./agent"; Image = "kube-cost/agent:dev" },
    @{ Target = "./operators/platform"; Image = "kube-cost/platform-operator:dev" },
    @{ Target = "./operators/action-executor"; Image = "kube-cost/action-executor:dev" }
)

foreach ($deployable in $deployables) {
    docker build `
        --build-arg "TARGET=$($deployable.Target)" `
        --tag $deployable.Image `
        --file build/Dockerfile `
        .
    Assert-LastExitCode "Building image $($deployable.Image)"
    $images += $deployable.Image
}

kind load docker-image --name $ClusterName @images
Assert-LastExitCode "Loading images into Kind"

helm upgrade --install kube-cost-platform deploy/helm/kube-cost-platform `
    --namespace $Namespace `
    --create-namespace `
    --values deploy/helm/local/platform-values.yaml `
    --wait `
    --timeout 5m
Assert-LastExitCode "Installing platform Helm chart"

helm upgrade --install kube-cost-agent deploy/helm/kube-cost-agent `
    --namespace $Namespace `
    --values deploy/helm/local/agent-values.yaml `
    --wait `
    --timeout 5m
Assert-LastExitCode "Installing agent Helm chart"

kubectl rollout status deployment `
    --namespace $Namespace `
    --selector app.kubernetes.io/part-of=kube-cost `
    --timeout 3m
Assert-LastExitCode "Waiting for Kubernetes deployments"
}
finally {
    Pop-Location
}
