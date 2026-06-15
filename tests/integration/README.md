# Integration Tests

Integration tests that exercise external dependencies are colocated with their
Go packages and gated by environment variables.

Run the ClickHouse inventory lifecycle coverage with:

```text
make clickhouse-integration-test
```
