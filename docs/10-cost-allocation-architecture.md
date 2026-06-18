# Cost Allocation Architecture

## Objective

Allocate the complete relevant infrastructure bill to accountable destinations while preserving the distinction between direct, idle, shared, overhead, credits, and unallocated cost.

## Cost bases

- **List**: public catalog rate.
- **Net**: negotiated discounts and applicable credits.
- **Amortized**: commitment upfront/recurring fees spread over covered usage.
- **Invoiced**: reconciled billing-export amount.

The API never mixes bases silently.

## Allocation pipeline

1. Normalize usage into resource-time quantities.
2. Resolve effective provider resource and price intervals.
3. Calculate direct pod/container CPU, memory, GPU, storage, network, and service cost.
4. Calculate node and cluster idle capacity/cost.
5. Apply commitment and discount attribution.
6. Allocate shared workloads and platform services.
7. Allocate cluster/cloud overhead.
8. Reconcile against billing totals and expose residual unallocated cost.
9. Publish versioned hourly facts and daily aggregates.

## Direct node allocation

Node cost is decomposed by configurable resource weights. Allocatable capacity is the denominator. A workload receives cost based on request, usage, or a weighted blend, capped by runtime and capacity. Unassigned allocatable cost remains idle. System-reserved capacity is classified separately.

Default policy:

- CPU and memory allocation use the greater of request and bounded usage.
- GPU uses assigned device-time.
- Spot and on-demand prices follow the actual node purchase option.
- Unschedulable or terminated windows retain the last valid price interval with quality status.

## Engine V1

The first implementation intentionally uses a constrained allocation model:

- Node price is static at `$0.10/hour`.
- The allocation denominator is total container CPU request on each node and hour.
- The numerator is namespace container CPU request on the same node and hour.
- Inputs are `container_metrics_10s.cpu_request_core_milliseconds` and `current_node` inventory. Namespace names are enriched from `current_namespace` when present.
- Output is hourly namespace cost through `GET /api/v1/namespaces/cost`.

Formula:

`namespace_node_hour_cost = 0.10 * namespace_cpu_request_core_milliseconds / node_cpu_request_core_milliseconds`

If a node has no positive CPU requests in the selected hour, V1 emits no namespace allocation for that node. Idle and unallocated cost remain future work for the full allocation policy engine.

## Idle allocation

Idle may remain visible, be distributed to workloads on the same node/cluster, or be assigned to a designated shared destination. Distribution weights may use direct cost, requests, usage, or equal share. The original idle amount remains traceable after allocation.

## Shared cost

Rules match source costs and select destinations by dimensions. Supported weight concepts: equal, direct cost, CPU, memory, custom fixed weights, or an approved business metric. Rules have priorities, effective intervals, cycle detection, and a required fallback destination.

## Commitments and credits

Commitment benefit is attributed to eligible covered usage according to provider rules and tenant policy. Unused commitment cost is explicit. General credits are not distributed unless a policy identifies scope and weight. Taxes and support fees remain separate components.

## Reconciliation

For each provider account/billing period:

`invoiced charges = allocated direct + shared + idle + overhead + unallocated + excluded`

Variance thresholds trigger quality warnings and block a period from becoming final. Preliminary periods can update; finalized periods require a correction version and audit event.

## Policy and replay

Allocation policies are immutable and effective-dated. A dry run reports affected spend and dimensional changes before activation. Backfills write a new computation version and can be compared before promotion.

## Explainability

Every allocated row can answer: source cost, source quantity, price, selected rule, destination, weight numerator/denominator, policy version, and computation run.
