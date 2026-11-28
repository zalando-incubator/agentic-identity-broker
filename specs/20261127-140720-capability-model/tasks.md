---

description: "Task list for the capability model for permission sets"
---

# Tasks: Capability Model for Permission Sets

**Input**: Design documents from `specs/20261127-140720-capability-model/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md)

**Tests**: Per Constitution Principle VIII, automated tests are mandatory and written first. Each user story phase starts with its tests. E2E tests for every scenario are written in Phase 2f.

**Organization**: Tasks are grouped by user story. All three stories are P1. US1 delivers the model and Admin API, US2 the migration and unchanged consent, and US3 the policy input.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1, US2, US3)
- Requirement, scenario and research references in parentheses give each task's traceability

## 🔒 Feature Scope [MANDATORY]

- Every task cites an approved requirement or acceptance scenario of `spec.md`, or a binding constraint applicable to the affected path. Tasks include only the work that makes that path functional, secure and compliant.
- If a finding blocks safe or compliant delivery, only the minimum necessary correction is included. For unrelated findings, ask the user before filing an issue, and record only user-approved issue URLs in the non-checkbox Future Work note below.
- Expanding scope requires explicit user approval and updates to `spec.md` and `tasks.md` before implementation.

**Future Work** (not tasks): none approved yet. research.md lists one deferred finding, the empty-body grant revoke documented in the end-user OpenAPI but not implemented. It awaits the user's decision on filing an issue.

---

## Phase 0: Pre-implementation Refactoring [OPTIONAL]

Skipped (plan.md, Implementation Phase Overview). The feature replaces fields in place and adds new code, and needs no behavior-neutral restructuring first.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Typed IDs that every later phase compiles against.

- [ ] T001 Add `{"CapabilityGroupID", "capability_group"}` and `{"CapabilityID", "capability"}` to `uuidTypes` in `internal/domain/id/gen_ids.go`. Run `go generate ./internal/domain/id/`, commit `internal/domain/id/uuid_ids_gen.go`, and add the two types to the catalogue in `internal/domain/id/AGENTS.md` (ADR `013-strongly-typed-entity-ids`; data-model.md § Typed IDs)
- [ ] T002 [P] Add parse, round-trip and zero-value tests for `CapabilityGroupID` and `CapabilityID` in `internal/domain/id/id_test.go` (ADR `013-strongly-typed-entity-ids`)

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey and E2E design before implementation.

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete. Implementation also needs the stakeholder confirmations in T013 and T015.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II, V

- [ ] T003 Confirm that the entities, aggregate, value objects and derived views in `specs/20261127-140720-capability-model/data-model.md` match spec.md § Domain Model and FR-001–FR-012. Record any correction in data-model.md before code starts (Principle V)
- [ ] T004 [P] Add the glossary terms **Capability**, **Capability Group**, **Capability Entry** and **Effective Capability** to `ARCHITECTURE.md`. Revise **PermissionSet**, **ServiceScope**, **AgentPermissionSetEntry** and **Grant Context**. Replace the stale **granted_permission_set_ids** entry with **GrantedPermissionSetEntry** (data-model.md § Glossary additions; spec.md: "New glossary terms")
- [ ] T005 [P] Add the root `AGENTS.md` Domain Glossary rows for Capability, Capability Group and Effective Capability. Add the ADR Decision Index row for `adrs/20261127-190555-capability-model.md` (Proposed) (Principle V; AGENTS.md ADR index)
- [ ] T006 Document the aggregate invariants and the change table (data-model.md § Relationships and lifecycle) in `ARCHITECTURE.md`. Cover the storage tables (§ Storage, "Key tables"), the Grant Context member list and the ExtProc enabled-mode input (§ 1029, § 1633) (FR-008, FR-014, research R3, R8; Principle II)

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [ ] T007 Confirm and record in plan.md that the feature adds no configuration parameter. It depends only on the existing `grant_context.enabled` and `authorization.enabled` (research R17)
- [ ] T008 [P] Update the header comment in `examples/config/extproc-grant-context.yaml` so it lists `granted_capabilities` among the enabled-mode `input.context` members (FR-014, FR-016)
- [ ] T009 [P] Verify that `examples/config/README.md` needs no new entry, because no new file is added under `examples/config/` (Principle VII)
- [ ] T010 [P] Verify the deployment boundary. `charts/agentic-identity-broker/` deploys no changed parameter, and ExtProc stays standalone under ADR `011-extproc-standalone-binary`. No chart change (Principle VII; research R17)

**Checkpoint**: Configuration confirmed unchanged; the deployment boundary is documented

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV, X

- [ ] T011 Design the end-user change in `api/enduser/openapi.yaml`: add the required `granted_capabilities` to `GrantContext` per `contracts/policy-input.md` § 1, add an example, and bump `info.version` to `1.4.0`. `TokenExchangeResponse` stays unchanged (FR-016, API-002, API-003; research R12)
- [ ] T012 Design the Admin change in `api/admin/openapi.yaml` per `contracts/admin-api.md`. Add the `/api/capability-groups` paths and schemas. Replace permission-set `service_scopes` with `capabilities[]` and forbid `service_scopes`. Add the optional `capabilities[]` to `AgentPermissionSetEntry` and remove `ServiceScope`. Bump `info.version` to `4.0.0` with a change summary in `info.description` (API-001, US1.1–US1.5; research R11)
- [ ] T013 [P] Obtain user/stakeholder confirmation of the end-user API 1.4.0 design (decision D2), including the broker-before-ExtProc upgrade order. Record the reference in the PR description (Principle X)
- [ ] T014 [P] Run `just check` and fix any OpenAPI lint or validation finding in both files (Principle IV)
- [ ] T015 [P] Obtain user/stakeholder confirmation of the Admin API 4.0.0 design (decision D1) and decisions D3–D5. Record the reference in the PR description (Principles IV, X)

**Checkpoint**: APIs designed and confirmed by the user or stakeholder

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [ ] T016 Write `tests/integration/migrations/capability_model_migration_test.go` (build tag `integration`) with the five tests in `contracts/migration-050.md` § Integration tests. Use `framework.go` (`Up`, `Down`, `QuerySQL`, `ExecuteSQL`, `Force`, `Version`) and seed the Journey 2 data. Assert that every generated canonical ID passes `storage.ValidateCanonicalID` (data-model.md `Capability.CanonicalID`) (US2.1–US2.3, FR-018–FR-020, SC-001)
- [ ] T017 Write `migrations/050_capability_model.up.sql` per `contracts/migration-050.md` § Up and § Generated capability canonical IDs. It creates the four tables of data-model.md § PostgreSQL schema, which implement `CapabilityGroup.CanonicalID`, `CapabilityGroup.Name`, `Capability.CanonicalID`, `Capability.Name`, `Capability.ReadOnly`, `CapabilityServiceScope.ServiceID` and `CapabilityEntry.RequirementType` from data-model.md. It then converts sets and grants, guards unmapped grant services and drops `permission_set_service_scopes`. Use explicit columns and keep it atomic (FR-018–FR-020, DB-001, DB-002, DB-004; research R9, R10)
- [ ] T018 Write `migrations/050_capability_model.down.sql` per `contracts/migration-050.md` § Down, raising an exception for every guard before any change (US2.3, DB-002; research R9, decision D6)
- [ ] T019 [P] Verify that data-model.md § PostgreSQL schema and § JSONB shapes match the SQL in T017 and T018. List the changed queries (data-model.md § Queries that change) for Phase 2.5 (DB-001)

**Checkpoint**: Schema designed, migration and its tests written

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

Not applicable: the React UI is unchanged (FR-013, plan.md Constitution Check).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII, XIV

- [ ] T020 [P] Add `tests/e2e/fixtures/capabilities.go` with the Journey 1 Calendar group (`google-calendar-view` read-only and `google-calendar-edit`) and a Drive group (`google-drive-view`, `google-drive-share`). Add two sets that both include `google-drive-view` (US3.3) and Admin request builders for the 4.0.0 bodies (US1.1, US3.1–US3.3)
- [ ] T021 [P] Write `tests/e2e/capability_groups_test.go` (dual servers per `tests/e2e/AGENTS.md`): one `It()` each for US1.1, US1.2 (a `DescribeTable` inside the single `It` for the five invalid inputs, with a list asserting nothing stored) and US1.5 (capability delete, group delete, `PUT` omission, and set update invalidating an agent entry, all `409`). Each `It` gets a `// Spec: US1.x` comment
- [ ] T022 [P] Rewrite `tests/e2e/permission_sets_test.go` for capability entries. Add the US1.3 `It()`: the set is stored, and an empty list, a duplicate entry and a `service_scopes` body each return `400`
- [ ] T023 [P] Add the US1.4 `It()` to `tests/e2e/agent_permission_requirements_test.go`: a relaxing entry and an entry with a foreign capability return `400`; a restated mandatory, a tightened optional and a subset are accepted
- [ ] T024 [P] Write `tests/e2e/capability_consent_test.go` with the US2.5 `It()`: the agent detail lists Google as `mandatory` for a set with a mandatory and an optional capability on Google, and approving today's request grants both. Then write `tests/e2e/capability_model_upgrade_test.go` under `Label("docker")`. Seed Journey 2 at version `49` via `bootstrap.NewPostgresFixtureAtVersion`, migrate to `50` and start the dual servers. Add one `It()` each for US2.1, US2.2 (exchange scopes `repo:read`, unchanged `granted_permission_sets`) and US2.4 (today's consent body accepted, response unchanged, pre-upgrade scopes)
- [ ] T025 [P] Write `tests/e2e/capability_policy_journey_test.go` under `Label("grant-context")`. Use the real broker with `bootstrap.NewExtProcEnvironmentWithOptions` and `GrantContext.Enabled`, the policy `examples/policies/capability-tool-map.rego` and the mock MCP target. Add one `It()` each for US3.1–US3.5, and one for US3.6 with grant context disabled
- [ ] T026 [P] Add `granted_capabilities` to the forbidden `delegation_members` list in `tests/e2e/extproc/fixtures/policies/grant_context_disabled_input.rego` (US3.6, FR-016)
- [ ] T027 [P] Make `tests/e2e/extproc/bootstrap/authzen_broker.go` (`MockAuthZENBroker`) return `granted_capabilities` in permits, so existing ExtProc E2E permits stay valid once the member is required (FR-016; research R8)
- [ ] T028 Move the Admin seeding in `tests/e2e/fixtures/grants.go`, `tests/e2e/canonical_resource_ids_test.go`, `tests/e2e/grant_diagnostics_test.go` and `tests/e2e/frontend/consent_flow_test.go` from `service_scopes` to capability entries, without changing their assertions (API-001)
- [ ] T029 Review spec.md Journeys 1 and 2 against US1.1, US1.3, US1.4, US2.1, US2.2 and US3.1–US3.2. Check that the HTTP exchanges match `contracts/admin-api.md` and the policy-input JSON matches `contracts/policy-input.md` § 1. Record the US2.3 integration-test mapping from plan.md's Complexity Tracking (Principle XIV)
- [ ] T030 Run the suites that run locally (`ginkgo -v --label-filter='!docker' --focus='Capability|Permission Sets' ./tests/e2e/`). Confirm that every new `It()` fails on real assertions, with no `Skip`, `XIt` or `PIt` and no red-phase comments. Docker-labelled tests may first run in CI (Principle XIII)

**Checkpoint**: E2E tests written before implementation and failing semantically where they run locally

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

**Constitution Reference**: Principle XV

- [ ] T031 Read `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`. Confirm that `contracts/observability.md` lists every added or changed log event and the `capability_id` diagnostic attribute, and that no span, span attribute or metric is added (SR-004)
- [ ] T032 [P] Confirm that no new metric instrument or span attribute namespace needs a registry entry in `OPENTELEMETRY.md` § 4.1 or § 5.4 (`contracts/observability.md` § Spans and metrics)
- [ ] T033 [P] List the credential and principal safety tests required by `contracts/observability.md` § Required tests: grant events through `consent.NewService` and capability events through `capability.NewService` (LOGGING.md § 9)

**Checkpoint**: Observability design is listed against the binding coding standards

---

## Phase 2.7: Entity Boilerplate (New Entities)

**Purpose**: Empty but compiling scaffolding for Capability Group and Capability (plan.md Implementation Phase Overview). Keep it as its own commit batch.

### Boilerplate: CapabilityGroup and Capability

- [ ] T034 Define `CapabilityGroup`, `Capability`, `CapabilityServiceScope` and `CapabilityEntry` in `internal/domain/storage/capability.go`, with fields per data-model.md (`CapabilityGroup.ID`, `CapabilityGroup.CanonicalID`, `CapabilityGroup.Name`, `CapabilityGroup.Description`, `CapabilityGroup.Capabilities`, `Capability.ID`, `Capability.GroupID`, `Capability.CanonicalID`, `Capability.Name`, `Capability.Description`, `Capability.ReadOnly`, `Capability.ServiceScopes`, `CapabilityServiceScope.ServiceID`, `CapabilityServiceScope.Scopes`, `CapabilityEntry.CapabilityID`, `CapabilityEntry.RequirementType`). Add `Copy()`, and `Validate` stubs that return nil (FR-001, FR-002)
- [ ] T035 Define `CapabilityGroupRepository`, `CapabilityRepository`, `CapabilityCanonicalIDRepository` and `AgentPermissionSetReferenceRepository` in `internal/ports/storage.go` with the methods of data-model.md § Ports (Principle IX)
- [ ] T036 [P] Add empty memory implementations in `internal/adapters/storage/memory/capability_repository.go` (zero values and not-found) (Principle IX)
- [ ] T037 [P] Add empty PostgreSQL implementations in `internal/adapters/storage/postgres/capability_repository.go` that return a not-implemented `StorageError` (Principle IX)
- [ ] T038 Add the repository accessors to both adapters and `internal/adapters/storage/factory.go` (Principle IX)
- [ ] T039 [P] Create `internal/adapters/http/handlers/admin/capability_groups_handler.go` with `List`, `Create`, `Get`, `Update` and `Delete` returning `501` (API-001)
- [ ] T040 Register the `/api/capability-groups` and `/api/capability-groups/{capability-group-id}` routes in `internal/adapters/http/routing/admin.go`. Add the handler to `app.AdminHandlers` and wire it in `internal/app/builder.go` (Principle XII)
- [ ] T041 Run `just build` and `just check` to confirm that the scaffolding compiles (plan.md Phase 2.7)

**Checkpoint**: Scaffolding compiles and handlers return 501, with no business logic yet

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Changed shapes of existing entities and their storage. Every user story depends on these.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T042 [P] Write table tests in `internal/domain/storage/capability_test.go` for the group and capability validation rules of data-model.md (`CapabilityGroup.CanonicalID`, `CapabilityGroup.Name`, `CapabilityGroup.Description`, `CapabilityGroup.Capabilities`, `Capability.CanonicalID`, `Capability.Name`, `Capability.Description`, `Capability.ServiceScopes`, `CapabilityServiceScope.ServiceID`, `CapabilityServiceScope.Scopes`, `CapabilityEntry.CapabilityID`, `CapabilityEntry.RequirementType`) (FR-001, FR-002, US1.2)
- [ ] T043 [P] Update table tests in `internal/domain/storage/permission_set_test.go` for data-model.md `PermissionSet.Capabilities` (FR-003, US1.3)
- [ ] T044 [P] Update table tests in `internal/domain/storage/agent_test.go` for data-model.md `AgentPermissionSetEntry.Capabilities`. Absent means nil, and `[]` and duplicates are invalid (FR-005, FR-007)
- [ ] T045 [P] Update table tests in `internal/domain/storage/user_grant_test.go` for data-model.md `GrantedPermissionSetEntry.GrantedCapabilityIDs` (FR-009)
- [ ] T046 Implement the `Validate` and `ValidateForCreate` rules in `internal/domain/storage/capability.go` per T042 (cites data-model.md `CapabilityGroup.*`, `Capability.*`, `CapabilityServiceScope.*` and `CapabilityEntry.*` as listed in T042). Reuse `internal/domain/canonical/id.go` for `Capability.CanonicalID`, which is required, unlike `storage.ValidateCanonicalID`'s nil-allowed wrapper (FR-002)
- [ ] T047 Replace `ServiceScopes []ServiceScope` with `Capabilities []CapabilityEntry` in `internal/domain/storage/permission_set.go`, including `validateCommon` and `Copy`. Remove `ServiceScope` (data-model.md `PermissionSet.Capabilities`; FR-003)
- [ ] T048 Add `Capabilities []CapabilityEntry` (`json:"capabilities,omitempty"`) to `AgentPermissionSetEntry` in `internal/domain/storage/agent.go`, with structural checks in `ValidatePermissionSets` and a deep `Copy` (data-model.md `AgentPermissionSetEntry.Capabilities`; FR-005, FR-007)
- [ ] T049 Replace `IncludedServiceIDs` with `GrantedCapabilityIDs []id.CapabilityID` (`json:"granted_capability_ids"`) in `internal/domain/storage/user_grant.go`, updating `Validate`, `ValidateForCreate` and `Copy` (data-model.md `GrantedPermissionSetEntry.GrantedCapabilityIDs`; FR-009)
- [ ] T050 [P] Write PostgreSQL integration tests in `internal/adapters/storage/postgres/capability_repository_test.go` (build tag `integration`) covering:
  - aggregate create, get, list and update with ID diff and position order;
  - unique-violation mapping to `ErrorKindValidation` for `Capability.CanonicalID` and `CapabilityGroup.CanonicalID` (data-model.md);
  - serializable delete returning `ErrorKindConflict` for set and active-grant references;
  - `GetByIDs` and canonical-ID resolution.

  (FR-008, DB-003)
- [ ] T051 [P] Update `internal/adapters/storage/postgres/permission_set_repository_test.go`, `user_grants_test.go` and `agent_repository_test.go` for capability entries, grant capability IDs, the service queries joined through `capability_service_scopes`, and `ListByPermissionSet` (data-model.md § Queries that change; DB-003)
- [ ] T052 [P] Write memory adapter tests in `internal/adapters/storage/memory/capability_repository_test.go`, mirroring T050 including the emulated `RESTRICT` and grant-reference checks (DB-003)
- [ ] T053 Implement `internal/adapters/storage/postgres/capability_repository.go` with sqlx and explicit columns, aggregate writes in one transaction, and serializable delete re-checks using `granted_permission_sets @> ...` (research R3; DB-004)
- [ ] T054 Update `internal/adapters/storage/postgres/permission_set_repository.go` to persist and load `permission_set_capabilities` in place of `insertServiceScopes` and `loadServiceScopesBatch`. Update `List(serviceID)`, `CountPermissionSetsForService` and the delete re-check (data-model.md § Queries that change; FR-003)
- [ ] T055 Update `internal/adapters/storage/postgres/user_grants.go` so `CountAgentsByPrincipalAndServiceID` and `ListByPrincipalAndServiceID` match services through `granted_capability_ids` joined to `capability_service_scopes`. Implement `AgentPermissionSetReferenceRepository.ListByPermissionSet` in `internal/adapters/storage/postgres/agent_repository.go` (FR-009, FR-015)
- [ ] T056 [P] Implement the memory equivalents in `internal/adapters/storage/memory/capability_repository.go`, `permission_set_repository.go`, `user_grants.go` and `agent_repository.go` (DB-003)
- [ ] T057 Run `go test ./internal/domain/storage/... ./internal/adapters/storage/memory/...` and `just test-integration-infra` for the PostgreSQL and migration packages (Principle IX)

**Checkpoint**: Foundation ready, so user story work can begin

---

## Phase 3: User Story 1 - Operators Define Capabilities and Compose Permission Sets (Priority: P1) 🎯 MVP

**Goal**: Operators create capability groups, compose sets from capability entries and tighten them per agent through Admin API 4.0.0.

**Independent Test**: `tests/e2e/capability_groups_test.go`, the US1.3 test in `permission_sets_test.go` and the US1.4 test in `agent_permission_requirements_test.go` pass (quickstart.md § 3).

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T058 [P] [US1] Write unit tests in `internal/domain/capability/service_test.go` covering create, get, list, update with ID diff and delete. Include: service and scope existence checks for data-model.md `CapabilityServiceScope.ServiceID` and `CapabilityServiceScope.Scopes`, `409` on referenced capabilities, and audit events `capability.capability_group.*` with their fields via an injected buffer logger (FR-001, FR-002, FR-008, US1.1, US1.2, US1.5; contracts/observability.md)
- [ ] T059 [P] [US1] Write table tests for `EffectiveCapabilities` in `internal/domain/permissionset/effective_test.go`. Cover nil agent list, subset, restated mandatory, tightened optional and set order (FR-005–FR-007; research R4)
- [ ] T060 [P] [US1] Update `internal/domain/permissionset/service_test.go`. Capability entries must exist via `CapabilityResolver`. A set update that drops an agent-listed capability or makes mandatory an agent-optional one returns `ErrorKindConflict`. Dropping a capability that only grants reference succeeds. The audit fields change to `capabilities` (FR-003, research R3, decision D3)
- [ ] T061 [P] [US1] Update `internal/domain/agents/service_test.go` and the reconcile tests. Cover the FR-005 subset and relaxation rules on create, update and reconcile, the reconcile no-op comparison including capability lists, and service coverage through capabilities (FR-005, US1.4)
- [ ] T062 [P] [US1] Write handler tests in `internal/adapters/http/handlers/admin/capability_groups_handler_test.go` and update `permission_sets_handler_test.go` and `agents_handler_test.go`. Cover decoding, canonical references (`capability_id` and `service_id` as UUID or canonical, `Prefer: reference-id=canonical`), `service_scopes` rejection, and the 400, 404 and 409 mapping (API-001; contracts/admin-api.md)

### Implementation for User Story 1

- [ ] T063 [US1] Implement `internal/domain/capability/service.go` (`NewService` over the T035 ports and the service provider). It enforces the invariants of data-model.md § CapabilityGroup, emits the audit events, and keeps a TTL cache for `GetByIDs` matching `permissionset.Service` (FR-001, FR-002, FR-008; research R1, R15)
- [ ] T064 [US1] Implement `EffectiveCapabilities` in `internal/domain/permissionset/effective.go` (research R4; FR-006, FR-007)
- [ ] T065 [US1] Update `internal/domain/permissionset/service.go`: add the `CapabilityResolver` dependency, entry validation, the R3 agent-entry check via `AgentPermissionSetReferenceRepository`, and the audit field changes (FR-003, FR-004; research R3)
- [ ] T066 [US1] Update `internal/domain/agents/service.go` and `reconcile.go`: validate agent capability lists against the resolved set (data-model.md `AgentPermissionSetEntry.Capabilities`, adding the FR-005 subset and no-relaxation rules), and make `validateServiceRequirementsCoverage` gather services through capabilities (FR-005, FR-006)
- [ ] T067 [US1] Implement `internal/adapters/http/handlers/admin/capability_groups_handler.go` with the request and response types of `contracts/admin-api.md`, canonical-reference resolution and `handleStorageError` mapping (API-001, US1.1, US1.2, US1.5)
- [ ] T068 [US1] Update `internal/adapters/http/handlers/admin/permission_sets_handler.go`. Use `capabilities` request and response types, reject `service_scopes` through the raw member map with `400`, and filter `?service_id=` through capabilities (US1.3; contracts/admin-api.md)
- [ ] T069 [US1] Update `internal/adapters/http/handlers/admin/agents_handler.go` so `PermissionSetRequest` and `PermissionSetDeclarationResponse` carry the optional `capabilities` (US1.4)
- [ ] T070 [US1] Wire `capability.Service` and the resolvers in `internal/app/builder.go`. Pass `CapabilityResolver` to `permissionset.Service`, and wire the handler (Principle XII)
- [ ] T071 [US1] Update `scripts/setup-dev-data.sh` to create capability groups and capability-based sets with Admin API 4.0.0 bodies (API-001)
- [ ] T072 [US1] Run the US1 unit tests and the US1 E2E tests (quickstart.md § 3) until green

**Checkpoint**: User Story 1 is fully functional and testable on its own

---

## Phase 4: User Story 2 - Existing Permission Sets and Grants Keep Working After Upgrade (Priority: P1)

**Goal**: After migration `050`, every legacy grant has the same effective scopes and policy-visible permission-set contents, and today's consent request still works.

**Independent Test**: The migration integration tests (T016) and `tests/e2e/capability_model_upgrade_test.go` pass (quickstart.md §§ 2, 4).

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T073 [P] [US2] Update `internal/domain/consent/service_test.go` for the R5 derivation:
  - stored capabilities are exactly the effective ones whose services are all included;
  - `ErrInvalidServiceInclusion` when a mandatory capability needs an excluded service, or an included service yields no capability (decision D5);
  - `grantMatchesRequest` compares capability IDs;
  - grant audit events carry `granted_capability_ids`, with no credential in the buffer.

  (FR-010, US2.4; contracts/observability.md)
- [ ] T074 [P] [US2] Add tests in `internal/domain/consent/` for the derived end-user service view (research R6). A service is listed when any effective capability needs it, and is mandatory when any capability mandatory for the agent needs it. Include the edge case of an agent entry that makes a capability mandatory (FR-010, FR-013, API-003, US2.5)
- [ ] T075 [P] [US2] Update `internal/adapters/http/handlers/consent/grants_handler_test.go`, the agent consent view tests and the grant read-model tests to assert unchanged request and response shapes (API-003, US2.4)

### Implementation for User Story 2

- [ ] T076 [US2] Update `GrantConsent`, `validateGrantStructureWithResolved`, `validateScopeInclusionWithResolved`, `ValidateSubmission` and `grantMatchesRequest` in `internal/domain/consent/service.go` to store data-model.md `GrantedPermissionSetEntry.GrantedCapabilityIDs` from the included-service request per research R5. Other validations are unchanged (FR-010, SR-003)
- [ ] T077 [US2] Derive the end-user `PermissionSetWithRequirement.permission_set.service_scopes` view and the grant read model's `granted_permission_sets` map from effective and granted capabilities, in the consent domain service and its handlers under `internal/adapters/http/handlers/consent/` (research R6; FR-013, FR-015, API-003)
- [ ] T078 [US2] Run T016 and `tests/e2e/capability_model_upgrade_test.go` (`just test-integration-infra`, `just test-e2e-docker`) until green (US2.1–US2.4, SC-001)

**Checkpoint**: User Stories 1 and 2 work independently

---

## Phase 5: User Story 3 - Policy Receives the Granted Capabilities (Priority: P1)

**Goal**: Token exchange derives scopes from granted capabilities and fails closed on stale or undeclared ones. The permit carries `granted_capabilities`, which enabled-mode ExtProc gives OPA.

**Independent Test**: `tests/e2e/capability_policy_journey_test.go` and `just test-e2e-extproc` pass (quickstart.md § 5).

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T079 [P] [US3] Update `internal/domain/tokenexchange/grant_resolver_test.go`. Cover the union per service over granted capabilities, the unchanged ceiling and intersection behavior, a stale capability (not in the set) producing `access_denied` with `grant_permission_set_missing` and the consent `error_uri`, and an undeclared capability (not in the agent's effective list) producing `grant_permission_set_undeclared` (FR-011, FR-012, SR-001, US3.4; research R7, decision D4)
- [ ] T080 [P] [US3] Update `internal/domain/tokenexchange/evaluation_test.go`. `project` emits `granted_capabilities` as `{id, canonical_id}` references, de-duplicated across sets and sorted by `canonical_id` (US3.3), and `permission_sets[].services` hold the union over granted capabilities (FR-014, FR-015)
- [ ] T081 [P] [US3] Update `internal/domain/tokenexchange/service_test.go` so the token-exchange response's `granted_permission_sets` is derived from capabilities and gains no capability member (FR-015, FR-016)
- [ ] T082 [P] [US3] Update `internal/adapters/http/enduser/authzen_evaluation_test.go` to assert the permit body carries `context.granted_capabilities` (API-002)
- [ ] T083 [P] [US3] Update `internal/extproc/grantcontext/client_test.go`. Cover decode into `Context.GrantedCapabilities` and rejection of absent, null and empty lists, entries without a non-empty `id` or `canonical_id`, duplicate entries and unsorted lists as malformed permits with no cache entry (FR-016; contracts/policy-input.md § 3)
- [ ] T084 [P] [US3] Update `internal/extproc/server/grant_context_test.go` and `internal/extproc/authorization/input_builder_test.go`. Enabled-mode JSON includes `granted_capabilities` on the body, batch and header-only paths. `TestGrantContext_DisabledModeMakesNoResolverCallsAndKeepsInput` stays unchanged and passes (FR-014, US3.6)
- [ ] T085 [P] [US3] Write `examples/policies/capability_tool_map_test.rego`, an `opa test` suite for allow on a held capability, deny on a missing one, deny on an unmapped tool and allow on non-tool-call methods. Add a Go test that runs it in `internal/extproc/authorization/` (FR-017, SC-003)

### Implementation for User Story 3

- [ ] T086 [US3] Update `resolveGrantCoverage` in `internal/domain/tokenexchange/grant_resolver.go`. Validate granted capability IDs against the set and the agent's effective list, union scopes per service from capabilities, and keep steps 5–7. Add the `capability_id` UUID attribute to the existing diagnostic log record (research R7; contracts/observability.md § Diagnostic logs)
- [ ] T087 [US3] Add `GrantedCapabilities []GrantEntityRef` (`json:"granted_capabilities"`) to `GrantContext` in `internal/domain/tokenexchange/evaluation.go` and populate it in `project`. Derive `permission_sets` services from capabilities (FR-014, FR-015, API-002)
- [ ] T088 [US3] Derive `GrantedPermissionSets` in `internal/domain/tokenexchange/service.go` from the granted capabilities' services (FR-015)
- [ ] T089 [US3] Add `GrantedCapabilities *[]*wireCapability` to `wireContext` in `internal/extproc/grantcontext/context.go`, validated in `parsePermitContext`, and `GrantedCapabilities []EntityRef` to `Context` in `internal/extproc/grantcontext/client.go` (FR-016; research R8)
- [ ] T090 [US3] Add `CapabilityContext` (`{id, canonical_id}`) and `GrantedCapabilities []CapabilityContext` (`json:"granted_capabilities,omitempty"`) to `ContextInput` in `internal/extproc/authorization/input.go`. Copy it only in `contextInputFromGrantContext` in `internal/extproc/server/grant_context.go` (FR-014, FR-016, US3.6)
- [ ] T091 [US3] Write `examples/policies/capability-tool-map.rego` per `contracts/policy-input.md` § 5, using the package and rule names of `tests/e2e/extproc/fixtures/policies/` (FR-017, US3.1, US3.2)
- [ ] T092 [US3] Update `internal/extproc/AGENTS.md` with the enabled-mode input member list and the required permit member (FR-014)
- [ ] T093 [US3] Run the US3 unit tests, `opa test examples/policies/`, `ginkgo -v --label-filter='grant-context' --focus='Capability Policy' ./tests/e2e/` and `just test-e2e-extproc` until green (US3.1–US3.6, SC-002, SC-003)

**Checkpoint**: All user stories work independently

---

## Feature-Specific Finishing Work

- [ ] T094 [P] Document `granted_capabilities` in `docs/api/authzen-evaluation.md` § ExtProc consumer and in the permit example. State the broker-before-ExtProc upgrade order (FR-016; research R8)
- [ ] T095 [P] Add capability-based policy to `docs/guides/token-exchange-gateway.md`: the sample policy, the requirement for grant-context mode, and the SR-002 deployment controls from research R14 (SR-002, FR-017)
- [ ] T096 [P] Add capability groups and the "Migrate administrative consumers to Admin API 4.0.0" section to `docs/guides/manage-agents-and-services.md` (API-001; research R11)

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T097 Verify every task traces to an approved requirement, acceptance scenario or binding constraint (Feature Scope & Deferred Findings)
- [ ] T098 Verify the domain model is documented in the `ARCHITECTURE.md` Glossary (Principle V)
- [ ] T099 Verify no configuration example is required beyond the T008 header update, and that `examples/config/` stays consistent (Principle VII)
- [ ] T100 Verify `examples/config/README.md` needs no new reference (Principle VII)
- [ ] T101 Verify API designs are documented in `api/enduser/openapi.yaml` (1.4.0) and `api/admin/openapi.yaml` (4.0.0) (Principles IV, X)
- [ ] T102 Verify the user or stakeholder confirmed both API designs and decisions D1–D6, with references in the PR (Principle X)
- [ ] T103 Verify the database schema design is documented and migration `050` exists (Principle IX)
- [ ] T104 Verify spec.md Journeys 1 and 2 are linked to their scenarios and match the contracts. No Wireloom or JWT examples apply, because there is no UI or token-shape change (Principle XIV)
- [ ] T105 Verify E2E acceptance tests exist in `tests/e2e/` for every scenario, and US2.3 is in `tests/integration/migrations/` as recorded (Principle XIII)
- [ ] T106 Verify E2E tests were written before implementation with realistic expectations, no placeholder failures, no `XIt`, `PIt` or `Skip()`, and no red-phase comments (Principle XIII)
- [ ] T107 Verify the changed log events and the diagnostic attribute are listed in `contracts/observability.md`, with no new instrument (Principle XV)

#### Implementation Phase Verification [MANDATORY]

**Feature Scope** (Development Requirements):
- [ ] T108 Verify unrelated remediation is excluded and only user-approved issue URLs appear under Future Work (Feature Scope & Deferred Findings)

**API & Documentation** (Principles IV, X):
- [ ] T109 [P] Verify both OpenAPI files reflect the implemented APIs
- [ ] T110 [P] Verify the implementation matches the confirmed OpenAPI exactly. Handler tests and E2E bodies agree with the schemas
- [ ] T111 Verify `docs/api/authzen-evaluation.md` documents the new permit member with an example (T094)

**Architecture & Documentation** (Principle II):
- [ ] T112 Verify `ARCHITECTURE.md` reflects the storage, Grant Context, ExtProc input and API-version changes (T006)
- [ ] T113 Verify the `ARCHITECTURE.md` Glossary has the new and revised terms (T004)
- [ ] T114 [P] Finalize `adrs/20261127-190555-capability-model.md` so it matches the implementation. Set Status to Accepted with the approval reference after stakeholder approval (decision D7), and update the root `AGENTS.md` ADR index row (Principle II)

**Configuration** (Principle VII):
- [ ] T115 [P] Verify ExtProc keeps its own configuration under ADR `011-extproc-standalone-binary`, and no configuration path was added
- [ ] T116 Verify no Helm change is required, because no chart-managed parameter changed (Principle VII)

**Database & Persistence** (Principle IX):
- [ ] T117 [P] Verify migration `050` follows sequential numbering and has both directions
- [ ] T118 [P] Verify the migration integration tests cover apply, rollback, re-apply, abort and the down guards
- [ ] T119 [P] Verify every new and changed PostgreSQL repository method is integration-tested
- [ ] T120 Verify the capability persistence follows `specs/004-persistence-layer/quickstart.md` patterns (factory accessors, `StorageError` wrapping, timeouts)

**Security** (Principles I, III):
- [ ] T121 Verify every validation and resolution failure fails closed (SR-003). Verify there is no fallback to all capabilities or set services, and the required permit member fails closed when absent
- [ ] T122 [P] Verify no custom cryptography is added. The migration's `md5` only derives identifiers
- [ ] T123 [P] Verify the audit events for capability groups, sets, agents and grants follow `coding-standards/LOGGING.md` (Principles I, XV)

**Coding Standards** (Principle XV):
- [ ] T124 Verify the added and modified code follows `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md` and their review checklists
- [ ] T125 Verify the required tests are present: credentials absent from grant and capability events, and principals typed (`TestLoggedPrincipalsAreTyped`)
- [ ] T126 Verify no lint rule or standard check is disabled or suppressed. Run `just telemetry-doc` and commit `docs/reference/telemetry.md` if it changed

**Architecture Patterns** (Principle VI):
- [ ] T127 Verify the domain uses ports. Handlers call domain services only, `EffectiveCapabilities` stays in the domain, and `domain/capability` imports no adapter

**Testing** (Principle VIII):
- [ ] T128 Verify unit tests were written first in red-green cycles. Red-run evidence is not required
- [ ] T129 Verify tests drove the design
- [ ] T130 Verify tests changed minimally during implementation
- [ ] T131 Verify automated unit and integration tests are included
- [ ] T132 Verify no Bash script validates code correctness. `scripts/setup-dev-data.sh` only seeds development data

**E2E Acceptance Testing** (Principle XIII):
- [ ] T133 Verify E2E tests exist in `tests/e2e/` for all acceptance scenarios
- [ ] T134 Verify each `It()` maps to exactly one scenario
- [ ] T135 Verify E2E tests were written before implementation. Docker-labelled suites may first run in CI
- [ ] T136 Verify E2E tests changed minimally during implementation, with fixture adjustments only
- [ ] T137 Verify the E2E tests turned green
- [ ] T138 Verify the E2E tests use Ginkgo and Gomega per `tests/e2e/README.md`
- [ ] T139 Verify the hierarchy Describe → Context → It
- [ ] T140 Verify the `// Spec: USx.y` comment references
- [ ] T141 Run `just verify`, the full gate including `ginkgo -v ./tests/e2e/`, the ExtProc and docker suites, and the integration layers. All must pass

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 1** (T001–T002) has no dependency.
- **Phase 2** (T003–T033) depends on Phase 1 and blocks all implementation. T013 and T015 (stakeholder confirmation) must be done before Phase 2.7. Sub-phases 2a–2g can run in parallel. T017 and T018 follow T016. T030 follows T020–T028.
- **Phase 2.7** (T034–T041) depends on Phase 2.
- **Phase 2.5** (T042–T057) depends on Phase 2.7. Tests T042–T045 and T050–T052 come before implementation T046–T049 and T053–T056.
- **User stories** depend on Phase 2.5:
  - US1 (T058–T072) has no story dependency.
  - US2 (T073–T078) uses `EffectiveCapabilities` (T064) for the consent derivation.
  - US3 (T079–T093) uses `EffectiveCapabilities` (T064) for the undeclared check. Its E2E journey seeds through the Admin API from US1 (T067, T068).
- **Finishing work** (T094–T096) follows the stories it documents.
- **Phase N** (T097–T141) follows everything.

### Within each story

Tests come before models, which come before services, then handlers, then wiring. Each story ends with its green run (T072, T078, T093).

### Parallel opportunities

- Phase 2f E2E files T020–T027 are separate files.
- Phase 2.5 tests T042–T045 and T050–T052.
- US1 tests T058–T062.
- US3 broker tests (T079–T082) and ExtProc tests (T083–T085) are separate modules.
- Once T064 is done, US2 and US3 implementation can proceed in parallel.

---

## Parallel Example: User Story 3

```bash
# Broker and ExtProc tests together:
Task: "Update grant_resolver_test.go for capability coverage and fail-closed cases (T079)"
Task: "Update evaluation_test.go for granted_capabilities projection (T080)"
Task: "Update grantcontext/client_test.go for required granted_capabilities decode (T083)"
Task: "Update server/grant_context_test.go and input_builder_test.go for enabled-mode input (T084)"
Task: "Write opa test suite for examples/policies/capability-tool-map.rego (T085)"
```

---

## Implementation Strategy

### MVP first (User Story 1)

1. Phase 1, then Phase 2, including API confirmation (T013, T015) and the migration with its tests (T016–T018).
2. Phase 2.7 scaffolding as its own commit batch, then Phase 2.5.
3. US1 (T058–T072). Stop and validate with quickstart.md § 3.

US1 alone is not releasable. The migration replaces the old model, so US2 must ship in the same release to keep existing grants working (FR-020). US3 completes the feature's purpose.

### Incremental delivery within the PR

1. Commit batches: Phase 2 design, Phase 2.7 scaffold, Phase 2.5 foundation, then US1, US2 and US3.
2. Each batch ends with `just check` and the smallest matching test command (AGENTS.md § Development Workflow).
3. Commit the finishing documentation and Phase N, then run `just verify`.

---

## Notes

- [P] tasks touch different files and have no dependencies on incomplete tasks.
- The constrained fields of data-model.md are cited by `Entity.field` in T017, T034, T042–T049, T050, T058, T066 and T076.
- Commit with DCO sign-off after each logical group.
