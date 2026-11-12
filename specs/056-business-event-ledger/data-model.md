# Data Model: Business Event Ledger

**Feature**: `056-business-event-ledger` | **Spec**: [spec.md](./spec.md) | **Research**: [research.md](./research.md)

## Domain concepts

| Concept | Kind | Package | Description |
|---|---|---|---|
| Business Event Definition | Value object | `internal/domain/bizevent` | The one declaration of a business event: identifier and owner-visibility flag. Immutable. Used as the `event` value of a log record. |
| Business Event | Entity (append-only) | `internal/domain/model` | One stored occurrence of a definition. Not updated, except by erasure redaction after it is stored. |
| Business Event ID | Typed ID | `internal/domain/id` | `id.BusinessEventID`, a UUIDv7 (ADR 013, research R6). |
| Business Event Ledger | Repository | `internal/ports` | Append, list, and delete operations on business events. |

Glossary entries for these four terms are added to `ARCHITECTURE.md` section 12 in the delivery
step that adds each concept to the code (steps 2 and 4).

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

## Business Event

```go
package model

type BusinessEvent struct {
	ID         id.BusinessEventID
	Event      string         // identifier of the definition
	OccurredAt time.Time      // time of the log record, UTC
	RecordedAt time.Time      // set by storage
	Principal  *id.Principal  // resource owner; nil for an operator event
	Actor      *id.Principal  // from the request security context; nil outside a request
	TraceID    string         // empty outside a request
	Attributes map[string]any // all other record attributes (research R7)
}
```

Invariants:

- `ID` is not zero. `Event` is not empty. `OccurredAt` is not zero.
- `Attributes` does not contain the keys `event`, `principal`, `actor`, or `trace_id`.
- `Attributes` contains nothing that LOGGING.md section 6.1 prohibits. The invariant holds because
  the event is built only from a log record; the repository does not check content.
- A stored event is not changed, with one exception: erasure of a principal clears that principal
  from `Actor` and from the `operator_principal` attribute of other owners' events (FR-010).
  An event is removed only by retention or erasure.

`Validate()` checks the first invariant. `Copy()` returns a deep copy, as other models do.

## Table `business_events` (migration `048_create_business_events`)

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

CREATE INDEX idx_business_events_principal
    ON business_events (principal, occurred_at DESC, id DESC)
    WHERE principal IS NOT NULL;

CREATE INDEX idx_business_events_recorded
    ON business_events (recorded_at, id);
```

Down migration: `DROP TABLE business_events;`. The migration is atomic and holds no data from
other tables, so apply and rollback can repeat without loss elsewhere.

| Column | Source in the log record | Used by |
|---|---|---|
| `id` | Minted by the handler | Primary key, cursor tie-break, later SSF `jti` |
| `event` | `event` attribute, `Definition.Name()` | Filter by event |
| `occurred_at` | `slog.Record.Time` | Order of per-principal reads |
| `recorded_at` | Database clock | Retention, later consumer cursors |
| `principal` | `principal` attribute (`id.Principal`) | Per-principal reads, erasure |
| `actor` | `actor` attribute from the context handler | Display, investigation |
| `trace_id` | `trace_id` attribute from the context handler | Correlation with traces and logs |
| `attributes` | All other attributes | Consumers, by the canonical keys of LOGGING.md section 2.3 |

No foreign keys. A row must survive the deletion of the grant, session, or agent that it
describes; that is its purpose.

`operator_principal` stays in `attributes`. LOGGING.md section 5.1 reserves the keys `principal`,
`operator_principal`, and `actor` for principals, so these are the only places where a row holds
one. Erasure of a principal P therefore does two things in one transaction:

```sql
DELETE FROM business_events WHERE principal = $1;

UPDATE business_events
SET actor = CASE WHEN actor = $1 THEN NULL ELSE actor END,
    attributes = CASE WHEN attributes->>'operator_principal' = $1
                      THEN attributes - 'operator_principal' ELSE attributes END
WHERE actor = $1 OR attributes->>'operator_principal' = $1;
```

The `UPDATE` scans without an index. Erasure is rare and operator-initiated; if it becomes slow, a
later migration adds an index on `actor`. An onboarded event must not put a principal under any
other attribute key (onboarding checklist).

## Memory adapter

A slice ordered by insertion, guarded by the adapter's lock. `Append` stores a deep copy and sets
`RecordedAt`. `List` filters and sorts by `(OccurredAt, ID)` descending. The delete operations
filter the slice. A duplicate ID returns `StorageError{Kind: Conflict}` in both adapters.

## State

A business event has no state transitions. Its lifecycle is: stored, possibly redacted by the
erasure of its actor or operator principal, then deleted by retention (`recorded_at` older than
the retention period) or by the erasure of its principal.

## Volume estimate

Onboarded in this feature: grant, session, and approval events, at most a few per user per day.
At 10,000 active users and 5 events per user per day, 90 days of retention hold about 4.5 million
rows. A plain table with two indexes is adequate. Token issuance and exchange are not onboarded
before SC-004 is measured.
