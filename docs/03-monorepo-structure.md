# Monorepo Structure

## Proposed layout

```text
/
  docs/                     Architecture, contracts, runbooks, ADRs
  api/
    openapi/                Public API specifications
    proto/                  Internal RPC and event contracts
    events/                 Event catalog and compatibility fixtures
  cmd/                      Deployable process entry points
  services/
    gateway/
    identity/
    tenant/
    cluster-registry/
    policy/
    integrations/
    pricing/
    ingestion/
    allocation/
    recommendations/
    query/
    workflow/
    export/
  agents/
    kubernetes/
  operators/
    platform/
    action-executor/
  packages/
    identity/
    contracts/
    telemetry/
    money/
    tenancy/
    quality/
    testing/
  analytics/
    clickhouse/
      migrations/
      views/
      dictionaries/
    allocation/
    recommendations/
  deploy/
    helm/
    gitops/
    terraform/
    local/
  tests/
    contract/
    integration/
    end-to-end/
    performance/
    chaos/
  tools/
    contract-check/
    migration-check/
    replay/
    data-generator/
  build/
  .github/
```

## Rules

- A deployable owns one directory under `services`, `agents`, or `operators`.
- Shared packages contain cross-cutting primitives only; domain logic stays with its owner.
- API and event contracts are reviewed independently from implementations.
- ClickHouse and PostgreSQL migrations are forward-only and deployable separately.
- Generated contract artifacts are reproducible and never manually edited.
- Documentation and contract checks are required in CI before service builds.

## Ownership

Use `CODEOWNERS` by domain: edge, ingestion, cost, optimization, platform, data, security, and experience. Changes spanning ownership boundaries require both producer and consumer approval.

## Versioning

One repository does not imply one release. Each deployable has an independent artifact version; a release manifest records the tested combination. Contract packages follow semantic versions, while schemas use monotonically increasing migration IDs.
