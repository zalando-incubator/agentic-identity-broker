# Tasks: Unique Agent External IDs and Idempotent Create-or-Update

**Input**: Design documents from `specs/055-agent-external-id-upsert/`.
**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/admin-agents-openapi.yaml`, `quickstart.md`.
**Status**: All 73 agent tasks complete under the revised decision approved by mjungsbluth on 2026-11-04, including the first container-validation repair. Agent copies now preserve empty string collections so PostgreSQL single and filtered reads agree without weakening replay assertions. All required non-container gates pass. PostgreSQL/migration and container E2E validation is owned by the workflow on the repair commit, not claimed passed locally. The repair evidence below supersedes earlier validation records.
**Tests**: Mandatory red-green-refactor under Principles VIII and XIII. Write tests first, make them compile using minimal scaffolding where needed, observe concrete semantic failures, implement, then refactor without weakening assertions.
**Organization**: Shared design and enforcement prerequisites precede independently verifiable user-story increments. Paths are relative to the repository root. `[P]` means different files and no mutual dependency within the stated batch; it does not waive earlier gates.

## 🔒 Feature Scope [MANDATORY]

Every task cites a specification requirement, acceptance scenario, success criterion, or binding constitutional principle. Implement sign-off items only when `context.approval` covers them. T001 records supplied approval; it does not grant approval. No application work is authorized by this task-generation session. Do not change `spec.md` or `plan.md` during implementation, alter historical feature 054 documents, introduce new dependencies/configuration, or bundle unrelated cleanup.

Sign-off inventory: API-UPSERT, API-OMISSION, API-UNIQUE, API-VERSION, API-ORDER, revised ADR-045, DB-046 and DEC-054 in `plan.md` §Approval Items. API-TIMESTAMPS is withdrawn, not a pending approval. The earlier approval remains historical and does not authorize the revised ADR. Leave tasks depending on uncovered items unchecked and continue independent permitted work.

**Resume from existing implementation**: Keep task IDs and unaffected completed work. Reopened tasks describe the current revision, not a clean-slate rebuild. Do not execute completed scaffolding tasks T009/T013, replace working migrations with no-ops, or restore a 501 route. All 22 acceptance scenarios remain required. Do not check an unchecked reviewer-owned checklist item yourself.

**Validation ownership**: Never run container-backed or aggregate container-including recipes in the sandbox. Container tests own PostgreSQL/migration, backend Docker and ExtProc agentgateway evidence on the pushed commit; absence of local containers is not a blocker. Locally runnable tests must proceed independently. Historical checked tasks record earlier work/handoff, not current CI passage; T067–T069 require fresh validation for the revision.

## Approved architecture remediation — F1–F5 (2026-11-04)

Scope: [P1–P5 approved by mjungsbluth](https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6629376767). These tasks supersede contradictory historical port/validation/reread wording below; no migration or timestamp change is included.

- [X] T074 [F1] Share one update-candidate builder/validator between `Service.Update` and reconcile, remove handler field policy, and retain update/no-op/audit tests (FR-012; Principle VI).
- [X] T075 [F2] Move FR-019 permission-set service coverage into `agents.Service` for Create, Update and reconcile; inject the resolver in the builder and test direct-domain rejection without mutation (Principles VI, VIII, XII).
- [X] T076 [F3] Share full External ID UTF-8/no-NUL/1–255-code-point validation across selectors and all Agent writes; test invalid values and the multibyte boundary, document the approved validation contract (FR-003; Principles IV, VIII).
- [X] T077 [F4] Replace the list-shaped unique-key port with GetByExternalID on both adapters, preserve HTTP array filtering, retry only ErrExternalIDConflict and test not-found/clone/error/bounded reread behavior (ADR 004; Principles VI, IX).
- [X] T078 [F5] Rename ADR 045 and update links/approval, use reconcile in OpenAPI and prefix outcome constants; align spec/plan/domain docs and run static, unit, self-contained integration and backend acceptance gates (Principles II, V, XIII).

### Apply-review evidence

- Semantic red before behavior changes: `TestWritesEnforceServiceCoverage` accepted uncovered requirements on Create/Update/reconcile/replay; `TestWriteExternalIDFormat` and `TestAgentExternalIDUnicodeBoundary` accepted invalid values; `TestReconcileDoesNotRereadUnrelatedConflict` observed two reads instead of one. All now pass.
- Update/reconcile tests verify identity preservation and exactly one service-validation/client-uniqueness check. Existing no-op timestamp, mutation audit, field presence, coverage, conflict and HTTP presentation assertions remain enabled.
- Passed: `just check`, `just test` (race-enabled package gate), `just test-integration`, `just test-e2e-backend` (928 passed, 0 failed, 18 excluded by existing performance/container label filters). All 15 non-container feature acceptance scenarios pass.
- PostgreSQL integration test binary compiles with `go test -c -tags=integration -o /tmp/gh-aw/agent/postgres.test ./internal/adapters/storage/postgres`; this is compile-only evidence, not a PostgreSQL test pass.
- Deferred to container tests on the pushed commit: `just test-integration-infra`, `just test-e2e-docker`, including US1.2–US1.4 and US3.1–US3.4. No container suite was run or claimed validated locally. No migration, workflow, hook, skill, constitution, configuration or feature-054 file changed.

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 Record only the new approval supplied by `context.approval` for the revised `specs/055-agent-external-id-upsert/adr-045-unique-agent-external-id-upsert.md` as `Accepted (approved by <login> on <date>: <url>)`; move the approved revision into `adrs/045-unique-agent-external-id-reconcile.md`, replacing this PR’s earlier text, and retain a feature-local forwarding document for immutable spec/plan links. Verify coverage of all eight current Approval Items; the 2026-10-31 approval does not cover the revised ADR. Update decision-record links/status in `specs/055-agent-external-id-upsert/research.md`, `data-model.md`, and `quickstart.md` only after recording the actual new approval (Principle II; ADR-045).
- [X] T002 Review the existing implementation and nearest instructions in `internal/AGENTS.md`, `internal/domain/AGENTS.md`, `internal/ports/AGENTS.md`, `internal/adapters/AGENTS.md`, `tests/e2e/AGENTS.md`, and `tests/integration/AGENTS.md`; confirm migration `migrations/046_unique_agent_external_id.up.sql` is this feature’s already implemented migration, not an unused number. Preserve completed invariant/migration work and record revision dependencies in `specs/055-agent-external-id-upsert/tasks.md`; do not recreate working code or renumber the approved migration (Principles II, VI, IX; DB-001).

Phase 0 is omitted: no new structural refactor is needed. T036’s small behavior-preserving request-helper extraction was already committed separately before feature behavior; preserve it. Any newly required structural refactor must likewise be its own no-behavior-change commit with existing tests passing, not a separate PR. Phase 2.7 is omitted: no new entity, table, typed ID or repository.

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation.
**Gate**: Complete applicable approval/design prerequisites and revised E2E assertions before changing production timestamp behavior. Original feature design/scaffolding is already present; do not rebuild it. T019 gathers revised red evidence after the test-authoring portions of T014/T030/T031, before T034/T037/T049.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [X] T003 Verify Agent aggregate, existing `id.ExternalID`/`id.AgentID`, reconcile outcomes, field presence, equality, and relationship-preservation rules against `specs/055-agent-external-id-upsert/data-model.md`; retain its documented invariant: "when non-nil it MUST be non-empty, and no two Agents may hold equal values (exact, case-sensitive byte equality). `nil` remains allowed on any number of Agents" (FR-001, FR-012, FR-013; Principles V, IX; ADR 013).
- [X] T004 Reconcile the Agent external-ID and versioning sections in `ARCHITECTURE.md` with revised ADR 045: keep uniqueness, stable UUIDs, bounded reread, URI-set equality and copied byte-sorted responses; replace microsecond precision, strict advancement and wall-clock-lead promises with UTC whole-second RFC 3339 presentation and existing current-time assignment. Retain the glossary, security/performance constraints and ADR index without changing unrelated timestamp guarantees (FR-001, FR-005, FR-008, FR-011; Principles II, V).

### Phase 2b: Configuration Design [MANDATORY]

- [X] T005 Verify no configuration key/default, example file, or Helm value/template change is needed; check the existing keys in Journey 1 of `specs/055-agent-external-id-upsert/spec.md` against `internal/ports/config.go` and confirm `charts/agentic-identity-broker/` still runs the separate migration Job without adding broker migration tooling (API-005, SR-001; Principle VII; ADR 009). Record applicability in `specs/055-agent-external-id-upsert/tasks.md`.

### Phase 2c: API Design [MANDATORY]

- [X] T006 Verify supplied approval covers API-UPSERT, API-OMISSION, API-UNIQUE, API-VERSION, API-ORDER and revised ADR-045, then reconcile the existing `api/admin/openapi.yaml` with `specs/055-agent-external-id-upsert/contracts/admin-agents-openapi.yaml`. Retain Admin API 3.0.0, all approved operations/security/selector/body/error/Location/Prefer/omission contracts and byte-sorted client URIs. Remove only the withdrawn API-TIMESTAMPS prose, examples and precision/strict-advancement promises: all Admin Agent times keep whole-second RFC 3339; creation and real updates use current time and no-op returns stored times (API-001–API-005; FR-008, FR-011; Principles IV, X).
- [X] T007 Verify the existing 2.1.0 → 3.0.0 compatibility/versioning note in `ARCHITECTURE.md` still covers unique/nonempty external IDs and shared URI ordering, but no longer calls microsecond timestamps an approved contract change; keep existing routes and end-user/ExtProc contracts in `api/enduser/openapi.yaml` unchanged (API-004, API-005; Principles II, IV, X).

### Phase 2d: Database Design [MANDATORY]

- [X] T008 Verify approved DB-046 design against `specs/055-agent-external-id-upsert/data-model.md`: partial unique index, non-empty check, old index removal/restoration, atomic DO blocks, NOWAIT locking, bounded diagnostics for both categories, complete runbook inventory, no row modification, and dirty-version recovery (FR-004, DB-001–DB-004; Principle IX).
- [X] T009 Create minimal no-op DO-block scaffolding in `migrations/046_unique_agent_external_id.up.sql` and `migrations/046_unique_agent_external_id.down.sql` solely to let migration tests execute and fail on the missing invariant; do not add constraints, cleanup, or functional DDL until T025 (DB-001; Principles VIII, IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

No frontend, design-system component, Wireloom sequence, screenshot, or JWT output changes apply. Verify that boundary through T010 and the final scope/configuration checks; do not add unrelated frontend work (API-005; Principles XI, XIV).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

- [X] T010 Review all four concrete journeys in `specs/055-agent-external-id-upsert/spec.md` against the revised approved contract and all 22 scenarios, especially US1.3’s rapid update: a changed definition may retain the same whole-second updated_at, and its next replay/GET must match exactly. Confirm real configuration keys, synthetic data, omission/null examples and no UI/JWT output work; record review in `specs/055-agent-external-id-upsert/tasks.md` without editing spec/plan during implementation (Principles XIII, XIV).
- [X] T011 [P] Add shared valid external-ID agent fixtures in `tests/e2e/fixtures/agent_external_id.go` and reusable collection-PUT HTTP helper in `tests/e2e/helpers/agent_external_id.go` using existing fixtures/request utilities, fresh IDs, nonempty permission declarations, and no production test-only API (US1–US4; Principle XIII).
- [X] T012 [P] Extend `tests/e2e/bootstrap/postgres.go` with `NewPostgresFixtureAtVersion(ctx, 45)` and migration up/down/version-inspection/recovery helpers required by legacy tests, retaining the existing latest-version constructor and cleanup, and production bootstrap for both admin brokers (US1.4, US3.1–US3.4; DB-003; ADR 007).
- [X] T013 Add a compiling `ReconcileAgent` stub returning 501 in `internal/adapters/http/handlers/admin/agents_handler.go` and register only `r.Put("/", h.Agents.ReconcileAgent)` in `internal/adapters/http/routing/admin.go` under existing admin middleware; add empty domain signatures only when needed for compiling unit tests (API-001; Principles VIII, XII, XIII).
- [X] T014 Revise the existing seven US1 `It()` blocks in `tests/e2e/agent_external_id_upsert_test.go` only for the changed timestamp requirement before production changes. Rename US1.3 to preservation of identity/creation time, not strict advancement; replace the future-time/+1 microsecond assertion with changed-definition, UTC whole-second output, current-time assignment, preserved id/created_at, then exact replay and GET equality. US1.2/US1.3 each retain one `Label("docker")` It with both store cases, reverse-lexical multi-URI create/update, sorted full response equality, ten replays and no replay write. Retain all omission/null/grant checks, US1.4’s two-broker/two-call and ten-call races, and the 22-scenario mapping. Do not require sleeps, a production clock API or a guaranteed same-second execution window (FR-005–FR-010, FR-012, FR-013; SC-002/SC-003; API-ORDER; Principles VIII, XIII).
- [X] T015 Write eight US2 `It()` blocks in `tests/e2e/agent_external_id_upsert_test.go` for US2.1–US2.8: duplicate POST, UUID/canonical-addressed conflicting update, self-update, all invalid selectors, mismatched/empty/null body external ID, invalid definition/references on both branches, canonical/client conflicts on both branches and both client-sharing modes, and multiple absent/null IDs versus empty writes. Assert no changes to IDs, timestamps, collections, or grants on failure (FR-001–FR-003, FR-006, FR-007, FR-010, FR-013; SC-004; Principle XIII).
- [X] T016 Write four `Label("docker")` US3 `It()` blocks in `tests/e2e/agent_external_id_upsert_test.go` for US3.1–US3.4: legacy duplicate inventory, empty/coexisting categories, deliberate runbook remediation and retry followed by real HTTP conflict/lookup checks, and rollback/reapply preserving agents and relationships. Use production admin servers before remediation and after migration, not isolated endpoint fragments (FR-004, DB-001–DB-004; SC-005; Principle XIII).
- [X] T017 Write three US4 `It()` blocks in `tests/e2e/agent_external_id_upsert_test.go` for US4.1–US4.3: exact/case-sensitive zero-or-one arrays, literal empty versus absent filter, and canonical nested references on create/update/replay with UUID identity and Prefer headers (FR-010, FR-011; SC-006; Principle XIII).
- [X] T018 Recheck `tests/e2e/agent_external_id_upsert_test.go` uses Describe → story Context → exactly one It per scenario with current nearby scenario references, fixtures, isolated production servers and concrete HTTP/read-back assertions. Refresh line mappings in `specs/055-agent-external-id-upsert/quickstart.md` after the timestamp-test edits, leaving plan.md unchanged (Principles VIII, XII, XIII).
- [X] T019 Before changing timestamp production code, revise the local timestamp assertions through T030/T031 and record compiling semantic failures against the old microsecond implementation in `specs/055-agent-external-id-upsert/tasks.md`, using the smallest package tests and `just test-e2e-backend` where applicable. T014’s both-store cases take container-backed red/green evidence from CI on the pushed commit. Preserve original feature red evidence below; do not restore a 501 stub or no-op migration, manufacture failures, count missing infrastructure as red, or delay locally runnable work for containers (Principles VIII, XIII).

**Checkpoint**: Revised decision approval recorded by implementation, current contract/design aligned, all 22 scenarios retained, and changed assertions authored before production changes. Obtain local semantic red evidence; CI supplies container-backed evidence without gating local work.

## Phase 2.5: Foundational Infrastructure

**Purpose**: Shared invariant and safe migration needed by all four stories. All test tasks in each batch precede their matching implementation task.

- [X] T020 [P] Write table-driven validation tests in `internal/domain/storage/agent_test.go` for non-nil empty external IDs on create/update/CIMD-snapshot validation, nil allowed, nonempty existing validation unchanged, and no mutation; run them and record semantic red evidence in `specs/055-agent-external-id-upsert/tasks.md` (FR-003, FR-013; data-model validation rule; Principle VIII).
- [X] T021 [P] Write memory tests in `internal/adapters/storage/memory/agent_repository_test.go` for duplicate create/update, self-update, move/clear/delete/reuse index entries, many nil IDs, literal case/whitespace distinctions, canonical/client-URI conflict failures leaving every index and collection unchanged, and concurrent mixed writes under `-race`; observe missing-invariant failures (FR-001–FR-003, FR-013; DB-004; Principle VIII). Verify ListByExternalID resolves the maintained index and returns cloned empty/one-element arrays under all move/clear/delete/reuse/conflict paths, including clone isolation; use in-package index assertions rather than a new production API (plan indexed-lookup goal).
- [X] T022 [P] Write real PostgreSQL tests in `internal/adapters/storage/postgres/agent_repository_test.go` using shared migrated clones for Create/Update `23505`/`uq_agents_external_id` conflict and `23514`/`chk_agents_external_id_not_empty` validation mapping, self-update, null IDs, and rollback of relationship writes on conflicts; assert `StorageError` kind and `errors.Is` sentinel, not adapter error text. Where direct constraint errors require an internal mapping helper, test it in-package and verify real constraint behavior separately; record semantic red evidence (FR-001–FR-003, FR-013; DB-002, DB-004; Principles VIII, IX).
- [X] T023 [P] Write migration integration tests in `tests/integration/migrations/agent_external_id_migration_test.go` with `//go:build integration` and existing framework/shared PostgreSQL clones at version 45: duplicates, empties, both categories including >100 entries/remainder/full inventory, clean UP, null IDs, exact equality, concurrent inserts/mixed ordinary writes, NOWAIT lock failure, atomic unchanged schema/rows on failure, dirty-version inspection, guarded retry, DOWN/reapply preserving permission declarations, service requirements, client URIs, grants, and other existing foreign-key relationships. Run against T009 scaffolding and record semantic red evidence (FR-004, DB-001–DB-004; Principles VIII, IX).
- [X] T024 Add `ports.ErrExternalIDConflict` and document Create/Update conflicts and zero-or-one `ListByExternalID` behavior in `internal/ports/storage.go`; reject empty external IDs in `internal/domain/storage/agent.go` with `external_id cannot be empty when provided` while retaining nonempty existing-write validation (FR-001–FR-003; data-model Ports/Validation; Principles VI, IX).
- [X] T025 Implement approved UP/DOWN behavior in `migrations/046_unique_agent_external_id.up.sql` and `migrations/046_unique_agent_external_id.down.sql`: `SET LOCAL statement_timeout = '5s'`, single atomic DO-block DDL, `ACCESS EXCLUSIVE MODE NOWAIT`, inventory both categories before DDL, first 100 of each plus remaining counts and runbook hint, `uq_agents_external_id`, `chk_agents_external_id_not_empty`, and inverse index restoration with no row changes. Prove T023 green through the production migration runner (DB-001–DB-004; DB-046; ADR 039 exception not extended).
- [X] T026 Implement `byExternalID map[id.ExternalID]id.AgentID` in `internal/adapters/storage/memory/agent_repository.go`, checking external IDs before other index mutations under the write lock, updating/moving/clearing entries atomically, deleting entries on Delete, and wrapping conflicts with the shared sentinel and required message; prove T020/T021 green (FR-001–FR-003, FR-013; data-model Memory adapter; ADR 004). Implement ListByExternalID through byExternalID under the read lock, cloning the primary-map result and returning empty/one-element arrays without a registry scan; prove the indexed lookup tests green.
- [X] T027 Implement PostgreSQL constraint-specific error mapping in `internal/adapters/storage/postgres/agent_repository.go` for Create/Update without exposing SQL errors or partially changing client URIs/definition fields; preserve sqlx, explicit SQL columns, timeouts, and existing transaction boundaries; prove T022 green (FR-002, FR-003, FR-013; DB-002, DB-004; ADR 004).
- [X] T028 Remove only the superseded shared-ID `It()` and `researchEU` fixture from `tests/e2e/agent_external_id_filter_test.go` in the same change as uniqueness enforcement; preserve remaining feature 054 tests and documents, and leave exactly one replacement US4.1 test in the new suite (FR-011; DEC-054; Principle XIII).
- [X] T029 Run `just check`, `just test`, and `just test-integration-infra`; record passing invariant/migration suites and still-expected reconcile failures in `specs/055-agent-external-id-upsert/tasks.md` without claiming unfinished story completion (DB-003, DB-004; Principles VIII, IX).

## Phase 3: User Story 1 — Reconcile an agent in one retry-safe call (Priority: P1) 🎯 MVP

**Goal**: One write creates, updates, or returns unchanged state under a stable external ID.
**Independent Test**: From a fresh registry, create, repeat ten times, change the name, omit/clear managed fields, and read back one stable UUID with appropriate timestamps and intact grants; race identical calls across two brokers.

### Tests for User Story 1 [MANDATORY - Principle VIII]

- [X] T030 [P] [US1] Amend timestamp cases in `internal/domain/agents/reconcile_test.go` before implementation while preserving field-policy/equality/conflict coverage. Replace microsecond truncation and `reconcileUpdatedAt` strict-minimum assertions with call-time bounds for current UTC creation/update, preserved CreatedAt, exact stored-time preservation and zero Update/audit on replay. A future observed UpdatedAt must not force a logical increment; keep fixture CreatedAt in the past so current-time assignment remains valid. Cover fractional stored values without imposing storage precision, and keep ten-replay/concurrency coverage. Also amend `tests/integration/agent_reconcile_test.go` to replace future-time/+1 microsecond expectations with changed-definition/current-time whole-second output and exact update → replay → GET equality, retaining ten concurrent first calls/replays and coherent mixed definitions. Do not add a clock abstraction/test-only production API; record semantic red before T034 (FR-008, FR-009, FR-012, FR-013; SC-002/SC-003; Principles VIII, IX).
- [X] T031 [P] [US1] Update `internal/adapters/http/handlers/admin/agents_presentation_test.go` and `agents_reconcile_canonical_test.go` before production changes to require `t.UTC().Format(time.RFC3339)` on POST, collection PUT, ID-addressed PUT, single GET and list/filter GET under both reference preferences. Use fractional/non-UTC fixtures to catch sub-second output and verify source timestamps/slices are not mutated. Preserve byte-sorted URI expectations, canonical/UUID fallback, body-presence/schema/64 KiB checks in `agents_handler_test.go`, UUID Location and exact replay equality; record semantic red before T037/T049 (FR-005, FR-007, FR-008, FR-011, FR-012; API-003, API-ORDER; Principle VIII).
- [X] T032 [US1] Add bounded conflict-reread tests to `internal/domain/agents/reconcile_test.go`: external/canonical/client-URI create collisions, same-selector holder appears, no holder returns original conflict, lookup errors propagate, >1 holder fails closed without survivor selection, and persistent update conflict returns 409 with no unbounded retry. Include ten concurrent identical calls for both backend behavior and verify different concurrent desired definitions remain coherent (FR-009, FR-010, FR-013; SC-003; research R3; Principles I, VIII).

### Implementation for User Story 1

- [X] T033 [US1] Implement `ReconcileInput`, Created/Updated/Unchanged outcomes, and unexported field-policy/effective-equality helpers in `internal/domain/agents/reconcile.go`: omitted identities preserved, metadata removed, optional collections emptied, supplied fields replaced; use exact documented comparison and never alter grants. Required fields retain the constraint "required"; `permission_sets` remains "nonempty and valid"; `client_id` values remain a "validated nonempty string" (FR-008, FR-012; data-model field policy/equality; Principles V, VI).
- [X] T034 [US1] In `internal/domain/agents/reconcile.go`, replace microsecond-truncated creation and `reconcileUpdatedAt` minimum advancement with existing `time.Now().UTC()` assignment: creation sets both times to the same current instant; a real update preserves CreatedAt and sets UpdatedAt to current time; no-op returns exact stored times without write/audit. Remove the now-unneeded logical-clock helper rather than replacing it with another clock or second-sized increment. Preserve selector validation, field policy, bounded conflict reread, coherent writes and audit behavior; prove T030 green (FR-005, FR-008–FR-010, FR-012, FR-013; SR-003; research R3/R4; Principles VI, XII).
- [X] T035 [US1] Prove amended T030 and retained T032 green in `internal/domain/agents/reconcile_test.go`, then refactor only feature-required duplication in `internal/domain/agents/service.go` and `reconcile.go`. Keep existing timestamp audit fields and all no-write/mutation-audit checks, repository methods and bounded context unchanged; no test-only clock APIs (FR-008, FR-013; Principles VI, VIII, IX).
- [X] T036 [US1] Extract the small shared AgentRequest-to-Agent conversion helper in `internal/adapters/http/handlers/admin/agents_handler.go` for CreateAgent/UpdateAgent/ReconcileAgent; keep existing create/update attribute/reference validation and error contracts unchanged, capture presence separately, and keep new selector/field-policy/no-op/race invariants in the domain service; prove T031 regression cases green (FR-007, FR-010, FR-012; Principles VI, VIII).
- [X] T037 [US1] Preserve the existing ReconcileAgent behavior in `internal/adapters/http/handlers/admin/agents_handler.go`: bounded parsing, create-schema validation, FinalizeRequestSecurityContext, domain call, 201 UUID Location versus 200 and standard errors. Update shared write presentation to `t.UTC().Format(time.RFC3339)` after T031 red, retaining copied byte-sorted client URIs, omission and canonical preferences; T049 completes list/read parity. Keep existing Builder and middleware wiring in `internal/adapters/http/routing/admin.go`, without reintroducing a stub or direct repository calls (FR-005, FR-007, FR-008; API-ORDER; SR-001; Principles VI, XII).
- [X] T038 [US1] Prove amended `tests/integration/agent_reconcile_test.go` and domain/handler tests green after T030/T031 red and T034/T037/T049 changes. Run `just check`, `just test`, `just test-integration`, and `just test-e2e-backend`, recording actual results in `specs/055-agent-external-id-upsert/tasks.md`. Both-store US1.2/US1.3 and race US1.4 validation belongs to container tests on the pushed commit; do not run container suites in the sandbox (FR-008, FR-009; SC-001–SC-003; Principles VIII, XIII).

**Checkpoint**: US1 is demonstrable, but it is not a deployable partial release without the uniqueness, invalid-input, safe-upgrade, and compatible-presentation prerequisites.

## Phase 4: User Story 2 — Keep external identities unambiguous across all writes (Priority: P1)

**Goal**: Every supported write enforces unique set external IDs and failures never mutate records.
**Independent Test**: Register two distinct agents; exercise conflicts, self-updates, invalid selectors/definitions on both collection-PUT branches, and nil versus empty external IDs; read back all originals and relationships.

### Tests for User Story 2 [MANDATORY - Principle VIII]

- [X] T039 [P] [US2] Add domain validation tables in `internal/domain/agents/service_test.go` for shared selector rules: "1–255 code points, valid UTF-8, no NUL", missing/empty/256 versus valid 255 multibyte code points, body mismatch/empty/null, literal whitespace/case/UUID-shaped IDs, and zero writes on error; preserve existing GET behavior and nonempty ordinary-write validation. Run and capture new semantic failures before T041 (FR-003, FR-006, FR-007; SR-002; Principle VIII).
- [X] T040 [P] [US2] Extend `internal/adapters/http/handlers/admin/agents_handler_test.go` with raw-query decoding/repeated/undecodable/semicolon cases, unknown or malformed unrelated query parts, missing/misspelled selector, null client ID, invalid required/reference inputs, canonical/client/client-URI conflicts on both branches, and exact 400/409 error envelopes on all three write operations; verify no data leaks/mutations (FR-002, FR-003, FR-006, FR-007, FR-010, FR-013; SR-002; Principle VIII).

### Implementation for User Story 2

- [X] T041 [US2] Extract/reuse selector validation in `internal/domain/agents/service.go` for ListByExternalID and reconcile, enforce omitted body external ID → selector versus null/empty/mismatch rejection, validate effective required definition/reference/client-ID rules before writes, and preserve external-ID-only selection and fail-closed errors (FR-006, FR-007, FR-010, FR-013; SR-002; Principle VI).
- [X] T042 [US2] Reuse `externalIDFromQuery` in `internal/adapters/http/handlers/admin/agents_handler.go`, explicitly require PUT's single selector, and map ErrInvalidExternalID → `400 invalid external_id`, other validation → `400 validation failed`, conflicts → `409 conflict` with `external_id is already held by another agent` for external-ID conflicts; preserve ordinary POST/update behavior except approved changes (FR-002, FR-003, FR-006, FR-007; API-UNIQUE, API-UPSERT).
- [X] T043 [US2] Run `just check`, `just test`, and `just test-e2e-backend` and verify US2.1–US2.8 through T015 in `tests/e2e/agent_external_id_upsert_test.go`; compare stored UUIDs, timestamps, collections, and grants before/after rejection and record SC-004 results in `specs/055-agent-external-id-upsert/tasks.md` (FR-013; Principles VIII, XIII).

**Checkpoint**: All write paths reject ambiguous identities and invalid inputs without partial changes.

## Phase 5: User Story 3 — Upgrade safely without losing governed agents (Priority: P1)

**Goal**: Operators account for every legacy agent and deliberately remediate before a non-destructive upgrade.
**Independent Test**: At schema 45 seed duplicate/empty IDs and relationships; observe atomic block and both inventory categories; remediate explicitly, retry, verify through HTTP, then rollback/reapply unchanged relationships.

### Tests for User Story 3 [MANDATORY - Principle VIII]

- [X] T044 [US3] Verify T016/T023 exercise every step of US3.1–US3.4 through the same production migration runner and approved admin remediation contract; preserve their original red assertions and confirm direct constraints, blocked schema/row preservation, guarded version recovery, and full/bounded inventory behavior in `tests/integration/migrations/agent_external_id_migration_test.go` (FR-004, DB-001–DB-004; SC-005; Principles VIII, IX, XIII).

### Implementation for User Story 3

- [X] T045 [US3] Write `docs/operations/agent-external-id-uniqueness.md` with explicit-column inventory SQL for every duplicate/empty affected agent, bounded error/remainder interpretation, writer coordination without weakening authentication, non-destructive ID-addressed Admin PUT remediation (distinct external ID or omission to clear), dirty-version inspection and `force 45` only after unchanged-schema verification, retry/postchecks, and rollback/reapply prerequisites; retain migration/runtime role separation (FR-004; DB-001, DB-003; SR-003; research R8/R9; ADR 009).
- [X] T046 [US3] Link the upgrade runbook from `docs/operations/deployment-checklist.md` and `docs/guides/manage-agents-and-services.md`, warn that release blocks on unresolved duplicates/empties, and document preservation of UUIDs/grants and ordinary API compatibility impact (FR-004; API-002, API-004; SC-005).
- [X] T047 [US3] Execute runbook inventory/remediation/recovery/verification/rollback/reapply against disposable fixtures in `tests/integration/migrations/agent_external_id_migration_test.go` and US3 E2E tests; run `just check`, `just test-integration-infra`, and `just test-e2e-docker`, recording full results in `specs/055-agent-external-id-upsert/tasks.md`. Never clear dirty metadata automatically or claim missing containers as passed validation (DB-003, DB-004; SC-005; Principles I, IX, XIII).

**Checkpoint**: Every affected value/UUID is discoverable and all relationships survive block, deliberate repair, UP, DOWN, and reapplication.

## Phase 6: User Story 4 — Keep lookup and reference presentation compatible (Priority: P2)

**Goal**: Existing lookup arrays and exact matching remain compatible; sync responses honor canonical reference preferences without changing identity.
**Independent Test**: GET zero/one matches, empty/absent filters, and PUT create/update/replay with Prefer; verify nested aliases/UUID fallback, UUID agent id, headers, and persisted UUID relationships.

### Tests for User Story 4 [MANDATORY - Principle VIII]

- [X] T048 [US4] Before T049, review the timestamp regression assertions in `internal/adapters/http/handlers/admin/agents_presentation_test.go` and `agents_reconcile_canonical_test.go` (after T031; same files, not parallel): all Admin Agent paths emit whole-second UTC RFC 3339, while canonical references, UUID fallback, Vary, conditional Preference-Applied, UUID Location and optional omission stay unchanged. Preserve exact replay/GET equality and reverse-lexical/reordered URI-set coverage; verify the T031 semantic-red evidence without requiring an update’s displayed timestamp to differ. Retain one E2E It per existing scenario in `tests/e2e/agent_external_id_upsert_test.go` (FR-008, FR-010, FR-011; API-003, API-ORDER; Principles VIII, XIII).

### Implementation for User Story 4

- [X] T049 [US4] Reconcile both shared and list-specific Admin Agent response paths in `internal/adapters/http/handlers/admin/agents_handler.go` to `t.UTC().Format(time.RFC3339)` with no microsecond truncation/RFC3339Nano presentation. Keep copied byte-sorted client_uris, unchanged stored timestamps/URI order/other collection order, canonical nested references and UUID persistence, Vary/Preference-Applied, optional omission and no-op semantics. Prove T031/T048 green across POST, both PUT forms and single/list/filter GET (FR-008, FR-010, FR-011; API-003, API-ORDER).
- [X] T050 [US4] Reconcile `docs/guides/manage-agents-and-services.md` with the revised contract: retain unique nonempty IDs, zero-or-one lookup, collection PUT/omission/null/error/UUID Location/Prefer examples and Admin API 3.0.0 migration warning. Replace microsecond precision/strict advancement/wall-clock-lead guidance with unchanged whole-second timestamps and existing current-time assignment: two changes in one second may display the same updated_at, but replay preserves the changed representation. Keep the approved shared URI ordering (FR-008, FR-010–FR-012; API-002–API-004, API-ORDER; Principle XIV).
- [X] T051 [US4] Run `just check`, `just test`, and `just test-e2e-backend`; prove T017/T031/T048 green in `tests/e2e/agent_external_id_upsert_test.go` and Admin handler tests, ensure retained feature 054 cases pass without duplicate US4.1 mapping, and record SC-006 in `specs/055-agent-external-id-upsert/tasks.md` (Principles VIII, XIII).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.
**Gate**: Complete all applicable verification tasks before declaring delivery complete. Missing approval, dependency, checklist evidence, or validation remains an explicit deferred task, not a silently checked item.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [X] T052 Verify every task in `specs/055-agent-external-id-upsert/tasks.md` traces to current approved scope/scenarios or a binding constraint, API-TIMESTAMPS is withdrawn, no unrelated cleanup entered the revision and only user-approved issue URLs appear in non-checkbox Future Work notes (Feature Scope & Deferred Findings).
- [X] T053 Verify `ARCHITECTURE.md` and `specs/055-agent-external-id-upsert/data-model.md` agree with revised approved ADR 045 on Agent invariants, current-time assignment, unchanged whole-second presentation, no-op timestamps, URI ordering, performance/security constraints and versioning; no logical minimum or timestamp revision guarantee remains in current guidance (Principles II, V).
- [X] T054 Verify no configuration/default or deployment-boundary changes exist in `internal/ports/config.go`, `examples/config/`, `examples/config/README.md`, or `charts/agentic-identity-broker/`; existing journey configuration remains accurate and the separate migration image owns 046 (Principle VII; ADR 009).
- [X] T055 Verify `api/admin/openapi.yaml` exactly expresses approved operations/body/null/omission/status/error/Location/Prefer contracts and Admin API 3.0.0; shared byte-sorted client_uris remains API-ORDER while timestamps keep whole-second RFC 3339/current-time assignment with no strict advancement. Record only the actual new approval and its covered items in `specs/055-agent-external-id-upsert/tasks.md`; do not reinstate API-TIMESTAMPS (Principles IV, X).
- [X] T056 Verify `migrations/046_unique_agent_external_id.up.sql` and `.down.sql` match documented indexes/check/lock/timeout/report/atomic/no-data-change semantics and sequential numbering; verify complete operator inventory and guarded recovery in `docs/operations/agent-external-id-uniqueness.md` (Principle IX).
- [X] T057 Verify linked journeys in `specs/055-agent-external-id-upsert/spec.md` and manual exchanges in `specs/055-agent-external-id-upsert/quickstart.md` match implemented whole-second/current-time behavior, unchanged replay representation, contract and configuration. Keep spec/plan immutable in implementation; ensure no frontend/Wireloom/JWT-output work entered scope (Principles XI, XIV).
- [X] T058 Verify all 22 scenarios retain one referenced Ginkgo It each in `tests/e2e/agent_external_id_upsert_test.go`, with refreshed quickstart locations, production bootstrap, fixtures/isolation, revised US1.3 name/assertions, recorded semantic red and no pending/Skip/always-fail annotations. Retained feature 054 tests keep their original mappings; container evidence comes from CI and is not a local prerequisite (Principles VIII, XIII).

#### Implementation Phase Verification [MANDATORY]

**API & Documentation / Architecture & Documentation**

- [X] T059 Verify response bodies/headers/errors/UUID references and all Admin Agent timestamp/URI presentations match confirmed `api/admin/openapi.yaml`; guide/runbook/deployment checklist retain upgrade guidance without obsolete microsecond/advancement promises. Record evidence in `specs/055-agent-external-id-upsert/tasks.md`; no end-user docs/api change is required (Principles IV, X, XIV).
- [X] T060 Verify `adrs/045-unique-agent-external-id-reconcile.md` contains the revised approved decision and actual new approval citation; `specs/055-agent-external-id-upsert/adr-045-unique-agent-external-id-upsert.md` forwards to it and research/data-model/quickstart links and current status agree. Historical evidence may retain the former approval, not claim it covers this revision. Do not amend main-merged ADRs or implementation-phase spec/plan (Principle II).

**Configuration / Database & Persistence**

- [X] T061 Verify no ad-hoc configuration path or broker migration tooling was introduced in `internal/domain/agents/service.go`, `internal/adapters/http/handlers/admin/agents_handler.go`, or `charts/agentic-identity-broker/`; preserve configured timeouts and role boundaries (Principle VII; ADRs 004, 009).
- [X] T062 Verify unchanged memory/PostgreSQL uniqueness/null/empty enforcement, atomic collections, migrations/races/retry coverage and sqlx/explicit columns in `internal/adapters/storage/postgres/agent_repository.go` and `tests/integration/migrations/agent_external_id_migration_test.go`. Verify revised reconcile current-time assignment and whole-second HTTP read-back/replay parity in `tests/e2e/agent_external_id_upsert_test.go`: no-op preserves exact stored times, without new precision/schema or strict advancement. Keep indexed/cloned memory lookup and no new repository APIs (Principle IX; FR-008, FR-013).

**Security**

- [X] T063 Verify new collection PUT inherits admin authentication/principal/Host/cross-origin/JSON protections through `internal/adapters/http/routing/admin.go`, including denied requests without writes or leaks; add/run route-security regression assertions in `internal/adapters/http/routing/admin_test.go` (SR-001, SR-002; Principles I, VIII, XII).
- [X] T064 Verify `internal/domain/agents/service.go` emits structured existing agent-created/updated audits only for actual mutations, no mutation audit or write on replay, no credentials/tokens in reports/logs, and no custom crypto/default-off security fallback (SR-003; Principles I, III).

**Architecture Patterns**

- [X] T065 Verify selector/field-policy/equality/race decisions stay in `internal/domain/agents/`, port errors/contracts in `internal/ports/storage.go`, store enforcement in driven adapters, no cross-adapter imports or handler repository bypass, and existing Builder injection with routing-only registration in `internal/app/builder.go` and `internal/adapters/http/routing/admin.go` (Principles VI, XII).

**Testing**

- [X] T066 Verify revised timestamp unit/integration/E2E tests were authored before production changes and failed semantically, with evidence in `specs/055-agent-external-id-upsert/tasks.md`. Limit changes to the withdrawn requirement’s assertions/fixtures, retain URI/replay/no-write/audit/identity/security tests, and preserve original feature red evidence as historical rather than current sign-off; no Bash correctness tests (Principle VIII).
- [X] T067 Run `just check`, `just test`, `just test-integration`, `just test-coverage`, and `just test-e2e-backend` after the revision; record actual passed commands, coverage and SC-001–SC-006 evidence in `specs/055-agent-external-id-upsert/tasks.md`. Record `just test-integration-infra` and `just test-e2e-docker` as validated by the workflow’s container tests on the pushed commit, not run or passed locally. Do not make locally runnable work wait for container evidence (Principles VIII, IX, XIII; ADR 004).

**E2E Acceptance Testing**

- [X] T068 Verify all US1–US4 journeys in `tests/e2e/agent_external_id_upsert_test.go` pass their current assertions: ten-call replays/races, atomic failures, safe upgrade and UUID/canonical preferences. US1.2/US1.3 retain both-store reverse-lexical URI create/update replay, whole-second current-time presentation and exact replay/GET equality without strict timestamp advancement. Record local and container evidence separately in `specs/055-agent-external-id-upsert/tasks.md` (Principles VIII, XIII).
- [X] T069 Complete the full E2E final guard layer without invoking containers in the sandbox: run `just test-e2e-backend`, `just test-e2e-frontend`, and the repository’s existing non-container ExtProc selection `ginkgo -v --procs=2 --skip-file=agentgateway ./tests/e2e/extproc/`; record exact commands/results in `specs/055-agent-external-id-upsert/tasks.md`. Container tests own `just test-e2e-docker`, `just test-integration-infra` and ExtProc agentgateway scenarios on the pushed commit; do not invoke aggregate `just test-e2e` locally because it includes those suites. Do not edit recipes, disable tests or count CI handoff as local passage (Principles VIII, XIII).
- [X] T070 Review the final intended diff, mark only genuinely completed tasks in `specs/055-agent-external-id-upsert/tasks.md` and record outstanding tasks/checklists/validation with path:line and reason. Preserve historical evidence under its old scope, keep current approvals/validation distinct, leave spec/plan and feature 054 documents unchanged, keep the PR draft and commit permitted progress with `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>` before a safe-output push (Feature Scope; Principles II, VIII, XIII).

Frontend-specific development compliance is not applicable because no UI changes are introduced; T054/T057 verify that boundary. The existing frontend suite still belongs to the final E2E regression guard. `just verify` is not required by this backend-only plan; the explicit static/unit/self-contained-integration/coverage/full-E2E gates remain mandatory, split between the sandbox and container tests as described above.

## Dependencies & Execution Order

### Current revision graph

```text
T001 new approval → T002 baseline review
  → T004/T006/T007/T010 current architecture + contract + journeys
  → T014 revised E2E assertions → T018 mapping
  → T030 domain/integration tests + T031 presentation tests → T019 semantic red
  → T034 domain current-time assignment → T035 domain green
  → T037 shared write presentation → T048 shared matrix review → T049 read/list parity
  → T038 US1 local gates → T050 guide → T051 US4 gates
  → T052/T053/T055/T057–T060/T062/T066 compliance rechecks
  → T067 local static/unit/integration/coverage → T068 acceptance → T069 final E2E guard
  → T070 honest completion/progress record
```

- The 38 retained completed tasks are the existing foundation and unaffected story work, not new dependencies to reimplement. Original T020–T029 invariant/migration tests and US2/US3 journeys must remain green.
- T030 and T031 can be authored in parallel after revised E2E design: they touch different files. Both must fail semantically before changing their respective production paths. T019 records evidence; do not manufacture a red test for behavior already correct.
- T048 reviews the complete presentation matrix after T031; both touch the same handler-test files, so do not edit them concurrently. T049 must finish shared read/list presentation before T038 claims a green handler suite; bring this US4 prerequisite forward rather than deferring US1 safety or accepting a failing package. No new behavior is required for canonical references.
- T038/T051 may share one final passing command invocation when it covers both completed stories; record its actual scope once. Never infer fresh validation from old results.
- Container evidence is a parallel CI lane on the pushed commit, not a predecessor of locally runnable work. Full release validation remains mandatory; no local container pass is implied.

### User story dependencies and independent criteria

- **US1 (P1; 9 tasks, 6 reopened)**: Existing uniqueness/migration/handler foundation is retained. Revised domain and presentation tests precede current-time assignment. Independently prove create → ten replays → changed definition → replay/GET, stable UUID/created_at, whole-second output, intact grants and one-agent concurrent creation.
- **US2 (P1; 5 tasks, retained complete)**: Depends on the common invariant and existing US1 operation, not a new timestamp mechanism. Re-run its eight scenarios in final backend acceptance: invalid/conflicting requests change zero definitions, timestamps or relationships, and self/absent-ID writes remain valid.
- **US3 (P1; 4 tasks, retained complete)**: Uses the already implemented migration and runbook. Independently verify dirty-version guarded recovery, complete inventory, deliberate remediation, UP/DOWN/reapply and unchanged relationships in container tests. No new schema or migration number.
- **US4 (P2; 4 tasks, all reopened)**: Depends on existing lookup/invariant and revised shared write presentation. Independently prove zero/one arrays, empty-filter rejection versus unfiltered list, canonical nested references/UUID fallback and exact replay/GET representation on every Admin Agent path.
- Every scenario uses fresh fixtures and does not depend on another test’s state; independent verification does not mean prerequisites can be skipped.

### Parallel examples per story

- **US1**: T030 domain plus self-contained integration tests can be authored alongside T031 presentation tests; run each red before T034/T037. T030 owns both of its files exclusively.
- **US2**: Existing domain validation in T039 and HTTP validation in T040 are separate-file regression-review opportunities; no new edits are planned. Their story acceptance runs independently in final verification.
- **US3**: Existing T044 migration coverage and T045 operator runbook can be reviewed in parallel in different files; CI executes real PostgreSQL journeys. Do not rewrite completed work merely to create parallel tasks.
- **US4**: T050 guide prose can be reconciled while T048 reviews presentation tests, then verified against T049. Do not overlap T048 with T031 edits to the same test files or T050 with T046 guide edits.
- Never parallelize edits to the same production handler, domain source or E2E file.

## Implementation Strategy

1. Record the supplied new approval for the revised ADR and align the current contract/architecture; preserve the working feature foundation and earlier evidence.
2. Amend only the changed timestamp expectations first and record semantic red. Retain all URI-ordering, no-op/write/audit, validation, security and relationship assertions. Original red-green evidence for unaffected requirements remains historical evidence, not something to recreate.
3. Deliver revised US1 as the MVP review milestone: one-call reconciliation, current-time stored timestamps, whole-second responses and identical replay. It is not permission to deploy without every story and compliance gate.
4. Verify unaffected US2/US3 safety, complete US4 shared presentation/docs, then run static/unit/integration/coverage followed by the full E2E guard split across local and container environments.
5. Keep implementation-phase spec/plan immutable. Report genuine failed dependencies and deferred tasks honestly while completing independent permitted work; preserve all tasks/checklists and do not claim delivery with unfinished revision validation.

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

## Implementation verification — 2026-11-04 revised timestamps

**Outcome:** All 32 reopened tasks completed; the 38 unaffected tasks and migration 046 remain intact. No additional approval, deferred agent task or unchecked feature checklist item remains. All 16 requirements-checklist items were already checked and remain unchanged. Container validation is a separate workflow gate, not a local prerequisite or a claimed local pass.

### Approved scope and compliance (T001–T010, T052–T065, T070)

- Recorded only the supplied approval: mjungsbluth on 2026-11-04 at https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6625694887. It covers API-UPSERT, API-OMISSION, API-UNIQUE, API-VERSION, API-ORDER, revised ADR-045, DB-046 and DEC-054. Replaced this PR’s ADR 045 with its accepted revision and retained the feature-local forwarding record. No default-branch ADR was amended.
- Reviewed the existing Agent invariant, shared selector validation, field presence/replacement rules, zero-write/no-audit replay, single conflict reread, atomic adapter indexes/constraints and relationship preservation. Reused the existing migration 046 unchanged; no schema, dependency, port, configuration/default, Helm or deployment-owner change is introduced.
- Aligned OpenAPI 3.0.0, architecture, domain guidance, management guide, runbook, deployment checklist and feature research/data model/contracts/quickstart with whole-second UTC presentation and current-time assignment. Existing URI sorting, identity/error/omission/preferences and all security boundaries remain unchanged. Unrelated timestamp contracts are untouched.
- Reviewed the four concrete journeys, all 31 requirements and 22 acceptance scenarios against implementation. Existing index/glossary entries already cover the feature. Scope is limited to the approved revision; no UI/Wireloom/JWT output, unrelated cleanup or future-work issue was added. spec.md, plan.md and feature 054 documents remain unchanged.

### Timestamp TDD and acceptance (T014/T018/T019, T030–T038, T048–T051, T057–T059/T062/T066)

1. Before changing production, revised domain field-policy tests required actual update timestamps inside call-time bounds even when the observed timestamp is in the future. They compiled and failed semantically because the logical minimum assigned a future instant.
2. Before changing production presentation, the full HTTP-path/reference-preference matrix, fractional/offset fixtures and canonical replay required whole-second UTC output. They compiled and failed semantically: for example, expected `2026-01-01T00:02:03Z` but received `2026-01-01T00:02:03.123456Z`. The self-contained concurrent/replay journey likewise failed on fractional output and future-time assignment. These are the semantic-red evidence; an initially misconfigured new fractional-replay fixture was corrected to reuse the existing permission declarations and is not counted as red evidence.
3. Removed microsecond truncation and the logical-clock helper; creation and real updates use time.Now().UTC(), preserving creation time on updates. Shared and fallback/list responses use UTC RFC 3339 whole seconds. No-op preserves exact stored values, including fractional and offset values, without writes or mutation audits. The amended domain, handler and integration tests now pass.
4. First full unit/coverage runs identified one additional legacy fractional-output assertion in agents_reconcile_test.go. Corrected only that withdrawn requirement’s expectation; both complete gates pass on rerun. No assertion for URI order, identity, replay, auditing, validation, security or relationships was removed.
5. Retained exactly 22 scenario-referenced It blocks and refreshed quickstart locations. US1.2/US1.3 retain one docker-labeled scenario each exercising memory and PostgreSQL, reverse-ordered URI input, ten exact replays, stored-state preservation and GET equality. US1.3 checks changed definition/current-time bounds rather than strict advancement. PostgreSQL read-back bounds allow one microsecond for storage precision, not a logical increment. US1.4 and upgrade/race/relationship coverage remain intact. No sleeps, clock API, pending or bypass annotations were added.

### Current validation (T038/T051/T067–T069)

| Check | Result |
| --- | --- |
| just check | Pass after final Go changes: formatting, vet, lint; 0 issues. |
| just test | Pass with race detection. |
| just test-integration | Pass; ten concurrent first calls produce one Agent, ten replays preserve stored state, changed definition uses current time, then replay/GET equality and coherent mixed writes. |
| just test-coverage | Pass; agents domain 92.9%, Admin handlers 83.5%, memory 77.6%, PostgreSQL 19.1% (unit-only, not database coverage). |
| just test-e2e-backend | Pass: 928/946 specs; 0 failures/pending. All 15 local feature scenarios and retained feature 054 regressions pass; 18 existing container/performance exclusions. |
| just test-e2e-frontend | Pass: 67/67; 0 failures/pending/skipped. |
| ginkgo -v --procs=2 --skip-file=agentgateway ./tests/e2e/extproc/ | Pass: 98/115; 0 failures/pending, 17 container scenarios excluded by the existing non-container file selection. |
| Focused domain/handler regression | go test -race -count=10 with TestReconcile, TestAgentPresentation, TestAgentTimestamp and TestAgentResponseStablePresentation passes. |
| Infrastructure compilation | go test -tags=integration -c compiles migration and PostgreSQL binaries without executing them. |

SC-001 is demonstrated by direct create/update; SC-002 by exact ten-replay representation and stored-state preservation; SC-003 by the self-contained ten-call race with one creation; SC-004 by all eight rejection/invariant scenarios; SC-006 by all three lookup/preference scenarios. Both-store SC-002/SC-003 and migration SC-005 are validated by the workflow’s container tests on the pushed commit: just test-integration-infra, just test-e2e-docker and ExtProc agentgateway scenarios. No local execution or passing result is claimed for those suites. The checkmarks record completed agent work and that required CI handoff, not release authorization before the gate passes.

## Container-validation repair — round 1, 2026-11-04

- [X] T071 Add table-driven `TestAgent_Copy_StringCollections` in `internal/domain/storage/agent_test.go` for nil, non-nil empty and populated redirect URIs, allowed scopes and client URIs, including independent backing arrays; reproduce the semantic empty-to-nil failure before production changes (US1.2; FR-008, FR-013; Principles VIII, IX).
- [X] T072 Preserve nil/empty state and backing-array independence with standard-library `slices.Clone` in `internal/domain/storage/agent.go`. Keep PostgreSQL reads, reconciliation, presentation and all existing E2E assertions unchanged; prove the new regression green (US1.2; FR-008, FR-013; Principles VI, VIII, IX).
- [X] T073 Re-run `just check`, `just test`, `just test-integration`, `just test-coverage`, `just test-e2e-backend`, `just test-e2e-frontend` and the existing non-container ExtProc selection. Record results and hand off unchanged container assertions for validation on the repair commit (US1–US4; Principles VIII, XIII).

**Failure and cause**: The failed Docker E2E run https://github.com/zalando-incubator/agentic-identity-broker/actions/runs/41802660769 reports US1.2 at `tests/e2e/agent_external_id_upsert_test.go:166`: PostgreSQL filtered lookup returns empty redirect/scope arrays while the earlier single lookup returns nil. `Agent.Copy()` used `append([]string(nil), values...)`, which loses a non-nil empty slice. Single lookup calls Copy; filtered lookup retains the scanned empty arrays. This is inconsistent read-back, not an observed replay write. Standard-library cloning preserves both nilness and independence without changing the approved HTTP omission or timestamp contract. No E2E assertion, selector, migration, API or approved decision is changed.

**Red → green**: `go test ./internal/domain/storage -run '^TestAgent_Copy_StringCollections$' -count=1` compiled and failed only its empty case before the fix, showing all three empty string collections becoming nil. After the fix, `go test ./internal/domain/storage -run '^TestAgent_Copy' -count=1` passes nil, empty, populated and existing copy cases. The original container US1.2 assertion remains intact for CI green evidence.

| Check | Repair evidence |
| --- | --- |
| `just check` | Pass: formatting, vet and lint. |
| `just test` | Pass with race detection. |
| `just test-integration` | Pass: self-contained integration, including reconciliation journeys. |
| `just test-coverage` | Pass: 64.2% total statement coverage; not a PostgreSQL execution result. |
| `just test-e2e-backend` | 928/946 specs pass, 0 failures/pending; 18 existing container/performance exclusions. All 15 local feature scenarios pass. |
| `just test-e2e-frontend` | 67/67 specs pass; 0 failures/pending/skipped. |
| `ginkgo -v --procs=2 --skip-file=agentgateway ./tests/e2e/extproc/` | 98/115 specs pass, 0 failures/pending; 17 container scenarios excluded by the existing selection. |

No local work or new approval is deferred. `just test-e2e-docker`, `just test-integration-infra` and ExtProc agentgateway scenarios are deferred to the workflow's container tests on the repair commit. The provided prior run is failure evidence, not validation of this fix. Release remains gated on a passing container run; none is claimed here.

## Historical implementation evidence — before the 2026-11-04 revision

The records below describe the former approved microsecond design. Preserve them as provenance, not current instructions or current approval/validation. Their completion counts apply to those runs only; the current 32 reopened tasks and new approval requirement above supersede earlier statements that no work or additional approval remains. No application tests were executed in this document-only task revision.

### Implementation progress — 2026-11-02

**Outcome: core implemented; delivery remains incomplete.** 40 tasks are checked; 30 remain unchecked. This update supersedes the earlier run's container prerequisite. Container-backed red/green evidence belongs to CI and does not gate locally runnable work.

- Approval remains mjungsbluth's 2026-10-31 sign-off on all nine items: https://github.com/zalando-incubator/agentic-identity-broker/pull/336#issuecomment-6541899974. No new API, ADR, schema or scope decision is introduced; spec.md and plan.md are unchanged.
- Implemented atomic memory external-ID indexing, aggregate nonempty validation, shared sentinel and PostgreSQL constraint mapping; authored migration 046 with NOWAIT locking, bounded diagnostics, atomic DDL and data-preserving DOWN.
- Implemented domain create/update/no-op reconciliation, identity preservation/null policy, selector validation, one conflict-driven reread, microsecond timestamps and mutation-only existing audits. Handler implements bounded JSON parsing, create-schema validation, UUID Location, canonical preferences and standard errors; all Admin Agent response paths share copied URI sorting and UTC microsecond rendering.
- All 22 E2E scenarios are authored and mapped in quickstart.md. Fifteen in-memory scenarios pass; seven PostgreSQL/both-store scenarios await CI. Added migration/adapter infrastructure tests and an additional self-contained ten-call concurrent/replay/clock-rollback HTTP journey. Retired only feature 054's superseded shared-ID scenario and fixture; historical feature documents are unchanged.
- Request conversion refactoring is a separate behavior-preserving commit, with existing handler regressions passing before the feature commit.

### Red/green and validation evidence

| Suite or command | Actual evidence |
| --- | --- |
| Aggregate + memory invariant tests | Before enforcement: empty IDs unexpectedly valid, duplicate create unexpectedly succeeded, concurrent create produced 10 successes instead of 1. After enforcement: pass with race detector. |
| Domain reconcile tests | Compiling signature first returned nil: lifecycle/field-policy and bounded-reread assertions failed; invalid-input cases received no error. Implemented behavior now passes. |
| Handler/presentation tests | First observed 501 instead of 201/400, unsorted URIs and offset whole-second timestamps. Implemented behavior now passes. |
| PostgreSQL mapping unit tests | Compiling helper first returned nil for both constraints on both write operations; all four cases now pass. Real database constraint/rollback tests are authored for CI. |
| Original E2E red | Previous run recorded 12 feature-only semantic failures (501 collection PUT, duplicate POST/update accepted, empty POST accepted). New container scenarios have no claimed local execution evidence. |
| just check | Passed after final Go changes; 0 lint issues. |
| just test | Passed after final Go changes, with race detection. |
| just test-integration | Passed, including ten concurrent requests yielding one creation, ten unchanged replays, and one-microsecond advancement under a future observed timestamp. |
| just test-coverage | Passed; agents domain 91.9%, memory package 77.5%, PostgreSQL package 19.1% (unit-only, not real-database coverage). |
| just test-e2e-backend | Passed: 928/946 specs; 0 failures, 0 pending, 18 excluded by the existing non-container/performance recipe. All 15 local feature scenarios pass. |
| just test-e2e-frontend | Passed: 67/67; 0 failed/pending/skipped. |
| ExtProc non-container file selection | Passed: 98/115; 17 container scenarios not selected. An earlier title-focused selection also selected Agentgateway and failed for unavailable container infrastructure; no code/test weakening was made. |
| PostgreSQL/migration and complete container E2E | Authored for CI validation on the pushed commit; not claimed passed. An attempted compile-only PostgreSQL package command reached its existing TestMain container requirement and supplied no execution evidence. |

SC evidence currently available: SC-001 collection PUT creates/updates directly; SC-002 ten local identical replays leave exact stored state; SC-003 ten concurrent local first requests yield one 201 and nine 200s; SC-004 current eight US2 scenarios preserve originals/grants; SC-006 current three US4 scenarios and retained 054 scenarios pass. Both-store SC-002/SC-003 and migration SC-005 require CI plus remaining exhaustive coverage. No container absence is a maintainer infrastructure request.

### Completion verification (earlier implementation run)

All remaining regression work is authored. No application behavior, API, approved decision, dependency, configuration, spec.md or plan.md was changed in this run. Additional tests verify existing implementation; fixture defects found while extending the matrices were corrected without weakening assertions.

- **T012/T018/T023/T044**: Added dirty-version inspection and guarded schema recovery to the PostgreSQL E2E fixture. Migration tests inventory every UUID across 102 duplicate groups and 102 empty agents, assert exact remainder counts, and snapshot grants, credentials, authorization codes, refresh sessions, approvals and client URIs across failure, UP, DOWN and reapplication. Upgrade E2E preserves service requirements and URIs during HTTP remediation.
- **T015/T021/T032**: Expanded US2.7 for both client-sharing modes, both reconcile branches and client-URI conflicts, with populated metadata, collections and grant rejection snapshots. Memory tests assert every index after failed canonical/URI writes and mixed concurrent move/clear/delete/reuse. Two-broker PostgreSQL acceptance tests exercise two-call and ten-call first-write races plus coherent mixed definitions; the self-contained integration journey verifies mixed definitions concurrently on memory.
- **T030/T031/T035/T039/T040/T048/T049**: Added supplied-field create/update tables, literal and 255-code-point selectors, initial ambiguity/read errors, all create-conflict sources, every order-sensitive collection, all HTTP-path timestamp/URI presentation, raw-query parsing, standard write error envelopes, canonical nested references and UUID fallback through create/update/replay. Existing no-op write/audit assertions remain intact.
- **T057–T070**: Reviewed the 22 one-to-one scenario mappings, quickstart and API examples, relationship snapshots, dependency boundaries and final diff. Updated the ADR index/glossary and domain guidance only for this feature. No reviewer-owned checklist marker was changed.

Container-specific portions of T019, T022–T023, T025, T027, T029, T038, T044, T047, T058, T062 and T066–T069 are delegated to the workflow's container tests on the pushed commit, per the implementation execution rules. The checkmarks record completed agent work and that handoff, **not** a local PostgreSQL/container pass or release authorization. No non-container task is waiting for container evidence.

Current passing validation:

| Check | Result |
| --- | --- |
| `just check` | Formatting, vet and lint pass; 0 issues. |
| `just test` | Fast package suite passes with race detection. |
| `just test-integration` | Self-contained integration passes, including ten first calls, ten replays, same-second advancement and mixed desired definitions. |
| `just test-coverage` | Pass; agents domain 93.2%, memory 77.5%, Admin handlers 83.0%, PostgreSQL 19.1% (unit-only). |
| `just test-e2e-backend` | 928/946 specs pass; 18 existing container/performance exclusions. All 15 local feature scenarios pass. |
| `just test-e2e-frontend` | 67/67 specs pass; no failures, pending or skipped tests. |
| ExtProc non-container selection | `ginkgo -v --procs=2 --skip-file=agentgateway ./tests/e2e/extproc/`: 98/115 pass; 17 container scenarios assigned to CI. |
| Infrastructure test compilation | Both migration and PostgreSQL test binaries compile with `-tags=integration -c`; no containers executed locally. |

No deferred agent work or additional approval item remains. Release still requires the workflow's PostgreSQL/migration and container E2E gate.

### Implementation verification — 2026-11-03 (historical)

The implementation at `bd75127cf6fbaf9c522dd566c66f07c4e892afb3` satisfies the approved feature artifacts. All 70 tasks remain complete; all 22 scenario-to-test locations in quickstart.md are current. No new application behavior, API, ADR, dependency, configuration or sign-off item was introduced. The architecture's Admin route diagram now includes the implemented collection PUT; spec.md, plan.md and reviewer-owned checklist markers are unchanged.

| Check | Current-run evidence |
| --- | --- |
| `just check` | Pass; formatting, vet and lint, 0 issues. |
| `just test` | Pass with race detection. |
| `just test-integration` | Pass, including concurrent first calls, unchanged replays, timestamp advancement and coherent mixed definitions. |
| `just test-coverage` | Pass on unchanged rerun; agents domain 93.2%, memory 77.6%, Admin handlers 83.5%, PostgreSQL 19.1% (unit-only). The first attempt failed in unrelated `internal/adapters/jwks/static_adapter_test.go:25`: process-wide goroutine count fell from 28 to 27. No test or production code was changed to obtain the passing rerun. |
| `just test-e2e-backend` | 928/946 pass, 0 failures/pending; 18 excluded by the existing non-container/performance recipe. All 15 local feature scenarios pass. |
| `just test-e2e-frontend` | 67/67 pass, no failures/pending/skips. |
| `ginkgo -v --procs=2 --skip-file=agentgateway ./tests/e2e/extproc/` | 98/115 pass, 0 failures/pending; 17 container scenarios assigned to CI. |
| Infrastructure compilation | `go test -tags=integration -c` compiles both migration and PostgreSQL test binaries. No database execution is claimed. |

Initial 240-second test invocations timed out during cold compilation/suite execution; the complete invocations above passed with sufficient time. These attempts are not counted as successful checks.

`just test-integration-infra`, `just test-e2e-docker` and ExtProc agentgateway scenarios are validated by the workflow's container tests on the pushed commit. They were not run locally and no container pass is claimed. All non-container work is finished; no task is waiting on container evidence. This record does not authorize deployment before that gate passes.
