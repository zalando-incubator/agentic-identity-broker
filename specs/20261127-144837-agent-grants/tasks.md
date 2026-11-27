---

description: "Task list for agent grants (admin-issued delegation to an agent for itself)"
---

# Tasks: Agent Grants (Admin-Issued Delegation to an Agent for Itself)

**Input**: Design documents from `specs/20261127-144837-agent-grants/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/admin-agent-grants-openapi.yaml`, `contracts/observability.md`, `quickstart.md`

**Tests**: Mandatory red-green-refactor under Constitution Principles VIII and XIII. Write each
test first, make it compile against the Phase 2.7 scaffold, see it fail on a concrete assertion
where it runs locally, then implement. Red-run evidence is not required. Container-backed suites
(`just test-integration-infra`, `just test-e2e-docker`) may first run in CI on the pushed commit.

**Organization**: Phase 0 refactors, shared design, scaffold and foundation precede one phase per
user story in spec priority order (US1, US2, US3 are P1; US4, US5 are P2). Paths are relative to
the repository root. `[P]` means a different file and no dependency on an unfinished task in the
same batch.

## 🔒 Feature Scope [MANDATORY]

- Every task cites a requirement, acceptance scenario, success criterion or binding constraint of
  this feature. No task hands out a token, changes what token exchange or the delegation check
  decides for an agent subject, adds a machine session, adds `subject_kind`, or touches the UI
  (FR-020).
- The two Phase 0 refactors are required by FR-011 (one effective-scope definition) and by the
  spec's Domain Model ("reuses its validation"). The memory agent-deletion cascade is the minimum
  correction User Story 4 Scenario 5 and SC-007 need (research R13).
- The maintainer approves the API contract (API-006, Principle X) and the other decisions in
  `plan.md` §Review Items by reviewing the PR. No task waits for or records a separate approval.
- The deferred finding in `plan.md` §Deferred Findings (memory service `Delete` without an
  active-grant guard) is not a task. Only a user-approved issue URL may be recorded, under a
  `Future Work` note in this file.

---

## Phase 0: Pre-implementation Refactoring

**Purpose**: Two behaviour-preserving moves, committed separately ahead of feature commits
(plan §Implementation Phase Overview). All existing tests pass unchanged.

- [ ] T001 Write `internal/domain/storage/effective_scopes_test.go` with table tests copied from the coverage cases of `internal/domain/tokenexchange/grant_resolver_test.go`: no service requirements, `require_all_scopes`, intersection with `required_scopes`, empty intersection dropped with reason `scope_intersection_empty`, service outside the requirements dropped with reason `service_requirement_excluded`, union over several entries (FR-011; research R8; DEC-SCOPES).
- [ ] T002 Move the per-service computation of `(*GrantResolver).resolveGrantCoverage` in `internal/domain/tokenexchange/grant_resolver.go` unchanged into `storage.EffectiveScopes(agent, entries, sets)` in `internal/domain/storage/effective_scopes.go`, returning effective scopes per service and the exclusion reason per dropped service; make the resolver call it and map the reasons to its existing `FailureDetail` values. Make T001 green with `internal/domain/tokenexchange` tests unchanged (FR-011, FR-018, SC-004; research R8).
- [ ] T003 Turn `(*Service).validateGrantStructureWithResolved` and `(*Service).validateScopeInclusionWithResolved` in `internal/domain/consent/service.go` into package-level functions with the same bodies and messages; `GrantConsent` calls them as before. `internal/domain/consent/service_test.go` stays unchanged and green (spec Domain Model "reuses its validation"; research R10).
- [ ] T004 Run `just check` and `just test`; confirm zero test changes outside T001 and commit Phase 0 on its own (Principle VIII; template Phase 0 review discipline).

**Checkpoint**: Refactoring complete, all existing tests pass, no behaviour change.

---

## Phase 1: Setup

- [ ] T005 Read `internal/AGENTS.md`, `internal/domain/AGENTS.md`, `internal/ports/AGENTS.md`, `internal/adapters/AGENTS.md`, `tests/AGENTS.md`, `tests/e2e/AGENTS.md`, `tests/integration/AGENTS.md`, `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`; confirm `051` is the next free number in `migrations/` and that `api/admin/openapi.yaml` is at `3.1.0` (DB-002, API-006; Principles II, IX, XV).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain, configuration, API, database, journey, E2E and observability design before
production code.

**⚠️ CRITICAL**: No production code beyond Phase 0 before this phase completes.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II, V

- [ ] T006 Verify `specs/20261127-144837-agent-grants/data-model.md` against `spec.md` §Domain Model and FR-001–FR-020: `PrincipalKind` values, `AgentGrant.Grant`, `AgentGrant.Bindings`, `AgentGrant.Version`, `ServiceAccountBinding.ServiceID`, `ServiceAccountBinding.ServiceAccountID`, rules 1–10 and their order, write outcomes, preconditions, state. Correct `data-model.md` only where it contradicts the spec (Principle V).
- [ ] T007 [P] Add Agent Grant, Service-Account Binding and Principal Kind to `ARCHITECTURE.md` §12 and to the Domain Glossary table in `AGENTS.md` with the definitions of `data-model.md` §Glossary entries to add, including the note on the end-user route comment "Get agent grants" (spec Domain Model; Principle V).
- [ ] T008 [P] Write `adrs/20261127-183359-agent-grants-principal-kind.md` (Context, Decision, Consequences, Status) covering every item of research R14: agent grant as a `UserGrant` of kind `agent`, kind in grant and session identity, human-scoped existing ports and the kind-agnostic guards of research R2, `ports.AgentGrantRepository`, bindings beside the grant with the composite `RESTRICT` key, `storage.EffectiveScopes`, `BindingAgents` and `bound_by_agents`, migration `051` and its rollback erasing agent grants, Admin API `3.2.0`. State that no accepted ADR is superseded and that ADR 045 decision 6 is unchanged. Set Status to Accepted; add it to `ARCHITECTURE.md` §10 and the ADR Decision Index in `AGENTS.md` by full file stem (ADR-AG; Principle II).
- [ ] T009 Add to `ARCHITECTURE.md`: the `AgentGrantService` entry in the consent-domain part of §3, the `agent_grants` and `agent_grant_service_account_bindings` tables and `principal_kind` in §4, and the human scope of the grant and session ports (plan §Constitution Check "Architecture Docs"; Principle II).

**Checkpoint**: Domain model documented, glossary updated, ADR accepted in this PR.

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [ ] T010 Confirm that no configuration parameter is added: no change to `internal/ports/config.go`, `docs/configuration.md`, `examples/config/` or `charts/agentic-identity-broker/`; the chart's existing migration Job applies `051`. Record the result in a `## Design notes` section at the end of `specs/20261127-144837-agent-grants/tasks.md` (spec Assumptions; Principle VII).

**Checkpoint**: No configuration or Helm change, confirmed.

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV, X

- [ ] T011 Merge `specs/20261127-144837-agent-grants/contracts/admin-agent-grants-openapi.yaml` into `api/admin/openapi.yaml` as its header comment instructs: `info.version` `3.2.0`, an `Agent Grants` tag, `/api/agents/{agent-id}/self-grant` with `GET`, `PUT`, `DELETE`, the new header, schemas and responses, and the replaced `409` response (`ServiceAccountBoundConflict` with `bound_by_agents`) and description of `deleteServiceAccount`. Reuse `AgentId`, `OperatorRequired`, `NotFoundError`, `InternalServerError`, `ErrorResponse`; leave every other operation unchanged (API-001–API-003, API-006; API-AG, API-AG-SHAPE, API-SA-409, API-VERSION).
- [ ] T012 Verify the merged contract against Journeys 1–2 of `spec.md` (status codes, `ETag`, bodies, messages including the Journey 2 refusals), add the Admin API `3.2.0` note to `ARCHITECTURE.md` §5, and confirm `api/enduser/openapi.yaml` is unchanged (API-004, API-005; Principles II, IV).

**Checkpoint**: Contract merged; the maintainer confirms it in PR review.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [ ] T013 Verify `data-model.md` §PostgreSQL schema against DB-001–DB-003, FR-007, FR-009, FR-015 and FR-016: `user_grants.principal_kind` and `user_sessions.principal_kind` with their checks, `chk_user_grants_agent_principal`, both kind-scoped unique constraints and the dropped `idx_user_sessions_principal_service`, `uq_service_accounts_id_service`, `agent_grants.version`, the binding table's primary key, `fk_agent_grant_bindings_account` with `ON DELETE RESTRICT`, `idx_agent_grant_bindings_account`, the cascade chain, the DOWN order, explicit-column queries, one atomic pair without `-- migrate:no-transaction` (DB-051; Principle IX). The SQL files are written in T043 after their tests.

**Checkpoint**: Schema designed; migration number reserved.

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

Not applicable. The feature has no frontend change (spec Assumptions: the admin UI comes later),
so no design-system review, Wireloom sequence, Playwright test or screenshot applies
(Principle XI).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII, XIV

E2E tests reach the broker over HTTP, so they compile before the scaffold; without routes they
fail on `404`, after Phase 2.7 on `501`.

- [ ] T014 Review Journeys 1–3 of `spec.md` against their linked scenarios and the merged contract: requests addressed by canonical ID and by UUID, `ETag` on `201` and `200`, the mandatory set added without being listed, `granted_permission_sets` coverage, the `409` body with `bound_by_agents`, the Journey 2 messages against `data-model.md` §Resolution and invariants, the end-user listings of Journey 3, the absence of configuration and of JWT claims. Record findings in `## Design notes` of `specs/20261127-144837-agent-grants/tasks.md`; change `spec.md` only with the user's approval (Principle XIV).
- [ ] T015 [P] Create `tests/e2e/fixtures/agent_grant.go` with the spec cast and the journeys' IDs: *triage-bot* and *planner-bot* with their declared mandatory and optional sets, *Platform reader*, *Platform operator*, *Platform admin*, *Company Platform*, accounts `platform-triage-bot` and `platform-planner-bot` built on the #471 fixtures in `tests/e2e/fixtures/service_account.go`, plus builders for request bodies; all values synthetic (US1–US5; Principle XIII).
- [ ] T016 [P] Create `tests/e2e/helpers/agent_grant.go` with `PUT`, `GET` and `DELETE` wrappers taking an optional `If-Match`, a decoder for the `AgentGrant` representation and a decoder for the service-account `409` body; create `tests/e2e/agent_grants_backend_test.go` with `withAgentGrantBackend(backend string, action func(*bootstrap.TestServer, *storageadapter.Adapter))` for `memory` and `postgres`, following `withServiceAccountBackend` in `tests/e2e/service_accounts_backend_test.go` (research R16; Principle XIII).
- [ ] T017 [US1] Write the fourteen User Story 1 `It()` blocks in `tests/e2e/agent_grants_test.go` under `Describe("Agent grants")`, one `Context` per precondition, each preceded by `// Scenario N from specs/20261127-144837-agent-grants/spec.md (User Story 1)`, with the descriptions of `plan.md` §Scenario Mapping. Scenarios 12 and 14 carry `Label("docker")` and run both stores through `withAgentGrantBackend`. Scenario 9 asserts no audit record through the bootstrap's test logger. Scenario 13 writes a `valid_until` a few seconds ahead and waits with `Eventually`, no sleep. Assert status, `ETag`, every representation member and stored state after refusals (US1.1–US1.14; FR-001–FR-006, FR-019).
- [ ] T018 [US2] Write the seven User Story 2 `It()` blocks in `tests/e2e/agent_grants_test.go` (Scenarios 1–7), asserting `400`, the message naming the service, and an unchanged stored grant for each refusal; Scenario 5 renames the account through the #471 API and Scenario 6 adds a second account (US2.1–US2.7; FR-004b, FR-007–FR-010; SC-002).
- [ ] T019 [US3] Write the six User Story 3 `It()` blocks in `tests/e2e/agent_grants_test.go` (Scenarios 1–6), asserting the message naming service, account and excess scopes for Scenarios 1–3, and the accepted ceiling and permission-set updates with the grant unchanged for Scenarios 5–6 (US3.1–US3.6; FR-011–FR-013; SC-002).
- [ ] T020 [US4] Write the six User Story 4 `It()` blocks in `tests/e2e/agent_grants_lifecycle_test.go` (Scenarios 1–6). Scenarios 3–6 carry `Label("docker")` and run both stores. Assert `409` with `error`, `message` and the sorted `bound_by_agents`, the unchanged account and grant, then `204`; Scenario 6 waits for expiry with `Eventually` (US4.1–US4.6; FR-015; SC-005).
- [ ] T021 [US5] Write the six User Story 5 `It()` blocks in `tests/e2e/agent_grants_lifecycle_test.go` (Scenarios 1–6) on the dual-server pattern. Scenario 2 and 6 carry `Label("docker")` and run both stores, using a human principal equal to the agent's UUID. Scenario 3 reuses the self-token issuance of `tests/e2e/agent_subject_impersonation_test.go` and compares status, error, description and absent `error_uri` with the same request before the grant. Scenario 4 configures an `agent_subject_expression` that never matches but references `subject_token.sub`. Scenario 5 carries `Label("docker")`, starts from `bootstrap.NewPostgresFixtureAtVersion(ctx, 50)`, seeds a grant and a session with SQL, migrates and exercises the end-user reads (US5.1–US5.6; FR-016–FR-018, SR-003, SR-005; SC-003, SC-004).
- [ ] T022 Check both suites: `Describe` → `Context` → exactly one `It()` per scenario (39), scenario comments, fresh servers and storage per test, fixtures from T015, every `It()` that needs a grant first asserts the `201`, no `XIt`, `PIt`, `Skip()` or red-phase comments. Run `ginkgo -v --label-filter="!performance && !docker" --focus "Agent grants" ./tests/e2e/` and confirm each fails on a concrete assertion. Fill the `file:line` of each row of `plan.md` §Scenario Mapping (Principle XIII).

**Checkpoint**: Journeys reviewed; 39 E2E scenarios written and failing semantically.

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

**Constitution Reference**: Principle XV

- [ ] T023 Verify `contracts/observability.md` against `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`: event names `consent.agent_grant.created|replaced|deleted|write_failed`, levels, canonical keys, `operator_principal` as `id.Principal`, the grant's principal not logged, the four `storage.*.agent_grant` spans through `startStorageSpan`, no new span attribute namespace (so no §4.1 registry change), no metric instrument (so no metric entry), and the test matrix of §6. Record in `## Design notes` that no standard is amended (FR-019, SR-004; Principle XV).

**Checkpoint**: Observability design checked against the standards; no amendment needed.

---

## Phase 2.7: Entity Boilerplate (New Aggregate: AgentGrant)

**Purpose**: Empty but compiling scaffold, committed separately ahead of business logic. Handlers
answer `501`; repositories answer not found; domain methods return zero values.

- [ ] T024 Create `internal/domain/storage/principal_kind.go` with `PrincipalKind` and the constants of `data-model.md` §Value object: PrincipalKind; add the `PrincipalKind` field to `UserGrant` in `internal/domain/storage/user_grant.go` (`data-model.md` `UserGrant.PrincipalKind`) and to `UserSession` in `internal/domain/storage/user_session.go` (`data-model.md` `UserSession.PrincipalKind`) without behaviour yet (FR-016).
- [ ] T025 Create `internal/domain/storage/agent_grant.go` with `AgentGrant` (`data-model.md` `AgentGrant.Grant`, `AgentGrant.Bindings`, `AgentGrant.Version`) and `ServiceAccountBinding` (`data-model.md` `ServiceAccountBinding.ServiceID`, `ServiceAccountBinding.ServiceAccountID`), with `AgentID()`, `PermissionSetIDs()` and `Copy()` returning zero values (FR-002, FR-007).
- [ ] T026 Add `AgentGrantRepository` with the four methods and the sentinels `ErrAgentGrantVersionMismatch`, `ErrAgentGrantReferenceMissing`, `ErrServiceAccountReferenced` of `data-model.md` §Repository port to `internal/ports/storage.go`, with the contract as doc comments; add a row to the catalogue in `internal/ports/AGENTS.md` (DB-001; research R4; Principle IX).
- [ ] T027 [P] Create the memory agent-grant store in `internal/adapters/storage/memory/user_grants.go` (type implementing `ports.AgentGrantRepository` on the shared grant store) with methods returning not found or an empty slice (DB-001).
- [ ] T028 [P] Create `internal/adapters/storage/postgres/agent_grant_repository.go` implementing the port with methods returning a `storage.StorageError` of kind not found (DB-001).
- [ ] T029 Add `AgentGrants() ports.AgentGrantRepository` to the adapter in `internal/adapters/storage/factory.go` and construct both repositories in the memory and PostgreSQL constructors (ADR 004; Principle IX).
- [ ] T030 Create `internal/domain/consent/agent_grant_service.go` with `AgentGrantService`, `AgentGrantRequest`, the `ServiceAccountReader` interface, the outcome type (`created`, `replaced`, `unchanged`) and the constructor of `data-model.md` §Domain service (panic on a nil logger), with `Get`, `Put` and `Delete` returning zero values (research R1; Principles VI, XII).
- [ ] T031 Create `internal/adapters/http/handlers/admin/agent_grants_handler.go` with `AgentGrantsHandler` (`Get`, `Put`, `Delete` answering `501`); add `AgentGrants` to `AdminHandlers` in `internal/app/handlers.go`; construct service and handler in `internal/app/builder.go` with `b.storage.Agents()`, the permission-set service, `b.storage.ServiceAccounts()`, `b.storage.AgentGrants()`, `time.Now` and `b.logger`; register the three routes `/api/agents/{agent-id}/self-grant` in `SetupAdminRoutes` in `internal/adapters/http/routing/admin.go` only, with its route comment (API-001, FR-001; plan §Design Notes; Principle XII).
- [ ] T032 Run `just build` and `just check`; confirm the E2E suites of T017–T021 now fail on `501` instead of `404`.

**Checkpoint**: Scaffold compiles; no business logic.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Migration `051`, principal kind as identity in both stores with the human scope of
the existing ports, and both agent-grant repositories. Tests first. Every existing test stays
green at the end of this phase, which proves SC-004 and User Story 5 Scenario 5 early.

### Tests

- [ ] T033 [P] Write `internal/domain/storage/agent_grant_test.go`: `PrincipalKind` values and the invalid zero value, `UserGrant.Copy` copying the kind, the `AgentGrant` invariants (`data-model.md` `AgentGrant.Grant` kind `agent` and principal equal to the agent UUID, `AgentGrant.Bindings` at most one per service and sorted, `AgentGrant.Version` ≥ 1), `Copy` independence, sorted `PermissionSetIDs()` (FR-002, FR-007, FR-016).
- [ ] T034 [P] Write `tests/integration/migrations/agent_grants_migration_test.go` (`integration` tag): apply, rollback and reapply twice; existing grants and sessions keep every column and read `human`; each check and unique constraint of `data-model.md` §PostgreSQL schema; an agent principal different from `agent_id` refused; a cross-service binding refused with `23503`; a bound account delete refused; agent delete cascades to `agent_grants` and bindings; rollback removes only agent grants and the new tables, and restores the original constraints and index (DB-002, US5.5; Principle IX).
- [ ] T035 [P] Write the shared human-scope contract `tests/integration/storage/contract/principal_kind_scope.go`: with an agent grant of agent A stored, every human-scoped `UserGrantRepository` and `UserSessionRepository` method (and `WithLockedSession`) never returns, updates or deletes it, also for a human principal equal to A's UUID; a human grant and the agent grant with equal principal text coexist; `Create` and `CreateIfAbsent` write `human` and refuse another kind with a `validation` error; `DeleteByAgent` and `CountGrantsReferencingPermissionSet` count both kinds; expired agent grants do not count for `CountGrantsReferencingPermissionSet` (FR-015–FR-017, SR-003; research R2; `data-model.md` `UserGrant.PrincipalKind`, `UserSession.PrincipalKind`).
- [ ] T036 [P] Write the shared contract `tests/integration/storage/contract/agent_grant_repository.go` for every bullet of `data-model.md` §Repository port: `Get` with sorted bindings and expired grants; `Save` create, replace keeping `ID` and `CreatedAt`, version increments, conditional save with absent and different versions as `conflict` wrapping `ErrAgentGrantVersionMismatch`, a lost first-insert race, missing permission set, missing account and account of another service as `not_found` wrapping `ErrAgentGrantReferenceMissing` with nothing written, refusal of a wrong kind or principal, persisted fields copied back; `Delete` with and without version and the returned snapshot; `ListAgentsBindingServiceAccount` distinct, ascending, expired included, empty slice (DB-001, FR-003, FR-006, FR-009, FR-015; SC-007).
- [ ] T037 [P] Run both contracts on the memory store in `tests/integration/storage/agent_grant_repository_test.go` (`just test-integration`), following `tests/integration/storage/service_account_repository_test.go` (DB-001; SC-007).
- [ ] T038 [P] Run both contracts on PostgreSQL in `tests/integration/storage/infra/agent_grant_test.go`, including the four spans of `contracts/observability.md` §3 with kind `Client` and `storage.outcome` for not found and version mismatch; write `internal/adapters/storage/postgres/agent_grant_repository_unit_test.go` (no database) for error mapping of `23505`, `23503` and version mismatch to `StorageError` and the sentinels, and record conversion (DB-001; OPENTELEMETRY.md; Principle IX).
- [ ] T039 [P] Write memory adapter tests in `internal/adapters/storage/memory/user_grants_test.go` and `internal/adapters/storage/memory/user_session_test.go`: kind-prefixed indexes, a human and an agent row with equal principal text coexisting, human methods never returning the agent row (FR-016, FR-017; `data-model.md` §Changed contracts on existing ports).

### Implementation

- [ ] T040 Implement `internal/domain/storage/principal_kind.go`, the `UserGrant.Copy` change in `internal/domain/storage/user_grant.go` and `internal/domain/storage/agent_grant.go` per `data-model.md` §Value object: PrincipalKind and §Entity: AgentGrant (`AgentGrant.Grant`, `AgentGrant.Bindings`, `AgentGrant.Version`, `ServiceAccountBinding.ServiceID`, `ServiceAccountBinding.ServiceAccountID`). Make T033 green (FR-002, FR-007, FR-016).
- [ ] T041 Document the human scope and the named kind-agnostic methods on `UserGrantRepository`, `UserSessionRepository`, `UserSessionRefreshRepository`, `UserSessionExpiryRepository` and `UserSessionHealthRepository` in `internal/ports/storage.go`, per `data-model.md` §Changed contracts on existing ports (DEC-HUMAN-SCOPE; research R2).
- [ ] T042 Write `migrations/051_agent_grants_principal_kind.up.sql` and `migrations/051_agent_grants_principal_kind.down.sql` exactly as `data-model.md` §PostgreSQL schema, with table, column and constraint comments stating the kind's meaning and the `RESTRICT` rule. Make T034 green where containers run (DB-002, DB-003; `data-model.md` `UserGrant.PrincipalKind`, `UserSession.PrincipalKind`, `AgentGrant.Version`, `ServiceAccountBinding.ServiceID`, `ServiceAccountBinding.ServiceAccountID`; Principle IX).
- [ ] T043 Make `internal/adapters/storage/memory/user_grants.go` and `internal/adapters/storage/memory/user_session.go` human-scoped: key indexes by `kind + ":" + principal + ":" + agent` (and service for sessions), write `human` when the field is empty and refuse any other value, keep `DeleteByAgent`, `CountGrantsReferencingPermissionSet`, `CountByService` and the expiry and health readers kind-agnostic. Make T039 and the memory run of T035 green (FR-016, FR-017; research R2).
- [ ] T044 Make `internal/adapters/storage/postgres/user_grants.go` and `internal/adapters/storage/postgres/user_session.go` human-scoped: `principal_kind = 'human'` in every human-scoped statement, `principal_kind` written on insert, `ON CONFLICT (principal_kind, principal, agent_id)` and `ON CONFLICT (principal_kind, principal, service_id)`, refusal of another kind; leave `permission_set_repository.go` and `thirdparty_provider.go` guard queries kind-agnostic. Span names unchanged; explicit columns. Make the PostgreSQL run of T035 green (FR-016–FR-018; DB-003; research R2).
- [ ] T045 Implement the memory agent-grant store in `internal/adapters/storage/memory/user_grants.go` on the same mutex and indexes as the human grants, with version, bindings, permission-set existence and account lookup through injected interfaces wired in `internal/adapters/storage/factory.go`; lock order agent-grant store, then service-account store. Make the memory run of T036 green (DB-001; research R9; `data-model.md` `AgentGrant.Bindings`, `AgentGrant.Version`).
- [ ] T046 Implement `internal/adapters/storage/postgres/agent_grant_repository.go` with sqlx, explicit columns, the read and write timeouts, one transaction for the `user_grants` row, the `agent_grants` row and the binding set, the permission-set existence lock the human `Create` takes, `version = $n` preconditions, error mapping to `StorageError` and the sentinels, and `startStorageSpan` with the four names of `contracts/observability.md` §3. Make T036 on PostgreSQL and T038 green (DB-001–DB-003; ADR 004; OPENTELEMETRY.md).
- [ ] T047 Add a description for each of the four spans to `docs/reference/telemetry-descriptions.yaml`, run `just telemetry-doc` and commit `docs/reference/telemetry.md` (OPENTELEMETRY.md; AGENTS.md Development Workflow).
- [ ] T048 Run `just check`, `just test`, `just test-integration` and the existing E2E suite `ginkgo -v --label-filter="!performance && !docker" ./tests/e2e/` excluding `--focus "Agent grants"`; every pre-existing test is green unchanged. `just test-integration-infra` runs locally only where a container runtime exists, otherwise in CI (SC-004, US5.5; Principles VIII, IX).

**Checkpoint**: Schema, principal kind and both repositories done; user stories can start.

---

## Phase 3: User Story 1 - Grant an agent permission sets for its own use (Priority: P1) 🎯 MVP

**Goal**: An administrator writes, reads, replaces and deletes an agent's one agent grant; the
broker adds the mandatory sets, computes coverage, stores bindings and answers with a strong ETag.

**Independent Test**: Create a service account (#471), write a grant that binds it, read it back,
replace it, delete it (spec User Story 1).

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T049 [P] [US1] Write `internal/domain/consent/agent_grant_service_test.go` for `Get`, `Put` and `Delete`: agent missing → agent not found; rules 1–6 of `data-model.md` §Resolution and invariants in order with their messages and first-failure determinism; mandatory sets added and a listed mandatory set ignored; coverage limited by service requirements; copy on write (a later agent change leaves the stored grant); the shared consent structure check and `UserGrant.ValidateForCreate` called; outcomes `created`, `replaced` (keeping `id`, `created_at`), `unchanged` (no `Save`, same version); every precondition row of `data-model.md` §Write outcomes including absent grant with `If-Match`; up to three re-reads on a concurrent commit; `412` on mismatch at save with `If-Match`; `ErrAgentGrantReferenceMissing` as a `400`-class error; a session repository double that fails if called; audit records per `contracts/observability.md` §§1–2 and §6, with a test principal policy that changes the value (FR-001–FR-006, FR-019, SR-002; `data-model.md` `AgentGrant.Grant`, `AgentGrant.Version`).
- [ ] T050 [P] [US1] Write `internal/adapters/http/handlers/admin/agent_grants_handler_test.go`: `401` before any read; `{agent-id}` by UUID and canonical ID and `404 agent not found`; `If-Match` weak or malformed → `400`; 64 KiB body limit, unknown members, `granted_permission_sets` in a request and `null` members → `400`; `201` without `Location`, `200` on replace and no-op, `ETag` on both; `GET`/`DELETE` without a grant → `404 agent grant not found`; `204` on delete; `412`; representation keys ordered by ascending UUID; error mapping (SR-001, API-002, API-003; research R11; API-AG, API-AG-SHAPE).

### Implementation for User Story 1

- [ ] T051 [US1] Implement `Get`, `Put` (rules 1–6, binding persistence, no-op detection, preconditions, conditional `Save` with re-read, audit) and `Delete` (precondition, conditional `Delete`, audit, `write_failed` record) in `internal/domain/consent/agent_grant_service.go`, using the package-level consent structure functions of T003. Make T049 green (FR-001–FR-006, FR-019, SR-002, SR-004).
- [ ] T052 [US1] Implement `Get`, `Put` and `Delete` in `internal/adapters/http/handlers/admin/agent_grants_handler.go` with `operatorPrincipalFromContext`, `agents.Service.ResolveID`, `parseStrongETag` and `strongETag`, and the check order of `plan.md` §Design Notes; the handler only parses input and maps errors. Make T050 green (API-001–API-003; Principle VI).
- [ ] T053 [US1] Run the User Story 1 E2E tests (memory) and confirm Scenarios 1–14 pass; Scenarios 12 and 14 also on PostgreSQL where containers run (US1.1–US1.14; SC-001, SC-006).

**Checkpoint**: User Story 1 works on its own.

---

## Phase 4: User Story 2 - Choose the service account explicitly for every covered service (Priority: P1)

**Goal**: Bindings are explicit, refer to an account of the same service by ID, are refused for
uncovered services, and are required for effectively mandatory services.

**Independent Test**: Write grants with a missing binding, an uncovered binding, a foreign and a
non-existent account; check which are stored (spec User Story 2).

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T054 [P] [US2] Add tests to `internal/domain/consent/agent_grant_service_test.go` for rules 7–9 of `data-model.md` §Resolution and invariants with their messages, iterating services in ascending UUID order: uncovered binding, account of another service and missing account answered alike, an effectively mandatory service (by service requirement, or by a granted set's `ServiceScope` within the requirements) left unbound, a non-mandatory service left unbound and stored without a binding; an account of status `failed` accepted; only the ceiling read, no credential accessor called on the account double; no audit on refusal (FR-004b, FR-007–FR-010, FR-014, SR-004; `data-model.md` `ServiceAccountBinding.ServiceID`, `ServiceAccountBinding.ServiceAccountID`).

### Implementation for User Story 2

- [ ] T055 [US2] Implement rules 7–9 in `Put` in `internal/domain/consent/agent_grant_service.go` through `ServiceAccountReader.Get(ctx, serviceID, accountID)`, with the definition of an effectively mandatory service that user consent uses. Make T054 green (FR-004b, FR-007–FR-010, FR-014).
- [ ] T056 [US2] Run the User Story 2 E2E tests and confirm Scenarios 1–7 pass (US2.1–US2.7; SC-002).

**Checkpoint**: User Stories 1 and 2 work.

---

## Phase 5: User Story 3 - Refuse a grant that exceeds the bound account's ceiling (Priority: P1)

**Goal**: For every bound service, the effective scopes from `storage.EffectiveScopes` must be a
literal subset of the account's ceiling at write time.

**Independent Test**: Bind sets whose scopes are inside, equal to and outside the ceiling; only the
last is refused, naming the excess (spec User Story 3).

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T057 [P] [US3] Add tests to `internal/domain/consent/agent_grant_service_test.go` for rule 10 of `data-model.md` §Resolution and invariants: excess named sorted and space-separated with service and account; byte-for-byte comparison (`write` does not contain `read`, `Read` is not `read`); the union over granted sets is checked; service-requirement intersection applied before the check; unbound services and services with no effective scopes not checked; the computation is `storage.EffectiveScopes` (FR-011, FR-012).
- [ ] T058 [P] [US3] Add tests to `internal/domain/thirdparty/service_account_service_test.go` and `internal/domain/permissionset/service_test.go` asserting that lowering a bound account's ceiling and widening a granted permission set are accepted and call no agent-grant repository method (FR-013; US3.5, US3.6).

### Implementation for User Story 3

- [ ] T059 [US3] Implement rule 10 in `Put` in `internal/domain/consent/agent_grant_service.go` with `storage.EffectiveScopes` from T002. Make T057 and T058 green (FR-011–FR-013).
- [ ] T060 [US3] Run the User Story 3 E2E tests and confirm Scenarios 1–6 pass (US3.1–US3.6; SC-002).

**Checkpoint**: All P1 stories work; MVP complete.

---

## Phase 6: User Story 4 - Revoke an agent grant and protect bound accounts (Priority: P2)

**Goal**: Deleting a bound account is refused with `409` and `bound_by_agents`, expired grants
included; deleting the agent removes its grant on both stores.

**Independent Test**: Delete a bound account (refused), delete the grant, delete the account
(accepted) (spec User Story 4).

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [ ] T061 [P] [US4] Add tests to `internal/domain/thirdparty/service_account_service_test.go`: a non-empty `BindingAgents` result refuses the delete with `*ServiceAccountBoundError` carrying sorted agents and satisfying `errors.Is(err, ErrServiceAccountBound)`; a store `conflict` wrapping `ports.ErrServiceAccountReferenced` re-reads the agents; an empty result deletes as before (FR-015; research R9; API-SA-409).
- [ ] T062 [P] [US4] Add tests to `internal/adapters/http/handlers/admin/service_accounts_handler_test.go`: the `409` keeps `error` and `message` of #471 and adds `bound_by_agents`, present only on this conflict (API-003, FR-015).
- [ ] T063 [P] [US4] Add memory tests: in `internal/adapters/storage/memory/agent_repository_test.go`, agent deletion removes every grant to the agent and the agent grant's bindings; in `internal/adapters/storage/memory/service_account_repository_test.go`, a bound account delete returns `conflict` wrapping `ErrServiceAccountReferenced`, and a concurrent `Save` and account `Delete` under `-race` never leave a binding to a missing account and never deadlock (US4.5, FR-015; research R9, R13; DEC-MEMORY-CASCADE).
- [ ] T064 [P] [US4] Add a test to `internal/adapters/storage/postgres/service_account_repository_unit_test.go` mapping `23503` from `fk_agent_grant_bindings_account` to `conflict` wrapping `ErrServiceAccountReferenced` (FR-015, DB-002).

### Implementation for User Story 4

- [ ] T065 [US4] Replace `IsBound` with `BindingAgents(ctx, accountID) ([]id.AgentID, error)` on `ServiceAccountBindingChecker`, add `ServiceAccountBoundError`, delete `NoServiceAccountBindings`, and refuse or re-read in `Delete` in `internal/domain/thirdparty/service_account_service.go`; wire a small `agentGrantBindings{b.storage.AgentGrants()}` adapter in `internal/app/builder.go`. Make T061 green (FR-015; research R9).
- [ ] T066 [US4] Map `23503` to `ErrServiceAccountReferenced` in `internal/adapters/storage/postgres/service_account_repository.go`; add `WithBindingGuard` to `internal/adapters/storage/memory/service_account_repository.go` and run the delete inside the agent-grant store's guard; add the grant cascade to `Delete` in `internal/adapters/storage/memory/agent_repository.go`; wire both in `internal/adapters/storage/factory.go`. Make T063 and T064 green (FR-015, DB-001; research R9, R13).
- [ ] T067 [US4] Add `bound_by_agents` from `ServiceAccountBoundError` to the `409` in `internal/adapters/http/handlers/admin/service_accounts_handler.go`. Make T062 green and run the User Story 4 E2E tests; Scenarios 1–2 on memory, 3–6 on both stores where containers run (US4.1–US4.6; SC-005).

**Checkpoint**: User Stories 1–4 work.

---

## Phase 7: User Story 5 - Agent grants stay out of every user's view and every human decision (Priority: P2)

**Goal**: No end-user listing and no human decision reads an agent grant, including for a human
principal whose text equals the agent's UUID; exchange and delegation-check answers are unchanged.

**Independent Test**: Create an agent grant for agent A; list delegations and grants as users,
including one whose principal equals A's UUID; exchange A's self token; the grant appears nowhere
(spec User Story 5).

The store-level human scope is built in Phase 2.5 (T035, T043, T044). This phase proves it through
the domain paths that read grants and sessions directly.

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

- [ ] T068 [P] [US5] Add a test to `internal/domain/tokenexchange/grant_resolver_test.go` on the memory store: with an agent grant for agent A present, the resolver's decision for a human principal equal to A's UUID is the same "no grant" denial as without it, and an agent-subject token is still rejected under ADR 045 decision 6 (FR-018, SR-005; US5.3, US5.4).
- [ ] T069 [P] [US5] Add tests on the memory store to `internal/domain/consent/service_test.go` and `internal/domain/oauth2session/service_test.go`: delegation listings, grants for an agent, consent revocation and the agents affected by ending a session ignore agent grants, also for a human principal equal to the agent's UUID; revoking the human consent leaves the agent grant and deleting the agent grant leaves the human consent (FR-016, FR-017, SR-003; US5.1, US5.2, US5.6).

### Implementation for User Story 5

- [ ] T070 [US5] List in `## Design notes` of `specs/20261127-144837-agent-grants/tasks.md` every caller of `UserGrantRepository` and `UserSessionRepository` in `internal/domain/` (including `oauth2.AuthorizationService.HandleAuthorization` and `oauth2session.Service`) and confirm each reaches only human-scoped methods or a named kind-agnostic guard; correct a caller only if it bypasses the ports. Make T068 and T069 green (FR-017, FR-018; research R2).
- [ ] T071 [US5] Run the User Story 5 E2E tests; Scenarios 1, 3 and 4 on memory, 2 and 6 on both stores and 5 on PostgreSQL where containers run (US5.1–US5.6; SC-003, SC-004).

**Checkpoint**: All user stories work independently.

---

## Phase 8: Polish & Documentation

- [ ] T072 [P] Add a "Grant an agent access for itself" section to `docs/guides/manage-agents-and-services.md`: what an agent grant is, mandatory sets added by the broker, explicit bindings, the ceiling check, expiry, the bound-account `409`, and that rolling back migration `051` erases agent grants; end-user documentation only (plan §Constitution Check "End-User Docs"; AGENTS.md Documentation Scope).
- [ ] T073 [P] Add the three `self-grant` operations and the changed service-account `409` to `docs/reference/api.md` (API-001, API-003).
- [ ] T074 [P] Update `internal/AGENTS.md` (handler list, key storage types `AgentGrant`, `PrincipalKind`) and confirm `internal/ports/AGENTS.md` from T026 (Principle II).
- [ ] T075 Run `just telemetry-doc` after the final Go change and commit `docs/reference/telemetry.md` if it changed (AGENTS.md Development Workflow).

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY]

### Design Phase Verification [MANDATORY]

- [ ] T076 Verify every task traces to a requirement, scenario, success criterion or binding constraint, and that the deferred finding is not a task (Feature Scope).
- [ ] T077 Verify Agent Grant, Service-Account Binding and Principal Kind are in `ARCHITECTURE.md` §12 and `AGENTS.md`, and the ADR is Accepted and indexed in both (Principles II, V).
- [ ] T078 Verify the no-configuration result of T010 holds: no new key, no example, no Helm change (Principle VII).
- [ ] T079 Verify `api/admin/openapi.yaml` is `3.2.0` with the merged delta and `api/enduser/openapi.yaml` is unchanged; reference the contract in the PR description for the maintainer's confirmation (API-004, API-006; Principles IV, X).
- [ ] T080 Verify migration `051` matches `data-model.md` §PostgreSQL schema and is atomic (Principle IX).
- [ ] T081 Verify `spec.md` Journeys 1–3 are linked to their scenarios and match the implemented HTTP exchanges; no configuration, Wireloom or JWT example applies (Principle XIV).
- [ ] T082 Verify the 39 E2E scenarios exist with one `It()` each, were written before implementation with concrete expectations, and have no `XIt`, `PIt`, `Skip()` or red-phase comments (Principle XIII).
- [ ] T083 Verify the log events and spans match `contracts/observability.md` and no metric was added (Principle XV).

### Implementation Phase Verification [MANDATORY]

- [ ] T084 Verify unrelated remediation is excluded and only user-approved issue URLs appear under `Future Work` (Feature Scope).
- [ ] T085 [P] Verify the handler matches the merged contract exactly: status codes, messages, `ETag`, `If-Match`, key order, `bound_by_agents` (API-001–API-005; Principles IV, X).
- [ ] T086 [P] Verify `ARCHITECTURE.md` reflects the consent-domain service, data stores, human-scoped ports and Admin API `3.2.0` (Principle II).
- [ ] T087 [P] Verify migration integration tests cover apply, rollback, reapply, backfill and each constraint, and both repositories pass the shared contracts on PostgreSQL in CI (Principle IX; SC-007).
- [ ] T088 Verify security posture: operator required before any read (SR-001), fail closed on missing references (SR-002), human scope enforced in the stores (SR-003), no credential read or decrypted (SR-004), ADR 045 decision 6 rejection unchanged (SR-005), no custom cryptography (Principles I, III).
- [ ] T089 Verify the coding standards: `operator_principal` as `id.Principal`, no plain-string principal key, `TestLoggedPrincipalsAreTyped` and `telemetrylint` pass, no lint rule suppressed, explicit SQL columns (Principle XV; AGENTS.md Code Style).
- [ ] T090 Verify domain logic sits in `internal/domain/consent` and `internal/domain/storage`, handlers only parse and map, wiring is in `internal/app/builder.go`, and no new `domain/X/` package exists (Principles VI, XII).
- [ ] T091 Verify unit tests were written first and changed minimally during implementation, and no Bash script validates correctness (Principle VIII).
- [ ] T092 Run `ginkgo -v --label-filter="!performance && !docker" ./tests/e2e/` and, where containers run, `ginkgo -v --procs=1 --label-filter="docker" --focus "Agent grants" ./tests/e2e/`; all pass (Principle XIII; SC-004).
- [ ] T093 Run the full gate `just verify` from `CONTRIBUTING.md` before the pull request; container-backed steps may first run in CI (Principles VIII, XIII).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 0** (T001–T004): starts immediately; must be green before Phase 2.
- **Phase 1** (T005): parallel with Phase 0.
- **Phase 2** (T006–T023): after Phase 1. 2a–2d and 2g can run in parallel; 2f's story suites
  (T017–T021) follow T015 and T016. All of Phase 2 precedes Phase 2.7.
- **Phase 2.7** (T024–T032): after Phase 2. T024 → T025 → T026 → T027/T028 [P] → T029 → T030 →
  T031 → T032.
- **Phase 2.5** (T033–T048): after Phase 2.7. Tests T033–T039 first; T040 → T041 → T042;
  T043/T044 after T042's tests; T045 after T043; T046 after T042; T047 after T046; T048 last.
- **User stories**: US1 (Phase 3) after Phase 2.5. US2 and US3 extend `Put` from US1, so they
  follow US1 in order. US4 needs only Phase 2.5 and US1's write path to create a binding. US5
  needs only Phase 2.5 and US1's write path.
- **Phase 8** after the desired stories; **Phase N** last.

### User Story Dependencies

- **US1 (P1)**: Phase 2.5 only.
- **US2 (P1)**: US1 (`Put` exists).
- **US3 (P1)**: US1 and Phase 0 T002 (`storage.EffectiveScopes`); independent of US2.
- **US4 (P2)**: US1 (a stored binding to protect).
- **US5 (P2)**: US1 (a stored agent grant to hide); independent of US2–US4.

### Within Each User Story

- Tests before implementation; domain service before handler; story E2E run last.

---

## Parallel Examples

### Phase 2.5 tests

```text
Task: "T033 agent_grant_test.go in internal/domain/storage/"
Task: "T034 agent_grants_migration_test.go in tests/integration/migrations/"
Task: "T035 principal_kind_scope.go in tests/integration/storage/contract/"
Task: "T036 agent_grant_repository.go in tests/integration/storage/contract/"
Task: "T039 memory user_grants_test.go and user_session_test.go"
```

### User Story 1

```text
Task: "T049 agent_grant_service_test.go in internal/domain/consent/"
Task: "T050 agent_grants_handler_test.go in internal/adapters/http/handlers/admin/"
```

### User Story 4

```text
Task: "T061 service_account_service_test.go in internal/domain/thirdparty/"
Task: "T062 service_accounts_handler_test.go in internal/adapters/http/handlers/admin/"
Task: "T063 memory agent_repository_test.go and service_account_repository_test.go"
Task: "T064 postgres service_account_repository_unit_test.go"
```

### After US1: US4 and US5 in parallel with US2 → US3

```text
Agent A: Phase 4 (US2) then Phase 5 (US3) in internal/domain/consent/agent_grant_service.go
Agent B: Phase 6 (US4) in internal/domain/thirdparty/ and storage adapters
Agent C: Phase 7 (US5) tests in internal/domain/tokenexchange/, consent/service_test.go, oauth2session/
```

---

## Implementation Strategy

### MVP First (User Stories 1–3, all P1)

1. Phase 0 refactors in their own commit, all tests green.
2. Phase 1 and Phase 2: ADR, contract `3.2.0`, glossary, 39 failing E2E tests.
3. Phase 2.7 scaffold in its own commit; E2E fails on `501`.
4. Phase 2.5: migration `051`, human scope, repositories; every existing test green.
5. US1 → validate independently; US2 → US3. The P1 set is the MVP: grants are written, bound and
   ceiling-checked.

### Incremental Delivery

1. US4 makes #471's `409` real and closes the agent-deletion cascade on memory.
2. US5 proves invisibility and unchanged decisions through the domain paths.
3. Phase 8 documentation, then Phase N gates and `just verify`.

---

## Notes

- `[P]` tasks touch different files and depend on no unfinished task in their batch.
- Commit Phase 0 and Phase 2.7 separately from feature logic.
- No E2E test changes during implementation beyond fixture adjustments.

## Design notes

Filled by T010, T014, T023 and T070.
