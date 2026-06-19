# Optimization Engine V1

## Scope

Optimization Engine V1 generates workload rightsizing recommendations from 30 days of hourly container metrics.

It recommends:

- CPU requests from CPU p95.
- Memory requests from memory working set p99.
- CPU limits from recommended CPU request.
- Memory limits from recommended memory request.
- Estimated monthly savings from positive request reductions.

## Inputs

The engine reads `kube_cost.scope_metrics_1h`:

- `scope_type = 'container'`
- `cpu_usage_core_milliseconds`
- `cpu_request_core_milliseconds`
- `cpu_limit_core_milliseconds`
- `memory_working_set_byte_seconds`
- `memory_request_byte_seconds`
- `memory_limit_byte_seconds`
- `bucket_seconds`
- `sample_count`

The default analysis window is the last 30 days.

## Algorithms

### CPU Request

For each hourly sample:

`cpu_usage_millicores = cpu_usage_core_milliseconds / bucket_seconds`

The recommendation uses nearest-rank p95 with headroom:

`recommended_cpu_request = max(min_cpu, ceil(cpu_p95 * 1.15))`

Default `min_cpu` is `10m`.

### Memory Request

For each hourly sample:

`memory_working_set_bytes = memory_working_set_byte_seconds / bucket_seconds`

The recommendation uses nearest-rank p99 with headroom:

`recommended_memory_request = max(min_memory, ceil(memory_p99 * 1.20))`

Default `min_memory` is `64Mi`.

### Limits

CPU limit:

`recommended_cpu_limit = max(recommended_cpu_request, ceil(recommended_cpu_request * 2.0))`

Memory limit:

`recommended_memory_limit = max(recommended_memory_request, ceil(recommended_memory_request * 1.5))`

### Savings

V1 estimates monthly savings from request reductions only:

`cpu_savings = positive_delta_cpu_cores * cpu_core_hour_rate * 730`

`memory_savings = positive_delta_memory_gib * memory_gib_hour_rate * 730`

Defaults:

- CPU: `$0.03/vCPU-hour`
- Memory: `$0.004/GiB-hour`
- Month: `730 hours`

Savings are not generated when the recommendation would increase or preserve total requested cost.

## Runtime Configuration

- `OPTIMIZATION_ANALYSIS_WINDOW`, default `720h`
- `OPTIMIZATION_CPU_REQUEST_HEADROOM`, default `1.15`
- `OPTIMIZATION_MEMORY_REQUEST_HEADROOM`, default `1.20`
- `OPTIMIZATION_CPU_LIMIT_MULTIPLIER`, default `2.0`
- `OPTIMIZATION_MEMORY_LIMIT_MULTIPLIER`, default `1.5`
- `OPTIMIZATION_MIN_CPU_MILLICORES`, default `10`
- `OPTIMIZATION_MIN_MEMORY_BYTES`, default `67108864`
- `OPTIMIZATION_CPU_CORE_HOUR_USD`, default `0.03`
- `OPTIMIZATION_MEMORY_GIB_HOUR_USD`, default `0.004`
- `OPTIMIZATION_MINIMUM_SAMPLE_COUNT`, default `24`

## Persistence

When `TENANT_ID` is set, the recommendations worker reads hourly scope metrics,
generates V1 rightsizing recommendations, and persists them to
`kube_cost.recommendation`.

Persisted recommendation facts use:

- Stable deterministic `recommendation_id` values from tenant, cluster, target,
  analysis window, and computation version.
- `recommendation_type = rightsizing`.
- `safety_class = review_required`.
- `status = open`.
- JSON `current_configuration`, `proposed_configuration`, and `evidence`
  payloads.
- USD gross and net monthly savings from the V1 savings estimate.
- A 30-day expiration from generation time.

The worker does not write `recommendation_action`; approval, suppression, and
execution state remain workflow-service responsibilities.

## Safety Limits

V1 suppresses recommendations when:

- The target has fewer than the configured minimum sample count.
- Estimated monthly savings is not positive.
- Required request or usage data is absent.

V1 does not yet inspect OOMs, throttling, deployment churn, HPA/VPA policy, PDBs, or rollout safety. Those checks are required before automated execution.
