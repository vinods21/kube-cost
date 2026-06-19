# Optimization Architecture

## Principles

- Recommendations are evidence-based, policy-aware, explainable, and reversible.
- Savings estimates include operational constraints and confidence.
- Detection is separate from approval and execution.
- Recent incidents, deployments, scaling events, and missing data reduce confidence or suppress action.

## Recommendation families

| Family | Examples |
|---|---|
| Workload rightsizing | CPU/memory requests and limits, JVM-aware bounds |
| Workload hygiene | idle workloads, completed jobs, orphaned volumes/services |
| Scheduling | topology, affinity, taints, bin-packing blockers |
| Node efficiency | instance family, architecture, consolidation, spot suitability |
| Autoscaling | HPA/VPA interaction, min/max bounds, pending capacity |
| Commitments | reservation/savings-plan coverage and utilization |
| Reliability-cost tradeoff | replicas, disruption budgets, zone spread constraints |

## Analysis pipeline

1. Select eligible targets under policy.
2. Validate observation coverage and stable identity.
3. Build distributions by workload and operating regime.
4. Detect seasonality, deployment changes, bursts, OOMs, throttling, and pending pods.
5. Generate constrained candidates.
6. Simulate resource, scheduling, reliability, and cost effects.
7. Rank by net savings, confidence, effort, and risk.
8. Deduplicate conflicting recommendations.
9. Publish with evidence and expiry.

## Rightsizing model

Use configurable percentile and headroom policies over multiple windows. Segment CPU and memory by replica, time-of-day, and operating regime. Hard lower bounds include observed working set, startup peaks, platform minimums, and policy. Recommendations are suppressed when coverage is low, OOM/throttling signals conflict, or workload behavior is unstable.

## Engine V1

The first implementation uses a constrained rightsizing model over 30 days of hourly container metrics:

- CPU request is based on nearest-rank CPU p95 with 15% headroom.
- Memory request is based on nearest-rank memory working set p99 with 20% headroom.
- CPU limit defaults to 2x recommended CPU request.
- Memory limit defaults to 1.5x recommended memory request.
- Monthly savings are estimated from positive CPU and memory request reductions using static resource-hour rates.

V1 suppresses recommendations with insufficient sample count or non-positive savings. It does not yet evaluate OOM events, CPU throttling, HPA/VPA interaction, rollout risk, or node-packing realizability.

## Savings

Gross savings are resource-price deltas. Net savings account for node packing realizability, commitment coverage, expected scale, and execution costs. Confidence combines data quality, model stability, pricing confidence, and simulation feasibility. Potential savings are never presented as guaranteed invoice reduction.

## Safety classes

- `INSIGHT_ONLY`: no direct configuration action.
- `LOW_RISK`: reversible, bounded changes eligible for approval automation.
- `GUARDED`: requires human approval and rollout controls.
- `PROHIBITED`: conflicts with tenant policy or unsupported workload.

## Lifecycle

States: open, acknowledged, approved, rejected, suppressed, executing, applied, verified, rolled_back, expired. Material target changes invalidate stale recommendations. Verification compares expected and observed utilization, reliability, and spend over a defined window.

## Evaluation

Offline evaluation uses historical replay. Online metrics include acceptance rate, realized savings, false-positive rate, rollback rate, incident correlation, and recommendation age. Model or heuristic changes are versioned and shadow-run before promotion.
