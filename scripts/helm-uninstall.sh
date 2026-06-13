#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

NAMESPACE="${NAMESPACE:-kube-cost-system}"

helm uninstall kube-cost-agent --namespace "$NAMESPACE" --ignore-not-found
helm uninstall kube-cost-platform --namespace "$NAMESPACE" --ignore-not-found
kubectl delete namespace "$NAMESPACE" --ignore-not-found --wait=false
