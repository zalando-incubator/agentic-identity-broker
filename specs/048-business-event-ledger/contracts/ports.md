# Contract: Definition Type, Ports, and Handler

**Feature**: `048-business-event-ledger` | FR-001 to FR-006, FR-008 to FR-011 | Research R2 to R4, R7 to R13

## 1. `internal/domain/bizevent`

The API is in [data-model.md](../data-model.md#business-event-definition). Contract:

- `MustDefine` is called only in package-level `var` declarations of the emitting service package.
- A definition is passed to the logger as the value of the key `event` and nowhere else.
- `Definition` implements `slog.LogValuer` and `fmt.Stringer`. Both return the identifier.
- The package imports only the standard library.

A guard test (`internal/adapters/telemetry`, beside `TestLoggedPrincipalsAreTyped`) type-checks
every `log/slog` call in `internal/` and `cmd/`, except ExtProc. It fails when a `bizevent.Definition`
is logged under a key other than `event`. LOGGING.md section 9 documents it.

## 2. Storage ports (`internal/ports/storage.go`)

```go
// BusinessEventAppender stores business events.
type BusinessEventAppender interface {
	// Append stores one event and sets its RecordedAt. It never joins a transaction carried by
	// ctx. A duplicate ID returns StorageError{Kind: Conflict}.
	Append(ctx context.Context, event *model.BusinessEvent) error
}

// BusinessEventRetention deletes business events.
type BusinessEventRetention interface {
	// DeleteRecordedBefore deletes up to limit events with RecordedAt before cutoff, oldest
	// first, and returns the number deleted.
	DeleteRecordedBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error)
}

```

| Adapter | Location | Notes |
|---|---|---|
| Memory | `internal/adapters/storage/memory/business_event.go` | Deep copies in and out. |
| PostgreSQL | `internal/adapters/storage/postgres/business_event.go` | `sqlx`. `Append` and `DeleteRecordedBefore` use `a.db`, not `storageExecutor(ctx)` (research R4). `DeleteRecordedBefore` uses `DELETE ... WHERE id IN (SELECT id ... ORDER BY recorded_at, id LIMIT $2)`. Each method starts a `storage.*` span with `startStorageSpan` ([observability.md](./observability.md#spans)). |

No port has an update operation (FR-010). The only delete operation is the one of retention.

`storage.Adapter` (`internal/adapters/storage/factory.go`) gets the accessors `BusinessEvents()
ports.BusinessEventAppender` and `BusinessEventRetention() ports.BusinessEventRetention`.
Both adapters have compile-time assertions. All errors are `storage.StorageError` values.
Operations respect the configured storage timeouts.

## 3. Ledger handler (`internal/adapters/telemetry/ledger_handler.go`)

```go
// NewLedgerHandler returns a slog.Handler that stores each record whose "event" attribute is a
// bizevent.Definition. fallback receives the failure record with the request context. It must
// not contain this handler, and it must start with a context handler: this handler adds no
// trace_id, actor, or calling_peer.
func NewLedgerHandler(appender ports.BusinessEventAppender, fallback slog.Handler,
	writeTimeout time.Duration) slog.Handler
```

The handler obtains `otel.Meter("business_events")` itself (OPENTELEMETRY.md section 1).
Tests install an SDK meter provider with a manual reader through the OTel globals, restore the
previous provider in cleanup, and do not run those global-mutating cases in parallel.

| Method | Contract |
|---|---|
| `Enabled` | True for `slog.LevelInfo` and above (FR-008). |
| `Handle` | No definition in the record or in the accumulated attributes: return nil. Otherwise build the event (research R7), call `Append` with `context.WithoutCancel(ctx)` and `writeTimeout`, record the duration, and on error follow research R8. Always return nil. |
| `WithAttrs` | Return a handler that includes the attributes, under the current group. |
| `WithGroup` | Return a handler that nests later attributes under the group. The reserved keys `event`, `principal`, `actor`, and `trace_id` are read only at the top level. A definition under a group is not a business event and is not stored (FR-001). The onboarding checklist forbids such a call. |

Builder wiring (`internal/app/builder.go`), when `business_events.enabled` is true:

```go
fallback := telemetry.NewContextHandler(telemetry.NewPrincipalPolicyHandler(baseHandler))
ledger := telemetry.NewLedgerHandler(b.storage.BusinessEvents(), fallback, timeout)
fanout := telemetry.NewMultiHandler(baseHandler, ledger)
b.logger = slog.New(telemetry.NewContextHandler(telemetry.NewPrincipalPolicyHandler(fanout)))
```

When it is false, the chain is the chain on `main` today. The builder fails at start when the flag
is true and the storage adapter has no appender.

The failure record takes the path `ledger → context handler → principal policy → sinks`. The
context handler reads the request context, so the record has the same `trace_id` and `actor` as
the record that could not be stored. Context values survive `context.WithoutCancel`.

The principal policy handler must preserve a `bizevent.Definition` under `event` without calling
`Resolve`; otherwise its existing `render` resolves the marker to a string before the ledger
can recognise it. It still renders all principals, including groups and `WithAttrs`, before
fan-out. The ledger stores only those rendered strings, including `operator_principal` in the
attribute document. It never reconstructs an `id.Principal` or reads a raw identity from context.

No read port, filter, cursor, or read accessor is part of this feature. Tests observe storage
with explicit-column SQL, adapter-private state, or test-local append capture (research R11).
Future owner-facing readers must apply the definition visibility rule in ADR 048, Decision 10.

## 4. Retention service (`internal/domain/bizevent/retention`)

```go
// NewService returns the retention service. It needs no ledger handler and no definition.
func NewService(repo ports.BusinessEventRetention, cfg ports.BusinessEventsRetentionConfig,
	logger *slog.Logger) (*Service, error)

// Run deletes expired events immediately and then at each interval, until ctx is cancelled.
// It returns only after the current delete has finished.
func (s *Service) Run(ctx context.Context)
```

The service is in its own package, so that `internal/domain/bizevent` keeps its standard-library
imports (research R13). It follows `oauth2server.SessionCleanup`. Each run fixes its cutoff at
the run's start time minus the retention period. It calls
`DeleteRecordedBefore(cutoff, batch_size)` until a call deletes fewer rows than the batch size,
the context ends, or a call fails. Runs do not overlap. The first run starts immediately.
A failed run logs one `Error` record ([observability.md](./observability.md)) and retries at the next scheduled interval.
The service does not log retention-driven deletion.

SC-005 defines the healthy-operation age bound. Startup or re-enable can expose overdue rows.
The first successful recovery run removes every row older than its fixed cutoff, including all required batches.
Rows that expire after that cutoff are eligible for a later run.
Failures, downtime, and recovery have no fixed maximum row age. The service continues to retry while enabled.

The builder starts the service when `business_events.enabled` is true, for every storage
adapter, and stops it on shutdown. When the flag is false, the builder does not create the
service (FR-009, SR-003).
