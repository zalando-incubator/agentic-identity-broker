# Implementation Plan: Unique Agent External IDs and Idempotent Create-or-Update

**Branch**: `speckit/issue-334` | **Date**: 2026-10-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/055-agent-external-id-upsert/spec.md`

**Status**: Planned. Every item in [Approval Items](#approval-items) is Proposed and needs the
maintainer's sign-off before implementation.

## Summary

Make a set Agent External ID globally unique and non-empty, and add an idempotent
`PUT /api/agents?external_id={value}` that creates or updates the agent holding that ID.

- **Invariant in the stores.** PostgreSQL enforces uniqueness with a partial unique index and a
  non-empty check constraint. The memory adapter keeps an external-ID index under its write lock.
  Both map violations to a conflict that wraps a new `ports.ErrExternalIDConflict` sentinel
  ([research R1](./research.md#r1--where-the-uniqueness-invariant-lives)).
- **Reconcile in the domain.** `agents.Service.ReconcileByExternalID` validates the selector and
  body, reads with the existing `ListByExternalID`, applies the FR-012 field policy, and skips the
  write when the effective definition is unchanged. It creates through the existing `Create`, and
  on any create conflict it re-reads once to fall through to update. No repository method is added
  ([R2–R4](./research.md#r2--repository-port-shape)).
- **Thin HTTP adapter.** A new `ReconcileAgent` handler on the existing `/api/agents` route group
  parses the selector and member presence, shares request validation with create and update, and
  maps outcomes to `201` + `Location` or `200`, with `Prefer` support ([R5](./research.md#r5--http-adapter-shape)).
- **Safe upgrade.** Atomic migration `046` fails with a full report of duplicate and empty values
  before changing anything. A new operator runbook covers inventory, remediation, retry, and
  rollback ([R8–R9](./research.md#r8--migration-design-proposed-db-046)).
- **Contract.** Admin API `3.0.0` ([contract delta](./contracts/admin-agents-openapi.yaml)) and
  draft [ADR 045](./adr-045-unique-agent-external-id-upsert.md).

## Technical Context

**Language/Version**: Go 1.27.3

**Primary Dependencies**: chi v5 (routing), sqlx/pgx v5 and `lib/pq` (PostgreSQL adapter),
`golang-migrate` v4.20.1 (migrations), `log/slog`, Ginkgo v2/Gomega and testify (tests),
testcontainers-go (PostgreSQL fixtures). No new dependency.

**Storage**: PostgreSQL (production) and in-memory (development/test). One migration,
`046_unique_agent_external_id.{up,down}.sql`; no new table or column.

**Testing**: Go unit tests (domain service, `storage.Agent` validation, memory adapter, handler);
PostgreSQL integration tests for the migration and adapter conflict mapping; Ginkgo E2E through the
production bootstrap, with `Label("docker")` PostgreSQL journeys for the race and upgrade
scenarios.

**Target Platform**: Linux broker container; Darwin for development.

**Project Type**: Backend Admin API feature in the Go monorepo. No React UI, ExtProc, end-user API,
configuration, or Helm change.

**Performance Goals**: Collection PUT adds one indexed lookup to a create or update. A no-op replay
performs only reads. The unique index replaces the existing non-unique external-ID index, so writes
maintain one index, as before.

**Constraints**: Fail closed on invalid selectors (SR-002); no partial writes (FR-013); migration
must be atomic and non-destructive (DB-001); memory and PostgreSQL behavior must agree (DB-004).

**Scale/Scope**: The agents table is an administrative registry of hundreds to low thousands of
rows, which permits a transactional, non-concurrent index build.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: [data-model.md](./data-model.md) documents the changed Agent invariant, the
  External ID value object, the reconcile operation, its field policy, and state transitions.
- [x] **Domain Concepts**: No new term. Implementation updates the existing ARCHITECTURE.md
  **External ID** glossary entry ("identifies at most one Agent") and adds the reconcile behavior to
  the agent-management architecture text.
- [x] **Entity IDs**: No new entity; `id.ExternalID` and `id.AgentID` exist (ADR 013).
- [x] **Configuration Design**: No configuration change. Journey 1 shows existing keys only.
- [x] **Config Examples**: Not applicable; no new keys.
- [x] **Helm Deployment Contract**: No parameter change. The existing migration Job runs `046`.
- [x] **API Design First**: [contracts/admin-agents-openapi.yaml](./contracts/admin-agents-openapi.yaml)
  defines the change; it merges into `api/admin/openapi.yaml` only after approval.
- [x] **API Documentation**: Admin API only (`api/admin/openapi.yaml`).
- [ ] **API Changes**: Pending maintainer sign-off on API-UPSERT, API-OMISSION, API-UNIQUE, and
  API-VERSION. This gate stays open until the approval comment receives a 👍.
- [x] **Database Design**: `046_unique_agent_external_id.{up,down}.sql` with go-migrate naming;
  SQL behavior in [data-model.md](./data-model.md#postgresql-schema--migration-046_unique_agent_external_id-proposed-db-046).
- [x] **E2E Acceptance Tests**: Planned for all 22 scenarios before implementation
  ([Testing Strategy](#testing-strategy)).
- [x] **E2E Test Mapping**: One `It()` per scenario.
- [x] **E2E Red Phase**: Tests assert concrete status codes, headers, bodies, and stored state.
- [x] **Frontend Playwright E2E**: Not applicable; no UI change.
- [x] **Frontend Screenshots**: Not applicable.

**Implementation Considerations**:

- [x] **Security-First**: Existing admin authentication, Host, cross-origin, and content-type
  middleware cover the new route. Invalid selectors fail closed. Audit logs stay on actual
  creations and updates (SR-001–SR-003).
- [x] **Architecture Docs**: ARCHITECTURE.md gets the glossary update, the Admin API `3.0.0`
  versioning note, and the ADR index entry.
- [x] **ADRs**: Draft [ADR 045](./adr-045-unique-agent-external-id-upsert.md) (Proposed) records the
  uniqueness invariant, collection-PUT reconcile pattern, migration, and version bump. ADR 004,
  013, and 017 are followed; none is superseded.
- [x] **Library-First Security**: No cryptography.
- [x] **Zalando Guidelines**: Collection `PUT` with a selector departs from the guidelines' usual
  single-resource `PUT`; the issue mandates the shape, and ADR 045 records the deviation and
  alternatives. Errors, headers, and `Prefer` follow existing conventions.
- [x] **End-User Docs**: Admin-only change; no `docs/api/` page. The agent-management guide and the
  new operations runbook are updated.
- [x] **Migration Testing**: Integration tests cover block, apply, rollback, and reapply with
  relationship preservation.
- [x] **Hexagonal Architecture**: Selector rules, body/selector agreement, field policy, no-op
  detection, and race handling live in `internal/domain/agents`. The handler only parses input and
  maps errors. Adapters enforce the store invariant. No cross-adapter import.
- [x] **Persistence Patterns**: No new repository method; both adapters implement the invariant;
  errors wrap `StorageError` (ADR 004).

**Post-design re-check (after Phase 1)**: All blocking design preconditions pass except the API
confirmation gate, which is the expected sign-off. No violation needs Complexity Tracking.

## Approval Items

| ID | Type | Decision | Details |
|---|---|---|---|
| API-UPSERT | API | Add `PUT /api/agents?external_id={value}`: required single selector, `AgentCreateRequest` body, `201` + UUID `Location` on create, `200` on update and on no-op replay with unchanged `updated_at`, `400 invalid external_id` for selector/body external-ID errors, `409` for identifier conflicts, `Prefer: reference-id=canonical` support | specs/055-agent-external-id-upsert/contracts/admin-agents-openapi.yaml:68-222 |
| API-OMISSION | API | Update-branch field policy: preserve omitted `client_id`/`canonical_id`, `canonical_id: null` clears, `client_id: null` → `400 validation failed`, metadata and optional collections replaced (omitted or null → absent/empty) | specs/055-agent-external-id-upsert/spec.md:473-484 |
| API-UNIQUE | API | External IDs unique when set: `POST /api/agents` and `PUT /api/agents/{agent-id}` return `409 conflict` (`external_id is already held by another agent`) and `400 validation failed` for `""`; `PUT /api/agents/{agent-id}` documents `409`; `GET ?external_id=` returns zero or one agent (ID-addressed PUT delta at lines 223-246 of the same file) | specs/055-agent-external-id-upsert/contracts/admin-agents-openapi.yaml:13-67 |
| API-VERSION | API | Bump Admin API from `2.1.0` to `3.0.0` on the existing routes | specs/055-agent-external-id-upsert/research.md:151-161 |
| ADR-045 | ADR | Draft ADR 045: unique Agent External ID and create-or-update by external ID | specs/055-agent-external-id-upsert/adr-045-unique-agent-external-id-upsert.md:1-77 |
| DB-046 | Decision | Migration `046`: lock `agents`, block with a report of all duplicate/empty values and agent UUIDs without changing data, then add `uq_agents_external_id` and `chk_agents_external_id_not_empty` and drop `idx_agents_external_id`; DOWN restores the old index without touching rows; new operator runbook | specs/055-agent-external-id-upsert/data-model.md:102-152 |
| DEC-054 | Decision | Supersede feature 054's non-uniqueness clarification and FR-011; replace its shared-ID E2E scenario; leave the 054 spec unchanged as history | specs/055-agent-external-id-upsert/research.md:219-230 |

Implementation must not start any item above until the maintainer's 👍 on the approval comment
covers it. The implementation phase then records that approval in ADR 045's Status line and moves
the draft to `adrs/045-unique-agent-external-id-upsert.md`.

## Project Structure

### Documentation (this feature)

```text
specs/055-agent-external-id-upsert/
├── spec.md
├── plan.md                                    # This file
├── research.md                                # Phase 0
├── data-model.md                              # Phase 1
├── quickstart.md                              # Phase 1
├── contracts/
│   └── admin-agents-openapi.yaml              # Phase 1 — Admin API 3.0.0 delta
├── adr-045-unique-agent-external-id-upsert.md # Draft ADR (Proposed)
├── checklists/requirements.md
└── tasks.md                                   # Phase 2 (/speckit-tasks; not created here)
```

### Source Code (repository root)

```text
api/admin/openapi.yaml                               # merge contract delta; version 3.0.0
adrs/045-unique-agent-external-id-upsert.md          # moved from the feature draft after approval
ARCHITECTURE.md                                      # glossary, API versioning, ADR index
docs/guides/manage-agents-and-services.md            # uniqueness, collection PUT, migration notes
docs/operations/agent-external-id-uniqueness.md      # new runbook
docs/operations/deployment-checklist.md              # link the runbook for this upgrade

migrations/
├── 046_unique_agent_external_id.up.sql
└── 046_unique_agent_external_id.down.sql

internal/
├── ports/storage.go                                 # ErrExternalIDConflict; Create/Update docs
├── domain/storage/agent.go                          # empty external_id validation
├── domain/agents/service.go                         # ReconcileByExternalID, shared selector validation
├── domain/agents/reconcile.go                       # field policy, effective-definition equality
├── adapters/storage/memory/agent_repository.go      # byExternalID index
├── adapters/storage/postgres/agent_repository.go    # 23505/23514 mapping
├── adapters/http/handlers/admin/agents_handler.go   # ReconcileAgent; shared request conversion
└── adapters/http/routing/admin.go                   # r.Put("/", h.Agents.ReconcileAgent)

tests/
├── e2e/agent_external_id_upsert_test.go             # US1–US4 (new)
├── e2e/agent_external_id_filter_test.go             # retire shared-ID scenario (DEC-054)
├── e2e/bootstrap/postgres.go                        # fixture migrated to a target version
└── integration/migrations/agent_external_id_migration_test.go
```

**Structure Decision**: Existing Go monorepo layout. The domain change stays in the existing
`internal/domain/agents` bounded context; no new package, port, or builder wiring. `builder.go`
already wires `agents.Service` and `AgentsHandler`.

## Implementation Phase Overview

*Detailed task breakdown is in `tasks.md` (generated by `/speckit-tasks`).*

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring — isolate structural changes from feature work | Included |
| **Phase 1** | Setup — record approval, move ADR draft | Required |
| **Phase 2** | Design Preconditions (domain model, API, DB, E2E tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate | Skipped |
| **Phase 2.5** | Foundational Infrastructure — invariant, sentinel, migration | Required |
| **Phase 3+** | User Stories — US1/US2 (P1), US3 (P1), US4 (P2) | Required |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): include — extract the shared `AgentRequest` → `storage.Agent`
  conversion and validation from `CreateAgent` and `UpdateAgent` into one private helper, with no
  behavior change, so the reconcile handler reuses it.
- [x] Phase 2.7 (entity boilerplate): skip — no new entity, table, or repository.

Ordering constraints for tasks:

1. Approval recorded and ADR moved; OpenAPI merged with version `3.0.0`.
2. E2E tests for all scenarios and migration integration tests written and failing semantically
   (a minimal `ReconcileAgent` stub returning `501` lets the suite compile).
3. Store invariant (sentinel, `Agent` validation, memory index, PostgreSQL mapping, migration).
4. Domain reconcile, then handler and route.
5. Retire the 054 shared-ID E2E scenario in the same change that enforces uniqueness.
6. Docs, runbook, ARCHITECTURE.md; compliance gates.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/agent_external_id_upsert_test.go`

**Framework**: Ginkgo/Gomega following [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:

- **Top-level Describe**: `"Admin agent create-or-update by external ID"`
- **Contexts**: one per user story; PostgreSQL scenarios carry `Label("docker")`.
- **It blocks**: one per acceptance scenario, each preceded by
  `// Scenario USx.y from specs/055-agent-external-id-upsert/spec.md`.

**Scenario Mapping** (line numbers are filled during Phase 2f):

| Spec Scenario | E2E Test Location | Test Description |
|---|---|---|
| US1.1 | `tests/e2e/agent_external_id_upsert_test.go` | `It("should create the agent with 201 and a UUID Location when no agent holds the external ID")` |
| US1.2 | same | `It("should return 200 with an unchanged representation and timestamps on replay")` |
| US1.3 | same | `It("should update the same agent and advance only updated_at when the definition changes")` |
| US1.4 | same, `Label("docker")` | `It("should create exactly one agent when two PostgreSQL brokers race on the first call")` |
| US1.5 | same | `It("should preserve identities and clear metadata and collections when optional fields are omitted")` |
| US1.6 | same | `It("should clear the canonical ID, metadata, and collections on explicit nulls without touching grants")` |
| US1.7 | same | `It("should reject a null client_id with 400 and change nothing")` |
| US2.1 | same | `It("should reject POST with an external ID held by another agent")` |
| US2.2 | same | `It("should reject an ID-addressed update to another agent's external ID")` |
| US2.3 | same | `It("should allow an agent to keep its own external ID on update")` |
| US2.4 | same | `It("should reject missing, empty, repeated, overlong, invalid UTF-8, NUL, and malformed selectors")` |
| US2.5 | same | `It("should reject a body external ID that differs from, empties, or nulls the selector")` |
| US2.6 | same | `It("should reject an invalid agent definition without creating or changing an agent")` |
| US2.7 | same | `It("should return 409 for canonical and client ID conflicts on both branches")` |
| US2.8 | same | `It("should allow many agents without an external ID and reject the empty string")` |
| US3.1 | same, `Label("docker")` | `It("should block the migration on duplicates and report the value and agent IDs")` |
| US3.2 | same, `Label("docker")` | `It("should block the migration on empty values and report both categories")` |
| US3.3 | same, `Label("docker")` | `It("should succeed after deliberate remediation and then enforce uniqueness")` |
| US3.4 | same, `Label("docker")` | `It("should preserve agents and relationships across rollback and reapply")` |
| US4.1 | same | `It("should return an empty or one-element array for a valid filter")` |
| US4.2 | same | `It("should keep the empty-filter 400 and the unfiltered list")` |
| US4.3 | same | `It("should honor canonical references on create-or-update responses and replays")` |

`tests/e2e/agent_external_id_filter_test.go`: the `It("should return every agent sharing an
external ID and no others")` scenario and its `researchEU` fixture are replaced (DEC-054); its
remaining feature 054 scenarios keep running unchanged.

**Red Phase Requirements**: Tests compile against a `ReconcileAgent` stub and fail on concrete
assertions (status codes, `Location`, timestamps, stored agents, migration errors). No `XIt`,
`PIt`, `Skip()`, or red-phase comments.

**Test Data Strategy**:

- Fixtures: `fixtures.AdminPrincipal()`, `fixtures.LocalAgent()`, permission sets seeded through
  storage, and the shared spec cast (UUIDs, canonical IDs, external IDs).
- New fixture helper: an external-ID agent builder in `tests/e2e/fixtures/` if two or more tests
  need it.

**Bootstrap Strategy**:

- In-memory scenarios: fresh `bootstrap.TestServer` and storage per test.
- US1.4: one `bootstrap.NewPostgresFixture`, two storage adapters and two servers, concurrent PUTs.
- US3: new `bootstrap.NewPostgresFixtureAtVersion(ctx, 45)` plus `MigrateUp()` on the fixture, so a
  test seeds legacy rows before `046` and starts the broker afterward.

**Helper Utilities**: Existing `server.AuthenticatedGET`/request helpers; a small PUT helper in
the test file unless `tests/e2e/helpers/` already provides one.

### Unit & Integration Tests

**Unit Tests** (TDD, written first):

- `internal/domain/agents/service_test.go` / `reconcile_test.go`: table tests for selector and body
  validation, each FR-012 row on both branches, effective-definition equality (including client-URI
  set comparison), conflict re-read, bounded retry, and >1 holder as an internal error.
- `internal/domain/storage/agent_test.go`: empty external ID rejected; nil allowed.
- `internal/adapters/storage/memory/agent_repository_test.go`: create/update/delete index behavior;
  concurrent creates under `-race`.
- `internal/adapters/http/handlers/admin/agents_handler_test.go`: selector parsing, member presence,
  `client_id: null`, status, `Location`, `Vary`, `Preference-Applied`, error mapping.

**Integration Tests** (`just test-integration-infra`):

- `tests/integration/migrations/agent_external_id_migration_test.go`: block on duplicates, empties,
  and both, with report content and unchanged data; clean apply; rollback/reapply preserving
  agents, permission declarations, client URIs, and grants; index rejects concurrent inserts.
- `internal/adapters/storage/postgres/agent_repository_test.go`: `23505`/`23514` mapping to the
  sentinel and validation error on create and update.

**Test Coverage Goals**: All new domain branches; both adapters' uniqueness paths; 100% of
acceptance scenarios in E2E (Principle XIII).

## Complexity Tracking

No constitution violations require justification.
