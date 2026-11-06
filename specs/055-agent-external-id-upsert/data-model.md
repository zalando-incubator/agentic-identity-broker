# Data Model: Unique Agent External IDs and Idempotent Create-or-Update

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Research**: [research.md](./research.md)

**Current decision record**: [ADR 047](../../adrs/047-unique-agent-external-id-reconcile.md) records mjungsbluth’s 2026-11-04 approval of all eight current items, including this timestamp revision: https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6625694887. The [feature-local link](./adr-047-unique-agent-external-id-upsert.md) forwards to the accepted record. Current implementation and validation evidence is in [tasks.md](./tasks.md); earlier results are historical. PostgreSQL/container acceptance remains the workflow release gate.

No new entity, aggregate, table, typed ID, or configuration is introduced. The feature changes one
invariant on the existing Agent aggregate, adds a domain operation, and adds one migration.

## Agent (existing aggregate root)

`internal/domain/storage/agent.go` — fields are unchanged.

| Field | Type | Change |
|---|---|---|
| `ID` | `id.AgentID` (UUID) | Unchanged. Stable reference for grants and all relationships. |
| `CanonicalID` | `*string` | Unchanged; still globally unique when set. |
| `ClientID` | `*id.ClientID` | Unchanged; uniqueness still depends on `multi_agent_client.enabled`. |
| `ExternalID` | `*id.ExternalID` | **New invariants**: when non-nil it MUST be non-empty, and no two Agents may hold equal values (exact, case-sensitive byte equality). `nil` remains allowed on any number of Agents. |
| Other definition fields | — | Unchanged. |

### Validation rules

| Rule | Where | Applies to | Failure |
|---|---|---|---|
| `ExternalID != nil` ⇒ `*ExternalID != ""` | `Agent.validateFields()` | create, update, reconcile, CIMD snapshot refresh | `ErrorKindValidation`, `external_id cannot be empty when provided` |
| External ID: 1–255 code points, valid UTF-8, no NUL | `storage.ValidateExternalID`, called by Agent validation and selector validation | all writes, reconcile, list filter | write `ErrorKindValidation`; selector `ErrInvalidExternalID` |
| Body external ID equals selector when present; not `null`/empty | `agents` service | reconcile | `ErrInvalidExternalID`, `body external_id must equal the selector` / `external_id must not be null` / `external_id must be 1 to 255 characters` |
| No other Agent holds the external ID | storage adapters | create, update, reconcile | `ErrorKindConflict` wrapping `ports.ErrExternalIDConflict`, `external_id is already held by another agent` |

All supplied External IDs use `storage.ValidateExternalID`: valid UTF-8, no NUL, 1–255 Unicode
code points (approved remediation F3). Ordinary writes keep the `validation failed` envelope;
selector errors use `ErrInvalidExternalID`.

## External ID (existing value object)

`id.ExternalID` (`internal/domain/id/string_ids.go`) is unchanged. Its glossary meaning changes from
"several Agents can share one External ID" to "identifies at most one Agent". It is not a Canonical
ID, Client ID, or Stable Identifier, and never selects an agent by those values.

## Reconcile operation (new domain behavior)

`agents.Service.ReconcileByExternalID(ctx, selector id.ExternalID, desired *storage.Agent, input ReconcileInput) (*storage.Agent, ReconcileOutcome, error)`

`ReconcileInput` carries only request-presence facts the handler parses:

| Field | Meaning |
|---|---|
| `CanonicalIDPresent` | `canonical_id` member present (value may be `null`) |
| `ExternalIDPresent` | `external_id` member present |
| `ExternalIDNull` | `external_id` member is `null` |

`client_id: null` never reaches the service; the handler rejects it as request-schema validation.

`ReconcileOutcome`: `ReconcileCreated`, `ReconcileUpdated`, `ReconcileUnchanged`.
Errors return zero, not a successful no-op outcome.

`Update` and existing-Agent reconciliation share one candidate-building and validation helper.
Create and that helper enforce FR-019 permission-set service coverage in the domain, using the
permission-set resolver injected by the builder (approved remediation F1/F2).

### Field policy (FR-012, Approved API-OMISSION)

| Field | Creation branch | Update branch: omitted | Update branch: `null` | Update branch: value |
|---|---|---|---|---|
| `client_id` | absent unless supplied | preserve existing | rejected by handler (`400 validation failed`) | replace; client-ID conflict rules |
| `canonical_id` | absent unless supplied | preserve existing | clear | replace; canonical conflict rules |
| `external_id` | selector | selector | `400 invalid external_id` | must equal selector |
| metadata URLs (3) | as supplied | clear | clear | replace |
| `service_requirements`, `redirect_uris`, `allowed_scopes`, `client_uris` | as supplied or empty | empty | empty | replace whole collection |
| `display_name`, `description`, `permission_sets` | required | required | rejected | replace |
| `created_at` | UTC now | preserved | preserved | preserved |
| `updated_at` | same instant as `created_at` | unchanged on no-op; UTC now on change | same | same |

### Effective-definition equality (FR-008)

Compared: canonical, client, and external IDs; display name; description; three metadata URLs;
service requirements in order (`nil` and empty `required_scopes` equal); permission declarations
in order; redirect URIs and allowed scopes in order (`nil` equals empty); client URIs as a set.
Not compared: `ID`, `CreatedAt`, `UpdatedAt`.

### Stable Agent presentation (Approved, API-ORDER)

Every Admin Agent response (POST, both PUT forms, single GET, and list/filter GET) presents
`client_uris` in ascending Go string byte order on a copied slice. Do not mutate the aggregate,
normalize URI spelling, reorder other collections, or change optional-field omission. Equality
remains set-based; stored client-URI order is not a domain invariant. Sorting the immediate write
response as well as subsequent reads guarantees identical create/update replay bodies on both stores,
independent of PostgreSQL collation or memory input order.

Timestamps follow the existing write paths. Collection PUT creation sets both timestamps to
`time.Now().UTC()`, as `POST` does. An actual update preserves `CreatedAt` and sets `UpdatedAt` to
`time.Now().UTC()`, as ID-addressed `PUT` does. No-op returns the stored timestamps without a
write. Every Admin Agent timestamp keeps the whole-second RFC 3339 presentation of Admin API
`2.1.0`, rendered as `t.UTC().Format(time.RFC3339)` ([R4](./research.md#r4--no-op-detection)). A change within the same second as the previous write may therefore present an unchanged
`updated_at`; it is not an ETag or a total ordering of writes, and the existing last-writer-wins
policy remains. No clock abstraction or minimum-advancement rule is added.

Replay tests assert exact equality of the full representation, including `updated_at`, and no
write; update tests assert the changed definition, preserved `id` and `created_at`, and a valid
whole-second `updated_at` that is not earlier than `created_at`.

### State transitions

```mermaid
stateDiagram-v2
    [*] --> Absent
    Absent --> Present: PUT (no holder) → ReconcileCreated, 201
    Present --> Present: PUT equal definition → ReconcileUnchanged, 200, no write
    Present --> Present: PUT different definition → ReconcileUpdated, 200, updated_at = now
    Absent --> Present: PUT loses external-ID create race → reread → ReconcileUnchanged/ReconcileUpdated, 200
```

A rejected request (400/409) is not a transition and writes nothing.

## Ports

| Port | Change |
|---|---|
| `ports.AgentRepository` | `GetByExternalID` replaces `ListByExternalID`, returning one Agent or `ports.ErrNotFound`. `Create`/`Update` document the external-ID conflict. |
| `ports.ErrExternalIDConflict` | New sentinel `errors.New("external_id already held by another agent")` in `internal/ports/storage.go`, wrapped by both adapters. |

## Persistence

### Memory adapter

`internal/adapters/storage/memory/agent_repository.go` adds `byExternalID map[id.ExternalID]id.AgentID`.
`Create` and `Update` check it under the write lock before any mutation (external ID first, then
canonical ID and client URIs); `Update` moves the entry when the value changes; `Delete` removes it.
`GetByExternalID` uses this map under the read lock, then clones the selected agent from the
primary map; an absent entry returns not-found. The domain list-filter method converts not-found
to `[]` for the unchanged HTTP list contract.
It never scans the registry. Move/clear/delete/reuse tests verify lookup and index consistency.

### PostgreSQL schema — migration `047_unique_agent_external_id` (Approved, DB-047)

Before (migration `001`):

```sql
external_id VARCHAR(255),
CREATE INDEX IF NOT EXISTS idx_agents_external_id ON agents(external_id) WHERE external_id IS NOT NULL;
```

After UP:

| Object | Definition |
|---|---|
| `uq_agents_external_id` | `UNIQUE INDEX ON agents (external_id) WHERE external_id IS NOT NULL` |
| `chk_agents_external_id_not_empty` | `CHECK (external_id <> '')` (NULL passes) |
| `idx_agents_external_id` | dropped; superseded by the unique index |

UP behavior, in one `DO $migration$` statement after `SET LOCAL statement_timeout = '5s'`:

1. `LOCK TABLE agents IN ACCESS EXCLUSIVE MODE NOWAIT`.
2. Inventory with explicit columns:

   ```sql
   SELECT external_id, array_agg(id ORDER BY created_at, id) AS agent_ids
   FROM agents
   WHERE external_id IS NOT NULL AND external_id <> ''
   GROUP BY external_id
   HAVING COUNT(*) > 1;

   SELECT id FROM agents WHERE external_id = '' ORDER BY created_at, id;
   ```

3. If either result is non-empty: `RAISE EXCEPTION 'agent external_id uniqueness migration blocked'
   USING DETAIL = <duplicate groups and empty-value agent IDs, first 100 of each plus remaining
   counts>, HINT = 'See docs/operations/agent-external-id-uniqueness.md; no agents were changed'`.
4. Otherwise create the unique index and check constraint and drop `idx_agents_external_id`.

DOWN, in one `DO` statement: recreate `idx_agents_external_id`, drop
`chk_agents_external_id_not_empty`, drop `uq_agents_external_id`. No row is modified.

Error mapping in `internal/adapters/storage/postgres/agent_repository.go`:

| SQLSTATE / constraint | Operation | StorageError |
|---|---|---|
| `23505` / `uq_agents_external_id` | Create, Update | `Conflict`, wraps `ports.ErrExternalIDConflict`, `external_id is already held by another agent` |
| `23514` / `chk_agents_external_id_not_empty` | Create, Update | `Validation`, `external_id cannot be empty when provided` (defense in depth; domain validation runs first) |

### Relationships preserved

`user_grants.agent_id`, `agent_client_uris.agent_id`, client credentials, and approvals reference
`agents.id`. Neither UP nor DOWN updates or deletes `agents` rows, so these relationships are
untouched (DB-004, SC-005).
