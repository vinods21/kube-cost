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

## Query constraints

- Default maximum range is 31 days at hourly granularity and 13 months at daily granularity.
- Arbitrary label grouping is allow-listed and cardinality-budgeted.
- Responses return `413` for excessive result cardinality and `422` for invalid semantic combinations.
- Rate limits are per tenant, principal, and endpoint class.

## Compatibility

Additive response fields are backward compatible. Removing or changing semantics requires a new major API path. Enum consumers must tolerate unknown values. Deprecations are announced for at least two supported client release cycles.
