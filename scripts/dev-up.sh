#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

CLUSTER_NAME="${CLUSTER_NAME:-kube-cost}"

for command in docker kind helm kubectl; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "Required command '$command' was not found in PATH" >&2
    exit 1
  }
done

docker info >/dev/null

cleanup_on_error() {
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "Development environment startup failed; cleaning up." >&2
    CLUSTER_NAME="$CLUSTER_NAME" sh scripts/dev-down.sh
  fi
  exit "$status"
}
trap cleanup_on_error EXIT

docker compose -f deploy/compose/docker-compose.yaml up --detach --wait
sh scripts/clickhouse-migrate.sh

if ! kind get clusters | grep -Fxq "$CLUSTER_NAME"; then
  kind create cluster --config deploy/kind/cluster.yaml
fi

kubectl config use-context "kind-$CLUSTER_NAME" >/dev/null
CLUSTER_NAME="$CLUSTER_NAME" sh scripts/helm-install.sh

trap - EXIT
printf '\n%s\n' \
  "Local development environment is ready." \
  "Grafana:    http://localhost:3000  (admin/admin)" \
  "ClickHouse: http://localhost:8123" \
  "Kubernetes: context kind-$CLUSTER_NAME"
