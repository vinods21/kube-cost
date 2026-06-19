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

Workflow mutation APIs are still pending; approvals, rejection, suppression, and
execution requests are not implemented by V1 recommendation reads.

## Query constraints

- Default maximum range is 31 days at hourly granularity and 13 months at daily granularity.
- Arbitrary label grouping is allow-listed and cardinality-budgeted.
- Responses return `413` for excessive result cardinality and `422` for invalid semantic combinations.
- Rate limits are per tenant, principal, and endpoint class.

## Compatibility

Additive response fields are backward compatible. Removing or changing semantics requires a new major API path. Enum consumers must tolerate unknown values. Deprecations are announced for at least two supported client release cycles.
