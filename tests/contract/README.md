# Contract Tests

The agent-ingestion suite:

- Compiles protobuf sources without requiring a local `protoc`.
- Verifies the `Connect` RPC remains bidirectional.
- Verifies every required observation payload exists.
- Enforces optional scalar presence for metrics.
- Parses and validates all agent protocol examples.
- Checks sequence and acknowledgement invariants.
- Verifies protobuf unknown-field preservation.
- Enforces `UNSPECIFIED` enum zero values.

Run with `go test ./tests/contract`.
