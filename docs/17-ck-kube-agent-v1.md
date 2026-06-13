# CK-Kube Agent V1 Design

## Scope

Agent V1 collects cluster, node, namespace, deployment, pod, and container inventory from Kubernetes APIs. It emits typed protobuf inventory observations over the bidirectional ingestion stream. Metrics and Kubernetes/Karpenter event collection are excluded.

## Components

1. **controller-runtime manager** provides process lifecycle, health endpoints, leader election, and signal handling.
2. **shared informer factory** maintains local API caches for Nodes, Namespaces, Deployments, and Pods.
3. **inventory builder** converts Kubernetes objects into `cost.v1.agent` messages.
4. **delta cache** stores deterministic semantic fingerprints by stable resource key.
5. **work queue** serializes informer changes and retries transient publication failures.
6. **memory buffer** assigns sequence numbers and retains unacknowledged observations.
7. **gRPC transport** negotiates the protocol, sends batches, processes cumulative acknowledgements, and reconnects with backoff.

## Collection flow

`Kubernetes list/watch -> informer callback -> typed work queue -> builder -> delta cache -> sequenced memory buffer -> gRPC stream`

Informer callbacks never perform network I/O. One worker processes changes to preserve source ordering and correctly detect removed containers.

## Initial inventory

The initial list/watch state is enclosed by `InventorySnapshotMarker` records:

1. `STARTED`
2. Initial informer objects
3. Cluster identity
4. `COMPLETED`

The ingestion service must apply absence-based reconciliation only after a completed snapshot.

## Delta detection

The cache fingerprints deterministic protobuf payloads while excluding sequence, event ID, collection timestamps, and Kubernetes resource version. Resync updates with unchanged cost-relevant inventory are suppressed. Semantic changes produce UPSERT records. Deletes and containers removed from a live Pod produce DELETE tombstones.

Stable cache keys:

- `cluster/{cluster_id}`
- `node/{uid}`
- `namespace/{uid}`
- `deployment/{uid}`
- `pod/{uid}`
- `container/{pod_uid}/{container_name}`
- `init-container/{pod_uid}/{container_name}`

## Inventory mapping

- Kubernetes UID is authoritative identity; names are attributes.
- Node capacity and allocatable values use canonical millicore and byte units.
- Pod owner reference, scheduling state, QoS, service account, and priority class are retained.
- Container image, runtime ID, requests, limits, restart count, and lifecycle times are retained.
- Labels are collected. Annotations are deliberately excluded in V1 to reduce sensitive metadata exposure.
- Secret contents, ConfigMap contents, environment variables, commands, arguments, and logs are never collected.

## gRPC behavior

The agent advertises only inventory capabilities. TLS is default, with optional mTLS. Local development may explicitly enable plaintext.

The sender keeps one batch in flight, emits deterministic checksums, and removes records only after `persisted_through_sequence` or a terminal rejection. Stream failures retain the memory buffer and reconnect with bounded exponential backoff.

## Leader election

The runtime implements controller-runtime's leader-election runnable contract. Standby replicas expose liveness but remain unready until elected and synchronized. Lease access is namespace-scoped through the Helm RBAC.

## Failure behavior

- API watch failures are recovered by client-go reflectors.
- Handler publication failures are rate-limited and retried.
- Ingestion outages apply backpressure when the bounded memory buffer is full.
- No-op informer resync updates are suppressed.
- A process crash loses the in-memory outbound queue and delta cache. Crash-durable WAL and sequence checkpointing are required before claiming zero-loss delivery across process restarts.

## Resource and security posture

- Read-only access to Nodes, Namespaces, Pods, and Deployments.
- No Secret or ConfigMap permissions.
- Non-root container, read-only root filesystem, no Linux capabilities.
- TLS 1.2 minimum for ingestion unless local insecure mode is explicitly enabled.

## Future work

- Durable WAL and sequence checkpoint.
- Signed dynamic collection configuration.
- Label allow/deny policy.
- Metrics, Kubernetes events, and Karpenter collectors.
- Agent telemetry and cardinality diagnostics.
