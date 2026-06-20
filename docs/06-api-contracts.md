# API Contracts

## Conventions

- Base path: `/api/v1`.
- HTTPS/JSON, UTF-8, OIDC bearer tokens.
- Tenant context comes from the authenticated principal and explicit `tenant_id` only for authorized multi-tenant operators.
- Timestamps use RFC 3339 UTC; intervals are half-open.
- Money is `{amount, currency}` with decimal strings.
- Pagination uses opaque cursors.
- Mutating requests support `Idempotency-Key`; updates use `If-Match`.
- Errors use Problem Details with `type`, `title`, `status`, `code`, `detail`, `trace_id`, and field violations.

## Gateway Authentication V1

The gateway is the public HTTP entrypoint for V1 JSON APIs. It requires
`Authorization: Bearer <token>`, resolves the token to a tenant, removes any
caller-supplied `X-Kube-Cost-Tenant-ID`, and injects the trusted tenant header
before forwarding to backend services.

The current implementation uses a static `GATEWAY_TOKEN_TENANTS` mapping in the
form `token-a:tenant-a,token-b:tenant-b`. This is a bootstrap control until the
OIDC/JWKS integration is implemented. Backend services continue to require
`X-Kube-Cost-Tenant-ID`. When `TRUSTED_GATEWAY_SECRET` is configured on a
backend, it also requires the gateway-injected
`X-Kube-Cost-Gateway-Secret`; direct callers without that shared secret are
rejected. When `TRUSTED_GATEWAY_SIGNING_KEY` is configured on a backend and
`GATEWAY_BACKEND_SIGNING_KEY` is configured on the gateway, the gateway also
adds `X-Kube-Cost-Gateway-Identity`, `X-Kube-Cost-Gateway-Timestamp`, and
`X-Kube-Cost-Gateway-Signature`. The signature binds method, path, query,
tenant, gateway identity, and timestamp with HMAC-SHA256 and is rejected if it
is missing, invalid, or outside the allowed clock skew.

Gateway routes:

- Cluster enrollment and cluster reads route to cluster registry.
- Pricing catalog imports, effective price lookups, and billing charge imports
  route to pricing.
- Data quality, usage, cost, allocation, and recommendation reads route to
  query.
- Recommendation workflow commands route to workflow.

## Resource APIs

| Method and path | Purpose |
|---|---|
| `POST /clusters` | Register a cluster and issue a short-lived enrollment token |
| `GET /clusters` | List cluster status, capabilities, version, and freshness |
| `GET /clusters/{cluster_id}` | Read cluster details and health |
| `PATCH /clusters/{cluster_id}` | Update display/configuration metadata |
| `DELETE /clusters/{cluster_id}` | Revoke enrollment and begin retention workflow |
| `GET /policies` | List policy families and active versions |
| `POST /policies/{family}/versions` | Create validated immutable draft |
| `POST /policies/{family}/versions/{version}/activate` | Atomically activate a policy |
| `POST /integrations` | Configure a cloud, billing, or notification integration |
| `POST /integrations/{id}/validate` | Run asynchronous connectivity validation |

### Pricing And Billing Imports V1

`POST /api/v1/prices/catalog` imports tenant-scoped provider catalog price
intervals. The pricing service uses `X-Kube-Cost-Tenant-ID` as the
gateway-provided tenant context until the OIDC gateway is implemented.

Request body:

- `prices` contains 1 to 1000 catalog price intervals.
- Required price fields are `provider`, `region`, `service`, `sku`,
  `resource_type`, `unit`, `currency`, `unit_price`, `effective_start`, and
  `price_version`.
- `account_id`, `purchase_option`, `effective_end`, `source`, and `attributes`
  are optional. `purchase_option` defaults to `on_demand`; `source` defaults to
  `import`.

`POST /api/v1/billing/charges` imports tenant-scoped provider invoice or billing
export lines.

Request body:

- `charges` contains 1 to 1000 billing charge lines.
- Required charge fields are `charge_id`, `provider`, `account_id`,
  `billing_period_start`, `billing_period_end`, `usage_start`, `usage_end`,
  `service`, `cost_category`, `currency`, `list_cost`, `net_cost`,
  `amortized_cost`, `invoiced_cost`, `credits`, `taxes`, and `invoice_id`.
- `sku`, `resource_id`, `source`, and `attributes` are optional. `source`
  defaults to `import`.

Money values are decimal strings. Currency values are 3-letter codes. Both
endpoints return `202 Accepted` with imported row count and ingestion time.

`GET /api/v1/prices/effective` resolves the best tenant-scoped catalog price
interval effective at a timestamp. Required query parameters are `provider`,
`region`, `service`, `resource_type`, and `unit`. Optional parameters are
`account_id`, `sku`, `purchase_option`, and `at`. `purchase_option` defaults to
`on_demand`; `at` defaults to the service clock. Exact `account_id` and `sku`
matches win over catalog rows where those fields are empty wildcards. Missing
matches return `404`.

Response fields include the matched catalog dimensions, `currency`,
`unit_price`, `effective_start`, optional `effective_end`, `source`,
`price_version`, optional `attributes`, and `matched_at`.

### Cluster Enrollment V1

`POST /api/v1/clusters` registers a cluster under the authenticated tenant
context and returns a short-lived enrollment token. The current implementation
uses `X-Kube-Cost-Tenant-ID` as the gateway-provided tenant context until the
OIDC gateway is implemented. Requests MUST NOT include `tenant_id` in the JSON
body.

Request fields:

- `cluster_name` is required.
- `provider`, `account_id`, `region`, `capabilities`, and `labels` are
  optional.

Response fields:

- `cluster` contains `tenant_id`, `cluster_id`, `cluster_name`, `provider`,
  `account_id`, `region`, `status`, `capabilities`, `labels`, `created_at`,
  and `updated_at`.
- `enrollment_token` is a bearer secret intended for immediate agent
  bootstrap.
- `token_expires_at` is the token expiration timestamp.

`GET /api/v1/clusters` and `GET /api/v1/clusters/{cluster_id}` return only
clusters belonging to the authenticated tenant context. Cross-tenant reads
return `404`.

## Analytics APIs

`GET /costs` requires `start`, `end`, `granularity`, and optional `group_by`, `filter`, `currency`, `cost_basis`, `allocation_view`, and `include_quality`.

Response contract:

- `data`: dimension keys and decimal measures.
- `summary`: totals by cost basis.
- `data_through`, `computed_at`, `computation_version`.
- `quality`: completeness, estimated percentage, missing scopes, warnings.
- `next_cursor`.

Related endpoints:

| Path | Purpose |
|---|---|
| `GET /usage` | Requests, usage, runtime, and utilization |
| `GET /allocation` | Direct, idle, shared, and overhead allocation |
| `GET /namespaces/cost` | V1 namespace cost from CPU-request allocation |
| `GET /efficiency` | Utilization and waste ratios |
| `GET /data-quality` | Freshness and coverage diagnostics |
| `GET /karpenter/snapshot` | Karpenter NodePool, NodeClass, and NodeClaim inventory snapshot |
| `GET /karpenter/scores` | Karpenter bin-packing, spot suitability, and node utilization scores |
| `POST /queries` | Asynchronous high-cardinality analytical query |
| `GET /queries/{id}` | Query status and result manifest |
| `POST /exports` | Create CSV/Parquet export |

### Analytics Query V1

`GET /api/v1/usage`, `GET /api/v1/costs`, and `GET /api/v1/allocation`
return tenant-scoped, bounded analytical reads from the current ClickHouse
facts. The current implementation uses `X-Kube-Cost-Tenant-ID` as the
gateway-provided tenant context until the OIDC gateway is implemented.

Common query parameters:

- `start` and `end` are required RFC 3339 UTC timestamps.
- `start` and `end` must be aligned to whole hours and form a half-open range.
- `cluster_id` is optional.
- `group_by` is optional and supports `namespace`, `cluster`, `team`,
  `project`, `environment`, and `cost_center`; the default is `namespace`.
- `limit` is optional, defaults to `100`, and is capped at `500`.
- `cursor` is an optional opaque cursor from a prior page.
- `include_quality=true` includes current freshness/coverage quality summary
  fields on the response.

`GET /api/v1/usage` reads `container_metrics_10s` and returns aggregate CPU,
memory, GPU, network, filesystem, OOM, throttling, and sample-count measures.
CPU values are returned as core-hours, memory as GiB-hours, and GPU as
milli-GPU-hours.

`GET /api/v1/costs` reads `current_namespace_cost_1h` and returns aggregate
direct, idle, network, control-plane, system-namespace, and allocated cost
measures.

`GET /api/v1/allocation` reads `current_namespace_cost_1h` and returns the same
cost measures plus CPU request milliseconds, network bytes, and allocation
weight. Rows include additive `group_key` and `group_value` fields. For
`namespace` grouping, namespace identity fields are also populated; for other
groupings, namespace identity fields are empty and unassigned promoted
dimensions return `__unassigned__`. These endpoints are synchronous V1 reads;
arbitrary raw label grouping and exported object-storage result manifests
remain future work.

### Async Query Jobs V1

`POST /api/v1/queries` creates a tenant-scoped asynchronous analytics query job
for the currently supported bounded query families. `GET
/api/v1/queries/{query_id}` returns job status and, after completion, an inline
result manifest and result payload. The current implementation uses an
in-process bounded job store; jobs are not durable across query-service restarts
and large exported result manifests remain future export service work.

Request fields:

- `query_type` is required and must be `usage`, `costs`, or `allocation`.
- `start` and `end` are required RFC 3339 UTC timestamps aligned to whole
  hours.
- `cluster_id`, `group_by`, `limit`, and `include_quality` match the
  synchronous analytics query parameters.

Job statuses are `queued`, `running`, `succeeded`, and `failed`. Completed jobs
include `manifest.result_type`, `manifest.row_count`, `manifest.generated_at`,
and `manifest.inline=true`.

### Data Quality V1

`GET /api/v1/data-quality` returns tenant-scoped freshness and coverage
diagnostics for the currently implemented metric facts. The current
implementation uses `X-Kube-Cost-Tenant-ID` as the gateway-provided tenant
context until the OIDC gateway is implemented.

Query parameters:

- `cluster_id` is optional.

Response fields:

- `tenant_id`, optional `cluster_id`, `generated_at`, `data_through`, and
  `computation_version`.
- `signals`: per-source diagnostics for `container_metrics_10s` and
  `node_metrics_10s`, including record count, latest bucket time, latest
  ingestion time, freshness seconds, status, and warnings.
- `quality`: summary status, estimated percentage, missing scopes, warnings,
  and freshness window.

Status values are `fresh`, `stale`, and `empty`.

### Namespace Cost V1

`GET /api/v1/namespaces/cost` returns hourly namespace cost using the V1 static-node-cost allocation method.

Query parameters:

- `tenant_id` is required.
- `cluster_id` is optional.
- `start` and `end` are optional RFC 3339 timestamps. Provided values must be whole-hour aligned. The default range is the last complete hour.

V1 response fields include `currency`, `allocation_method`, `node_hourly_cost_usd`, `control_plane_hourly_cost_usd`, `network_cost_per_gib_usd`, `start`, `end`, and `items`.

Each item contains `tenant_id`, `cluster_id`, `namespace_uid`, `namespace_name`, `bucket_start`, `cpu_request_core_milliseconds`, `network_bytes`, `allocation_weight`, `direct_cost`, `idle_cost`, `network_cost`, `control_plane_cost`, `system_namespace_cost`, `allocated_cost`, `currency`, `allocation_method`, and `computation_version`.

Idle capacity is returned as a synthetic namespace item where `namespace_uid` and `namespace_name` are both `__idle__`.

## Recommendation APIs

| Method and path | Purpose |
|---|---|
| `GET /recommendations` | Filter by type, scope, confidence, risk, state, and savings |
| `GET /recommendations/{id}` | Read evidence and calculation lineage |
| `POST /recommendations/{id}/approve` | Approve with expected version |
| `POST /recommendations/{id}/reject` | Reject with reason |
| `POST /recommendations/{id}/suppress` | Suppress by target/rule and expiry |
| `POST /recommendations/{id}/execute` | Request policy-gated execution |
| `GET /actions/{id}` | Read action plan, status, and audit trail |

### Recommendation Reads V1

`GET /api/v1/recommendations` returns tenant-scoped recommendation facts from
ClickHouse. The current implementation uses `X-Kube-Cost-Tenant-ID` as the
gateway-provided tenant context until the OIDC gateway is implemented.

Query parameters:

- `cluster_id` is optional.
- `status` is optional.
- `type` is optional and maps to `recommendation_type`.
- `target_kind` and `target_uid` are optional.
- `min_monthly_savings` is an optional decimal string filter against net
  monthly savings.
- `limit` is optional, defaults to `100`, and is capped at `500`.

Response fields:

- `tenant_id`, optional `cluster_id`, `generated_at`, `result_count`, `limit`,
  and `recommendations`.
- Each recommendation includes target identity, type, safety class, status,
  analysis window, generated and expiration timestamps, JSON current/proposed
  configuration, JSON evidence, savings, confidence, risk, policy/model
  versions, computation version, and storage version.

`GET /api/v1/recommendations/{recommendation_id}` reads one recommendation by
ID within the authenticated tenant context. Cross-tenant or missing reads return
`404`.

### Recommendation Workflow V1

`POST /api/v1/recommendations/{recommendation_id}/approve`,
`/reject`, `/suppress`, and `/execute` record tenant-scoped workflow commands
for persisted recommendations. The workflow service uses
`X-Kube-Cost-Tenant-ID` as the gateway-provided tenant context until the OIDC
gateway is implemented.

Request fields:

- `expected_version` is optional. When provided, commands fail with `409` if
  the current recommendation version differs.
- `actor_id` and `reason` are optional audit fields.
- `details` is optional JSON metadata recorded with the action.

Workflow behavior:

- `approve` moves `open` or `acknowledged` recommendations to `approved`.
- `reject` moves `open`, `acknowledged`, or `approved` recommendations to
  `rejected`.
- `suppress` moves `open`, `acknowledged`, or `approved` recommendations to
  `suppressed`.
- `execute` records a policy-gated execution request and moves only
  `approved` recommendations to `executing`. The response includes an
  additive `execution_request` object and the action includes `execution_id`;
  the request is also stored in action `details` for executor handoff. It does
  not apply Kubernetes changes.

Each command appends `kube_cost.recommendation_action` and inserts a replacement
`kube_cost.recommendation` row with the new status and storage version.

## Query constraints

- Default maximum range is 31 days at hourly granularity and 13 months at daily granularity.
- Arbitrary label grouping is allow-listed and cardinality-budgeted.
- Responses return `413` for excessive result cardinality and `422` for invalid semantic combinations.
- Rate limits are per tenant, principal, and endpoint class.

## Compatibility

Additive response fields are backward compatible. Removing or changing semantics requires a new major API path. Enum consumers must tolerate unknown values. Deprecations are announced for at least two supported client release cycles.
