# Contract: Observability

**Feature**: `056-business-event-ledger` | FR-006, FR-009, FR-010, SC-004 | `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`

## New instruments

Meter scope: `business_events`. No instrument has a principal, an event ID, or a caller-supplied
value as an attribute (OPENTELEMETRY.md section 5.3).

| Instrument | Kind | Unit | Attributes | Recorded when | Step |
|---|---|---|---|---|---|
| `business_events.write.duration` | `Float64Histogram` | `s` | `outcome` (`stored`, `failed`) | Each ledger write attempt | 4 |
| `business_events.write_failures` | `Int64Counter` | `{event}` | `error_kind` (the `storage.ErrorKind` values, and `timeout`) | Each failed or timed-out ledger write | 4 |
| `business_events.retention.deleted` | `Int64Counter` | `{event}` | `reason` (`retention`, `erasure`) | Each retention batch and each erasure, by the number of rows deleted | 6 |

The event identifier is not a metric attribute in this feature. Its value set is closed (the
declared definitions), so a later feature can add it with a contract change.

## New log records

All records use a `*Context` method and a string `event`. None of them is a business event, so
none enters the ledger.

| Message | Level | `event` | Attributes | Emitted when | Step |
|---|---|---|---|---|---|
| `business event write failed` | `Error` | `business_event.ledger.write_failed` | `failed_event` (identifier), `error_kind`, `duration_ms` | A ledger write fails or times out. Written to the operational sinks only. | 4 |
| `business events expired` | `Info` | `business_event.ledger.rows_expired` | `row_count`, `duration_ms` | A retention run deleted at least one row | 6 |
| `business event retention failed` | `Error` | `business_event.ledger.retention_failed` | `error_kind` | A retention run fails | 6 |
| `business events erased` | `Info` | `business_event.ledger.principal_erased` | `principal` (the erased principal), `operator_principal`, `row_count` | An erasure request completes | 6 |

`business event write failed` has no `principal` and no `error` value: a storage error can quote
the row. `trace_id` and `actor` come from the context handler.

`business events erased` is an audit record with a string `event` on purpose. As a business event
it would store a new row for the principal that was erased.

## Spans

No new span and no new span attribute namespace. The ledger write runs inside the request span;
the storage adapter's existing spans cover the insert.

## Safety tests

- A handler test logs a business event on a path that also handles a `model.Secret` and asserts
  that the stored row does not contain the secret (SR-001).
- A handler test asserts that the failure record does not contain the principal of the failed event.
