# Karpenter Integration Architecture

## Purpose

The integration correlates Karpenter intent and lifecycle with actual capacity cost, identifies scheduling constraints that block efficient provisioning, and optionally proposes guarded configuration changes.

## Inputs

- Karpenter NodePool, EC2NodeClass or provider-equivalent class, and NodeClaim resources.
- Kubernetes nodes, pods, scheduling events, disruption budgets, and pending reasons.
- Provider instance catalogs, availability, price, spot interruption signals, and billing.
- Karpenter controller version and discovered CRD schemas.

## Compatibility

CRDs are discovered dynamically and mapped through versioned adapters. Unknown fields are retained only where safe and are never assumed to have stable semantics. A tested compatibility matrix covers supported Kubernetes and Karpenter versions.

## Identity correlation

NodeClaim UID links provisioning intent to Kubernetes node UID and provider instance ID. Temporal intervals preserve replacements and consolidation events. Failed provisioning attempts are represented independently from resulting nodes.

## Insights

- NodePool utilization, idle cost, purchase-option mix, and instance diversification.
- Constraints preventing cheaper candidates: architecture, zones, capacity type, affinities, taints, resource limits, and daemon overhead.
- Consolidation opportunity with disruption and PDB feasibility.
- Spot suitability based on workload tolerance and interruption history.
- Provisioning latency, failed launches, drift, and churn cost.
- NodePool overlap, excessive minimum capacity, and incompatible limits.

## Recommendation simulation

Candidate changes are evaluated against recent pod shapes and constraints. Simulation must include daemon overhead, topology spread, PDBs, local storage, GPUs, startup taints, limits, and provider availability. Savings are discounted by uncertainty and interruption risk.

## Action boundary

The optimization service emits a declarative proposal and evidence. The action operator:

1. Verifies current resource version and policy.
2. Produces a server-side dry-run result.
3. Applies only approved field paths.
4. Monitors provisioning, disruption, pending pods, and spend.
5. Rolls back or halts when guardrails trip.

Direct deletion of nodes or NodeClaims is outside the default automation scope.

## Guardrails

- Maximum percentage of fleet affected per rollout.
- Freeze windows and maintenance schedules.
- Minimum instance/category diversification.
- Maximum estimated interruption and pending-pod risk.
- Mandatory canary NodePool for guarded changes.
- No action when Karpenter health, data freshness, or pricing confidence is degraded.
