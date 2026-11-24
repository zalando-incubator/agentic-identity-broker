# Research: Domain Event Ledger

**Feature**: `20261118-125900-domain-event-ledger` | **Date**: 2026-11-12 | **Spec**: [spec.md](./spec.md)

Checked against `main` at commit `01da81b8`. Each entry gives the decision, the rationale, and the
alternatives that were rejected. [ADR 20261118-125900-domain-events-as-typed-log-records](../../adrs/20261118-125900-domain-events-as-typed-log-records.md)
holds the architectural decisions; this file holds the design choices under it.

## R1. Where a domain event is recorded

**Decision**: Use the audit log call that LOGGING.md section 8 already requires. A typed definition
under `event` makes the record eligible for the ledger (ADR 20261118-125900-domain-events-as-typed-log-records, Decisions 1 to 3).

**Rationale**: Every domain constructor requires a `*slog.Logger` (LOGGING.md section 1), and
`forbidigo` blocks log output outside slog. One handler in `internal/app/builder.go` therefore
reaches every call site. The context handler (`internal/adapters/telemetry/context_handler.go`)
already adds `trace_id`, `actor`, and `calling_peer`.

**Alternatives**: A recorder port in each producing service (PR #381): 368 changed files and a
second catalogue. Persist every record with an `event` attribute: diagnostics such as
`session.oauth2.refresh_retry` would enter a table that is served to end users.

## R2. How the handler selects records

**Decision**: By the Go type of the `event` attribute value, `domainevent.Definition`. The handler
tests `attr.Value.Any().(domainevent.Definition)` for a `slog.KindLogValuer` value before it resolves
the value. This is the pattern of `principalPolicyHandler.render` for `id.Principal`.

**Rationale**: `slog.AnyValue` keeps a `slog.LogValuer` as `KindLogValuer` until a handler resolves
it. The JSON handler, the text handler, and the `otelslog` bridge resolve it to the identifier
string, so all sinks print what LOGGING.md section 2.2 prescribes.

**Alternatives**: A marker attribute beside `event` (for example `ledger=true`): two attributes
that can disagree, and the marker would appear in stdout. A second logger for domain events:
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
policy handler would remove the typed marker the ledger uses to select records.

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
(ADR 20261118-125900-domain-events-as-typed-log-records, Revisit). Same-transaction append: rejected by ADR 20261118-125900-domain-events-as-typed-log-records, Decision 5.

## R5. Table shape

**Decision**: One plain table, `domain_events`. Promoted columns: `id`, `event`, `occurred_at`,
`recorded_at`, `principal`, `actor`, `trace_id`. All other attributes in one `JSONB` column. No
partitioning. Details in [data-model.md](./data-model.md).

**Rationale**: The retention query needs `(recorded_at, id)`. At the user's explicit request on
2026-11-18, principal and agent history also receive B-tree indexes with `occurred_at DESC, id DESC`
as ordering keys. The principal index uses the policy-rendered column; the agent expression index
uses `attributes->>'agent_id'`, keeping the flexible document without adding a column or UUID cast.
These indexes introduce no read port or endpoint. JSONB lets new event attributes arrive without
a migration (FR-004). The first event is low volume; batched deletion keeps the migration atomic.

**Alternatives**: Per-type columns or per-type JSON Schema (PR #381): each new event needs a
schema change. Range partitioning by day with partition drop: the primary key must include the
partition key, and partition management needs `pg_partman` or a job. Revisit with measured row
counts when token events are onboarded.

## R6. Event ID

**Decision**: The handler calls `id.NewDomainEventID()` to mint a typed UUIDv7
(`013-strongly-typed-entity-ids`).

**Rationale**: UUIDv7 is time-ordered, so index inserts are local. It does not define commit order
and is not a delivery cursor (see [Consumer cursor visibility](./data-model.md#consumer-cursor-visibility)).
The ID can later be the SSF `jti` and an idempotency key for an ingest path.
`gen_ids.go` generates `id.NewDomainEventID()` with UUIDv7. Other typed UUID constructors use UUIDv4.

**Alternatives**: `BIGSERIAL`: leaks volume and cannot be minted before the insert. UUIDv4: no
order, random index inserts.

## R7. Attribute encoding

**Decision**: The handler resolves each attribute with `slog.Value.Resolve`.
Strings, booleans, integers, and floats encode as JSON scalars.
A `time.Time` encodes as RFC 3339 with nanoseconds and its original UTC offset.
A `time.Duration` encodes as integer nanoseconds. A group encodes as a JSON object.
For other values, match
`slog.JSONHandler`: an `error` without `json.Marshaler` encodes as its `Error()` string;
`json.Marshaler` takes precedence over `error`; remaining values use `encoding/json`.
If value encoding returns an error, store the JSON string `fmt.Sprintf("!ERROR:%v", err)`,
not the original value's printed representation. This also applies to non-finite floats.
These rules apply after resolution and within slog groups, not recursively inside arbitrary Go values.
The projection includes attributes from `WithAttrs` and `WithGroup`. Top-level `event`, `principal`, `actor`, and `trace_id` move to columns.
Grouped keys stay in the document. For an eligible grouped log record, the context-supplied actor and trace ID also populate their columns.
`calling_peer` stays in the document.

**Rationale**: The ledger uses the same JSON value encoding as `slog.JSONHandler` for attributes that PostgreSQL can store.
A typed ID has `MarshalJSON` and encodes as its string form. OpenTelemetry can use a different encoding for structured attributes.
See the [storage contract](./contracts/ports.md#2-storage-ports-internalportsstoragego) for JSONB restrictions and failed appends.

**Alternatives**: Store the stdout JSON line as text: not queryable and includes the message.

## R8. Failure handling

**Decision**: On an `Append` error or timeout the handler (1) increments
`domain_events.write_failures` with the storage `error_kind`, (2) writes one record at `Error`
with `event=domain_event.ledger.write_failed` to the operational branch of the chain, not to the
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
checks each branch separately. With `log.level=warn`, an `Info` domain event reaches the ledger
branch and not stdout (FR-008). `Debug` records never carry a domain event (LOGGING.md section 3).

## R10. Retention

**Decision**: Retention is part of the ledger foundation. It runs in the broker process on a
ticker, as `oauth2server.SessionCleanup` does (`internal/domain/oauth2server/session_cleanup.go`),
in the package `internal/domain/domainevent/retention`. Each run deletes rows with `recorded_at`
older than the retention period in batches. It runs when
`domain_events.enabled` is true and not otherwise: one flag switches the ledger and its
retention (maintainer decision, 2026-11-12). Retention is the only operation that
removes rows. No operation updates a row (ADR 20261118-125900-domain-events-as-typed-log-records, Decision 9).

**Rationale**: Retention does not depend on an event type, and the ledger must not store rows
without it (ADR 20261118-125900-domain-events-as-typed-log-records, Decision 7; maintainer decision, 2026-11-12). An in-process ticker needs no
CronJob and works with the memory adapter. A batched `DELETE ... WHERE id IN (SELECT ... LIMIT n)`
keeps lock time short. Several replicas run the same idempotent delete; no leader election is
needed.

`038-proactive-token-refresh` rejects a per-replica ticker for the Session Sweep, which calls providers and rotates
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

**Decision**: The identifier and one flag, owner visibility. `domainevent.MustDefine` panics at
package initialisation when the identifier does not match LOGGING.md section 2.2 or is declared
twice.

**Rationale**: The emitting service owns the resource-owner disclosure decision. A future
resource-owner consumer must look up the current definition by identifier, fail closed for an
unknown identifier or `OwnerVisible() == false`, and intersect that permission with its own
supported projections and authenticated ownership. Consumer allow-lists never override the
definition. Sentences and SSF mappings belong to consumers; SSF disclosure needs its own policy
(ADR 20261118-125900-domain-events-as-typed-log-records, Decision 10). A duplicate or malformed identifier is a programming error and
must fail at start, as `regexp.MustCompile` does.

**Alternatives**: Store the flag on each row: old rows keep an outdated flag when the declaration
changes. A central registry file: the mirror that ADR 20261118-125900-domain-events-as-typed-log-records rejects.

## R13. Import boundaries

**Decision**: `internal/domain/domainevent` imports only the standard library. Domain services import
it. The retention service is in the subpackage `internal/domain/domainevent/retention`, which imports
`ports`. The ledger handler (`internal/adapters/telemetry`) imports `domainevent`, `id`, `model`, and
`ports`. ExtProc does not import `domainevent`.

**Rationale**: `internal/domain/architecture_test.go` enforces zero infrastructure imports in the
domain. `internal/adapters/telemetry` already imports `internal/domain/id` and
`internal/domain/security`. `027-extproc-telemetry-shared-dependency` forbids broker domain imports in ExtProc; ExtProc keeps string
`event` values and is out of scope.

## R14. Configuration precedence and duplicate attributes

**Decision**: Expose all five configuration settings as dotted CLI flags through the existing
broker command and loader binding. CLI overrides environment, which overrides file values,
which override defaults. For duplicate log attributes in the same object, the final attribute
wins before event recognition and promotion; inherited attributes precede record attributes.

**Rationale**: Constitution Principle VII requires all configuration paths. The existing
`cmd/agentic-identity-broker/agent_subject_config_test.go` demonstrates command binding and
precedence tests. Last-value precedence matches decoded operational JSON and gives one
unambiguous event identifier and principal per stored occurrence.

**Alternatives**: File/environment-only settings violate Principle VII. First-value precedence
can persist a different identifier or principal from decoded operational JSON. Persisting every
duplicate cannot be represented by the promoted columns and JSONB object contract.

## R15. Append deadline and benchmark environment

**Decision**: `write_timeout` sets the context deadline immediately before `Append`, after event construction.
It does not limit encoding, instrumentation, operational sinks, or total request latency.
SC-004 uses the environment-recording protocol in [plan.md](./plan.md#sc-004-measurement-protocol).
The PR description records the environment and results of each acceptance run.

**Rationale**: The append context does not control synchronous work outside the repository call.
Host resources and database placement affect an absolute latency threshold.
An environment record makes the result reproducible without inventing a fixed CI runner.

**Alternatives considered**: A total logging deadline requires new behavior outside the approved append design.
A fixed runner specification lacks an agreed reference host. Unrecorded local measurements do not establish reproducible acceptance.

## R16. Timestamp precision across storage adapters

**Decision**: Preserve `slog.Record.Time` in UTC with nanoseconds in the handler and memory adapter.
PostgreSQL occurrence timestamps equal the input instant truncated to microseconds, not rounded.
Only `RecordedAt` is assigned back to the caller by `Append`. See [the precision contract](./data-model.md#timestamp-precision).

**Rationale**: The pinned pgx v5.11.0 `TimestamptzCodec` binary encoder computes microseconds as
`Time.Unix()*1000000 + int64(Time.Nanosecond())/1000`, discarding sub-microsecond digits.
This explains Journey 1's `.123456789Z` log time and `.123456Z` column value without adding
a conversion to the handler. Existing PostgreSQL storage fixtures also use `Truncate(time.Microsecond)`.
Adapter tests use sub-microsecond inputs and compare instants with `time.Time.Equal`.

**Alternatives considered**: Truncating in the handler or memory adapter loses precision without
a storage constraint. Rounding differs from pgx encoding. Storing an additional nanosecond column
adds schema and query complexity without a requirement. Pre-truncated test inputs hide the boundary.

## Open points

| Point | Resolved in |
|---|---|
| Approval of ADR 20261118-125900-domain-events-as-typed-log-records as a whole | Task T004 |
| Measured request latency against SC-004 | Task T053 |
