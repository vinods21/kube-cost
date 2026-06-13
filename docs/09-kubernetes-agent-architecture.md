# Kubernetes Agent Architecture

## Deployment

The agent runs as a namespace-scoped Helm release with:

- One leader-elected Deployment for discovery, metrics aggregation, buffering, and transport.
- Optional DaemonSet collectors only for node-local signals unavailable through cluster APIs.
- ServiceAccount with read-only, resource-specific RBAC.
- Persistent volume for the write-ahead log where durability is required.

## Collection sources

- Kubernetes API watches for nodes, namespaces, pods, controllers, jobs, PV/PVC, services, and selected CRDs.
- Metrics API for baseline CPU and memory.
- Optional Prometheus-compatible endpoints for kube-state-metrics, cAdvisor, GPU, network, and storage signals.
- Karpenter CRDs when installed.
- No Secret contents, ConfigMap contents, environment variables, logs, or container command lines are collected.

## Pipeline

1. Discover capability and API versions.
2. Maintain UID-keyed local caches from list/watch with resource-version recovery.
3. Resolve owner chains and node/provider identity.
4. Sample into fixed windows using monotonic counters where available.
5. Apply metadata allow/deny and cardinality policy.
6. Serialize immutable batches to a local WAL.
7. Stream batches and delete only through contiguous acknowledgement.

## Correctness

- Initial list and watch transitions must not create duplicate lifetimes.
- Counter resets and pod recreation are identified through UID and container restart identity.
- Missing samples are marked; interpolation is bounded and explicit.
- Clock skew is measured against gateway time and reported in quality.
- Agent upgrades preserve WAL compatibility for at least one prior minor version.

## Connectivity and backpressure

The transport uses outbound-only mTLS. Batches are compressed and sequence-numbered. Retry uses exponential backoff with jitter. When offline, the agent retains data up to a configured byte/time quota, then sheds oldest high-resolution samples while preserving inventory, lifecycle, and diagnostics; all shedding is reported.

## Configuration

The gateway sends signed, revisioned configuration: collection interval, enabled sources, metadata policy, batch limits, feature gates, and sampling budget. Invalid revisions are rejected without replacing the last valid configuration.

## Security

- Enrollment tokens are single-use and short-lived.
- Long-lived identity uses rotatable client certificates or SPIFFE-compatible identity.
- RBAC is generated from enabled collectors.
- NetworkPolicy allows Kubernetes API, configured metric sources, DNS, and platform egress only.
- Diagnostics redact labels and provider account identifiers by default.

## Agent SLOs

- CPU under 100 millicores per 1,000 pods at standard sampling, subject to validation.
- Memory bounded by cache/cardinality budgets.
- API watch recovery under 5 minutes.
- WAL health and oldest unacknowledged age exposed locally and centrally.

## Upgrade strategy

The platform operator performs staged upgrades by compatibility window. Gateway rejects unsupported major contracts but provides a clear status. Canary clusters validate CPU, memory, API pressure, and observation parity before fleet rollout.
