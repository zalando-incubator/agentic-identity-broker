# Data Model: Business Event Ledger

**Feature**: `20261118-125900-business-event-ledger` | **Spec**: [spec.md](./spec.md) | **Research**: [research.md](./research.md)

## Domain concepts

| Concept | Kind | Package | Description |
|---|---|---|---|
| Business Event Definition | Value object | `internal/domain/bizevent` | The one declaration of a business event: identifier and owner-visibility flag. Immutable. Used as the `event` value of a log record. |
| Business Event | Entity (append-only) | `internal/domain/model` | One stored occurrence of a definition. Immutable after it is stored. |
| Business Event ID | Typed ID | `internal/domain/id` | `id.BusinessEventID`, a UUIDv7 (ADR 013, research R6). |
| Business Event Ledger | Repository | `internal/ports` | Append and retention delete operations on business events. Read ports are introduced with production consumers. |

T052 adds these terms and their relationship to domain events to the `ARCHITECTURE.md` glossary alongside implementation. Until then, this document owns their design definitions.

## Business Event Definition

```go
package bizevent

// Definition is the single declaration of a business event. It renders as its identifier.
type Definition struct {
	name         string
	ownerVisible bool
}

func MustDefine(name string, opts ...Option) Definition // panics: invalid or duplicate identifier
func OwnerVisible() Option                              // the resource owner may see the event

func (d Definition) Name() string
func (d Definition) OwnerVisible() bool
func (d Definition) LogValue() slog.Value // slog.StringValue(d.name)
func (d Definition) String() string       // d.name

func Definitions() []Definition // all declared definitions, sorted by name
```

Invariants:

- The identifier matches `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2}$` (LOGGING.md section 2.2:
  `<bounded_context>.<subject>.<past_tense_verb>`).
- An identifier is declared at most one time in the process.
- The zero `Definition` is not a business event. The handler ignores it.
- `OwnerVisible()` is authoritative for resource-owner disclosure. Consumers resolve the current
  definition by identifier through `Definitions()` and deny unknown or non-owner-visible events.
  Their supported-projection allow-list can narrow, never broaden, this permission. Ownership
  authorization is also required; the flag is not authorization to disclose to an SSF receiver.

## Business Event

```go
package model

type BusinessEvent struct {
	ID         id.BusinessEventID
	Event      string         // identifier of the definition
	OccurredAt time.Time      // time of the log record, UTC
	RecordedAt time.Time      // set by storage
	Principal  *string        // policy-rendered resource owner; nil when absent
	Actor      *string        // policy-rendered request actor; nil outside a request
	TraceID    string         // empty outside a request
	Attributes map[string]any // all other record attributes (research R7)
}
```

Invariants:

- `ID` is not zero. `Event` is not empty. `OccurredAt` is not zero.
- `Attributes` has no top-level key named `event`, `principal`, `actor`, or `trace_id`. These names can occur inside groups.
- `Attributes` contains nothing that LOGGING.md section 6.1 prohibits. The invariant holds because
  the event is built only from a log record; the repository does not check content.
- A stored event is immutable (ADR 20261118-125900-business-events-as-typed-log-records, Decision 9). No port has an update operation. An event is
  removed only as a whole, by retention.

`Validate()` checks the first invariant. `Copy()` returns a deep copy, as other models do.

### Timestamp precision

The handler sets `OccurredAt` to `slog.Record.Time.UTC()`, retaining its nanoseconds rather than
substituting the write time. The memory adapter preserves that value without truncation.
PostgreSQL's `TIMESTAMPTZ` stores microseconds: through the pinned pgx codec, the stored instant
equals `input.OccurredAt.UTC().Truncate(time.Microsecond)`, not a rounded value (research R16).
`Append` does not rewrite the caller's `OccurredAt`; it sets only `RecordedAt`.

`RecordedAt` is assigned by storage: the memory adapter's clock retains its native precision,
while PostgreSQL's `now()` has microsecond precision. Compare timestamps by instant with
`time.Time.Equal`, not Go struct equality or formatted timezone spelling. Time-valued attributes
remain RFC 3339 strings with nanoseconds under research R7; column precision does not alter them.

## Table `business_events` (migration `049_create_business_events`)

```sql
CREATE TABLE business_events (
    id          UUID        PRIMARY KEY,
    event       TEXT        NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    principal   TEXT,
    actor       TEXT,
    trace_id    TEXT,
    attributes  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_business_events_event_not_empty CHECK (event <> ''),
    CONSTRAINT chk_business_events_attributes_object CHECK (jsonb_typeof(attributes) = 'object')
);

CREATE INDEX idx_business_events_recorded
    ON business_events (recorded_at, id);

CREATE INDEX idx_business_events_principal
    ON business_events (principal, occurred_at DESC, id DESC);

CREATE INDEX idx_business_events_agent
    ON business_events ((attributes->>'agent_id'), occurred_at DESC, id DESC);
```

Down migration: `DROP TABLE business_events;`. The migration is atomic and holds no data from
other tables, so apply and rollback can repeat without loss elsewhere. Both adapters store nil
`Attributes` as an empty object, so `chk_business_events_attributes_object` holds.

The principal and agent B-tree indexes support equality filtering followed by newest-first
history ordered by `(occurred_at DESC, id DESC)`. Principal filters use the stored policy-rendered
value, not a raw identity. Agent filters use `attributes->>'agent_id'`; no UUID cast, foreign key,
or promoted agent column is added. Events without an agent attribute remain valid. These indexes
are included at the user's explicit request on 2026-11-18; they do not introduce a reader contract
or authorize disclosure. Migration tests assert all three secondary index definitions and exercise
principal and agent filters with tied occurrence timestamps, absent principals, and absent agent IDs.

| Column | Source in the log record | Used by |
|---|---|---|
| `id` | Minted by the handler | Primary key; later consumers may use it as an SSF `jti` |
| `event` | `event` attribute, `Definition.Name()` | Identifies the recorded fact |
| `occurred_at` | `slog.Record.Time` | Time of the recorded fact |
| `recorded_at` | Database transaction-start time (`now()`), not commit time | Retention, not a delivery cursor without the visibility guarantee below |
| `principal` | Policy-rendered `principal` attribute | Resource-owner evidence, subject to policy changes |
| `actor` | Policy-rendered `actor` from the context handler | Investigation |
| `trace_id` | `trace_id` attribute from the context handler | Correlation with traces and logs |
| `attributes` | All other attributes | Consumers, by the canonical keys of LOGGING.md section 2.3 |

No foreign keys. A row must survive the deletion of the grant, session, or agent that it
describes; that is its purpose.

`operator_principal` stays in `attributes`. LOGGING.md section 5.1 reserves the keys `principal`,
`operator_principal`, and `actor` for principals, so these are the only places where a row holds
one. An onboarded event must not put a principal under any other attribute key (onboarding
checklist).

Every stored principal value is the central policy's rendered string, including
`operator_principal` in the attribute document. The model must not type rendered values as
`id.Principal`, re-render them, or preserve a separate raw identity. Historical values may no
longer match current policy output. The future owner-facing reader must specify that boundary.

### Event `consent.grant.revoked`

The event states that the grant row is deleted. `user_grants` remains authoritative for current
grant state; the ledger can lack a row (ADR 20261118-125900-business-events-as-typed-log-records, Consequences). Grant deletion ends refresh
authority because local refresh verifies the active grant before issuing tokens, even if
follow-up session cleanup fails. Existing access tokens remain valid until their expiry.

The event does not guarantee successful refresh-session cleanup. `refresh_cleanup=failed`
reports that the cleanup returned an error after the committed delete; its absence means the
emitting path completed cleanup successfully. Consumers must evaluate this attribute when
describing cleanup status, but must not suppress a grant-revocation signal merely because cleanup
failed. A future SSF consumer must define its own mapping and delivery guarantee (ADR 20261118-125900-business-events-as-typed-log-records).
Both committed-delete paths include `valid_until`, `updated_at`, `granted_permission_sets`, and
`revoked_at` from the deletion audit. `occurred_at` is the canonical record time. The `revoked_at`
attribute is not proof of the deletion's exact commit time.

### Consumer cursor visibility

`recorded_at` and UUIDv7 IDs do not define commit order. A transaction can commit after a consumer advances past its `(recorded_at, id)` position.
The index alone is therefore not a safe delivery cursor.

The index remains useful for retention and queries. The consuming feature defines its delivery protocol.

## Memory adapter

A slice ordered by insertion, guarded by the adapter's lock. `Append` stores a deep copy and sets
`RecordedAt`. `DeleteRecordedBefore` removes the oldest matching events, up to the limit. A
duplicate ID returns `StorageError{Kind: Conflict}` in both adapters. Same-package memory tests
inspect private state; no read API is added for tests.

## State

A business event has no state transitions. It is stored, does not change, and is deleted by
retention when `recorded_at` is older than the retention period.

## Volume estimate

Onboarded in this feature: grant revocation only, well below one event per user per day. For the
later events (grants, sessions, approvals), 10,000 active users at 5 events per user per day
give about 4.5 million rows in 90 days. Start with a plain table, the retention index, and the
principal and agent history indexes requested by the user. Further query indexes need a
production reader's query contract and measured needs.
Token issuance and exchange are not onboarded before SC-004 is measured.
