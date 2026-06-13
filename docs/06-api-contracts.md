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
| `GET /efficiency` | Utilization and waste ratios |
| `GET /data-quality` | Freshness and coverage diagnostics |
| `POST /queries` | Asynchronous high-cardinality analytical query |
| `GET /queries/{id}` | Query status and result manifest |
| `POST /exports` | Create CSV/Parquet export |

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
