#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

CLUSTER_NAME="${CLUSTER_NAME:-kube-cost}"
NAMESPACE="${NAMESPACE:-kube-cost-system}"
SERVICES="gateway identity tenant cluster-registry policy integrations pricing ingestion allocation recommendations query workflow export audit"
IMAGES=""

for service in $SERVICES; do
  image="kube-cost/${service}:dev"
  docker build --build-arg "TARGET=./services/${service}" --tag "$image" --file build/Dockerfile .
  IMAGES="$IMAGES $image"
done

for target_image in \
  "./agent|kube-cost/agent:dev" \
  "./operators/platform|kube-cost/platform-operator:dev" \
  "./operators/action-executor|kube-cost/action-executor:dev"; do
  target="${target_image%%|*}"
  image="${target_image#*|}"
  docker build --build-arg "TARGET=${target}" --tag "$image" --file build/Dockerfile .
  IMAGES="$IMAGES $image"
done

# Intentional word splitting passes each image as a separate argument.
# shellcheck disable=SC2086
kind load docker-image --name "$CLUSTER_NAME" $IMAGES

helm upgrade --install kube-cost-platform deploy/helm/kube-cost-platform \
  --namespace "$NAMESPACE" \
  --create-namespace \
  --values deploy/helm/local/platform-values.yaml \
  --wait \
  --timeout 5m

helm upgrade --install kube-cost-agent deploy/helm/kube-cost-agent \
  --namespace "$NAMESPACE" \
  --values deploy/helm/local/agent-values.yaml \
  --wait \
  --timeout 5m

kubectl rollout status deployment \
  --namespace "$NAMESPACE" \
  --selector app.kubernetes.io/part-of=kube-cost \
  --timeout 3m
