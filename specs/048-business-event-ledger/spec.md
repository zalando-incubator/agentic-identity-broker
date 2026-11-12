# Feature Specification: Business Event Ledger

**Feature Branch**: `048-business-event-ledger`

**Created**: 2026-11-12

**Status**: Review revisions proposed. The original design was approved by jbspeakr on 2026-11-12; revised ADR decisions and storage scope require approval as recorded in [plan.md](./plan.md#approval-items).

**Input**: Issue #188, "Activity API for the broker UI", reduced to its storage foundation. This
feature delivers the ledger, its retention, and the first stored event. Further events, the
activity API, the activity page, and the SSF transmitter are separate features that build on it.

## Scope

In scope:

- The business event definition type and the ledger handler in the logger chain (ADR 048).
- One ledger table with a flexible attribute document, repository ports, and both storage adapters.
- Append and retention ports only. Read ports belong to the future features that consume them.
- Retention: the broker deletes rows that are older than a configured period. It does not depend
  on any event type.
- The first business event: `consent.grant.revoked`. It is onboarded by the
  [onboarding checklist](./checklists/onboarding.md), which later features reuse.

Out of scope:

- Every other business event. A later feature onboards each one by the onboarding checklist.
- Erasure of the events of one principal on request. Retention limits how long a row stays.
- An API or UI that reads the ledger (issue #188 presentation requirements).
- SSF streams, Security Event Tokens, and their delivery guarantee.
- Events from ExtProc and agentgateway.
- A transactional guarantee between a state change and its ledger row (ADR 048, Decision 5).

## Clarifications

### Session 2026-11-12

- Q: Should deletion of ledger rows under the configured retention policy be logged? → A: No; retention-driven deletion does not produce a log record. Retention failure logs remain unchanged.
- Q: Does retention-driven deletion need a deleted-row counter? → A: No; retention-driven deletion does not produce a counter metric. Retention failure logs remain unchanged.


## Delivery

The feature is delivered on one branch, `048-business-event-ledger`, and in one pull request. The
pull request holds the documents and the code. [tasks.md](./tasks.md) gives the order of work.

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

1. **Given** the ledger is enabled and Alice has created a grant for an agent, **When** Alice
   revokes the grant, **Then** the ledger has exactly one row. The row has event
   `consent.grant.revoked`, Alice as principal and actor, the request's trace ID, and the agent
   ID and grant ID as attributes. The audit record of the grant creation, which is not a business
   event, has no row.
2. **Given** the ledger is enabled and Alice has revoked her grant for an agent, **When** Alice
   revokes the grant again and receives `404`, **Then** the ledger still has exactly one row.
3. **Given** the ledger is disabled, **When** Alice revokes a grant, **Then** the revocation
   succeeds, the audit record with event `consent.grant.revoked` is logged, and the ledger has no
   row.
4. **Given** the ledger is enabled and the ledger write fails, **When** Alice revokes a grant,
   **Then** the revocation succeeds with `204`, the audit record is logged, the failure counter
   increases by one, and one `Error` record reports the failed write.

---

### User Story 2 - An operator limits how long events are kept (Priority: P2)

An operator must not keep personal data longer than needed. The broker removes ledger rows that
are older than the retention period. The operator sets the period. Retention is part of the
ledger foundation: it works the same for every event type. It runs when the ledger is enabled.
One flag switches both.

**Why this priority**: The ledger holds principals. It must not store rows without retention
(ADR 048, Decision 7).

**Independent Test**: Store two events, make one older than the retention period, wait for one
retention run, and read the ledger.

**Acceptance Scenarios**:

1. **Given** the ledger holds one row older than the retention period and one newer row,
   **When** a retention run completes, **Then** the ledger holds only the newer row and no log
   record reports the deletion.

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
- A grant is deleted and the cleanup of its refresh sessions then fails: the deletion is
  committed, so the audit record of that branch is a `consent.grant.revoked` business event.
  Grant deletion ends refresh authority: local refresh checks the active grant before issuing
  tokens, even when cleanup leaves session rows behind. `refresh_cleanup=failed` reports failed
  row cleanup, not continuing authorization. The operation still returns the cleanup error
  ([continuing consent](../../docs/configuration.md#continuing-consent-for-local-refresh)).
- The ledger is disabled after it stored rows: retention does not run, and the rows stay until
  the ledger is enabled again or an operator deletes them. The operations documentation states
  this.
- Several replicas run retention at the same time: each delete is idempotent, and no row newer
  than the period is deleted.
- The ledger write times out: the result is unknown. The failure is counted, and the row can
  exist.
- The process stops between the commit and the ledger write: the row is missing. This is the
  accepted limit of ADR 048.

## Concrete Example Journeys *(mandatory)*

### Journey 1 — Alice revokes a grant and the ledger records it (User Story 1, Scenarios 1 and 2)

**Cast**: Alice (`alice@example.com`, asserted by the SSO proxy as `X-Remote-User`); agent *Data
Analysis Assistant* (`550e8400-e29b-41d4-a716-446655440000`); Alice's grant for that agent
(`990e8400-e29b-41d4-a716-446655440020`) with permission set *GitHub Read Access*
(`770e8400-e29b-41d4-a716-446655440001`). All times are 2026-11-12 UTC. All IDs are synthetic.

**Step 1 — Broker configuration (excerpt)**

```yaml
business_events:
  enabled: true   # default false; when false the ledger handler is not installed
```

All keys are in [contracts/configuration.md](./contracts/configuration.md).

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

**Step 5 — 10:16:00: Alice revokes the grant again**

```http
DELETE /api/consent/agents/550e8400-e29b-41d4-a716-446655440000/grants HTTP/1.1
Host: broker.example.com
X-Remote-User: alice@example.com
traceparent: 00-5c0d7a2e9b8f4e3d8a1b2c3d4e5f6a7b-1a2b3c4d5e6f7a8b-01

HTTP/1.1 404 Not Found
Content-Type: application/json

{"error": "not found", "message": "no active grant exists for this agent"}
```

No grant exists, so the consent service changes no state and logs no audit record. The ledger
handler receives nothing to store.

**Observable result**: The ledger has one row for the revocation. Its `event` value and attributes
equal those of the stdout record and of the OpenTelemetry log record with the same trace ID. The
second revocation adds no row.

### Journey 2 — An old event expires (User Story 2, Scenario 1)

**Cast**: the operator of the broker; Alice's row from Journey 1
(`019a7f3e-5b7b-7c1e-9d2a-3f4b5c6d7e8f`, stored 2026-11-12 10:15:00 UTC); a second row for Bob
(`bob@example.com`, `019ae5d2-1c00-7a3b-8e4f-5a6b7c8d9e0f`, stored 2027-02-10 09:00:00 UTC).

**Step 1 — Broker configuration (excerpt)**

```yaml
business_events:
  enabled: true
  retention:
    period: 2160h    # 90 days; a row older than this is deleted
    interval: 1h     # time between retention runs in each replica
    batch_size: 1000 # maximum rows per delete statement
```

**Step 2 — 2027-02-10 10:00:00: the ledger before the run**

| `id` | `event` | `principal` | `recorded_at` | Age |
|---|---|---|---|---|
| `019a7f3e-…` | `consent.grant.revoked` | `alice@example.com` | `2026-11-12T10:15:00Z` | 90 days minus 15 minutes |
| `019ae5d2-…` | `consent.grant.revoked` | `bob@example.com` | `2027-02-10T09:00:00Z` | 1 hour |

The run at 10:00:00 deletes nothing: no row is older than 90 days.

**Step 3 — 2027-02-10 11:00:00: the next run**

Alice's row is now 90 days and 45 minutes old. The run deletes it without logging the deletion.

**Observable result**: The ledger holds only Bob's row. No log record reports the deletion, and
retention-driven deletion produces no counter metric.

### Journey 3 — Alice revokes a grant while the ledger is disabled (User Story 1, Scenario 3)

**Cast**: as in Journey 1.

**Step 1 — Broker configuration (excerpt)**

```yaml
business_events:
  enabled: false   # the default; the ledger handler is not installed and retention does not run
```

**Step 2 — 10:15:00: Alice revokes the grant**

The request and the `204 No Content` response are those of Journey 1, Step 2.

**Step 3 — 10:15:00: the consent service writes its audit record**

The stdout record is the record of Journey 1, Step 3, with `"event": "consent.grant.revoked"`.

**Observable result**: The table `business_events` has no row. No `business_events.*` metric is
recorded.

### Journey 4 — The ledger write fails (User Story 1, Scenario 4)

**Cast**: as in Journey 1, with a new grant of Alice for the same agent
(`990e8400-e29b-41d4-a716-446655440021`), created after the revocation of Journey 1. A user has at
most one grant per agent. PostgreSQL rejects every insert into `business_events`.

**Step 1 — Broker configuration (excerpt)**

```yaml
business_events:
  enabled: true
  write_timeout: 1s
```

**Step 2 — 10:20:00: Alice revokes the grant**

```http
DELETE /api/consent/agents/550e8400-e29b-41d4-a716-446655440000/grants HTTP/1.1
Host: broker.example.com
X-Remote-User: alice@example.com
traceparent: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01

HTTP/1.1 204 No Content
```

**Step 3 — 10:20:00: the audit record and the failure record on stdout**

The audit record is the record of Journey 1, Step 3, with this request's grant ID and trace ID.
The ledger handler then logs one failure record to the operational sinks:

```json
{
  "time": "2026-11-12T10:20:00.131Z",
  "level": "ERROR",
  "msg": "business event write failed",
  "event": "business_event.ledger.write_failed",
  "failed_event": "consent.grant.revoked",
  "error_kind": "unknown",
  "duration_ms": 3,
  "trace_id": "0af7651916cd43dd8448eb211c80319c",
  "actor": "alice@example.com"
}
```

The record has no `principal`, no `error`, and none of the attributes of the failed event.
`error_kind` is the `storage.ErrorKind` of the failed insert; the adapter classifies an
unexpected database error as `unknown`.

**Step 4 — the metrics**

| Instrument | Attributes | Change |
|---|---|---|
| `business_events.write_failures` | `error_kind="unknown"` | +1 |
| `business_events.write.duration` | `outcome="failed"` | one measurement |

**Observable result**: The revocation succeeded and the grant is gone. The ledger has no row for
it.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The broker MUST store one ledger row for each log record whose top-level `event`
  attribute is a business event definition, and for no other record. An `event` attribute inside
  an attribute group is not top-level.
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
  When it is off, the broker stores no business event and the logger chain is the chain without
  this feature.
- **FR-008**: A business event MUST be stored independently of the configured log level.
- **FR-009**: The broker MUST delete ledger rows whose storage time is older than a configurable
  retention period, with a default of 90 days. Retention MUST run when FR-007 enables the
  ledger and MUST NOT run when the ledger is disabled. No separate key switches retention.
  Retention-driven deletion MUST NOT produce a log record or a counter metric. Retention
  failure logs remain unchanged.
- **FR-010**: A stored ledger row MUST NOT be updated by any operation. A row is removed only by FR-009.
- **FR-011**: Both storage adapters (memory and PostgreSQL) MUST implement the ledger repository.
- **FR-012**: The onboarding of `consent.grant.revoked` MUST satisfy the [onboarding checklist](./checklists/onboarding.md).

### Domain Model

Business Event Definition (value object), Business Event (append-only entity), Business Event ID
(typed ID), and the ledger repository. Invariants are in [data-model.md](./data-model.md).

### Configuration Requirements

One flag that enables the ledger (default off), a write timeout, and three retention settings.
Keys, defaults, and a YAML example are in [contracts/configuration.md](./contracts/configuration.md).

### API Requirements

- End-user API: no change.
- Admin API: no change.

### Database Requirements

- One new table, added by one migration with up and down scripts and migration tests (Principle IX).
- Promoted columns: ID, event identifier, record time, storage time, principal, actor, trace ID.
  All other attributes are one JSON document.
- One index on storage time for retention. Query indexes belong to the future production readers.
- Table, column, and index names are in [data-model.md](./data-model.md).

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: A ledger row MUST NOT contain data that LOGGING.md section 6.1 prohibits. A ledger
  handler test MUST log a business event with a credential attribute and assert that the
  credential is absent from the stored row. When an onboarded path handles a credential, a test
  on that path MUST also assert that the credential is absent from its stored row. The grant
  revocation path of this feature handles no credential.
- **SR-002**: The ledger MUST store only central principal-policy output for `principal`, `actor`,
  and `operator_principal`, identical to other sinks. It MUST NOT persist raw identities through
  another path. A future owner-facing reader MUST specify safe ownership matching across policy
  changes before exposing rows.
- **SR-003**: The ledger MUST NOT store rows without retention (ADR 048, Decision 7). FR-009
  makes retention run whenever the ledger is enabled. The retention period MUST have an upper
  limit in the configuration validation. The operations documentation MUST state the period, how
  to change it, and that rows stay when the ledger is disabled after it stored rows.
- **SR-004**: This feature MUST NOT add an endpoint that reads the ledger.

### Key Entities

- **Business Event Definition**: The single declaration of a business event: its identifier and
  authoritative owner-visibility permission. Used as the `event` value of a log record. A future
  owner-facing consumer MUST require a known owner-visible definition, a supported projection,
  and matching authenticated ownership; its allow-list cannot override a visibility denial.
- **Business Event**: One stored occurrence: ID, identifier, times, principal, actor, trace ID,
  attributes.
- **Business Event Ledger**: The table of business events and its repository.

*The glossary in `ARCHITECTURE.md` defines these terms and their relationship to domain events.*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The onboarding of `consent.grant.revoked` changes at most three production code
  files, all in the consent service package. Besides them it changes only tests and documents,
  and no migration, schema, API contract, or builder wiring.
- **SC-002**: With the ledger disabled, every existing test passes. The only changed assertions
  in existing tests are those on the audit record of the grant revocation, whose key changes
  from `action` to `event`.
- **SC-003**: In the E2E suite, a grant revocation produces exactly one ledger row, and a rejected
  revocation produces none.
- **SC-004**: On the PostgreSQL adapter, the 99th percentile of the grant revocation request
  with the ledger on is at most 5 ms above the 99th percentile with the ledger off. The
  measurement protocol is in [plan.md](./plan.md#sc-004-measurement-protocol).
- **SC-005**: During healthy continuous operation, a row MUST be deleted within one retention
  interval plus the duration of its deleting run after its storage age exceeds the retention period.
  Healthy operation means every run succeeds, completes before the next scheduled start, and starts no more than one interval apart.
  Startup, re-enable, a shorter retention period, and failed runs can leave overdue rows.
  Startup and re-enable MUST trigger an immediate run. A failed run MUST be retried at the next scheduled interval.
  The first successful recovery run MUST delete every row older than its start time minus the retention period.
  The healthy-operation bound resumes after that run completes, provided subsequent runs meet the healthy-operation conditions.
  No fixed maximum age applies during downtime, disabled operation, failed runs, or recovery.

## Assumptions

- Audit records are logged after the mutation commits, as LOGGING.md section 8 requires. The
  grant revocation call site follows this today.
- The activity page tolerates a missing row in the crash window that ADR 048 accepts, because the
  Delegations and Sessions pages show current state from the source tables.
- Retention runs in the broker process on a timer, as the existing OAuth2 record cleanup does.
  It needs no separate workload.
- Grant revocation is low volume. High-volume events such as token issuance are onboarded only
  after SC-004 is measured.
