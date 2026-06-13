# Data Model

## Identity model

Stable identifiers are opaque UUID/ULID values. Kubernetes names are attributes, never primary identity.

| Entity | Key fields |
|---|---|
| Tenant | tenant_id, name, status, home_region |
| Project | project_id, tenant_id, name, mapping rules |
| Cluster | cluster_id, tenant_id, provider, external identity, timezone |
| Kubernetes object | cluster_id, object_uid, kind, namespace, name, valid interval |
| Workload | workload_id, owner kind/UID, controller chain, labels, valid interval |
| Resource | resource_id, provider resource identity, type, region, zone |

## Observation model

An observation is an immutable measurement for `[window_start, window_end)`:

- Identity: tenant, cluster, object UID, workload, node, resource.
- Requested resources: CPU, memory, ephemeral storage, GPUs, extended resources.
- Usage: CPU core-seconds, memory byte-seconds, network bytes, storage byte-seconds.
- Runtime: scheduled seconds, ready seconds, active seconds.
- Metadata snapshot reference and source quality.
- Event identity, schema version, sequence, event time, ingestion time.

## Pricing model

- `PriceCatalog`: provider/SKU/location/tenancy/effective interval/currency/unit price.
- `BillingCharge`: provider invoice line, account, resource, service, category, amount, credits, taxes, effective interval.
- `Commitment`: reservation or savings-plan scope, quantity, term, amortized fee.
- `FXRate`: base currency, quote currency, date, source, rate.
- `EffectiveRate`: derived price with catalog, discount, commitment, credit, and policy lineage.

## Cost fact model

Each fact represents a cost component over a time bucket:

- Dimensions: tenant, cluster, namespace, workload, pod, node, project/team, provider dimensions.
- Measures: usage quantity, list cost, net cost, amortized cost, allocated cost.
- Component: CPU, memory, GPU, storage, network, load balancer, control plane, license, tax, credit, shared, idle.
- Lineage: observation set, price version, billing source, allocation policy version, computation version.
- Quality: complete, estimated, fallback-priced, late, corrected, or unknown.

## Temporal metadata

Labels, annotations, owners, namespace mappings, and node properties use valid-time intervals. Allocation joins metadata as of the observation window, preventing current labels from rewriting historical ownership unless an explicit backfill policy requests it.

## Recommendation model

`Recommendation` contains type, target, analysis window, evidence, current configuration, proposed bounds, monthly gross/net savings, confidence, risk, constraints, policy version, status, and expiration. `RecommendationAction` separately tracks approval and execution.

## Control model

Policies are immutable versions with draft/active/retired state. Activating a version records author, timestamp, validation result, and affected scope. Secrets are stored only as external secret references.

## Retention

- Raw envelopes: 30-90 days hot object storage, then configurable archive.
- Minute observations: 30-90 days.
- Hourly facts: 13-25 months.
- Daily/monthly facts: 7 years or tenant policy.
- Audit and action history: compliance policy, minimum 1 year.

## Data quality

Quality dimensions include freshness, completeness, coverage, price confidence, identity resolution, and reconciliation variance. Every query response carries summary quality; detailed diagnostics are queryable by scope and time.
