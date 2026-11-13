# Research: Business Event Ledger

**Feature**: `048-business-event-ledger` | **Date**: 2026-11-12 | **Spec**: [spec.md](./spec.md)

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

**Decision**: `ContextHandler → PrincipalPolicyHandler → MultiHandler(sinks, LedgerHandler)`.
The principal policy renders every principal before any sink receives the record. It preserves
the typed `event` definition without resolving it; all other attributes keep the existing policy.

**Rationale**: LOGGING.md section 5.1 requires one rendering authority and identical output at
every sink. The ledger stores policy output, not a raw identity copy. The current policy is
identity. A future policy change can make historical principal values incomparable; the future
activity reader must define safe ownership matching across that change rather than bypass policy.
`NewMultiHandler` clones the record for each branch.

**Alternatives**: Beside the policy handler: violates section 5.1 by persisting raw principals.
Before the context handler: duplicates context extraction. Resolving the definition in the
policy handler: loses the typed marker, so the ledger could not distinguish a business event.

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

**Rationale**: The retention query needs `(recorded_at, id)`. Principal and trace columns retain
the record's canonical identity and correlation facts; no principal-query index or read contract
is introduced without a production reader. JSONB lets new event attributes arrive without a
migration (FR-004). The first event is low volume; batched deletion keeps the migration atomic.

**Alternatives**: Per-type columns or per-type JSON Schema (PR #381): each new event needs a
schema change. Range partitioning by day with partition drop: the primary key must include the
partition key, and partition management needs `pg_partman` or a job. Revisit with measured row
counts when token events are onboarded.

## R6. Event ID

**Decision**: UUIDv7, minted by the handler with `uuid.NewV7()` from `github.com/google/uuid`
v1.6.0 (already a dependency), as a typed `id.BusinessEventID` (ADR 013).

**Rationale**: UUIDv7 is time-ordered, so index inserts are local. It does not define commit order
and is not a delivery cursor (see [Consumer cursor visibility](./data-model.md#consumer-cursor-visibility)).
The ID can later be the SSF `jti` and an idempotency key for an ingest path.
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
logger that contains the ledger handler, and (3) returns nil from `Handle`. After a timeout the result of the insert is unknown; the row can
exist.

**Rationale**: slog discards the error of `Handle`, so returning it has no effect; a metric and a
log record make the miss visible. Writing to the operational branch prevents recursion. The
failure record enters that branch through a second context handler with the request context, so
it gets `trace_id` and `actor` from the owner of these attributes and the ledger handler adds
none (LOGGING.md section 1). The
failure record has a string `event`, so even a wiring mistake could not store it.

**Alternatives**: Retry in the handler: holds the request for longer than the timeout. A dead
letter file: a second storage path with its own retention problem.

## R9. Level independence

**Decision**: The ledger handler reports `Enabled` for `Info` and above, whatever `log.level` is.
The operational branch keeps its own level.

**Rationale**: `multiHandler.Enabled` is true when one branch is enabled, and `multiHandler.Handle`
checks each branch separately. With `log.level=warn`, an `Info` business event reaches the ledger
branch and not stdout (FR-008). `Debug` records never carry a business event (LOGGING.md section 3).

## R10. Retention

**Decision**: Retention is part of the ledger foundation. It runs in the broker process on a
ticker, as `oauth2server.SessionCleanup` does (`internal/domain/oauth2server/session_cleanup.go`),
in the package `internal/domain/bizevent/retention`. Each run deletes rows with `recorded_at`
older than the retention period in batches. It runs when
`business_events.enabled` is true and not otherwise: one flag switches the ledger and its
retention (maintainer decision, 2026-11-12). Retention is the only operation that
removes rows. No operation updates a row (ADR 048, Decision 9).

**Rationale**: Retention does not depend on an event type, and the ledger must not store rows
without it (ADR 048, Decision 7; maintainer decision, 2026-11-12). An in-process ticker needs no
CronJob and works with the memory adapter. A batched `DELETE ... WHERE id IN (SELECT ... LIMIT n)`
keeps lock time short. Several replicas run the same idempotent delete; no leader election is
needed.

ADR 038 rejects a per-replica ticker for the Session Sweep, which calls providers and rotates
refresh tokens under a row lock. This retention job makes no provider calls and deletes only
expired rows. Concurrent replicas can repeat scans and wait on row locks, but do not need a
leader for correctness. After another replica deletes a batch, a delete can return fewer rows
and end that run while expired rows remain. If the competing run then fails, later scheduled
runs resume cleanup. A successful run on one replica does not prove that recovery across the
deployment is complete. Without concurrent retention deletes, a successful recovery run clears
all rows older than its fixed cutoff. The healthy-operation age bound resumes after overdue
rows are cleared and subsequent runs meet SC-005's healthy-operation conditions. It is not a
load-independent guarantee. This follows the existing local `SessionCleanup`, which
ARCHITECTURE.md distinguishes from provider-session maintenance. Replica count still multiplies
database work; measure it before increasing event volume rather than adding coordination now.

**Alternatives**: A Helm CronJob that calls an admin endpoint (session sweep pattern): more moving
parts for a delete that needs no operator decision. Retention that runs also with the ledger
disabled: it removes leftover rows after an operator disables the ledger, but it adds a job to
every deployment that never enabled the ledger; rejected for one switch and no job when off. Retention in a later feature: the ledger
could then store principals without a limit. An Admin API operation that erases the events of
one principal: not necessary for the first iteration (maintainer decision, 2026-11-12). If
erasure on request is needed later, it must delete whole rows.

## R11. Reading the ledger in this feature

**Decision**: This feature exposes only append and retention ports. No production reader,
filter, cursor, or read accessor is introduced before an activity or SSF consumer exists.

**Rationale**: Ports expose operations production consumers need (`internal/ports/AGENTS.md`).
PostgreSQL integration and E2E tests inspect rows with explicit-column SQL in test code. Memory
repository tests inspect private state in the adapter package. Handler and consent tests use a
test-local capturing appender; no production inspection API is needed. Future consumers design
their own authorization, identity-policy, filtering, and pagination contracts.

## R12. What a definition carries

**Decision**: The identifier and one flag, owner visibility. `bizevent.MustDefine` panics at
package initialisation when the identifier does not match LOGGING.md section 2.2 or is declared
twice.

**Rationale**: The emitting service owns the resource-owner disclosure decision. A future
resource-owner consumer must look up the current definition by identifier, fail closed for an
unknown identifier or `OwnerVisible() == false`, and intersect that permission with its own
supported projections and authenticated ownership. Consumer allow-lists never override the
definition. Sentences and SSF mappings belong to consumers; SSF disclosure needs its own policy
(ADR 048, Decision 10). A duplicate or malformed identifier is a programming error and
must fail at start, as `regexp.MustCompile` does.

**Alternatives**: Store the flag on each row: old rows keep an outdated flag when the declaration
changes. A central registry file: the mirror that ADR 048 rejects.

## R13. Import boundaries

**Decision**: `internal/domain/bizevent` imports only the standard library. Domain services import
it. The retention service is in the subpackage `internal/domain/bizevent/retention`, which imports
`ports`. The ledger handler (`internal/adapters/telemetry`) imports `bizevent`, `id`, `model`, and
`ports`. ExtProc does not import `bizevent`.

**Rationale**: `internal/domain/architecture_test.go` enforces zero infrastructure imports in the
domain. `internal/adapters/telemetry` already imports `internal/domain/id` and
`internal/domain/security`. ADR 027 forbids broker domain imports in ExtProc; ExtProc keeps string
`event` values and is out of scope.

## Open points

| Point | Resolved in |
|---|---|
| Approval of ADR 048 as a whole | Task T004 |
| Measured request latency against SC-004 | Task T053 |
