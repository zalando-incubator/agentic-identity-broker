# Contract: Configuration

**Feature**: `048-business-event-ledger` | FR-007, FR-009 | Research R4, R10

## Keys (`ports.BusinessEventsConfig`, `mapstructure:"business_events"`)

The `retention.*` keys are the nested `ports.BusinessEventsRetentionConfig`
(`mapstructure:"retention"`), which the retention service receives.

| Key | Type | Default | Valid values | Environment variable | Description |
|---|---|---|---|---|---|
| `business_events.enabled` | boolean | `false` | `true`, `false` | `IDENTITY_BROKER_BUSINESS_EVENTS_ENABLED` | Installs the ledger handler. When false, the broker stores no business event and the logger chain is unchanged. |
| `business_events.write_timeout` | Go duration | `1s` | `100ms` to `10s` | `IDENTITY_BROKER_BUSINESS_EVENTS_WRITE_TIMEOUT` | Upper limit for one ledger write. The operation that logged the event waits at most this long. |
| `business_events.retention.period` | Go duration | `2160h` | `24h` to `8760h` | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD` | Age, by storage time, after which an event is deleted. `2160h` is 90 days. `8760h` is 365 days. |
| `business_events.retention.interval` | Go duration | `1h` | `1s` to `24h` | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_INTERVAL` | Time between retention runs in each replica. The first run is at start. |
| `business_events.retention.batch_size` | integer | `1000` | 1 to 10000 | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_BATCH_SIZE` | Maximum rows per delete statement. |

Validation (loader and `internal/config/validator.go`): a value outside the valid range fails at
start with the key name. The retention keys are validated also when `enabled` is false.
Retention runs when `enabled` is true and not otherwise (FR-009). No separate key switches it.
If an operator disables the ledger after it stored rows, those rows stay until the ledger is
enabled again.

SC-005 defines the healthy-operation age bound, not a deletion deadline during failures.
Startup and re-enable trigger an immediate run. Failed runs retry at the next scheduled interval.
Without concurrent retention deletes, the first successful recovery run deletes all rows older than its start-time cutoff.
With concurrent replicas, a short batch can end one run while expired rows remain for another run to delete.
If the competing run fails before it clears the backlog, later scheduled runs resume cleanup.
A successful run on one replica does not prove that recovery across the deployment is complete.
The healthy-operation bound resumes after overdue rows are cleared and subsequent runs meet SC-005's healthy-operation conditions.
Downtime, disabled operation, and recovery can leave older rows without a fixed maximum age.

## Example (`examples/config/business-events.yaml`)

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

- `docs/configuration.md`: one row per key.
- `charts/agentic-identity-broker/`: `broker.businessEvents.*` in the values,
  `templates/configmap.yaml`, `values.schema.json`, and the chart README. No new workload:
  retention runs in the broker Deployment.
- `docs/operations/business-events.md`: how to enable the ledger, the accepted loss window
  (ADR 048), the retention period and how to change it (SR-003), and the absence of an erasure
  operation. Include the healthy-operation age bound and recovery behavior of SC-005.
