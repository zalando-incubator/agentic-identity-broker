# Contract: Configuration

**Feature**: `20261118-125900-business-event-ledger` | FR-007, FR-009 | Research R4, R10

## Keys (`ports.BusinessEventsConfig`, `mapstructure:"business_events"`)

The `retention.*` keys are the nested `ports.BusinessEventsRetentionConfig`
(`mapstructure:"retention"`), which the retention service receives.

| Key | Type | Default | Valid values | Environment variable | Description |
|---|---|---|---|---|---|
| `business_events.enabled` | boolean | `false` | `true`, `false` | `IDENTITY_BROKER_BUSINESS_EVENTS_ENABLED` | Installs the ledger handler. When false, the broker stores no business event and the logger chain is unchanged. |
| `business_events.write_timeout` | Go duration | `1s` | `100ms` to `10s` | `IDENTITY_BROKER_BUSINESS_EVENTS_WRITE_TIMEOUT` | Context deadline for the ledger `Append` call, not a limit on the entire logging operation or request. |
| `business_events.retention.period` | Go duration | `2160h` | `24h` to `8760h` | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD` | Age, by storage time, after which an event is deleted. `2160h` is 90 days. `8760h` is 365 days. |
| `business_events.retention.interval` | Go duration | `1h` | `1s` to `24h` | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_INTERVAL` | Time between retention runs in each replica. The first run is at start. |
| `business_events.retention.batch_size` | integer | `1000` | 1 to 10000 | `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_BATCH_SIZE` | Maximum rows per delete statement. |

The handler starts this deadline immediately before `Append`, after it builds the event.
The append context retains request values but ignores request cancellation and its deadline.
Attribute encoding, instrumentation, and operational sinks, including failure diagnostics, are outside this deadline.
The timeout does not guarantee that the database aborts the insert. A timed-out append has an unknown outcome.

Each key also has a CLI flag with the same dotted spelling, for example
`--business_events.enabled=true` and `--business_events.retention.period=2160h`.
Register the boolean, duration, and integer flags in `cmd/agentic-identity-broker/root.go`
and use the existing loader command binding. Precedence is CLI over environment over file
over defaults. Tests cover all five flags, precedence, and an explicit CLI `false` overriding
an enabled file value. CLI flag defaults must not shadow file or environment values.

Validation (loader and `internal/config/validator.go`): a value outside the valid range fails at
start with the key name. The retention keys are validated also when `enabled` is false.
Retention runs when `enabled` is true and not otherwise (FR-009). No separate key switches it.
If an operator disables the ledger after it stored rows, those rows stay until the ledger is
enabled again.

In [healthy operation](../spec.md#measurable-outcomes), retention deletes expired rows within one interval plus the duration of the deleting run (SC-005).
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
  write_timeout: 1s      # context deadline for Append, not total logging or request latency
  retention:
    period: 2160h        # 90 days
    interval: 1h
    batch_size: 1000
```

## Operating the ledger

Choose a retention period that meets your privacy requirements. There is no principal-erasure operation.
For PostgreSQL, apply migrations through `049_create_business_events` before you enable the ledger.
To enable it, copy the [YAML example](../../../examples/config/business-events.yaml) into the broker configuration. Then restart the broker.
For Helm, set `broker.businessEvents.enabled` to `true`.
If `storage.type=postgres` and `migration.enabled=true`, the chart runs the migration job.
If you disable that job for PostgreSQL, apply the migrations separately before you enable the ledger.

To change retention, update `business_events.retention.period` and restart every replica.
During a rolling update, a replica with a shorter period can delete rows that other replicas do not yet consider eligible.
Increasing the period does not restore deleted rows.

[20261118-125900-business-events-as-typed-log-records](../../../adrs/20261118-125900-business-events-as-typed-log-records.md) describes the post-commit loss window and the privacy limits.

## Deployment contract

- `docs/configuration.md`: one row per key.
- `charts/agentic-identity-broker/`: `broker.businessEvents.*` in the values,
  `templates/configmap.yaml`, `values.schema.json`, and the chart README. No new workload:
  retention runs in the broker Deployment.
