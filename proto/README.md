# Protocol Buffers

Source contracts are organized by versioned package under `cost/v1`. Generated Go files belong in `proto/gen/go`.

## Agent to ingestion protocol

The `cost.v1.agent.AgentIngestionService/Connect` RPC is a bidirectional stream:

1. The agent sends `AgentHello`.
2. Ingestion selects a compatible major/minor protocol and sends `ServerHello`.
3. The agent sends contiguous, sequence-numbered `ObservationBatch` frames.
4. Ingestion returns cumulative persisted acknowledgements and optional retry or terminal rejection details.
5. Either side may send heartbeat, configuration, or flow-control frames while the stream is active.

An agent may discard WAL records only through `persisted_through_sequence`, excluding explicitly terminally rejected records. Reconnect uses `resume_after_sequence`; ingestion remains authoritative about the accepted resume point.

## Compatibility

- Package major version identifies the breaking-change boundary.
- `ProtocolVersion.major` must match; minor versions negotiate additive capability.
- Existing field numbers and enum values are never reused.
- New payloads are added to `Observation.payload`.
- Enum zero values remain `UNSPECIFIED`.
- Unknown fields are preserved by protobuf runtimes.
- Consumers must ignore unknown fields and tolerate unknown enum values.

## Generation

Install `protoc` and run:

```text
make proto-tools
make proto
```

PowerShell users may run `scripts/install-proto-tools.ps1` followed by `scripts/generate-proto.ps1`. Generator versions are pinned as Go tool dependencies in `go.mod`.

## Validation

`go test ./tests/contract` compiles the source descriptors without `protoc`, validates the streaming shape and compatibility rules, and parses the JSON examples under `proto/examples/agent`.
