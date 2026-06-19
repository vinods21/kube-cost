# Kubernetes Cost Intelligence Platform

## Purpose

This directory is the architecture baseline for a production-grade, multi-cluster Kubernetes cost intelligence platform. It defines system boundaries, contracts, storage, allocation, optimization, operations, deployment, and the staged execution plan. It intentionally contains no implementation code.

## Architectural goals

- Produce auditable hourly and daily Kubernetes cost allocation.
- Preserve tenant and cluster isolation across ingestion, storage, and APIs.
- Separate observed usage, provider prices, allocation policy, and derived cost.
- Support replay when prices, labels, ownership, or allocation policy change.
- Generate explainable recommendations without making unsafe autonomous changes.
- Operate in disconnected or intermittently connected clusters.
- Scale control-plane ingestion and analytics independently.

## Document map

1. [System Architecture](01-system-architecture.md)
2. [Component Architecture](02-component-architecture.md)
3. [Monorepo Structure](03-monorepo-structure.md)
4. [Service Boundaries](04-service-boundaries.md)
5. [Data Model](05-data-model.md)
6. [API Contracts](06-api-contracts.md)
7. [Proto Contracts](07-proto-contracts.md)
8. [ClickHouse Schema Strategy](08-clickhouse-schema-strategy.md)
9. [Kubernetes Agent Architecture](09-kubernetes-agent-architecture.md)
10. [Cost Allocation Architecture](10-cost-allocation-architecture.md)
11. [Optimization Architecture](11-optimization-architecture.md)
12. [Karpenter Integration Architecture](12-karpenter-integration-architecture.md)
13. [Operator Architecture](13-operator-architecture.md)
14. [Deployment Architecture](14-deployment-architecture.md)
15. [ADR Register](15-adr-register.md)
16. [Execution Plan](16-execution-plan.md)
17. [CK-Kube Agent V1](17-ck-kube-agent-v1.md)
18. [Ingestion Service](18-ingestion-service.md)
19. [ClickHouse Physical Schema](19-clickhouse-physical-schema.md)
20. [Cost Allocation Engine V1](20-cost-allocation-engine-v1.md)
21. [Optimization Engine V1](21-optimization-engine-v1.md)

## Normative language

`MUST`, `SHOULD`, and `MAY` indicate required, recommended, and optional behavior. Contract changes follow semantic versioning and the compatibility rules in the API and proto documents.

## Baseline assumptions

- The platform is SaaS-capable but can also be deployed in a single-tenant environment.
- Kubernetes metadata and metrics are collected in-cluster; cloud billing and price catalogs are collected centrally.
- ClickHouse is the analytical system of record. PostgreSQL stores transactional configuration and workflow state. Object storage retains raw replayable envelopes and exports.
- Internal streaming uses Kafka-compatible semantics. Internal synchronous APIs use gRPC. User-facing APIs use HTTPS/JSON.
- All monetary values retain currency and source; reporting currency conversion is explicit and versioned.
