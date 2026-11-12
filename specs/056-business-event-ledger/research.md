# Research: Business Event Ledger

**Feature**: `056-business-event-ledger` | **Date**: 2026-11-12 | **Spec**: [spec.md](./spec.md)

Checked against `main` at commit `01da81b8`. Each entry gives the decision, the rationale, and the
alternatives that were rejected. [ADR 048](../../adrs/048-business-events-as-typed-log-records.md)
holds the architectural decisions; this file holds the design choices under it.

## R1. Where a business event is recorded

**Decision**: In the audit log call that LOGGING.md section 8 already requires. A typed definition
as the `event` value marks the record as a business event (ADR 048, Decisions 1 to 3).

**Rationale**: Every domain constructor requires a `*slog.Logger` (LOGGING.md section 1), and
`forbidigo` blocks log output outside slog. One handler in `internal/app/builder.go` therefore
reaches every call site. The context handler (`internal/adapters/telemetry/context_handler.go`)
already adds `trace_id`, `actor`, and `calling_peer`.

**Alternatives**: A recorder port in each producing service (PR #381): 368 changed files and a
second catalogue. Persist every record with an `event` attribute: diagnostics such as
`session.oauth2.refresh_retry` would enter a table that is served to end users.

## R2. How the handler recognises a business event

**Decision**: By the Go type of the `event` attribute value, `bizevent.Definition`. The handler
tests `attr.Value.Any().(bizevent.Definition)` for a `slog.KindLogValuer` value before it resolves
the value. This is the pattern of `principalPolicyHandler.render` for `id.Principal`.

**Rationale**: `slog.AnyValue` keeps a `slog.LogValuer` as `KindLogValuer` until a handler resolves
it. The JSON handler, the text handler, and the `otelslog` bridge resolve it to the identifier
string, so all sinks print what LOGGING.md section 2.2 prescribes.

**Alternatives**: A marker attribute beside `event` (for example `ledger=true`): two attributes
that can disagree, and the marker would appear in stdout. A second logger for business events:
call sites would hold two loggers and could log a fact to only one of them.

## R3. Position of the handler in the chain

**Decision**: `ContextHandler → MultiHandler(PrincipalPolicyHandler → sinks, LedgerHandler)`. The
ledger handler is a sibling of the principal policy handler, behind the context handler.

**Rationale**: Behind the context handler, the record already has `trace_id` and `actor`. Beside
the policy handler, the ledger receives the typed `id.Principal`, not the policy output. Today the
policy is identity. If it becomes a pseudonymisation, the ledger still stores the value that
deletion by principal and per-user reads need (ADR 048, Decision 7). `NewMultiHandler` clones the
record for each branch, so the branches cannot change each other's attributes.

**Alternatives**: Behind the policy handler: the ledger would store the rendered principal and
break when the policy changes. Before the context handler: the handler would have to read the
security context itself and duplicate `ContextHandler`.

## R4. When the row is written, and with which connection

**Decision**: In `Handle`, synchronously, after the commit. The repository `Append` uses the
connection pool directly and ignores a transaction in the context. The handler calls it with
`context.WithoutCancel(ctx)` and a timeout from configuration (default 1 s).

**Rationale**: LOGGING.md section 8 places the audit record after the commit. The PostgreSQL
adapter stores its transaction in the context (`storageTransactionKey` in
`internal/adapters/storage/postgres/transaction.go`), and `storageExecutor(ctx)` would select it.
A log call with the context of a committed transaction would then fail with "transaction has
already been committed". `Append` must therefore not use `storageExecutor`. A request context is
often cancelled when the response is written; the ledger write must not stop for that reason.

**Alternatives**: An in-memory queue with a batch writer: lower request latency, but a larger loss
window on a crash and non-deterministic tests. Deferred until high-volume events are onboarded
(ADR 048, Revisit). Same-transaction append: rejected by ADR 048, Decision 5.

## R5. Table shape

**Decision**: One plain table, `business_events`. Promoted columns: `id`, `event`, `occurred_at`,
`recorded_at`, `principal`, `actor`, `trace_id`. All other attributes in one `JSONB` column. No
partitioning. Details in [data-model.md](./data-model.md).

**Rationale**: A column is promoted only when a query filters or sorts on it: per-principal reads
(`principal`, `occurred_at`, `id`), retention and later cursors (`recorded_at`, `id`). JSONB lets
a new event type or attribute arrive without a migration (FR-004). The first onboarded events are
low volume; a plain table with a batched delete is sufficient and keeps the migration atomic.

**Alternatives**: Per-type columns or per-type JSON Schema (PR #381): each new event needs a
schema change. Range partitioning by day with partition drop: the primary key must include the
partition key, and partition management needs `pg_partman` or a job. Revisit with measured row
counts when token events are onboarded.

## R6. Event ID

**Decision**: UUIDv7, minted by the handler with `uuid.NewV7()` from `github.com/google/uuid`
v1.6.0 (already a dependency), as a typed `id.BusinessEventID` (ADR 013).

**Rationale**: UUIDv7 is time-ordered, so `(occurred_at, id)` cursors are stable and index inserts
are local. The ID can later be the SSF `jti` and an idempotency key for an ingest path.
`gen_ids.go` generates `New<Type>()` with `uuid.New()` (version 4); the handler does not use that
constructor and converts the version 7 value instead.

**Alternatives**: `BIGSERIAL`: leaks volume and cannot be minted before the insert. UUIDv4: no
order, random index inserts.

## R7. Attribute encoding

**Decision**: The handler resolves each attribute (`slog.Value.Resolve`), then encodes: string,
bool, integer, and float as JSON scalars; `time.Time` as RFC 3339 with nanoseconds in UTC;
`time.Duration` as integer nanoseconds; a group as a JSON object; any other value by
`encoding/json`, and by `fmt.Sprint` if that fails. Attributes from `WithAttrs` and `WithGroup`
are included. The keys `event`, `principal`, `actor`, and `trace_id` move to their columns and are
not repeated in the document. `calling_peer` stays in the document.

**Rationale**: This equals the output of `slog.JSONHandler`, so a ledger row and the stdout line
of the same record have the same attribute values (spec Journey 1, observable result). A typed ID
has `MarshalJSON` and encodes as its string form.

**Alternatives**: Store the stdout JSON line as text: not queryable and includes the message.

## R8. Failure handling

**Decision**: On an `Append` error or timeout the handler (1) increments
`business_events.write_failures` with the storage `error_kind`, (2) writes one record at `Error`
with `event=business_event.ledger.write_failed` to the operational branch of the chain, not to the
logger that contains the ledger handler, and (3) returns nil from `Handle`.

**Rationale**: slog discards the error of `Handle`, so returning it has no effect; a metric and a
log record make the miss visible. Writing to the operational branch prevents recursion. The
failure record has a string `event`, so even a wiring mistake could not store it.

**Alternatives**: Retry in the handler: holds the request for longer than the timeout. A dead
letter file: a second storage path with its own retention problem.

## R9. Level independence

**Decision**: The ledger handler reports `Enabled` for `Info` and above, whatever `log.level` is.
The operational branch keeps its own level.

**Rationale**: `multiHandler.Enabled` is true when one branch is enabled, and `multiHandler.Handle`
checks each branch separately. With `log.level=warn`, an `Info` business event reaches the ledger
branch and not stdout (FR-008). `Debug` records never carry a business event (LOGGING.md section 3).

## R10. Retention and erasure

**Decision**: Retention runs in the broker process on a ticker, as `oauth2server.SessionCleanup`
does (`internal/domain/oauth2server/session_cleanup.go`). Each run deletes rows with
`recorded_at` older than the retention period in batches. Erasure by principal is an Admin API
operation, `POST /api/business-events/erasure`, with the principal in the JSON body.

**Rationale**: An in-process ticker needs no CronJob and works with the memory adapter. A batched
`DELETE ... WHERE id IN (SELECT ... LIMIT n)` keeps lock time short. Several replicas run the same
idempotent delete; no leader election is needed. Erasure is rare and operator-initiated; the
Admin API is where operators act (`POST /api/sessions/sweep` is the precedent). The principal goes
in the body because LOGGING.md section 6.1 treats URL queries as sensitive and proxies log them.

Erasure deletes the principal's own events and redacts the principal from `actor` and
`operator_principal` in other owners' events, because those values are personal data too.
Deleting the other owners' events instead would remove their history; leaving the values would
make erasure incomplete.

**Alternatives**: A Helm CronJob that calls an admin endpoint for retention (session sweep
pattern): more moving parts for a delete that needs no operator decision. A CLI subcommand for
erasure: the binary has one command today and operators do not have database credentials.

**Approval**: The erasure endpoint is a new Admin API operation and needs maintainer confirmation
(plan.md, Approval Items, API-ERASURE).

## R11. Reading the ledger in this feature

**Decision**: The repository has a `List` operation with a filter (principal, event, time cursor,
limit). No HTTP endpoint uses it in this feature.

**Rationale**: E2E and service tests must assert stored rows through production code (LOGGING.md
section 9 forbids test-only APIs). The activity feature needs the same operation, and the
per-principal index exists for it.

## R12. What a definition carries

**Decision**: The identifier and one flag, owner visibility. `bizevent.MustDefine` panics at
package initialisation when the identifier does not match LOGGING.md section 2.2 or is declared
twice.

**Rationale**: A consumer that serves resource owners needs to know which events it may show; the
service that emits the event knows that. Other consumer concerns (sentence, SSF mapping) belong to
the consumer (ADR 048, Decision 9). A duplicate or malformed identifier is a programming error and
must fail at start, as `regexp.MustCompile` does.

**Alternatives**: Store the flag on each row: old rows keep an outdated flag when the declaration
changes. A central registry file: the mirror that ADR 048 rejects.

## R13. Import boundaries

**Decision**: `internal/domain/bizevent` imports only the standard library. Domain services import
it. The ledger handler (`internal/adapters/telemetry`) imports `bizevent`, `id`, `model`, and
`ports`. ExtProc does not import `bizevent`.

**Rationale**: `internal/domain/architecture_test.go` enforces zero infrastructure imports in the
domain. `internal/adapters/telemetry` already imports `internal/domain/id` and
`internal/domain/security`. ADR 027 forbids broker domain imports in ExtProc; ExtProc keeps string
`event` values and is out of scope.

## Open points

| Point | Resolved in |
|---|---|
| Approval of ADR 048, Decision 5 (after-commit write) | Delivery step 1 review |
| Approval of the erasure endpoint (API-ERASURE) | Before delivery step 6 |
| Measured write latency against SC-004 | Delivery step 5 |
