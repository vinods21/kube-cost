#!/usr/bin/env sh
set -u

cd "$(dirname "$0")/.."

CLUSTER_NAME="${CLUSTER_NAME:-kube-cost}"

if command -v kind >/dev/null 2>&1 && kind get clusters 2>/dev/null | grep -Fxq "$CLUSTER_NAME"; then
  if command -v helm >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1; then
    kubectl config use-context "kind-$CLUSTER_NAME" >/dev/null 2>&1
    sh scripts/helm-uninstall.sh || true
  fi
  kind delete cluster --name "$CLUSTER_NAME"
fi

if command -v docker >/dev/null 2>&1; then
  docker compose -f deploy/compose/docker-compose.yaml down --volumes --remove-orphans
fi
