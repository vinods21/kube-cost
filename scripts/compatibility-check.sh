#!/usr/bin/env sh
set -eu

export GOCACHE="${GOCACHE:-$(pwd)/.cache/go-build}"
export GOTMPDIR="${GOTMPDIR:-$(pwd)/.cache/gotmp}"
export GOMODCACHE="${GOMODCACHE:-$(pwd)/.cache/go-mod}"

go test ./...

helm template kube-cost deploy/helm/kube-cost-platform >/dev/null
helm template kube-cost deploy/helm/kube-cost-platform \
  --set gateway.tokenTenants=token-a:tenant-a \
  --set ingress.enabled=true \
  --set ingress.host=kube-cost.example.com \
  --set ingestion.insecure=true \
  --set ingestion.rawArchive.enabled=true \
  --set ingestion.sequenceCheckpoint.enabled=true \
  --set podDisruptionBudget.enabled=true \
  --set topologySpread.enabled=true >/dev/null
