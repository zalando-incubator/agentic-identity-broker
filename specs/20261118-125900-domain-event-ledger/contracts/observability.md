# Contract: Observability

**Feature**: `20261118-125900-domain-event-ledger` | FR-006, FR-009, SC-004 | `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`

## New instruments

Meter scope: `domain_events`, a new scope that OPENTELEMETRY.md section 2 lists after the
STD-OTEL-2 amendment (T025). No instrument has a principal, an event ID, or a caller-supplied
value as an attribute (OPENTELEMETRY.md section 5.3). The handler obtains the meter with
`otel.Meter("domain_events")`; it does not accept a meter constructor argument.

| Instrument | Kind | Unit | Attributes | Recorded when |
|---|---|---|---|---|
| `domain_events.write.duration` | `Float64Histogram` | `s` | `outcome` (`stored`, `failed`) | Each ledger write attempt |
| `domain_events.write_failures` | `Int64Counter` | `{event}` | `error_kind` (the `storage.ErrorKind` values, and `timeout`) | Each failed or timed-out ledger write |

The event identifier is not a metric attribute in this feature. Its value set is closed (the
declared definitions), so a later feature can add it with a contract change.

## New log records

All records use a `*Context` method and a string `event`. Constructor warnings use
`context.Background()` because `NewLedgerHandler` receives no construction context.
Runtime write failures use the request context. None of these diagnostics enters the ledger.

| Message | Level | `event` | Attributes | Emitted when |
|---|---|---|---|---|
| `domain event write failed` | `Error` | `domain_event.ledger.write_failed` | `failed_event` (identifier), `error_kind`, `duration_ms` | A ledger write fails or times out. Written to the operational sinks only. |
| `domain event retention failed` | `Error` | `domain_event.ledger.retention_failed` | `error_kind` | A retention run fails |
| `domain event instrument creation failed` | `Warn` | `domain_event.instrument.initialization_failed` | `instrument` (one of the two instrument names above) | Once per failed instrument constructor, through the operational fallback; continue with the returned instrument as OPENTELEMETRY section 5.1 requires. |

`domain event write failed` has no `principal` and no `error` value: a storage error can quote
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
| `storage.append.domain_event` | `Client` | `Append` | Set by `recordStorageResult` |
| `storage.current_time.domain_event` | `Client` | `StorageTime` | Set by `recordStorageResult` |
| `storage.delete_recorded_before.domain_event` | `Client` | `DeleteRecordedBefore` | Set by `recordStorageResult` |

The helper leaves status unset on success. It records not found, conflict, validation, and caller cancellation as `storage.outcome`, without setting `Error`.
Other failures set `Error` and `error.type`.

The append span is a child of the request span. The clock and delete spans are root spans because retention has no request.
The memory adapter and ledger handler create no spans of their own.

## Safety tests

- A handler test logs a domain event with a `model.Secret` attribute and asserts that the
  stored row does not contain the secret (SR-001). An onboarded path that handles a credential
  adds its own test. Grant revocation handles none.
- A handler test asserts that the failure record has no `principal` attribute and none of the
  attributes of the failed event. The rule is about attributes that the ledger handler supplies.
  The `actor` attribute comes from the context handler and passes the principal log policy, as on
  every record. It can name the same person as the principal of the failed event.
- A handler-chain test with a non-identity principal policy asserts identical rendered
  `principal`, `actor`, and `operator_principal` values in the ledger and operational sinks,
  including grouped and `WithAttrs` principals, while preserving typed-event recognition (SR-002).
- Adapter tests verify the repository span names and status through the shared helpers.
- An instrument-construction failure test asserts the warning's event and bounded instrument
  attribute, no raw error cause or principal, no ledger recursion, and continued handler operation.
