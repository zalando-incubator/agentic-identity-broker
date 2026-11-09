# Research: Unique Agent External IDs and Idempotent Create-or-Update

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-10-30

**Current decision record**: [ADR 047](../../adrs/047-unique-agent-external-id-reconcile.md) records mjungsbluth’s 2026-11-04 approval of all eight current items, including this timestamp revision: https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6625694887. The [feature-local link](./adr-047-unique-agent-external-id-upsert.md) forwards to the accepted record. Current implementation and validation evidence is in [tasks.md](./tasks.md); earlier results are historical. PostgreSQL/container acceptance remains the workflow release gate.

The Technical Context has no open `NEEDS CLARIFICATION` items. Each section records one design
decision, its rationale, and the main alternatives. Items marked **Proposed** are registered in
[plan.md](./plan.md#approval-items) and need maintainer sign-off.

## R1 — Where the uniqueness invariant lives

**Decision**: Enforce uniqueness in each storage adapter, where the write is atomic, and map a
violation to `StorageError{Kind: Conflict}` that wraps a new sentinel
`ports.ErrExternalIDConflict`.

- PostgreSQL: partial unique index `uq_agents_external_id ON agents (external_id) WHERE
  external_id IS NOT NULL`. `Create` and `Update` map SQLSTATE `23505` with that constraint name
  to the sentinel. This follows `uq_agents_canonical_id` from migration `029`.
- Memory: a `byExternalID map[id.ExternalID]id.AgentID` index maintained under the existing
  repository mutex, checked in `Create` and `Update` before any index mutation, and released on
  `Delete`. This mirrors `byCanonicalID`.
- Domain: `storage.Agent.validateFields()` rejects a non-nil empty `ExternalID`
  (`external_id cannot be empty when provided`). Create, update, and reconcile all pass through it.

**Rationale**: Only the store can make "check then write" atomic across processes (FR-001, FR-009,
DB-002). A domain pre-check without a constraint would race, as the existing `client_id` check
does. A sentinel lets the domain service tell an external-ID collision apart from canonical-ID,
client-ID, and client-URI conflicts without parsing messages (ADR 004 error wrapping).

**Alternatives considered**:

- Domain-only pre-check through `ListByExternalID`: racy under concurrent writers; rejected.
- Full (non-partial) `UNIQUE` constraint: PostgreSQL treats NULLs as distinct, so it would also
  work; rejected only to match the established `uq_agents_canonical_id` partial-index style.
- PostgreSQL advisory locks keyed by the external ID: adds a second serialization mechanism and
  does not protect the existing POST and ID-addressed PUT paths by itself.

## R2 — Repository port shape

**Decision (approved remediation F4)**: Replace the list-shaped port with
`AgentRepository.GetByExternalID`, returning one Agent or not-found. Memory resolves `byExternalID`
under the read lock and clones the selected Agent; PostgreSQL uses the unique external-ID index.
The domain list-filter method retains the HTTP zero-or-one array contract.

**Rationale**: ADR 004 and Principle IX cap repository interfaces. `AgentRepository` already
exceeds the guideline, and a dedicated upsert method would duplicate the validated
`Create`/`Update` transactions, client-URI replacement, and reference checks. Conflict-driven
retry (R3) gives the required race behavior on both adapters with the existing methods.

**Alternatives considered**:

- `UpsertByExternalID` repository method using `INSERT … ON CONFLICT (external_id) DO UPDATE`:
  cannot express the domain field policy (FR-012), the no-op rule (FR-008), or client-ID
  uniqueness, which lives in the service; it would also need a parallel memory implementation.
- A separate `AgentExternalIDRepository` interface: not needed; replace the method on the
  existing Agent repository without adding an interface or duplicating persistence transactions.

## R3 — Reconcile algorithm and race handling

**Decision**: `agents.Service.ReconcileByExternalID(ctx, selector, desired, presence)` runs:

1. Validate the selector with the feature 054 rules (1–255 code points, valid UTF-8, no NUL) and
   the body external ID (omitted → selector; `null`, empty, or different → `ErrInvalidExternalID`).
2. `GetByExternalID(selector)`.
3. **No match**: build the creation definition (FR-012 creation column), validate service
   requirements and client-ID uniqueness as `Create` does, and call `repo.Create`. On
   any conflict, re-read by selector once. If an agent now holds the selector, continue with step 4
   against it; otherwise return the original conflict. If step 4's update finds the agent deleted,
   run step 3 once.
4. **One match**: apply the FR-012 update policy to obtain the effective definition. If it equals
   the stored definition (R4), return the stored agent with outcome `unchanged` and write nothing.
   Otherwise set `UpdatedAt` to UTC now (R4), validate as `Update` does (client-ID uniqueness excludes the selected
   agent), and call `repo.Update`; outcome `updated`.
5. Return the agent and an outcome (`created`, `updated`, `unchanged`). The handler maps `created`
   to `201` + `Location` and the others to `200`.

Step 3 re-reads after any conflict: a concurrent identical first call can win on `canonical_id` or
another unique key before `external_id`, so retrying only the external-ID sentinel returned `409`
to callers FR-009 requires to get `200` (revised after review on 2026-11-05). Without a holder the
conflict is real. Retry remains bounded to one reread; persistent conflicts return `409` (FR-010).

**Rationale**: Satisfies FR-005, FR-008, FR-009, FR-013, and SC-003 on both adapters without
locking or a new port. Each write is a single existing transaction, so a definition is never
applied partially. Without `If-Match` (out of scope), concurrent different definitions are
last-writer-wins, as the spec's edge cases allow.

**Alternatives considered**:

- `SELECT … FOR UPDATE` inside a repository transaction: needs a new port method and still needs
  conflict handling for the insert race.
- Unbounded retry loops: can spin under adversarial concurrent renames; one retry is enough
  because a committed holder is visible on re-read.

## R4 — No-op detection

**Decision**: An unexported domain function compares the effective definition with the stored
agent field by field: canonical, client, and external IDs; display name; description; the three
metadata URLs; service requirements (order-sensitive, with `nil` and empty `required_scopes` equal);
permission declarations (order-sensitive); redirect URIs and allowed scopes (order-sensitive, `nil`
equals empty); and client URIs as a set. Timestamps are excluded.

**Stable ordering (Approved, API-ORDER)**: Sort a copied `client_uris` slice in ascending Go
string byte order in every Admin Agent response (POST, both PUT forms, single GET, list/filter
GET), including the immediate create/update response. Preserve URI spelling, optional omission,
other collection order, and stored order. PostgreSQL retrieval order/collation and memory input
order cannot then change a replay representation. Test creation and actual update with at least
two valid client URIs submitted in reverse lexical order, followed by identical and reordered-set
replays on both stores and under both reference preferences.

**Timestamps (revised 2026-11-04; API-TIMESTAMPS withdrawn)**: Reuse the existing write-path
assignment. Collection PUT creation sets both timestamps to `time.Now().UTC()`, as `POST` does; an
actual reconcile update preserves `CreatedAt` and sets `UpdatedAt` to `time.Now().UTC()`, as
ID-addressed `PUT` does. No-op writes nothing and returns the stored times. Every Admin Agent
timestamp keeps the whole-second RFC 3339 presentation of Admin API `2.1.0`, formatted from the
UTC instant (`t.UTC().Format(time.RFC3339)`) so that a creation response and a later PostgreSQL
read of the same instant render identically; the `Z` form matches the documented examples. A change
within the same second as the previous write may present an unchanged `updated_at`. R8 needs only
that a replay without an intervening change returns the stored representation, which holds because
a no-op performs no write. No schema, clock abstraction, or minimum-advancement rule is added.

**Rationale**: FR-008 and FR-012 need "same effective state" semantics. Set equality alone does
not stabilize write/replay presentation, so client-URI ordering is explicit at the presentation
boundary. Other collections already round-trip in order in both adapters. Keeping the released
timestamp presentation avoids a response change for every Admin Agent consumer; `updated_at` was
never a revision, and the issue requires only R8. Tests use real store read-back, update then
replay, exact replay equality, and unchanged audit/write counts without sleeps or test-only
production APIs.

**Alternatives considered**: Preserve input order (needs persisted ordinal positions in PostgreSQL);
compare rendered JSON (depends on `Prefer`); always write then restore timestamps (still mutates);
microsecond presentation with a `+1 microsecond` minimum advancement (the withdrawn API-TIMESTAMPS:
changes every Admin Agent response and can lead wall time for a guarantee the issue does not
require); whole-second presentation with a full-second minimum advancement (larger wall-clock
drift); nanosecond persistence (PostgreSQL cannot round-trip it). The ordering change is covered by the maintainer approval recorded in ADR 047 because existing
response behavior is externally visible.

## R5 — HTTP adapter shape

**Decision**: Register `r.Put("/", h.Agents.ReconcileAgent)` inside the existing `/agents` route
group in `SetupAdminRoutes()`. The handler:

- parses the selector with the existing `externalIDFromQuery`; absent selector →
  `400 invalid external_id` / `external_id selector is required`;
- reads the body with the existing 64 KiB limit, decodes `AgentRequest`, and inspects raw members
  for `canonical_id`, `client_id`, and `external_id` presence and `null`;
- rejects `client_id: null` with `400 validation failed`
  (`client_id must be a nonempty string when supplied`), because `AgentCreateRequest.client_id` is
  not nullable;
- reuses the existing permission-set, service-requirement, coverage, and client-URI request
  validation, extracted into one private helper shared with `CreateAgent` and `UpdateAgent` so the
  three operations cannot drift;
- calls the domain service after `FinalizeRequestSecurityContext`;
- always sets `Vary: Prefer` and applies canonical references with `Preference-Applied` when
  requested, as `GetAgent` does; sets `Location: /api/agents/{uuid}` on `201`.

Error mapping: `ErrInvalidExternalID` → `400 invalid external_id`; other validation →
`400 validation failed`; conflicts → `409 conflict` with the store message. The external-ID
conflict message is `external_id is already held by another agent` on every operation.

**Rationale**: Constitution XII (routing only registers), Principle VI (field policy, selector
rules, and race handling live in the domain service; the handler only parses input), and the
existing admin error envelope (API-003).

**Alternatives considered**: A separate `/api/agents/by-external-id/{value}` path. Rejected
because the issue fixes the shape (R5) and external IDs can contain `/`, which path parameters
handle poorly behind proxies.

## R6 — Error values on existing operations (Approved, API-UNIQUE)

**Decision**: Existing POST and ID-addressed PUT return `400 validation failed` for an empty
`external_id` (domain validation) and `409 conflict` / `external_id is already held by another
agent` for a duplicate. The ID-addressed PUT documents its `409` response, which today exists for
canonical-ID and client-ID conflicts but is undocumented.

**Rationale**: Reuses the existing envelope of each operation. Main alternative: a new
`invalid external_id` error value on create and update.

## R7 — Admin API version (Approved, API-VERSION)

**Decision**: Bump `api/admin/openapi.yaml` `info.version` from `2.1.0` to `3.0.0`; keep all
routes. Released tags since `v0.1.118` contain the feature 054 filter, and the latest tag carries
Admin API `2.1.0`, which accepts duplicate and empty external IDs on create and update.

**Rationale**: ARCHITECTURE.md §5 says a confirmed breaking change increments the affected API's
major version on the existing route. Feature 051 followed the same path (`1.0.0` → `2.0.0`).
Main alternative: `2.2.0`, treating uniqueness as additive; rejected because existing clients that
rely on shared external IDs would break silently.

## R8 — Migration design (Approved, DB-047)

**Decision**: One guarded, atomic migration pair `047_unique_agent_external_id.{up,down}.sql`.

UP is one `DO $migration$` block, so it applies or fails as a single statement regardless of how
the `golang-migrate` driver batches statements:

1. `LOCK TABLE agents IN ACCESS EXCLUSIVE MODE NOWAIT`, as migration `044` does — blocks
   concurrent agent writers for the check, index build, and constraint addition without a lock
   upgrade, and fails immediately instead of queuing behind a long transaction. A preceding
   `SET LOCAL statement_timeout = '5s'` bounds the whole block.
2. Inventory: aggregate duplicate values with their agent UUIDs and empty values with their agent
   UUIDs using explicit columns. If either set is non-empty, `RAISE EXCEPTION` with
   `MESSAGE 'agent external_id uniqueness migration blocked'`, `DETAIL` listing the first 100
   duplicate groups with affected UUIDs and the first 100 empty-value UUIDs, with remaining counts,
   and `HINT` naming the runbook's complete inventory queries. Nothing is changed. The message
   covers both categories in one report (US3.2).
3. `CREATE UNIQUE INDEX uq_agents_external_id ON agents (external_id) WHERE external_id IS NOT NULL`.
4. `ALTER TABLE agents ADD CONSTRAINT chk_agents_external_id_not_empty CHECK (external_id <> '')`.
5. `DROP INDEX idx_agents_external_id` — the unique index now serves the filter.

DOWN is one `DO` block that recreates `idx_agents_external_id`, drops the check constraint and
the unique index, and deletes no row.

The agents table is an administrative registry with low cardinality, so a non-concurrent
transactional index build is acceptable; ADR 039's non-atomic exception is not needed and not
extended. `golang-migrate` marks the version dirty when UP fails; the runbook tells operators to
`migrate force 46` only after confirming the schema is unchanged, then retry.

**Rationale**: FR-004, DB-001 to DB-004, and US3 require a fail-safe, non-destructive upgrade with
an actionable report. Value and agent IDs are governance identifiers, not credentials (SR-003).
`DETAIL` is truncated to the first 100 duplicate groups and 100 empty values with a count of the
remainder; the runbook's read-only queries list everything.

**Alternatives considered**:

- Automatic cleanup (clear empties, suffix duplicates): forbidden by FR-004 and DB-001.
- A pre-flight Go command: no such hook exists in the migration image (ADR 009, ADR 039).
- `CREATE INDEX CONCURRENTLY`: needs the non-atomic exception and is unnecessary at this size.

## R9 — Operator runbook

**Decision**: New `docs/operations/agent-external-id-uniqueness.md`, linked from
`docs/operations/deployment-checklist.md` and the agent-management guide. It covers:

- read-only inventory SQL for duplicates and empties with explicit columns (`id`, `canonical_id`,
  `display_name`, `external_id`, `created_at`, `updated_at`);
- coordinating writers: pause governance sync jobs and administrative writes, or scale the broker
  to zero, while remediating and migrating;
- deliberate remediation through the Admin API (`PUT /api/agents/{agent-id}` with a distinct
  external ID or with `external_id` omitted to clear it), never `DELETE`;
- retry, including dirty-version inspection and `migrate force 46` after confirming the unchanged
  schema;
- verification queries for the index, the constraint, and unchanged agent and grant counts;
- rollback with `migrate down 1` and reapply prerequisites.

**Rationale**: FR-004 and DB-003 require runbook procedures with explicit result columns.

## R10 — Superseding feature 054 behavior (Approved, DEC-054)

**Decision**: Feature 054's non-uniqueness clarification and FR-011 are superseded by this
feature. `specs/054-agent-external-id-filter/` stays unchanged as a historical record. Its E2E file
`tests/e2e/agent_external_id_filter_test.go` currently seeds two agents with one external ID and
asserts both are returned. Implementation removes that superseded `It` and its shared-ID fixture.
The new suite contains exactly one US4.1 `It` for zero-or-one lookup results; US2.1 separately covers
rejection of the second creation. This avoids two tests mapping to US4.1. The ARCHITECTURE.md glossary entry for
External ID and the agent-management guide drop "several Agents can share one External ID".

**Rationale**: Principle XIII forbids an E2E test that asserts superseded behavior; Principle V
requires the glossary to match.

## R11 — Testing approach

**Decision**:

- **E2E** (`tests/e2e/agent_external_id_upsert_test.go`, Ginkgo): one `It` per US1–US4 scenario
  through the production bootstrap. US1.4 uses two broker instances on one PostgreSQL fixture
  (`Label("docker")`) to exercise the cross-process race, as
  `cimd_private_key_jwt_replay_test.go` does. US3 scenarios use a new bootstrap helper that
  migrates a PostgreSQL fixture to version `46`, seeds legacy rows, attempts `47`, and then starts
  the broker on the migrated database.
- **Integration** (`tests/integration/migrations/agent_external_id_migration_test.go`): migration
  `046` blocking on duplicates, empties, and both; clean apply; rollback and reapply preserving
  agents, permission declarations, client URIs, and grants; concurrent inserts rejected by the
  index.
- **Unit**: domain service reconcile table tests with a fake repository (selector validation,
  FR-012 table rows, no-op detection, conflict retry); `storage.Agent` empty external-ID
  validation; memory adapter uniqueness under `-race`; PostgreSQL adapter conflict mapping;
  handler tests for selector parsing, `client_id: null`, presence detection, status codes,
  `Location`, and `Prefer` headers.

**Rationale**: Principles VIII, IX, and XIII.
