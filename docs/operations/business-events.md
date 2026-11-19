# Operate the business event ledger

## Storage and privacy limits

The ledger stores typed business-event audit records in `business_events`. The first event is `consent.grant.revoked`.
It adds no ledger-read API, activity UI, SSF delivery, or events from ExtProc and agentgateway.
Only an effective top-level typed `event` is stored. A later nonempty logger group named `event` replaces that value and prevents storage; an empty group does not.
PostgreSQL stores durable rows. The memory adapter loses rows when the process exits.

The ledger is disabled by default because it stores principals. The central principal policy renders `principal`, `actor`, and `operator_principal` before storage.
The ledger receives the same rendered identities as stdout and OpenTelemetry, with no raw-identity side channel.
A policy change can make historical identities incomparable. This feature does not define owner-facing queries across policy changes.

The [logging standard's prohibited data](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/coding-standards/LOGGING.md#61-never-log) is also prohibited in ledger rows.
This includes credentials, tokens, key material, encrypted payloads, cookies, raw bodies, credential-bearing connection strings, and profile data beyond the principal.
The ledger has **no principal-erasure operation**. Retention deletes whole rows, not individual fields.

## Enable the ledger

1. Choose a retention period that meets your deployment's privacy requirements.
2. For PostgreSQL, apply the production migrations through `049_create_business_events` before enablement.
3. Merge this fragment into the broker configuration:

   ```yaml
   business_events:
     enabled: true
     write_timeout: 1s
     retention:
       period: 2160h
       interval: 1h
       batch_size: 1000
   ```

4. Restart the broker with the updated configuration.

An enabled ledger always runs retention. There is no separate retention switch.
Each replica runs retention inside the broker process, immediately on startup and then at the configured interval.
Retention uses both storage adapters and requires no separate workload.

For Helm deployments, merge the corresponding values into your release configuration:

```yaml
broker:
  businessEvents:
    enabled: true
    writeTimeout: "1s"
    retention:
      period: "2160h"
      interval: "1h"
      batchSize: 1000
```

The chart maps these values into the broker ConfigMap. Its schema enforces types and batch-size bounds.
The broker validates full Go-duration syntax and duration bounds at startup, even when the ledger is disabled.
The existing chart migration job applies PostgreSQL migrations. No retention Job or CronJob is added.

## Configuration limits

| Broker key | Default | Inclusive bounds |
|---|---|---|
| `business_events.enabled` | `false` | Boolean `true` or `false` |
| `business_events.write_timeout` | `1s` | `100ms`–`10s` |
| `business_events.retention.period` | `2160h` (90 days) | `24h`–`8760h` (365 days) |
| `business_events.retention.interval` | `1h` | `1s`–`24h` |
| `business_events.retention.batch_size` | `1000` | 1–10000 rows per delete statement |

Durations use Go syntax, including fractional and compound values. For example, `0.75s` and `1h30m` are valid.
Day suffixes such as `90d` are not valid. Invalid configuration stops startup with the key name, even when the ledger is disabled.

All five keys support CLI flags, environment variables, YAML files, and defaults, in that order of precedence.
Flags use the exact dotted key, such as `--business_events.retention.period=720h`.
Environment variables use `IDENTITY_BROKER_` plus the uppercase key with underscores, such as `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD=720h`.
An explicit `--business_events.enabled=false` overrides an enabled file value. Omitted flags preserve environment and file values.
The [configuration reference](../configuration.md#business-event-configuration) lists every flag and environment variable.

## Write timeout and accepted loss

Accepted [20261118-125900-business-events-as-typed-log-records](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/adrs/20261118-125900-business-events-as-typed-log-records.md) defines a post-commit ledger, not transactional delivery.
The business mutation commits before the service logs its event. The synchronous append uses the connection pool outside that transaction.
Its deadline starts immediately before `Append`, after event construction.
The append retains request values, but ignores request cancellation and the request deadline.
Attribute encoding, instrumentation, and operational sinks, including failure diagnostics, are outside this deadline.
The timeout does not bound the entire logging operation or request.

The accepted outcomes are:

- A process stop after commit but before the log call can lose both the row and the operational audit record. No failure metric counts that loss.
- A process stop during the log call can leave either sink without the record.
- A confirmed insert failure leaves no row. The original audit record still reaches the operational sinks.
  For example, PostgreSQL JSONB rejects U+0000 in an attribute string or key, even when stdout and the memory adapter accept it.
- A timed-out append has an **unknown outcome**. The database can commit the insert after the handler stops waiting.

A failed or timed-out append does not change the business operation's result.
It increases `business_events.write_failures` and emits one operational error record with `event=business_event.ledger.write_failed`.
The original audit record still reaches the operational sinks. A timeout counter increase does not prove that a row is absent.

The PostgreSQL acceptance budget permits at most 5 ms additional p99 grant-revocation latency with the ledger enabled.
This budget is not the write timeout or a measured deployment guarantee.
The [SC-004 protocol](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/20261118-125900-business-event-ledger/plan.md#sc-004-measurement-protocol) defines the measurement.

## Change retention

CAUTION: A shorter period makes more rows eligible for permanent deletion. Increasing the period cannot restore deleted rows.

1. Set `business_events.retention.period` to the required Go duration within the inclusive bounds.
2. If necessary, adjust `retention.interval` and `retention.batch_size` within their bounds.
3. Restart every broker replica with the updated configuration.

For Helm, use `broker.businessEvents.retention.period`, `interval`, and `batchSize` in the release configuration.
A shorter period can create an overdue backlog. Different replica periods during a rolling update can apply different cutoffs.
A replica with a shorter period can delete rows that another replica still considers current.

Retention uses **storage time** (`recorded_at`), not occurrence time (`occurred_at`).
Each run samples the same clock that assigns `recorded_at` and subtracts the retention period.
This cutoff stays fixed across batches. PostgreSQL uses its own clock, not the broker clock.
Retention deletes only rows strictly older than the cutoff, oldest first.
Each statement deletes at most `batch_size` rows. The run continues until a short batch, cancellation, or an error.
Rows that expire after the fixed cutoff remain eligible for a later run.
Successful retention deletion emits neither a log record nor a deleted-row counter.

## Healthy operation and recovery

SC-005 bounds deletion delay only during healthy continuous operation.
After a row's storage age exceeds the period, deletion occurs within one interval plus the duration of its deleting run.
Healthy operation requires all three conditions:

- Every run succeeds.
- Every run finishes before the next scheduled start.
- Runs start no more than one interval apart.

Startup, re-enable, a shorter period, and failed runs can leave overdue rows.
Startup and re-enable trigger immediate cleanup. A failed run emits `event=business_event.ledger.retention_failed` and retries at the next scheduled interval.

Without concurrent retention deletes, the first successful recovery run removes all rows older than its start-time cutoff.
With concurrent replicas, a short batch can end one run while another run still has expired rows to delete.
If that competing run fails, later scheduled runs resume cleanup of the remaining backlog.
A successful run on one replica does not prove that recovery across the deployment is complete.

The healthy-operation bound resumes after overdue rows are cleared and subsequent runs meet all three healthy-operation conditions.
There is **no fixed maximum row age** during downtime, disabled operation, failed runs, or recovery.
Absence of a retention error alone does not prove deployment-wide cleanup, because successful deletion has no log record or counter.

## Disable or re-enable the ledger

To disable the ledger, set `business_events.enabled: false` and restart the broker.
For Helm, set `broker.businessEvents.enabled: false` in the release configuration.

Disabled operation stores no new business event and preserves the original logger chain. Operational audit logging continues under its configured level.
It also stops retention. Existing rows remain, even after they exceed the configured period.
Disabling is not an erasure operation or a way to keep retention active without new writes.

To resume writes and immediate cleanup, enable the ledger again and restart the broker.
Recovery follows the concurrent-replica limits in the previous section. It does not restore events missed while disabled or lost after commit.

## Governing contracts

- [Configuration contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/20261118-125900-business-event-ledger/contracts/configuration.md)
- [Handler and retention contracts](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/20261118-125900-business-event-ledger/contracts/ports.md)
- [Observability contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/20261118-125900-business-event-ledger/contracts/observability.md)
- [SC-005 retention criterion](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/20261118-125900-business-event-ledger/spec.md#measurable-outcomes)
