# Service Boundaries

## Boundary principles

- Split by business capability and scaling/failure profile, not by database table.
- Keep the initial deployment consolidatable; logical boundaries may share a process until load or ownership justifies separation.
- Each service is authoritative for its commands and transactional state.

## Service catalog

| Service | Owns | Does not own |
|---|---|---|
| Identity | principals, memberships, roles, authorization decisions | tenant preferences |
| Tenant | tenant lifecycle, projects, reporting settings | authentication |
| Cluster Registry | cluster identity, enrollment, credentials, capabilities | collected telemetry |
| Policy | versioned allocation and optimization policy | calculated costs |
| Integrations | provider connections, billing source configuration | price calculations |
| Ingestion | accepted envelopes, sequencing, validation status | semantic allocation |
| Pricing | catalogs, rates, commitments, FX, effective-price decisions | workload usage |
| Allocation | allocation runs, lineage, policy application | source observations |
| Recommendations | findings, evidence, projected savings | workflow approval |
| Workflow | approval, suppression, execution state | recommendation calculation |
| Query | read models, query planning, pagination | source-of-truth commands |
| Export | report jobs, manifests, delivery | interactive query state |
| Audit | immutable audit events and retention | operational application logs |

## Communication

- Public commands and queries: HTTPS/JSON through the API gateway.
- Internal low-latency lookups: gRPC with deadlines, retries only for idempotent calls, and circuit breakers.
- State propagation: Kafka-compatible events with schema registry.
- Bulk transfer: object storage manifests with checksums.

## Consistency

- Cluster registration and credential issuance require strong consistency.
- Policy activation is transactional and produces one immutable version.
- Cost views are eventually consistent and expose `data_through`, `computed_at`, and quality.
- Recommendation approval uses optimistic concurrency.
- Cross-service workflows use sagas; distributed transactions are prohibited.

## Initial deployable grouping

To reduce operational overhead, phase 1 MAY combine:

- Tenant, cluster registry, policy, and integrations as `control-api`.
- Allocation and recommendations as separate workers sharing scheduling infrastructure.
- Query and export orchestration as `analytics-api`.

The contracts and data ownership remain separate so these can split without API redesign.
