# Contract: Configuration

**Feature**: `056-business-event-ledger` | FR-007, FR-009 | Research R4, R10

## Keys (`ports.BusinessEventsConfig`, `mapstructure:"business_events"`)

| Key | Type | Default | Valid values | Environment variable | Step | Description |
|---|---|---|---|---|---|---|
| `business_events.enabled` | boolean | `false` | `true`, `false` | `IDENTITY_BROKER_BUSINESS_EVENTS_ENABLED` | 4 | Installs the ledger handler. When false, the broker stores no business event and the logger chain is unchanged. |
| `business_events.write_timeout` | Go duration | `1s` | `100ms` to `10s` | `IDENTITY_BROKER_BUSINESS_EVENTS_WRITE_TIMEOUT` | 4 | Upper limit for one ledger write. The operation that logged the event waits at most this long. |
| `business_events.retention.period` | Go duration | `2160h` | `24h` or more | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD` | 6 | Age (by storage time) after which an event is deleted. `2160h` is 90 days. |
| `business_events.retention.interval` | Go duration | `1h` | `1m` to `24h` | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_INTERVAL` | 6 | Time between retention runs in each replica. |
| `business_events.retention.batch_size` | integer | `1000` | 1 to 10000 | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_BATCH_SIZE` | 6 | Maximum rows per delete statement. |

Validation (loader and `internal/config/validator.go`): values outside the valid range fail at
start with the key name. Retention keys are validated also when `enabled` is false. Retention
runs only when `enabled` is true.

## Example (`examples/config/business-events.yaml`, added in step 4, extended in step 6)

```yaml
business_events:
  enabled: true          # default false
  write_timeout: 1s      # one ledger write may delay the logging operation by at most 1 s
  retention:
    period: 2160h        # 90 days
    interval: 1h
    batch_size: 1000
```

## Deployment contract

- `docs/configuration.md`: one row per key, in the step that adds the key.
- `charts/agentic-identity-broker/`: `broker.businessEvents.*` values, `templates/configmap.yaml`,
  `values.schema.json`, and the chart README, in the step that adds the key. No new workload:
  retention runs in the broker Deployment.
- `docs/operations/business-events.md` (step 6): enabling, retention, erasure, the accepted loss
  window (ADR 048), and the rule that the ledger is not enabled in production before step 6 is
  deployed (SR-003).
