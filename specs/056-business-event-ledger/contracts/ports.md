# Contract: Definition Type, Ports, and Handler

**Feature**: `056-business-event-ledger` | FR-001 to FR-006, FR-008 to FR-011 | Research R2 to R4, R7 to R13

## 1. `internal/domain/bizevent` (delivery step 2)

The API is in [data-model.md](../data-model.md#business-event-definition). Contract:

- `MustDefine` is called only in package-level `var` declarations of the emitting service package.
- A definition is passed to the logger as the value of the key `event` and nowhere else.
- `Definition` implements `slog.LogValuer` and `fmt.Stringer`. Both return the identifier.
- The package imports only the standard library.

A guard test (`internal/adapters/telemetry`, beside `TestLoggedPrincipalsAreTyped`) type-checks
every `log/slog` call in `internal/` and `cmd/`, except ExtProc. It fails when a `bizevent.Definition`
is logged under a key other than `event`. LOGGING.md section 9 documents it in step 2.

## 2. Storage ports (`internal/ports/storage.go`)

```go
// BusinessEventAppender stores business events. Delivery step 2.
type BusinessEventAppender interface {
	// Append stores one event and sets its RecordedAt. It never joins a transaction carried by
	// ctx. A duplicate ID returns StorageError{Kind: Conflict}.
	Append(ctx context.Context, event *model.BusinessEvent) error
}

// BusinessEventReader reads business events. Delivery step 2.
type BusinessEventReader interface {
	// List returns events that match filter, ordered by (OccurredAt, ID) descending.
	List(ctx context.Context, filter BusinessEventFilter) ([]*model.BusinessEvent, error)
}

// BusinessEventRetention deletes business events. Delivery step 6.
type BusinessEventRetention interface {
	// DeleteRecordedBefore deletes up to limit events with RecordedAt before cutoff and
	// returns the number deleted.
	DeleteRecordedBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error)
	// EraseByPrincipal deletes all events whose Principal equals principal and returns the
	// number deleted. It changes no other event: a stored event is immutable.
	EraseByPrincipal(ctx context.Context, principal id.Principal) (int64, error)
}

type BusinessEventFilter struct {
	Principal *id.Principal        // nil: any principal, including none
	Events    []string             // empty: any event
	Before    *BusinessEventCursor // nil: from the newest
	Limit     int                  // 1 to 1000
}

type BusinessEventCursor struct {
	OccurredAt time.Time
	ID         id.BusinessEventID
}
```

| Adapter | Location | Notes |
|---|---|---|
| Memory | `internal/adapters/storage/memory/business_event.go` | Deep copies in and out. |
| PostgreSQL | `internal/adapters/storage/postgres/business_event.go` | `sqlx`. `Append` and the delete operations use `a.db`, not `storageExecutor(ctx)` (research R4). `EraseByPrincipal` runs its two statements in its own transaction ([data-model.md](../data-model.md)). `DeleteRecordedBefore` uses `DELETE ... WHERE id IN (SELECT id ... ORDER BY recorded_at, id LIMIT $2)`. |

`storage.Adapter` (`internal/adapters/storage/factory.go`) gets the accessors `BusinessEvents()
ports.BusinessEventAppender`, `BusinessEventReader()`, and, in step 6, `BusinessEventRetention()`.
Both adapters have compile-time assertions. All errors are `storage.StorageError` values.
Operations respect the configured storage timeouts.

## 3. Ledger handler (`internal/adapters/telemetry/ledger_handler.go`, delivery step 4)

```go
// NewLedgerHandler returns a slog.Handler that stores each record whose "event" attribute is a
// bizevent.Definition. fallback receives the failure record and must not contain this handler.
func NewLedgerHandler(appender ports.BusinessEventAppender, fallback slog.Handler,
	meter metric.Meter, writeTimeout time.Duration) slog.Handler
```

| Method | Contract |
|---|---|
| `Enabled` | True for `slog.LevelInfo` and above (FR-008). |
| `Handle` | No definition in the record or in the accumulated attributes: return nil. Otherwise build the event (research R7), call `Append` with `context.WithoutCancel(ctx)` and `writeTimeout`, record the duration, and on error follow research R8. Always return nil. |
| `WithAttrs` | Return a handler that includes the attributes, under the current group. |
| `WithGroup` | Return a handler that nests later attributes under the group. The reserved keys `event`, `principal`, `actor`, and `trace_id` are read only at the top level. |

Builder wiring (`internal/app/builder.go`), when `business_events.enabled` is true:

```go
operational := telemetry.NewPrincipalPolicyHandler(baseHandler)
ledger := telemetry.NewLedgerHandler(b.storage.BusinessEvents(), operational, meter, timeout)
b.logger = slog.New(telemetry.NewContextHandler(telemetry.NewMultiHandler(operational, ledger)))
```

When it is false, the chain is the chain on `main` today. The builder fails at start when the flag
is true and the storage adapter has no appender.

## 4. Retention service (delivery step 6)

`internal/domain/bizevent.RetentionService` runs on a ticker, as
`oauth2server.SessionCleanup.Run` does. Each tick calls `DeleteRecordedBefore(now - period, batch)`
until a call deletes fewer rows than the batch size or the context ends. It logs one `Info` record
for a run that deleted rows and one `Error` record for a failed run
([observability.md](./observability.md)).

## 5. Admin API (delivery step 6, needs approval API-ERASURE)

Draft in [admin-business-events-openapi.yaml](./admin-business-events-openapi.yaml).
`api/admin/openapi.yaml` changes only in step 6, after approval.
