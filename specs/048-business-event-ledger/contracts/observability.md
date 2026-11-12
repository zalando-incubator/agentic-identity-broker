# Contract: Observability

**Feature**: `048-business-event-ledger` | FR-006, FR-009, SC-004 | `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`

## New instruments

Meter scope: `business_events`, a new scope that OPENTELEMETRY.md section 2 lists after the
STD-OTEL-2 amendment (T025). No instrument has a principal, an event ID, or a caller-supplied
value as an attribute (OPENTELEMETRY.md section 5.3). The handler obtains the meter with
`otel.Meter("business_events")`; it does not accept a meter constructor argument.

| Instrument | Kind | Unit | Attributes | Recorded when |
|---|---|---|---|---|
| `business_events.write.duration` | `Float64Histogram` | `s` | `outcome` (`stored`, `failed`) | Each ledger write attempt |
| `business_events.write_failures` | `Int64Counter` | `{event}` | `error_kind` (the `storage.ErrorKind` values, and `timeout`) | Each failed or timed-out ledger write |

The event identifier is not a metric attribute in this feature. Its value set is closed (the
declared definitions), so a later feature can add it with a contract change.

## New log records

All records use a `*Context` method and a string `event`. None of them is a business event, so
none enters the ledger.

| Message | Level | `event` | Attributes | Emitted when |
|---|---|---|---|---|
| `business event write failed` | `Error` | `business_event.ledger.write_failed` | `failed_event` (identifier), `error_kind`, `duration_ms` | A ledger write fails or times out. Written to the operational sinks only. |
| `business event retention failed` | `Error` | `business_event.ledger.retention_failed` | `error_kind` | A retention run fails |

`business event write failed` has no `principal` and no `error` value: a storage error can quote
the row. The ledger handler logs it with the request context to its fallback handler. The fallback
is a context handler in front of the operational branch, so the context handler adds `trace_id`,
`actor`, and `calling_peer`, as for every other record. The ledger handler adds none of them
(LOGGING.md section 1). The fallback does not contain the ledger handler.

Retention-driven deletion produces no log record or counter metric. Retention failures still
produce the `Error` record above.

## Spans

Each repository method of the PostgreSQL adapter creates one span with the existing helper
`startStorageSpan` (OPENTELEMETRY.md section 3.1). The helper sets the kind, the status, and the
attributes as for every other storage span. No new span attribute namespace is added.

| Span | Kind | Method | Status |
|---|---|---|---|
| `storage.append.business_event` | `Client` | `Append` | `Error` when the method returns an error, as `recordStorageResult` decides |
| `storage.delete_recorded_before.business_event` | `Client` | `DeleteRecordedBefore` | The same |

The append span is a child of the request span, because the ledger write keeps the trace context
of the request. The delete span is a root span of the retention run, which has no request. The
memory adapter creates no span, as its other repositories do not. The ledger
handler creates no span of its own.

## Safety tests

- A handler test logs a business event with a `model.Secret` attribute and asserts that the
  stored row does not contain the secret (SR-001). An onboarded path that handles a credential
  adds its own test. Grant revocation handles none.
- A handler test asserts that the failure record has no `principal` attribute and none of the
  attributes of the failed event. The rule is about attributes that the ledger handler supplies.
  The `actor` attribute comes from the context handler and passes the principal log policy, as on
  every record. It can name the same person as the principal of the failed event.
- A handler-chain test with a non-identity principal policy asserts identical rendered
  `principal`, `actor`, and `operator_principal` values in the ledger and operational sinks,
  including grouped and `WithAttrs` principals, while preserving typed-event recognition (SR-002).
- An adapter test asserts the two span names and their error status, as `span_test.go` does for the existing spans.
