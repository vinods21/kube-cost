# Cost Allocation Engine V1

## Scope

Cost Allocation Engine V1 produces hourly namespace cost from Kubernetes node inventory and container metric facts. It is intentionally deterministic and uses static demo pricing until cloud price catalog and billing reconciliation are implemented.

## Inputs

- `kube_cost.container_metrics_10s`
  - `cpu_request_core_milliseconds`
  - `network_rx_bytes`
  - `network_tx_bytes`
  - `tenant_id`, `cluster_id`, `node_uid`, `namespace_uid`, `bucket_start`
- `kube_cost.current_node`
  - `allocatable_cpu_millicores`
- `kube_cost.current_namespace`
  - `namespace_name`

## Static Rates

- Node cost: `$0.10/hour`
- Control plane cost: `$0.05/hour` per cluster
- Network cost: `$0.01/GiB`

Runtime environment variables:

- `ALLOCATION_NODE_HOURLY_COST_USD`
- `ALLOCATION_CONTROL_PLANE_HOURLY_COST_USD`
- `ALLOCATION_NETWORK_COST_PER_GIB_USD`

## Algorithms

### Direct Node Cost

For each node-hour, namespace CPU request is divided by node allocatable CPU.

`direct_cost = node_hourly_cost * namespace_cpu_request / node_allocatable_cpu`

If total requests exceed allocatable CPU, the denominator is clamped to total requested CPU to avoid charging more than the static node price.

### Idle Capacity Cost

Idle is the unrequested allocatable CPU share on a node-hour.

`idle_cost = node_hourly_cost * max(node_allocatable_cpu - node_requested_cpu, 0) / node_allocatable_cpu`

Idle is emitted as a synthetic namespace row:

- `namespace_uid = "__idle__"`
- `namespace_name = "__idle__"`

### Control Plane Cost

Control plane cost is allocated across real namespaces by cluster-hour CPU request share.

`control_plane_cost = control_plane_hourly_cost * namespace_cpu_request / cluster_cpu_request`

### Network Cost

Network cost is charged directly to the namespace that generated the bytes.

`network_cost = (network_rx_bytes + network_tx_bytes) / GiB * network_cost_per_gib`

### System Namespace Cost

`system_namespace_cost` is a classification field for Kubernetes system namespaces:

- `kube-system`
- `kube-public`
- `kube-node-lease`

It is not an extra charge. It identifies the portion of `allocated_cost` attributable to system namespaces.

## Output

The REST API is:

`GET /api/v1/namespaces/cost`

The response contains one row per namespace-hour plus optional synthetic idle rows. Component columns are:

- `direct_cost`
- `idle_cost`
- `network_cost`
- `control_plane_cost`
- `system_namespace_cost`
- `allocated_cost`

`allocated_cost = direct_cost + idle_cost + network_cost + control_plane_cost`

## Known Limits

- Static prices are demo assumptions, not provider billing rates.
- Memory, GPU, storage, discounts, credits, and amortization are not allocated.
- System namespace cost is classified, not redistributed.
- Idle is exposed as a synthetic namespace rather than assigned to tenants.
- Correctness depends on metrics persistence populating `container_metrics_10s`.
