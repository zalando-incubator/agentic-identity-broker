# Feature Specification: Business Event Ledger

**Feature Branch**: `056-business-event-ledger`

**Created**: 2026-11-12

**Status**: Draft. [ADR 048](../../adrs/048-business-events-as-typed-log-records.md) is Proposed and awaits maintainer sign-off.

**Input**: Issue #188, "Activity API for the broker UI", reduced to its storage foundation. This
feature delivers the ledger and the rule to onboard events. The activity API, the activity page,
and the SSF transmitter are separate features that read the ledger.

## Scope

In scope:

- The business event definition type and the ledger handler in the logger chain (ADR 048).
- One ledger table with a flexible attribute document, a repository port, and both storage adapters.
- Retention and deletion by principal.
- The onboarding of events, one use case per pull request, by the
  [onboarding checklist](./checklists/onboarding.md). `consent.grant.revoked` is the first.

Out of scope:

- An API or UI that reads the ledger (issue #188 presentation requirements).
- SSF streams, Security Event Tokens, and their delivery guarantee.
- Events from ExtProc and agentgateway.
- A transactional guarantee between a state change and its ledger row (ADR 048, Decision 5).

## Delivery

The feature merges as a sequence of small pull requests. Each one leaves `main` releasable.

| Step | Content | Ledger state after merge |
|---|---|---|
| 1 | ADR 048, this specification, LOGGING.md section 8 amendment | No code |
| 2 | Definition type, model, repository port with the append operation, memory adapter, LOGGING.md section 2.2 rule | Not wired |
| 3 | Migration for the ledger table, PostgreSQL adapter, migration tests | Not wired |
| 4 | Ledger handler, builder wiring, configuration flag (default off), failure metric | Wired, no event onboarded |
| 5 | First use case: `consent.grant.revoked`, with the E2E scenarios of User Story 1 | One event |
| 6 | Retention and deletion by principal (User Story 3) | Can be enabled in production |
| 7 and later | One use case per pull request (User Story 2) | Grows |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A logged business event is stored (Priority: P1)

An operator enables the ledger. A user revokes a grant. The broker stores one ledger row that says
who was affected, who acted, what happened, when, and which request it belonged to. The operator
did nothing else, and the service that revoked the grant only wrote its audit record.

**Why this priority**: Every consumer (activity page, SSF) needs stored events. Without this story
there is nothing to read.

**Independent Test**: Enable the ledger, revoke a grant through the end-user API, and read the
ledger table. The test needs no consumer API.

**Acceptance Scenarios**:

1. **Given** the ledger is enabled and Alice has a grant for an agent, **When** Alice revokes the
   grant, **Then** the ledger has exactly one row with event `consent.grant.revoked`, Alice as
   principal and actor, the request's trace ID, and the agent ID and grant ID as attributes.
2. **Given** the ledger is enabled and Alice has no grant for an agent, **When** Alice revokes the
   grant and receives `404`, **Then** the ledger has no new row.
3. **Given** the ledger is disabled, **When** Alice revokes a grant, **Then** the revocation
   succeeds, the audit record is logged, and the ledger has no new row.
4. **Given** the ledger is enabled and the ledger write fails, **When** Alice revokes a grant,
   **Then** the revocation succeeds with `204`, the audit record is logged, the failure counter
   increases by one, and one `Error` record reports the failed write.

---

### User Story 2 - A contributor onboards an event with one small change (Priority: P2)

A contributor wants an existing audit record to become a business event. The contributor declares
the event next to the service and changes the log call. No migration, schema file, API contract,
or wiring changes.

**Why this priority**: The feature exists to make each use case a small, separately reviewable
pull request. This story is the rule that keeps it so.

**Independent Test**: Onboard a second event after the first. The pull request passes the
[onboarding checklist](./checklists/onboarding.md) and touches only the service package and its tests.

**Acceptance Scenarios**:

1. **Given** the ledger is enabled and an event is onboarded by the checklist, **When** the
   service performs the operation, **Then** the ledger has one row whose attributes are the
   attributes of the audit record.
2. **Given** a record whose `event` is a string identifier, **When** the service logs it,
   **Then** the ledger has no new row.

---

### User Story 3 - An operator limits how long events are kept (Priority: P3)

An operator must not keep personal data longer than needed. The broker removes ledger rows that
are older than the retention period, and an operator can remove all rows of one principal.

**Why this priority**: The ledger holds principals. It must not be enabled in production without
this story (ADR 048, Decision 7).

**Independent Test**: Insert rows with old and new timestamps for two principals. Run the
retention operation and the deletion by principal. Read the table.

**Acceptance Scenarios**:

1. **Given** rows older and newer than the retention period, **When** the retention operation
   runs, **Then** only the newer rows remain.
2. **Given** rows of Alice and of Bob, **When** an operator deletes Alice's events, **Then** no row
   has Alice as principal and Bob's rows are unchanged.

---

### Edge Cases

- The request context is cancelled after the commit and before the log call returns: the ledger
  write still completes or times out by its own timeout.
- The log call uses the context of a committed storage transaction: the ledger write uses the
  connection pool, not that transaction.
- `log.level` is `warn` or `error`: a business event logged at `Info` is still stored.
- A logger was derived with `With(...)`: attributes added that way are part of the row.
- A record carries a nested group or a `slog.LogValuer` attribute: the row holds the resolved value.
- An operator event has no resource owner: the row has no principal and is never returned by a
  per-user query.
- The process stops between the commit and the ledger write: the row is missing. This is the
  accepted limit of ADR 048.

## Concrete Example Journeys *(mandatory)*

### Journey 1 — Alice revokes a grant and the ledger records it (User Story 1, Scenario 1)

**Cast**: Alice (`alice@example.com`, asserted by the SSO proxy as `X-Remote-User`); agent *Data
Analysis Assistant* (`550e8400-e29b-41d4-a716-446655440000`); Alice's grant for that agent
(`990e8400-e29b-41d4-a716-446655440020`) with permission set *GitHub Read Access*
(`770e8400-e29b-41d4-a716-446655440001`). All times are 2026-11-12 UTC. All IDs are synthetic.

**Step 1 — Broker configuration (excerpt)**

```yaml
business_events:
  enabled: true   # default false; when false the ledger handler is not installed
```

The key name is provisional until delivery step 4 fixes it in `contracts/configuration.md`.

**Step 2 — 10:15:00: Alice revokes the grant**

```http
DELETE /api/consent/agents/550e8400-e29b-41d4-a716-446655440000/grants HTTP/1.1
Host: broker.example.com
X-Remote-User: alice@example.com
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01

HTTP/1.1 204 No Content
```

**Step 3 — 10:15:00: the consent service writes its audit record after the delete commits**

The record on stdout, as LOGGING.md prescribes. It is the same with the ledger on or off.

```json
{
  "time": "2026-11-12T10:15:00.123456789Z",
  "level": "INFO",
  "msg": "grant revoked",
  "event": "consent.grant.revoked",
  "principal": "alice@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "grant_id": "990e8400-e29b-41d4-a716-446655440020",
  "granted_permission_sets": ["770e8400-e29b-41d4-a716-446655440001"],
  "revoked_at": "2026-11-12T10:15:00.123Z",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "actor": "alice@example.com"
}
```

`valid_until` and `updated_at` are omitted here for brevity.

**Step 4 — 10:15:00: the ledger handler stores the same record**

| Column | Value |
|---|---|
| `id` | `019a7f3e-5b7b-7c1e-9d2a-3f4b5c6d7e8f` (UUIDv7, minted by the handler) |
| `event` | `consent.grant.revoked` |
| `occurred_at` | `2026-11-12T10:15:00.123456Z` |
| `principal` | `alice@example.com` |
| `actor` | `alice@example.com` |
| `trace_id` | `4bf92f3577b34da6a3ce929d0e0e4736` |
| `attributes` | `{"agent_id": "550e8400-…", "grant_id": "990e8400-…", "granted_permission_sets": ["770e8400-…"], "revoked_at": "2026-11-12T10:15:00.123Z"}` |

**Observable result**: The ledger has one row for the revocation. Its identifier and attributes
equal those of the stdout record and of the OpenTelemetry log record with the same trace ID.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The broker MUST store one ledger row for each log record whose `event` attribute is
  a business event definition, and for no other record.
- **FR-002**: A business event definition MUST render in every log sink as its LOGGING.md
  section 2.2 identifier.
- **FR-003**: A ledger row MUST contain a unique time-ordered ID, the event identifier, the record
  time, the time of storage, the principal when the record has one, the actor and trace ID when
  the request context has them, and all other record attributes.
- **FR-004**: Storing a new event type or a new attribute MUST NOT need a schema migration or a
  change to an API contract.
- **FR-005**: The ledger write MUST NOT run in the storage transaction of the state change and
  MUST NOT use a transaction that the context carries.
- **FR-006**: A failed or timed-out ledger write MUST NOT change the result of the operation that
  logged the event. It MUST increase a failure counter and produce one `Error` log record that
  does not enter the ledger handler again.
- **FR-007**: The ledger MUST be controlled by one configuration flag that is off by default.
  When it is off, the broker behaves as it does without this feature.
- **FR-008**: A business event MUST be stored independently of the configured log level.
- **FR-009**: The broker MUST delete ledger rows older than a configurable retention period, with
  a default of 90 days.
- **FR-010**: An operator MUST be able to delete all ledger rows of one principal.
- **FR-011**: Both storage adapters (memory and PostgreSQL) MUST implement the ledger repository.
- **FR-012**: Each onboarded event MUST satisfy the [onboarding checklist](./checklists/onboarding.md).

### Database Requirements

- One new table, added by one migration with up and down scripts and migration tests (Principle IX).
- Promoted columns: ID, event identifier, record time, storage time, principal, actor, trace ID.
  All other attributes are one JSON document.
- One index for per-principal reads in descending time order. One index on storage time for
  retention and for later cursor-based consumers.
- Table, column, and index names are fixed in `data-model.md` at delivery step 3.

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: A ledger row MUST NOT contain data that LOGGING.md section 6.1 prohibits. A test
  MUST assert that a credential on an onboarded path is absent from the stored row.
- **SR-002**: The ledger MUST store the principal that the service logged, taken before the
  principal log policy renders it, so that deletion by principal is exact.
- **SR-003**: The ledger MUST NOT be enabled in a production deployment before FR-009 and FR-010
  are delivered. The deployment documentation MUST state this.
- **SR-004**: This feature MUST NOT add an endpoint that reads the ledger.

### Key Entities

- **Business Event Definition**: The single declaration of a business event: its identifier and
  its consumer flags. Used as the `event` value of a log record.
- **Business Event**: One stored occurrence: ID, identifier, times, principal, actor, trace ID,
  attributes.
- **Business Event Ledger**: The table of business events and its repository.

*Glossary entries are added to `ARCHITECTURE.md` in the pull request that adds each concept to the code.*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A pull request that onboards one event after the first changes at most three
  production files and no migration, schema, or API contract file.
- **SC-002**: With the ledger disabled, every existing test passes without change.
- **SC-003**: In the E2E suite, each onboarded operation produces exactly one ledger row, and each
  rejected or unchanged operation produces none.
- **SC-004**: The ledger write adds at most 5 ms at the 99th percentile to an onboarded request on
  the PostgreSQL adapter.
- **SC-005**: After the retention operation runs, no row is older than the retention period.

## Assumptions

- Audit records are logged after the mutation commits, as LOGGING.md section 8 requires. Call
  sites that do not yet follow the standard are corrected in the pull request that onboards them.
- The activity page tolerates a missing row in the crash window that ADR 048 accepts, because the
  Delegations and Sessions pages show current state from the source tables.
- The first onboarded events are low volume (grants, sessions, approvals). Token issuance and
  token exchange are onboarded after SC-004 is measured.
- The retention operation follows the existing session sweep pattern.
