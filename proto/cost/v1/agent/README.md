# Agent Ingestion Contract

## Stream lifecycle

1. Agent opens `AgentIngestionService.Connect`.
2. Agent sends exactly one `AgentHello` as its first frame.
3. Ingestion authenticates the mTLS identity against `tenant_id` and `cluster_id`.
4. Ingestion replies with exactly one `ServerHello`.
5. Both sides exchange data and control frames until disconnect.
6. Reconnect resumes from the server-confirmed persisted sequence.

Frames received before handshake completion are protocol violations.

## Ordering and identity

- Sequence numbers are scoped to one stable `cluster_id` and increase monotonically across agent restarts.
- A batch contains a contiguous inclusive range; every sequence in the range appears exactly once.
- `event_id` is stable across retries and enables deduplication beyond sequence tracking.
- `observed_at` is source event time; `collected_at` is when the agent materialized the record.
- Ingestion records its own receive and persistence timestamps outside this source contract.

## Acknowledgements

- `received_through_sequence` is diagnostic and does not permit WAL deletion.
- `persisted_through_sequence` is the cumulative durability boundary.
- `retry_ranges` identify gaps or transient failures after the cumulative boundary.
- Non-retryable `RecordRejection` entries are terminal and may be removed from the WAL after the rejection is durably recorded.
- Re-sending an acknowledged record is valid and must be idempotent.

## Inventory

Inventory records support UPSERT and DELETE tombstones. A full resync is enclosed by `InventorySnapshotMarker` STARTED and COMPLETED records sharing `snapshot_id`. Ingestion applies snapshot-based absence only after receiving a valid completion marker.

Labels and annotations are already filtered by agent collection policy. Secret data, environment variables, command lines, and ConfigMap contents are outside this contract.

## Metrics

Metrics cover half-open windows `[start, end)`. Integral units avoid dependence on sample interval:

- CPU: core-nanoseconds.
- Memory and filesystem: byte-seconds.
- Network: bytes transferred during the window.
- GPU utilization: ratio from 0 through 1.

Metric scalars use field presence so missing data remains distinct from a measured zero.

## Evolution

Additive fields and observation payloads may be introduced in protocol minor versions. Breaking semantic or wire changes require a new protobuf package major version. Implementations must retain unknown fields when decoding and re-encoding persisted envelopes.
