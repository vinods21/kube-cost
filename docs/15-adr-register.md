# Architecture Decision Record Register

ADRs use states `Proposed`, `Accepted`, `Superseded`, or `Rejected`. Acceptance requires owner, date, context, decision, alternatives, consequences, security impact, operational impact, and revisit trigger.

## Initial decisions

| ID | Decision | Initial status |
|---|---|---|
| ADR-001 | Use a modular monorepo with independently releasable deployables | Proposed |
| ADR-002 | Separate edge, ingestion, analytics, control, and experience planes | Proposed |
| ADR-003 | Use outbound mTLS gRPC streaming for cluster agents | Proposed |
| ADR-004 | Adopt at-least-once delivery with deterministic idempotency | Proposed |
| ADR-005 | Use Kafka-compatible durable streams for domain propagation | Proposed |
| ADR-006 | Archive raw accepted envelopes in object storage for replay | Proposed |
| ADR-007 | Use ClickHouse as analytical system of record | Proposed |
| ADR-008 | Use PostgreSQL per logical control-plane ownership boundary | Proposed |
| ADR-009 | Use HTTPS/JSON publicly and protobuf/gRPC internally | Proposed |
| ADR-010 | Use immutable, effective-dated allocation policies | Proposed |
| ADR-011 | Preserve event time, valid time, and computation version | Proposed |
| ADR-012 | Model missing data and estimation explicitly | Proposed |
| ADR-013 | Use fixed-point money and explicit currency conversion | Proposed |
| ADR-014 | Separate source cost from allocated destination cost | Proposed |
| ADR-015 | Reconcile derived cost to provider billing exports | Proposed |
| ADR-016 | Keep arbitrary labels bounded and promote common dimensions | Proposed |
| ADR-017 | Use regional cells with tenant home-region pinning | Proposed |
| ADR-018 | Separate recommendation generation, approval, and execution | Proposed |
| ADR-019 | Require typed, allow-listed operator actions with rollback | Proposed |
| ADR-020 | Dynamically adapt to versioned Karpenter CRDs | Proposed |
| ADR-021 | Prefer centralized metrics collection over mandatory node agents | Proposed |
| ADR-022 | Use release manifests instead of a single monorepo version | Proposed |
| ADR-023 | Use expand/backfill/switch/contract for analytical schema change | Proposed |
| ADR-024 | Use saga workflows and prohibit distributed transactions | Proposed |
| ADR-025 | Establish per-tenant resource and cardinality budgets | Proposed |
| ADR-026 | Maintain raw, canonical, derived, aggregate, and serving data layers | Proposed |
| ADR-027 | Treat recommendation savings as probabilistic, not guaranteed | Proposed |
| ADR-028 | Make CRDs declarative and prohibit embedded executable payloads | Proposed |

## ADR sequencing

Before implementation starts, accept ADR-001 through ADR-016. Before production topology work, accept ADR-017 and ADR-022 through ADR-026. Before automated optimization, accept ADR-018 through ADR-020, ADR-027, and ADR-028.
