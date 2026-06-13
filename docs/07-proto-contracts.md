# Proto Contracts

## Scope

This document specifies protobuf package and message contracts without providing executable `.proto` definitions.

## Packages

| Package | Services/events |
|---|---|
| `cost.v1.common` | identity, time window, money, quantity, quality, pagination |
| `cost.v1.agent` | agent session, configuration, observation streaming |
| `cost.v1.pricing` | effective price lookup, catalog and billing events |
| `cost.v1.query` | cost, usage, allocation, and quality queries |
| `cost.v1.recommendation` | recommendation reads and workflow commands |
| `cost.v1.events` | normalized domain event envelopes |

## Common messages

- `ResourceIdentity`: tenant, cluster, provider, region, zone, Kubernetes UID/kind/name, provider resource ID.
- `TimeWindow`: start and exclusive end.
- `DecimalQuantity`: decimal string, unit.
- `Money`: decimal amount, ISO 4217 currency.
- `Quality`: status enum, completeness ratio, reason codes, source timestamps.
- `Lineage`: event IDs, schema version, policy version, price version, computation version.

## Agent service

`AgentIngestionService`:

- `OpenSession`: agent identity/capabilities -> session, accepted schemas, limits, configuration revision.
- `StreamObservations`: bidirectional stream of batches and acknowledgements.
- `ReportInventorySnapshot`: chunked inventory snapshot with manifest/checksum.
- `ReportHealth`: heartbeat, collection lag, WAL usage, source health.

`ObservationBatch` includes session, cluster, monotonically increasing sequence range, compression, schema version, checksum, and repeated observation envelopes. `BatchAck` reports highest contiguous accepted sequence plus retryable and terminal rejections.

## Pricing service

- `GetEffectiveRates`: identities and time windows -> priced intervals with source and confidence.
- `ResolveProviderResource`: provider identity -> canonical resource/SKU mapping.
- Events: `CatalogPriceUpserted`, `BillingChargeImported`, `CommitmentCoverageCalculated`, `FxRatePublished`.

## Query service

- `QueryCost`, `QueryUsage`, `QueryAllocation`, `QueryQuality`.
- Requests carry scope, time range, granularity, dimensions, typed filters, currency, cost basis, and page token.
- Responses carry rows, totals, next token, data watermark, computation version, and quality summary.

## Recommendation service

- `ListRecommendations`, `GetRecommendation`.
- `ApproveRecommendation`, `RejectRecommendation`, `SuppressRecommendation`, `RequestExecution`.
- Commands carry actor context, expected entity version, reason, and idempotency key.

## Event envelope

Every event contains event ID, event type, schema version, tenant, optional cluster, producer, occurred time, observed time, published time, trace context, partition key, payload, and payload checksum.

## Compatibility rules

- Field numbers are never reused.
- Removed fields are reserved.
- New fields are optional with safe defaults.
- Existing scalar meaning and unit never change.
- Enum zero value is `UNSPECIFIED`; consumers preserve unknown values.
- Breaking changes require a new package major version and dual-publish migration.
- Contract CI performs descriptor compatibility and golden-message round trips.
