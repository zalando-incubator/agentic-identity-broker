# Tasks: Unique Agent External IDs and Idempotent Create-or-Update

**Input**: Design documents from `specs/055-agent-external-id-upsert/`.
**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/admin-agents-openapi.yaml`, `quickstart.md`.
**Status**: Tasks generated; implementation not started. All seven approval items remain Proposed.
**Tests**: Mandatory red-green-refactor under Principles VIII and XIII. Write tests first, make them compile using minimal scaffolding where needed, observe concrete semantic failures, implement, then refactor without weakening assertions.
**Organization**: Shared design and enforcement prerequisites precede independently verifiable user-story increments. Paths are relative to the repository root. `[P]` means different files and no mutual dependency within the stated batch; it does not waive earlier gates.

## 🔒 Feature Scope [MANDATORY]

Every task cites a specification requirement, acceptance scenario, success criterion, or binding constitutional principle. Implement sign-off items only when `context.approval` covers them. T001 records supplied approval; it does not grant approval. No application work is authorized by this task-generation session. Do not change `spec.md` or `plan.md` during implementation, alter historical feature 054 documents, introduce new dependencies/configuration, or bundle unrelated cleanup.

Sign-off inventory: API-UPSERT, API-OMISSION, API-UNIQUE, API-VERSION, ADR-045, DB-046, DEC-054 in `plan.md` §Approval Items. Leave tasks depending on uncovered items unchecked and record the exact blocker while continuing independent permitted work.

## Phase 1: Setup (Shared Infrastructure)

- [ ] T001 Record only the supplied maintainer approval in `specs/055-agent-external-id-upsert/adr-045-unique-agent-external-id-upsert.md` as `Accepted (approved by <login> on <date>: <url>)`, then move it to `adrs/045-unique-agent-external-id-upsert.md` and leave a feature-local forwarding document at the original path so immutable spec/plan links still resolve; verify the approval covers every item needed by implementation and retain feature-local links to the authoritative record in `specs/055-agent-external-id-upsert/research.md` and `data-model.md` (Principle II; ADR-045).
- [ ] T002 Review affected source and nearest instructions in `internal/AGENTS.md`, `internal/domain/AGENTS.md`, `internal/ports/AGENTS.md`, `internal/adapters/AGENTS.md`, `tests/e2e/AGENTS.md`, and `tests/integration/AGENTS.md`; confirm migration number 046 remains unused and note blockers in `specs/055-agent-external-id-upsert/tasks.md` without renumbering the approved design silently (Principles II, VI, IX; DB-001).

Phase 0 is omitted: there is no significant standalone refactor. The small request-helper extraction is feature-integral work in T036. Phase 2.7 is omitted: no new entity, table, typed ID, or repository is introduced.

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation.
**Gate**: No feature behavior implementation until all applicable sub-phases are complete. Minimal compiling test scaffolding is allowed; it must not implement the missing behavior.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [ ] T003 Verify Agent aggregate, existing `id.ExternalID`/`id.AgentID`, reconcile outcomes, field presence, equality, and relationship-preservation rules against `specs/055-agent-external-id-upsert/data-model.md`; retain its documented invariant: "when non-nil it MUST be non-empty, and no two Agents may hold equal values (exact, case-sensitive byte equality). `nil` remains allowed on any number of Agents" (FR-001, FR-012, FR-013; Principles V, IX; ADR 013).
- [ ] T004 Update the existing External ID glossary entry and agent-management architecture in `ARCHITECTURE.md` to identify at most one Agent, distinguish it from UUID/canonical/client IDs, document no-op reconcile and performance/security constraints, and add the approved ADR 045 index entry (FR-001, FR-005, FR-008; Principles II, V).

### Phase 2b: Configuration Design [MANDATORY]

- [ ] T005 Verify no configuration key/default, example file, or Helm value/template change is needed; check the existing keys in Journey 1 of `specs/055-agent-external-id-upsert/spec.md` against `internal/ports/config.go` and confirm `charts/agentic-identity-broker/` still runs the separate migration Job without adding broker migration tooling (API-005, SR-001; Principle VII; ADR 009). Record applicability in `specs/055-agent-external-id-upsert/tasks.md`.

### Phase 2c: API Design [MANDATORY]

- [ ] T006 Verify supplied approval covers API-UPSERT, API-OMISSION, API-UNIQUE, and API-VERSION, then merge `specs/055-agent-external-id-upsert/contracts/admin-agents-openapi.yaml` into `api/admin/openapi.yaml` as Admin API 3.0.0, preserving unchanged operations/components/security; include selector/body rules, all response codes, UUID Location, Prefer headers, existing-write conflicts, nullable/omitted fields, and zero-or-one GET descriptions (API-001–API-005; Principles IV, X).
- [ ] T007 Document the approved 2.1.0 → 3.0.0 compatibility/versioning change on existing routes in `ARCHITECTURE.md` and verify there is no end-user/ExtProc contract change in `api/enduser/openapi.yaml` (API-004, API-005; Principles II, IV, X).

### Phase 2d: Database Design [MANDATORY]

- [ ] T008 Verify approved DB-046 design against `specs/055-agent-external-id-upsert/data-model.md`: partial unique index, non-empty check, old index removal/restoration, atomic DO blocks, NOWAIT locking, bounded diagnostics for both categories, complete runbook inventory, no row modification, and dirty-version recovery (FR-004, DB-001–DB-004; Principle IX).
- [ ] T009 Create minimal no-op DO-block scaffolding in `migrations/046_unique_agent_external_id.up.sql` and `migrations/046_unique_agent_external_id.down.sql` solely to let migration tests execute and fail on the missing invariant; do not add constraints, cleanup, or functional DDL until T025 (DB-001; Principles VIII, IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

No frontend, design-system component, Wireloom sequence, screenshot, or JWT output changes apply. Verify that boundary through T010 and the final scope/configuration checks; do not add unrelated frontend work (API-005; Principles XI, XIV).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

- [ ] T010 Review all four concrete journeys in `specs/055-agent-external-id-upsert/spec.md` against the approved OpenAPI and all 22 acceptance scenarios; confirm real configuration keys, RFC-style exchanges, synthetic IDs/principals, timestamp/omission/null examples, and the absence of applicable UI/JWT outputs (Principles XIII, XIV).
- [ ] T011 [P] Add shared valid external-ID agent fixtures in `tests/e2e/fixtures/agent_external_id.go` and reusable collection-PUT HTTP helper in `tests/e2e/helpers/agent_external_id.go` using existing fixtures/request utilities, fresh IDs, nonempty permission declarations, and no production test-only API (US1–US4; Principle XIII).
- [ ] T012 [P] Extend `tests/e2e/bootstrap/postgres.go` with `NewPostgresFixtureAtVersion(ctx, 45)` and migration up/down/version-inspection/recovery helpers required by legacy tests, retaining the existing latest-version constructor and cleanup, and production bootstrap for both admin brokers (US1.4, US3.1–US3.4; DB-003; ADR 007).
- [ ] T013 Add a compiling `ReconcileAgent` stub returning 501 in `internal/adapters/http/handlers/admin/agents_handler.go` and register only `r.Put("/", h.Agents.ReconcileAgent)` in `internal/adapters/http/routing/admin.go` under existing admin middleware; add empty domain signatures only when needed for compiling unit tests (API-001; Principles VIII, XII, XIII).
- [ ] T014 Write seven US1 `It()` blocks in `tests/e2e/agent_external_id_upsert_test.go` for US1.1–US1.7: create/UUID Location, unchanged replay, real update, two-broker concurrent first calls, omitted optional fields, explicit nullable fields with grant preservation, and rejected null client ID plus valid replacement. US1.4 carries `Label("docker")`, checks two concurrent calls, then ten-call concurrency with a fresh selector; replay coverage submits ten identical requests in both backend modes to prove SC-002/SC-003 (FR-005–FR-010, FR-012, FR-013; Principle XIII).
- [ ] T015 Write eight US2 `It()` blocks in `tests/e2e/agent_external_id_upsert_test.go` for US2.1–US2.8: duplicate POST, UUID/canonical-addressed conflicting update, self-update, all invalid selectors, mismatched/empty/null body external ID, invalid definition/references on both branches, canonical/client conflicts on both branches and both client-sharing modes, and multiple absent/null IDs versus empty writes. Assert no changes to IDs, timestamps, collections, or grants on failure (FR-001–FR-003, FR-006, FR-007, FR-010, FR-013; SC-004; Principle XIII).
- [ ] T016 Write four `Label("docker")` US3 `It()` blocks in `tests/e2e/agent_external_id_upsert_test.go` for US3.1–US3.4: legacy duplicate inventory, empty/coexisting categories, deliberate runbook remediation and retry followed by real HTTP conflict/lookup checks, and rollback/reapply preserving agents and relationships. Use production admin servers before remediation and after migration, not isolated endpoint fragments (FR-004, DB-001–DB-004; SC-005; Principle XIII).
- [ ] T017 Write three US4 `It()` blocks in `tests/e2e/agent_external_id_upsert_test.go` for US4.1–US4.3: exact/case-sensitive zero-or-one arrays, literal empty versus absent filter, and canonical nested references on create/update/replay with UUID identity and Prefer headers (FR-010, FR-011; SC-006; Principle XIII).
- [ ] T018 Verify the new suite uses `Describe` → story `Context` → one `It()` per scenario, nearby `// Scenario USx.y from specs/055-agent-external-id-upsert/spec.md` references, fixtures, fresh storage/servers, production Builder, bounded polling without sleeps, and complete HTTP/read-back/relationship assertions. Record scenario-to-`It()` line mappings in `specs/055-agent-external-id-upsert/quickstart.md`, leaving `plan.md` unchanged (Principles VIII, XII, XIII).
- [ ] T019 Run `just check`, `just test-e2e-backend`, and `just test-e2e-docker` against compiling scaffolding; record concrete missing-feature failures in `specs/055-agent-external-id-upsert/tasks.md`, with unchanged compatibility cases allowed to remain green. Do not count missing tooling/containers, compilation failures, pending tests, or always-fail assertions as red evidence (Principles VIII, XIII).

**Checkpoint**: Design reviewed and approved, 22 acceptance tests authored before feature behavior, semantic red evidence recorded. Required container runs must be performed in a permitted Docker/Podman environment; this worker cannot supply Docker-in-Docker.

## Phase 2.5: Foundational Infrastructure

**Purpose**: Shared invariant and safe migration needed by all four stories. All test tasks in each batch precede their matching implementation task.

- [ ] T020 [P] Write table-driven validation tests in `internal/domain/storage/agent_test.go` for non-nil empty external IDs on create/update/CIMD-snapshot validation, nil allowed, nonempty existing validation unchanged, and no mutation; run them and record semantic red evidence in `specs/055-agent-external-id-upsert/tasks.md` (FR-003, FR-013; data-model validation rule; Principle VIII).
- [ ] T021 [P] Write memory tests in `internal/adapters/storage/memory/agent_repository_test.go` for duplicate create/update, self-update, move/clear/delete/reuse index entries, many nil IDs, literal case/whitespace distinctions, canonical/client-URI conflict failures leaving every index and collection unchanged, and concurrent mixed writes under `-race`; observe missing-invariant failures (FR-001–FR-003, FR-013; DB-004; Principle VIII).
- [ ] T022 [P] Write real PostgreSQL tests in `internal/adapters/storage/postgres/agent_repository_test.go` using shared migrated clones for Create/Update `23505`/`uq_agents_external_id` conflict and `23514`/`chk_agents_external_id_not_empty` validation mapping, self-update, null IDs, and rollback of relationship writes on conflicts; assert `StorageError` kind and `errors.Is` sentinel, not adapter error text. Where direct constraint errors require an internal mapping helper, test it in-package and verify real constraint behavior separately; record semantic red evidence (FR-001–FR-003, FR-013; DB-002, DB-004; Principles VIII, IX).
- [ ] T023 [P] Write migration integration tests in `tests/integration/migrations/agent_external_id_migration_test.go` with `//go:build integration` and existing framework/shared PostgreSQL clones at version 45: duplicates, empties, both categories including >100 entries/remainder/full inventory, clean UP, null IDs, exact equality, concurrent inserts/mixed ordinary writes, NOWAIT lock failure, atomic unchanged schema/rows on failure, dirty-version inspection, guarded retry, DOWN/reapply preserving permission declarations, service requirements, client URIs, grants, and other existing foreign-key relationships. Run against T009 scaffolding and record semantic red evidence (FR-004, DB-001–DB-004; Principles VIII, IX).
- [ ] T024 Add `ports.ErrExternalIDConflict` and document Create/Update conflicts and zero-or-one `ListByExternalID` behavior in `internal/ports/storage.go`; reject empty external IDs in `internal/domain/storage/agent.go` with `external_id cannot be empty when provided` while retaining nonempty existing-write validation (FR-001–FR-003; data-model Ports/Validation; Principles VI, IX).
- [ ] T025 Implement approved UP/DOWN behavior in `migrations/046_unique_agent_external_id.up.sql` and `migrations/046_unique_agent_external_id.down.sql`: `SET LOCAL statement_timeout = '5s'`, single atomic DO-block DDL, `ACCESS EXCLUSIVE MODE NOWAIT`, inventory both categories before DDL, first 100 of each plus remaining counts and runbook hint, `uq_agents_external_id`, `chk_agents_external_id_not_empty`, and inverse index restoration with no row changes. Prove T023 green through the production migration runner (DB-001–DB-004; DB-046; ADR 039 exception not extended).
- [ ] T026 Implement `byExternalID map[id.ExternalID]id.AgentID` in `internal/adapters/storage/memory/agent_repository.go`, checking external IDs before other index mutations under the write lock, updating/moving/clearing entries atomically, deleting entries on Delete, and wrapping conflicts with the shared sentinel and required message; prove T020/T021 green (FR-001–FR-003, FR-013; data-model Memory adapter; ADR 004).
- [ ] T027 Implement PostgreSQL constraint-specific error mapping in `internal/adapters/storage/postgres/agent_repository.go` for Create/Update without exposing SQL errors or partially changing client URIs/definition fields; preserve sqlx, explicit SQL columns, timeouts, and existing transaction boundaries; prove T022 green (FR-002, FR-003, FR-013; DB-002, DB-004; ADR 004).
- [ ] T028 Remove only the superseded shared-ID `It()` and `researchEU` fixture from `tests/e2e/agent_external_id_filter_test.go` in the same change as uniqueness enforcement; preserve remaining feature 054 tests and documents, and leave exactly one replacement US4.1 test in the new suite (FR-011; DEC-054; Principle XIII).
- [ ] T029 Run `just check`, `just test`, and `just test-integration-infra`; record passing invariant/migration suites and still-expected reconcile failures in `specs/055-agent-external-id-upsert/tasks.md` without claiming unfinished story completion (DB-003, DB-004; Principles VIII, IX).

## Phase 3: User Story 1 — Reconcile an agent in one retry-safe call (Priority: P1) 🎯 MVP

**Goal**: One write creates, updates, or returns unchanged state under a stable external ID.
**Independent Test**: From a fresh registry, create, repeat ten times, change the name, omit/clear managed fields, and read back one stable UUID with appropriate timestamps and intact grants; race identical calls across two brokers.

### Tests for User Story 1 [MANDATORY - Principle VIII]

- [ ] T030 [P] [US1] Write table-driven reconcile tests with hand-written repository function mocks in `internal/domain/agents/reconcile_test.go`: zero/one/>1 holders; effective definitions; each FR-012 field row on create/update; nil/empty equivalence; order-sensitive permission/service/redirect/scope collections; client-URI set equality across stores; UUID/created_at preservation; unchanged updated_at/no Update/no mutation audit on replay; actual update audit; ten replays/concurrent identical requests. Add minimal compiling domain types/signatures and observe concrete semantic failures before T033 (FR-005, FR-008, FR-009, FR-012, FR-013; SC-002/SC-003; Principles VIII, IX).
- [ ] T031 [P] [US1] Write handler tests in `internal/adapters/http/handlers/admin/agents_handler_test.go` for JSON member presence, omitted/null canonical/external/client IDs, create-schema required fields and 64 KiB body limit, 201 UUID Location versus 200, unchanged response/timestamps, and regression parity for existing CreateAgent/UpdateAgent conversion; observe concrete failures before T036/T037 (FR-005, FR-007, FR-008, FR-012; API-003; Principle VIII).
- [ ] T032 [US1] Add bounded conflict-reread tests to `internal/domain/agents/reconcile_test.go`: external/canonical/client-URI create collisions, same-selector holder appears, no holder returns original conflict, lookup errors propagate, >1 holder fails closed without survivor selection, and persistent update conflict returns 409 with no unbounded retry. Include ten concurrent identical calls for both backend behavior and verify different concurrent desired definitions remain coherent (FR-009, FR-010, FR-013; SC-003; research R3; Principles I, VIII).

### Implementation for User Story 1

- [ ] T033 [US1] Implement `ReconcileInput`, Created/Updated/Unchanged outcomes, and unexported field-policy/effective-equality helpers in `internal/domain/agents/reconcile.go`: omitted identities preserved, metadata removed, optional collections emptied, supplied fields replaced; use exact documented comparison and never alter grants. Required fields retain the constraint "required"; `permission_sets` remains "nonempty and valid"; `client_id` values remain a "validated nonempty string" (FR-008, FR-012; data-model field policy/equality; Principles V, VI).
- [ ] T034 [US1] Implement `Service.ReconcileByExternalID` in `internal/domain/agents/service.go` using existing ListByExternalID/Create/Update ports and domain validation: select only by external ID, reject >1 holder, apply field policy, skip writes for unchanged state, preserve creation identity/time, timestamp actual changes, and retry a create conflict by rereading once before update/no-op; retain configured client-ID uniqueness and actual mutation audit events (FR-005, FR-008–FR-010, FR-012, FR-013; SR-003; research R3; Principles VI, XII).
- [ ] T035 [US1] Prove T030/T032 green and refactor only duplicated domain validation/equality within `internal/domain/agents/service.go` and `internal/domain/agents/reconcile.go`; do not introduce repository methods, new bounded contexts, test-only APIs, or alternative audit/no-op mechanisms (FR-013; Principles VI, VIII, IX).
- [ ] T036 [US1] Extract the small shared AgentRequest-to-Agent conversion helper in `internal/adapters/http/handlers/admin/agents_handler.go` for CreateAgent/UpdateAgent/ReconcileAgent; keep existing create/update attribute/reference validation and error contracts unchanged, capture presence separately, and keep new selector/field-policy/no-op/race invariants in the domain service; prove T031 regression cases green (FR-007, FR-010, FR-012; Principles VI, VIII).
- [ ] T037 [US1] Replace the ReconcileAgent stub in `internal/adapters/http/handlers/admin/agents_handler.go` with bounded body parsing, create-schema validation, parsed presence facts, `FinalizeRequestSecurityContext`, domain reconcile call, response presentation, 201 UUID Location versus 200, and standard errors; do not instantiate services or call repositories directly. Keep existing Builder wiring and route middleware in `internal/adapters/http/routing/admin.go` (FR-005, FR-007, FR-008; SR-001; Principles VI, XII).
- [ ] T038 [US1] Run `just check`, `just test`, `just test-e2e-backend`, and `just test-e2e-docker`; verify US1.1–US1.7 and SC-002/SC-003 through T014 in `tests/e2e/agent_external_id_upsert_test.go`, preserving assertions and recording results in `specs/055-agent-external-id-upsert/tasks.md` (Principles VIII, XIII).

**Checkpoint**: US1 is demonstrable, but it is not a deployable partial release without the uniqueness, invalid-input, safe-upgrade, and compatible-presentation prerequisites.

## Phase 4: User Story 2 — Keep external identities unambiguous across all writes (Priority: P1)

**Goal**: Every supported write enforces unique set external IDs and failures never mutate records.
**Independent Test**: Register two distinct agents; exercise conflicts, self-updates, invalid selectors/definitions on both collection-PUT branches, and nil versus empty external IDs; read back all originals and relationships.

### Tests for User Story 2 [MANDATORY - Principle VIII]

- [ ] T039 [P] [US2] Add domain validation tables in `internal/domain/agents/service_test.go` for shared selector rules: "1–255 code points, valid UTF-8, no NUL", missing/empty/256 versus valid 255 multibyte code points, body mismatch/empty/null, literal whitespace/case/UUID-shaped IDs, and zero writes on error; preserve existing GET behavior and nonempty ordinary-write validation. Run and capture new semantic failures before T041 (FR-003, FR-006, FR-007; SR-002; Principle VIII).
- [ ] T040 [P] [US2] Extend `internal/adapters/http/handlers/admin/agents_handler_test.go` with raw-query decoding/repeated/undecodable/semicolon cases, unknown or malformed unrelated query parts, missing/misspelled selector, null client ID, invalid required/reference inputs, canonical/client/client-URI conflicts on both branches, and exact 400/409 error envelopes on all three write operations; verify no data leaks/mutations (FR-002, FR-003, FR-006, FR-007, FR-010, FR-013; SR-002; Principle VIII).

### Implementation for User Story 2

- [ ] T041 [US2] Extract/reuse selector validation in `internal/domain/agents/service.go` for ListByExternalID and reconcile, enforce omitted body external ID → selector versus null/empty/mismatch rejection, validate effective required definition/reference/client-ID rules before writes, and preserve external-ID-only selection and fail-closed errors (FR-006, FR-007, FR-010, FR-013; SR-002; Principle VI).
- [ ] T042 [US2] Reuse `externalIDFromQuery` in `internal/adapters/http/handlers/admin/agents_handler.go`, explicitly require PUT's single selector, and map ErrInvalidExternalID → `400 invalid external_id`, other validation → `400 validation failed`, conflicts → `409 conflict` with `external_id is already held by another agent` for external-ID conflicts; preserve ordinary POST/update behavior except approved changes (FR-002, FR-003, FR-006, FR-007; API-UNIQUE, API-UPSERT).
- [ ] T043 [US2] Run `just check`, `just test`, and `just test-e2e-backend` and verify US2.1–US2.8 through T015 in `tests/e2e/agent_external_id_upsert_test.go`; compare stored UUIDs, timestamps, collections, and grants before/after rejection and record SC-004 results in `specs/055-agent-external-id-upsert/tasks.md` (FR-013; Principles VIII, XIII).

**Checkpoint**: All write paths reject ambiguous identities and invalid inputs without partial changes.

## Phase 5: User Story 3 — Upgrade safely without losing governed agents (Priority: P1)

**Goal**: Operators account for every legacy agent and deliberately remediate before a non-destructive upgrade.
**Independent Test**: At schema 45 seed duplicate/empty IDs and relationships; observe atomic block and both inventory categories; remediate explicitly, retry, verify through HTTP, then rollback/reapply unchanged relationships.

### Tests for User Story 3 [MANDATORY - Principle VIII]

- [ ] T044 [US3] Verify T016/T023 exercise every step of US3.1–US3.4 through the same production migration runner and approved admin remediation contract; preserve their original red assertions and confirm direct constraints, blocked schema/row preservation, guarded version recovery, and full/bounded inventory behavior in `tests/integration/migrations/agent_external_id_migration_test.go` (FR-004, DB-001–DB-004; SC-005; Principles VIII, IX, XIII).

### Implementation for User Story 3

- [ ] T045 [US3] Write `docs/operations/agent-external-id-uniqueness.md` with explicit-column inventory SQL for every duplicate/empty affected agent, bounded error/remainder interpretation, writer coordination without weakening authentication, non-destructive ID-addressed Admin PUT remediation (distinct external ID or omission to clear), dirty-version inspection and `force 45` only after unchanged-schema verification, retry/postchecks, and rollback/reapply prerequisites; retain migration/runtime role separation (FR-004; DB-001, DB-003; SR-003; research R8/R9; ADR 009).
- [ ] T046 [US3] Link the upgrade runbook from `docs/operations/deployment-checklist.md` and `docs/guides/manage-agents-and-services.md`, warn that release blocks on unresolved duplicates/empties, and document preservation of UUIDs/grants and ordinary API compatibility impact (FR-004; API-002, API-004; SC-005).
- [ ] T047 [US3] Execute runbook inventory/remediation/recovery/verification/rollback/reapply against disposable fixtures in `tests/integration/migrations/agent_external_id_migration_test.go` and US3 E2E tests; run `just check`, `just test-integration-infra`, and `just test-e2e-docker`, recording full results in `specs/055-agent-external-id-upsert/tasks.md`. Never clear dirty metadata automatically or claim missing containers as passed validation (DB-003, DB-004; SC-005; Principles I, IX, XIII).

**Checkpoint**: Every affected value/UUID is discoverable and all relationships survive block, deliberate repair, UP, DOWN, and reapplication.

## Phase 6: User Story 4 — Keep lookup and reference presentation compatible (Priority: P2)

**Goal**: Existing lookup arrays and exact matching remain compatible; sync responses honor canonical reference preferences without changing identity.
**Independent Test**: GET zero/one matches, empty/absent filters, and PUT create/update/replay with Prefer; verify nested aliases/UUID fallback, UUID agent id, headers, and persisted UUID relationships.

### Tests for User Story 4 [MANDATORY - Principle VIII]

- [ ] T048 [US4] Add handler regression tests in `internal/adapters/http/handlers/admin/agents_handler_test.go` for Vary on normal responses, Preference-Applied only when honored, canonical nested permission/service references with UUID fallback, relative UUID Location despite canonical preference, absent metadata/empty collection presentation, and no-op timestamps under different presentation choices. Observe missing preference behavior before T049 (FR-010, FR-011; API-003; Principle VIII).

### Implementation for User Story 4

- [ ] T049 [US4] Apply existing canonical-reference presentation to ReconcileAgent responses in `internal/adapters/http/handlers/admin/agents_handler.go`, retaining `id` and persisted relationships as UUIDs, `Vary: Prefer`, conditional `Preference-Applied: reference-id=canonical`, normal omitted empty optional fields and canonical_id null, and unchanged no-op timestamps; leave GET shape and selector behavior intact (FR-010, FR-011; API-003).
- [ ] T050 [US4] Update `docs/guides/manage-agents-and-services.md` with unique nonempty external IDs, zero-or-one exact-match arrays, collection PUT create/replay/update/omission/null/error examples, UUID Location, canonical preferences, and approved Admin API 3.0.0 migration warning (API-002–API-004; FR-010–FR-012; Principle XIV).
- [ ] T051 [US4] Run `just check`, `just test`, and `just test-e2e-backend`; prove T017/T048 green in `tests/e2e/agent_external_id_upsert_test.go` and `internal/adapters/http/handlers/admin/agents_handler_test.go`, ensure remaining 054 tests pass and no duplicate US4.1 mapping exists, and record SC-006 results in `specs/055-agent-external-id-upsert/tasks.md` (Principles VIII, XIII).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.
**Gate**: Complete all applicable verification tasks before declaring delivery complete. Missing approval, dependency, checklist evidence, or validation remains an explicit deferred task, not a silently checked item.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T052 Verify every task in `specs/055-agent-external-id-upsert/tasks.md` traces to approved scope/scenarios or a binding constraint; exclude unrelated remediation and place only user-approved issue URLs in non-checkbox Future Work notes (Feature Scope & Deferred Findings).
- [ ] T053 Verify `ARCHITECTURE.md` documents the Agent/External ID invariants, reconcile behavior, performance/security constraints, versioning, and approved ADR 045; verify `specs/055-agent-external-id-upsert/data-model.md` remains aligned (Principles II, V).
- [ ] T054 Verify no configuration/default or deployment-boundary changes exist in `internal/ports/config.go`, `examples/config/`, `examples/config/README.md`, or `charts/agentic-identity-broker/`; existing journey configuration remains accurate and the separate migration image owns 046 (Principle VII; ADR 009).
- [ ] T055 Verify `api/admin/openapi.yaml` exactly expresses approved paths/parameters/body/null/omission/status/error/Location/Prefer contracts and Admin API 3.0.0, and record the supplied stakeholder approval reference in `specs/055-agent-external-id-upsert/tasks.md` (Principles IV, X).
- [ ] T056 Verify `migrations/046_unique_agent_external_id.up.sql` and `.down.sql` match documented indexes/check/lock/timeout/report/atomic/no-data-change semantics and sequential numbering; verify complete operator inventory and guarded recovery in `docs/operations/agent-external-id-uniqueness.md` (Principle IX).
- [ ] T057 Verify linked concrete journeys in `specs/055-agent-external-id-upsert/spec.md` match implementation/contract/configuration and validate manual quickstart exchanges in `specs/055-agent-external-id-upsert/quickstart.md`; confirm no applicable frontend/Playwright/screenshots/Wireloom/JWT output work has been introduced (Principles XI, XIV).
- [ ] T058 Verify all 22 scenarios have one correctly referenced Ginkgo `It()` each in `tests/e2e/agent_external_id_upsert_test.go`, current quickstart line mappings, production bootstrap, hierarchical structure, fixtures, isolation, semantic red evidence, and no pending/Skip/always-fail/red-phase annotations; remaining 054 tests retain their own mapping (Principles VIII, XIII).

#### Implementation Phase Verification [MANDATORY]

**API & Documentation / Architecture & Documentation**

- [ ] T059 Verify implemented response bodies, headers, errors and persisted UUID references match confirmed `api/admin/openapi.yaml`; verify agent guide/runbook/deployment checklist cover integration and release impact, and no end-user `docs/api/` changes are required (Principles IV, X, XIV).
- [ ] T060 Verify `adrs/045-unique-agent-external-id-upsert.md` carries the actual supplied approval citation and final approved decision, and feature links in `specs/055-agent-external-id-upsert/research.md` and `data-model.md` resolve after the move; do not amend main-merged ADRs or immutable implementation-phase spec/plan artifacts (Principle II).

**Configuration / Database & Persistence**

- [ ] T061 Verify no ad-hoc configuration path or broker migration tooling was introduced in `internal/domain/agents/service.go`, `internal/adapters/http/handlers/admin/agents_handler.go`, or `charts/agentic-identity-broker/`; preserve configured timeouts and role boundaries (Principle VII; ADRs 004, 009).
- [ ] T062 Verify both adapters enforce identical uniqueness/null/empty behavior, atomic failure and coherent collection replacement; real PostgreSQL tests cover constraints, race paths, UP/DOWN/reapply/data integrity and retry; use sqlx and explicit result columns in `internal/adapters/storage/postgres/agent_repository.go` and `tests/integration/migrations/agent_external_id_migration_test.go`, with no new repository method/entity boilerplate (Principle IX).

**Security**

- [ ] T063 Verify new collection PUT inherits admin authentication/principal/Host/cross-origin/JSON protections through `internal/adapters/http/routing/admin.go`, including denied requests without writes or leaks; add/run route-security regression assertions in `internal/adapters/http/routing/admin_test.go` (SR-001, SR-002; Principles I, VIII, XII).
- [ ] T064 Verify `internal/domain/agents/service.go` emits structured existing agent-created/updated audits only for actual mutations, no mutation audit or write on replay, no credentials/tokens in reports/logs, and no custom crypto/default-off security fallback (SR-003; Principles I, III).

**Architecture Patterns**

- [ ] T065 Verify selector/field-policy/equality/race decisions stay in `internal/domain/agents/`, port errors/contracts in `internal/ports/storage.go`, store enforcement in driven adapters, no cross-adapter imports or handler repository bypass, and existing Builder injection with routing-only registration in `internal/app/builder.go` and `internal/adapters/http/routing/admin.go` (Principles VI, XII).

**Testing**

- [ ] T066 Verify recorded unit/integration red-green-refactor evidence in `specs/055-agent-external-id-upsert/tasks.md`, concrete assertions in `internal/domain/agents/reconcile_test.go` and adapter/handler tests, minimal test changes, automated edge/error coverage, and no Bash correctness-validation scripts (Principle VIII).
- [ ] T067 Run `just check`, `just test`, `just test-integration-infra`, `just test-e2e-backend`, and `just test-e2e-docker`; record only actual passing results and measurement counts for SC-001–SC-006 in `specs/055-agent-external-id-upsert/tasks.md`, plus domain/store coverage using `just test-coverage` (Principles VIII, IX, XIII; ADR 004).

**E2E Acceptance Testing**

- [ ] T068 Verify all US1–US4 complete journeys in `tests/e2e/agent_external_id_upsert_test.go` turned green without weakening assertions, including ten-call replays/concurrency on both stores, no partial failures, migration/runbook recovery, and UUID/canonical preference compatibility; preserve test design and fixture-only adjustments (Principles VIII, XIII).
- [ ] T069 Run full `just test-e2e` as the final guard layer, including existing backend Docker/ExtProc/frontend suites, and record passed commands or genuine blockers with affected task dependencies in `specs/055-agent-external-id-upsert/tasks.md`; do not bypass missing system/container dependencies (Principles VIII, XIII).
- [ ] T070 Review final intended file diff, update only genuinely completed checkboxes in `specs/055-agent-external-id-upsert/tasks.md`, and record each deferred task/checklist/validation with path:line, reason, and required maintainer action. Keep the PR draft, preserve immutable spec/plan and historical feature 054 docs, and commit progress with the required Copilot co-author trailer before requesting a safe-output push (Feature Scope; Principles II, VIII, XIII).

Frontend-specific compliance is not applicable because no UI changes are introduced; T054/T057 verify that boundary. `just verify` is not required by this backend-only plan; the explicit static/unit/coverage/infra/complete-E2E gates above are mandatory and must not be reduced.

## Dependencies & Execution Order

### Phase Dependencies

```text
T001 approval → T002 source/version review
  → Phase 2a–2e design/OpenAPI/scaffolding
  → T011/T012 test utilities + T013 compiling route stub
  → T014 → T015 → T016 → T017 → T018 mappings → T019 E2E red
  → T020/T021/T022/T023 unit/infra red
  → T024 shared contract/validation → T025 migration
  → T026 memory + T027 PostgreSQL → T028 retire superseded test → T029 foundation checks
  → US1 tests T030/T031/T032 → T033/T034/T035 domain → T036/T037 handler → T038 checks
  → US2 T039/T040 red → T041/T042 validation/errors → T043 checks
  → US3 T044 coverage → T045/T046 operator docs → T047 runbook + infra/E2E checks
  → US4 T048 red → T049 presentation → T050 guide → T051 checks
  → Phase N T052–T070 compliance/final E2E/progress
```

- All design tests are written before feature implementation. Foundational invariant/migration tasks are shared because every story relies on them; US3 owns operator recovery and final upgrade acceptance.
- E2E tasks T014–T017 edit the same file and are sequential, not `[P]`. Fixture and bootstrap work T011/T012 can run in parallel after design review, before those tests.
- T020–T023 touch different files and can be authored in parallel after E2E red; each must fail semantically before its corresponding implementation. T022/T023 may add the compile-only sentinel declaration in `internal/ports/storage.go` when first referenced; this adds no enforcement behavior. T024 finalizes the contract/validation after red evidence.
- T026/T027 may execute concurrently only after T024/T025 and their respective red tests; neither is marked `[P]` globally because the foundational gate remains required.
- New unit-test API references may require empty compile-only signatures in T013/T030. Stub assertions always target intended behavior, never 501 as success.

### User Story Dependencies

- **US1 (P1)**: Requires all shared design/enforcement/migration prerequisites. T034/T037 must incorporate the already-designed selector/error and preference behavior for the operation to be complete; US2/US4 tasks exhaustively verify and finish those aspects, not postpone core safety.
- **US2 (P1)**: Ordinary-write uniqueness is delivered by shared foundation; collection-PUT invalid-input acceptance requires US1's functioning handler/service. Its new tables must precede any remaining validation/error code change.
- **US3 (P1)**: Migration red/green work is shared foundation; runbook HTTP remediation tests require US1/US2 and approved migration. Documentation T045 can be drafted alongside US1 once schema semantics are green.
- **US4 (P2)**: GET coverage depends on foundation; new write presentation depends on US1. T048 precedes any remaining presentation code change.
- Independently testable does not mean independent prerequisites: each story starts from fresh fixtures and proves its own journey without relying on another test's state.

### Parallel Examples per Story

- **US1**: T030 domain-policy tests and T031 handler contract/regression tests in separate files. T032 follows T030 because it edits the same test file.
- **US2**: T039 domain validation tables and T040 raw HTTP/error tables in separate files; service and handler changes follow both red checks.
- **US3**: Draft T045 runbook while reviewing T044 migration coverage in a different file; T046 links and T047 execution follow the completed runbook.
- **US4**: T050 guide prose can be drafted while T048 preference tests are authored; verify final prose only after T049 implementation matches the approved contract. Do not overlap T050 with US3's T046 edits to the same guide.
- No cross-story parallel editing of `agents_handler.go`, `service.go`, or the single E2E file.

## Implementation Strategy

1. Record actual approval, verify design preconditions, author all 22 E2E tests, and obtain semantic red evidence.
2. Deliver shared uniqueness/safe-migration prerequisites with unit/infra red-green cycles; remove the superseded 054 shared-ID assertion atomically with enforcement.
3. Demonstrate US1 as the MVP: create → ten replays → actual update → read back, plus concurrent creation. It is a review milestone, not permission to deploy without US2/US3/US4 and compliance.
4. Complete invalid-input/ordinary-write protections, operator recovery, and compatible presentation in priority order. Validate each independently with fresh fixtures.
5. Run all mandatory gates and full E2E as the final guard layer. Do not rewrite approved spec/plan, skip acceptance scenarios, or claim completion with unfinished validation. If a dependency genuinely fails, record and defer only dependent tasks while saving other permitted progress.

## Coverage Index

| Requirements / scenarios | Task coverage |
|---|---|
| FR-001–FR-003; DB-002, DB-004 | T003, T015, T020–T028, T039–T043, T062 |
| FR-004; DB-001, DB-003; US3.1–US3.4 | T008–T009, T012, T016, T023–T025, T044–T047, T056 |
| FR-005, FR-008, FR-009; US1.1–US1.4 | T013–T014, T030–T038, T067–T068 |
| FR-006, FR-007, FR-013; US2.1–US2.8 | T015, T021–T023, T039–T043, T063 |
| FR-012; US1.5–US1.7 | T014, T030–T031, T033–T037 |
| FR-010, FR-011; US4.1–US4.3 | T017, T028, T048–T051, T058–T059 |
| API-001–API-005 | T006–T007, T031, T037, T040–T042, T048–T050, T055, T059 |
| SR-001–SR-003 | T005, T013, T034, T037, T039–T042, T045, T063–T065 |
| SC-001–SC-006 | T014–T017, T030–T032, T038, T043, T047, T051, T067–T068 |
| Principles II, V–XIV / approval / scope | T001–T019, T052–T070 |
