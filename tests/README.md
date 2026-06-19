# Tests

Test suites are separated by execution scope.

Run the baseline compatibility gate with:

```text
make compatibility-check
```

The gate runs all Go tests plus Helm rendering for default values and a
production-style gateway/ingestion configuration. ClickHouse integration and
performance checks remain separate because they require Docker dependencies and
larger fixtures.
