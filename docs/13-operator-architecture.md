# Operator Architecture

## Operators

Two logical operators are defined:

- **Platform Operator**: installs and reconciles in-cluster agent resources and cluster connectivity.
- **Action Executor Operator**: executes explicitly approved Kubernetes/Karpenter recommendations under strict field and scope controls.

They MAY ship together initially but use separate controllers, service accounts, and permissions.

## Custom resources

Conceptual APIs:

- `CostAgent`: endpoint, collection profile, secret reference, buffering, status.
- `CostCollectionPolicy`: allowed metadata, sources, intervals, cardinality limits.
- `CostRecommendationAction`: immutable recommendation reference, approved patch intent, guardrails, rollout state.

Custom resources contain no provider secrets or arbitrary executable content.

## Reconciliation

Controllers are level-based and idempotent. Each reconciler:

1. Validates generation and policy.
2. Reads current dependent state.
3. Calculates desired resources or approved action.
4. Applies owned fields with server-side apply.
5. Records conditions and observed generation.
6. Requeues using events or bounded backoff.

## Status and conditions

Standard conditions include `Ready`, `Progressing`, `Degraded`, `Blocked`, and `RollbackRequired`, each with reason, message, transition time, and observed generation. Action status records precondition hashes, approval identity, applied changes, verification, and rollback.

## Safety

- Admission validation rejects unknown action types, stale recommendations, excessive scope, and unapproved fields.
- The action operator has no permission to change Secrets, RBAC, admission webhooks, or arbitrary resources.
- Each action type has a compiled field allow list and precondition evaluator.
- Leader election prevents concurrent active reconcilers; per-target leases serialize actions.
- Finalizers are limited to cleanup that cannot orphan external state and have timeout escape paths.

## Upgrades

CRDs use conversion only when multiple served versions are necessary. Storage version migrations are explicit. Operator upgrades follow canary, compatibility, and rollback checks; CRD removal is never part of routine uninstall.

## Observability

Expose reconcile duration, error/requeue rate, queue depth, action phase duration, policy rejection count, stale-action count, and Kubernetes API throttling. Events and status are user-readable; detailed audit is streamed centrally.
