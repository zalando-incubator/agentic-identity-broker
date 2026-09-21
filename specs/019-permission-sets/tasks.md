# Tasks: Permission Sets (019)

**Input**: Design documents from `/specs/019-permission-sets/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

**Tests**: Per Constitution Principle VIII (TDD), automated tests are MANDATORY. Test tasks are included in each user story phase and MUST be written before or alongside implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

---

## Phase 1: Setup

**Purpose**: Confirm environment baseline and orient to existing patterns before implementation.

- [X] T001 Verify `just check` and `just verify` pass clean before any changes (baseline confirmation)
- [X] T002 [P] Read implementation reference files: `internal/adapters/storage/memory/agent_repository.go`, `internal/adapters/storage/postgres/agent_repository.go`, and `internal/adapters/http/handlers/admin/agents_handler.go` for established patterns

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: All design artifacts are documented as complete in plan.md. Phase 2 tasks finalize code-level outputs of the design phase before implementation begins.

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [X] T003 Update `ARCHITECTURE.md` Glossary with four new domain terms: `PermissionSet`, `ServiceScope`, `AgentPermissionSetEntry`, `granted_permission_sets` (definitions from `data-model.md` §Glossary Updates)
- [X] T004 [P] Document `PermissionSetID` typed ID in `internal/domain/id/AGENTS.md` per ADR-013 rules

**Checkpoint**: Domain model documented in ARCHITECTURE.md and AGENTS.md

### Phase 2b: Configuration Design [MANDATORY]

- [X] T005 Confirm no new configuration parameters are required for this feature — document this in the PR description (N/A: cache TTL is a code constant; no Helm chart update needed)

**Checkpoint**: Configuration design confirmed (no changes required)

### Phase 2c: API Design [MANDATORY]

- [X] T006 Confirm API-005 stakeholder sign-off on `contracts/admin-permission-sets.yaml` and `contracts/enduser-permission-sets.yaml` before implementation begins — document confirmation in PR

**Checkpoint**: API contracts confirmed by stakeholder

### Phase 2d: Database Design [MANDATORY]

- [X] T007 [P] Verify migration numbers 008/009/010 are the next available in `/migrations/` (check existing files for sequence gaps)

**Checkpoint**: Migration numbering confirmed, schema design verified

### Phase 2e: Frontend / Design System Review [MANDATORY]

- [X] T008 Review `web/src/design-system/docs/DECISION_TREES.md`, `COMPONENT_PAIRING_GUIDE.md`, and `COMMON_MISTAKES.md` for `PermissionSetCard` component selection
- [X] T009 [P] Document planned design system primitives: Card, Badge, Checkbox, Button from `web/src/design-system/`; confirm semantic tokens: `trust` (mandatory cards), `neutral` (optional cards), `success-primary` (connected service indicator)

**Checkpoint**: Design system usage planned, no extended palettes (`navy-*`, `gray-*`) to be used

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

- [ ] T010 Write Ginkgo/Gomega E2E acceptance tests in `tests/e2e/permission_sets_test.go` — all 24 `It()` blocks per plan.md scenario mapping (US1.S1–S6, US2.S1–S7, US3.S1–S5, US4.S1–S4, plus edge case); each block contains realistic assertions against actual HTTP responses; no `XIt`/`PIt`/`Skip()`; no placeholder `Expect(true).To(BeFalse())`
- [ ] T011 [P] Write Playwright frontend E2E tests in `tests/e2e/frontend/permission_sets_frontend_test.go` — 8 UI scenarios per plan.md with screenshot assertions to `tests/e2e/screenshots/consent_permission_sets_*.png`: (1) PS section above services, (2) mandatory PS card locked, (3) optional PS card togglable, (4) PS card per-service toggles for SR-intersecting services, (5) dynamic service connections update on PS toggle, (6) service already connected, (7) Approve blocked when services unconnected, (8) Approve enabled when all connected
- [ ] T012 Run `ginkgo build ./tests/e2e/` and `ginkgo build ./tests/e2e/frontend/` — verify all 24+8 tests compile, then run and confirm they FAIL semantically (red phase)

**Checkpoint**: All E2E tests compile, fail semantically in red phase, no placeholder assertions

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Domain primitives, storage port, and migrations that all user stories depend on. No user story code can begin until this phase is complete.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T013 Add `PermissionSetID` entry to `internal/domain/id/gen_ids.go` (`{"PermissionSetID", "permission_set"}`) then run `go generate ./internal/domain/id/` to regenerate typed ID boilerplate
- [X] T014 Create `internal/domain/storage/permission_set.go`: define `ServiceScope` (value object) and `PermissionSet` (entity) structs per `data-model.md` §New Entities
- [X] T015 [P] Extend `internal/domain/storage/agent.go`: add `AgentPermissionSetEntry` type and `PermissionSets []AgentPermissionSetEntry` field to `Agent` struct per `data-model.md` §Modified Entities
- [ ] T016 Modify `internal/domain/storage/user_grant.go`: remove `DelegatedOAuth2Tokens []DelegatedToken` field; add `GrantedPermissionSets []GrantedPermissionSetEntry` field (structured: `{PermissionSetID id.PermissionSetID, IncludedServiceIDs []id.ServiceID}`) per spec.md §Domain Model — positive inclusion model where each entry pairs a PS ID with the explicit list of service IDs the user included at consent time
- [X] T017 Add `PermissionSetRepository` interface (8 methods: `Create`, `Get`, `GetByIDs`, `Update`, `Delete`, `List`, `CountAgentsReferencingPermissionSet`, `CountPermissionSetsForService`) to `internal/ports/storage.go` per `data-model.md` §New Port Interface
- [X] T018 Create `migrations/008_add_permission_sets.up.sql` (create `permission_sets` table + `permission_set_service_scopes` join table with FK + RESTRICT constraint) and `migrations/008_add_permission_sets.down.sql`
- [X] T019 [P] Create `migrations/009_add_agent_permission_sets.up.sql` (add `permission_sets JSONB` column + GIN index to `agents` table) and `migrations/009_add_agent_permission_sets.down.sql`
- [ ] T020 [P] Create `migrations/010_migrate_user_grants_to_permission_sets.up.sql` (DELETE all existing grants; ADD `granted_permission_sets JSONB` column + GIN index for structured `{ps_id: [service_ids]}` entries; DROP `delegated_oauth2_tokens` column) and `migrations/010_migrate_user_grants_to_permission_sets.down.sql`
- [X] T021 Verify `just build` succeeds after all foundational changes (domain types + ports must compile cleanly)
- [X] T021a [P] Write unit tests (red phase) for `PermissionSetService` in `internal/domain/permissionset/service_test.go` using hand-rolled mock of `ports.PermissionSetRepository` — cover: `GetByIDs` cache miss → fetches from repo; cache hit within TTL → no repo call; cache expired (TTL >60s) → re-fetches; `ValidateIDs` returns error listing missing IDs; `Delete` returns typed conflict error when `CountAgentsReferencingPermissionSet > 0`; `Create`/`Update`/`Delete` emit structured audit log entries (SR-004); background eviction goroutine stops cleanly on context cancel
- [X] T021b Create `internal/domain/permissionset/service.go`: `PermissionSetService` with constructor `NewPermissionSetService(repo ports.PermissionSetRepository, log *slog.Logger) *PermissionSetService`; CRUD methods delegate to repo; `GetByIDs(ctx, ids)` uses `sync.RWMutex` + `map[id.PermissionSetID]psCacheEntry{ps, expiresAt}` TTL cache (≈60s); background eviction goroutine started in constructor, stops on context cancel; `ValidateIDs(ctx, ids)` calls `GetByIDs` and returns error listing missing IDs; `Delete(ctx, id)` calls `repo.CountAgentsReferencingPermissionSet` first and returns typed conflict error if > 0; `Create`/`Update`/`Delete` emit structured `slog.Info` audit log entries (SR-004)

**Checkpoint**: Foundation ready — domain types, port interface, migrations, and `PermissionSetService` defined; user story implementation can now begin

---

## Phase 3: User Story 1 — Admin Manages Permission Sets (Priority: P1) 🎯 MVP

**Goal**: Administrators can create, read, update, delete, and list permission sets via the admin API on port `:14000`. Deletion is blocked with 409 when the set is referenced by an agent (`CountAgentsReferencingPermissionSet > 0`) OR when any active user grant references the set (`CountGrantsReferencingPermissionSet > 0`). Both conflict paths return HTTP 409 with a descriptive message.

**Independent Test**: Call admin API to POST a permission set → GET by ID → PUT update → GET list with service filter → attempt DELETE while agent references it (expect 409) → remove agent reference → attempt DELETE while active user grant references the permission set (expect 409; active grant is one where `valid_until IS NULL OR valid_until > NOW()`) → revoke that grant → DELETE (expect 204).

### Tests for User Story 1 [MANDATORY — Principle VIII]

- [X] T022 [US1] Write unit tests (red phase) for `PermissionSetsHandler` in `internal/adapters/http/handlers/admin/permission_sets_handler_test.go` using `testify/mock` mock of `*permissionset.PermissionSetService` — cover 201 create, 409 name conflict, 422 validation, 200 GET, 200 PUT, 204 DELETE, 409 deletion-protection (agent reference), 409 deletion-protection (active user grant reference via `CountGrantsReferencingPermissionSet`), 200 list with service filter
- [X] T023 [P] [US1] Write integration tests (red phase) in `internal/adapters/storage/postgres/permission_set_repository_test.go` — cover migration apply/rollback/repeat, all 8 `PermissionSetRepository` methods against real Postgres via testcontainers

### Implementation for User Story 1

- [X] T024 [P] [US1] Create `internal/adapters/storage/memory/permission_set_repository.go`: in-memory implementation using `sync.RWMutex` + deep-copy pattern; `byID map[id.PermissionSetID]*storage.PermissionSet` + `byName map[string]id.PermissionSetID`; return `StorageError{Kind: KindConflict}` for duplicate names; `CountAgentsReferencingPermissionSet` iterates injected agent repository
- [X] T025 [P] [US1] Create `internal/adapters/storage/postgres/permission_set_repository.go`: sqlx + pgx v5 implementation; `Create` inserts into `permission_sets` + `permission_set_service_scopes`; `List(serviceID)` JOINs scopes table when serviceID non-zero; `CountAgentsReferencingPermissionSet` uses JSONB containment `@>` query per `research.md` §6
- [X] T026 [US1] Extend `internal/adapters/storage/memory/factory.go`: include `PermissionSetRepository` in in-memory storage factory
- [X] T027 [US1] Extend `internal/adapters/storage/postgres/factory.go`: include `PermissionSetRepository` in postgres storage factory
- [X] T028 [US1] Extend `internal/adapters/storage/factory.go`: expose `PermissionSetRepository` from top-level storage adapter factory
- [X] T029 [US1] Create `internal/adapters/http/handlers/admin/permission_sets_handler.go`: `PermissionSetsHandler` with constructor `NewPermissionSetsHandler(svc *permissionset.PermissionSetService, log *slog.Logger) *PermissionSetsHandler`; implement `Create`, `Get`, `List`, `Update`, `Delete` methods that delegate entirely to `svc`; handler responsibility is HTTP-to-domain translation only — map domain conflict/not-found/validation errors to 409/404/422 HTTP responses; audit log entries and deletion protection are handled by `PermissionSetService` (SR-004)
- [X] T030 [US1] Extend `internal/app/handlers.go`: add `PermissionSets *admin.PermissionSetsHandler` field to `AdminHandlers` struct
- [X] T031 [US1] Extend `internal/app/builder.go`: instantiate `permissionset.PermissionSetService` (from T021b) using `PermissionSetRepository` from storage factory; instantiate `PermissionSetsHandler` with the service; assign to `AdminHandlers.PermissionSets`
- [X] T032 [US1] Register admin routes in `internal/adapters/http/routing/admin.go`: add `r.Route("/permission-sets", ...)` block with POST, GET list, GET by ID, PUT, DELETE per `quickstart.md` §Step 6
- [X] T033 [US1] Inject `ports.PermissionSetRepository` into `internal/domain/thirdparty/` `ThirdpartyOAuth2ProviderService`: update `NewThirdpartyOAuth2ProviderService` constructor to accept `psRepo ports.PermissionSetRepository`; in `Delete()`, call `psRepo.CountPermissionSetsForService(ctx, serviceID)` before delegating to the provider repo — return a typed conflict error if count > 0; update `internal/app/builder.go` to pass `PermissionSetRepository` when constructing `ThirdpartyOAuth2ProviderService`; `ServicesHandler` requires no changes — it already calls `providerService.Delete()` and maps errors to HTTP 409 (DB-006 application layer)
- [X] T034 [US1] Merge permission set contract into `/api/admin/openapi.yaml`: add `PermissionSets` tag, all five paths (`/permission-sets`, `/permission-sets/{id}`), `CreatePermissionSetRequest`, `PermissionSetResponse`, `ServiceScopeRequest/Response` schemas; add `permission_sets` field to `CreateAgentRequest` and `UpdateAgentRequest` schemas

**Checkpoint**: User Story 1 fully functional — admin CRUD works via `:14000`, 409 deletion protection active, unit + integration tests green, E2E US1 scenarios green

---

## Phase 4: User Story 2 — Consent Screen Shows Permission Sets First (Priority: P1)

**Goal**: The consent screen renders an "Agent Permissions" section (stacked cards, mandatory locked, optional togglable) above the service connections section. Services with active sessions show a satisfied indicator and no connect button.

**Independent Test**: Load consent screen for an agent with one mandatory + one optional permission set, two service requirements (one mandatory, one optional), and one already-connected service. Verify PS cards appear first with SR-intersecting services only; mandatory PS card is non-interactive; optional PS card toggles; per-service toggles appear for optional SR services within each PS card; dynamic service connections update when PS is toggled; the connected service shows a satisfied indicator and no connect button; Approve button disabled until all included services connected.

### Tests for User Story 2 [MANDATORY — Principle VIII]

- [ ] T035 [P] [US2] Write unit tests (red phase) for `ConsentService` in `internal/domain/consent/service_test.go` using hand-rolled mocks — cover: `ResolvedPermissionSets` populated from `agent.PermissionSets` via `psService.GetByIDs()` (mock `*permissionset.PermissionSetService`); `ActiveSessionServiceIDs` populated from active `UserSessions` via `ports.UserSessionRepository`; service with active session represented as connected with no connect action; `AvailableServices` populated from `thirdpartyService.List()` (mock `*thirdparty.ThirdpartyOAuth2ProviderService`); **NEW: PS card filtering** — resolved PS entries only include services intersecting with agent's `service_requirements`; **dynamic service connections** — service connections computed as union of mandatory SR services + optional SR services from selected PS `included_service_ids`; **submission gating** — `ValidateSubmission` rejects when any included service lacks an active session

### Implementation for User Story 2

- [ ] T036 [US2] Extend `internal/domain/consent/service.go`: add `ResolvedPermissionSetEntry` struct + `ResolvedPermissionSets []ResolvedPermissionSetEntry`, `ActiveSessionServiceIDs []id.ServiceID`, and `AvailableServices []*model.ThirdpartyOAuth2ProviderEntity` fields to `AgentConsentInfo`; inject `*permissionset.PermissionSetService`, `ports.UserSessionRepository`, and `*thirdparty.ThirdpartyOAuth2ProviderService` into `ConsentService` — no direct repository dependencies; populate all three new fields in `GetAgentConsentInfo` (resolved PSets via `psService.GetByIDs()`, active sessions via `sessionRepo.ListByPrincipal()`, available services via `thirdpartyService.List()`); add `ValidateSubmission(ctx, agent, grantedPermissionSets, activeSessionServiceIDs)` method — validates that every service in the grant's `included_service_ids` has an active session, returns descriptive error listing unconnected services if not (FR-020); **also update `internal/app/builder.go`** to pass the two new dependencies (`*permissionset.PermissionSetService`, `*thirdparty.ThirdpartyOAuth2ProviderService`) when constructing `ConsentService`
- [X] T037 [US2] Extend consent-info HTTP handler (end-user server): serialize `ResolvedPermissionSets` as `permission_sets` array, `ActiveSessionServiceIDs` as `active_session_service_ids`, and `AvailableServices` as `available_services` array in the JSON response per `contracts/enduser-permission-sets.yaml` §`AgentConsentInfoResponse` (all three fields are required in the response schema)
- [ ] T038 [US2] Create `web/src/components/consent/PermissionSetCard.tsx`: mandatory PS card renders with `role="checkbox"` `aria-checked="true"` `aria-disabled="true"` (no onClick); optional PS card renders with `role="checkbox"` toggle handler (default unchecked); body shows name (heading), description; **within each PS card, only services intersecting agent's `service_requirements` are shown** — PS-covered services not in SR are hidden; for displayed services: mandatory SR services shown as locked (always included), optional SR services shown with per-service toggle (default on when PS is selected); use `trust` design tokens for mandatory, `neutral` for optional (FR-008, DS spec)
- [ ] T039 [P] [US2] Create `web/src/components/consent/PermissionSetsList.tsx`: renders "Agent Permissions" section header + maps `permission_sets` array to `PermissionSetCard` components in order; lifts `selectedOptionalIds: string[]` state and **per-PS per-service inclusion state** to parent via `onSelectionChange` callback; computes `includedServiceIds` per PS from user's per-service toggle state
- [ ] T040 [US2] Restructure `web/src/pages/AgentGrantDetailPage.tsx`: render `<PermissionSetsList>` first, then **dynamic** service connections section; service connections computed as union of: (1) all mandatory SR services (always shown), (2) optional SR services that appear in `includedServiceIds` of any selected PS; render each selected service with its connection status and show a connect button only when it lacks an active session; **Approve button disabled** (FR-020) until all displayed services have active sessions; wire structured `granted_permission_sets: {ps_id: [included_service_ids]}` into consent submission payload
- [ ] T041 [US2] Verify Playwright tests from T011 now pass: all 8 UI scenarios — PS section above services, mandatory PS locked, optional PS toggles, per-service toggles within PS card, dynamic service connections on PS toggle, connected service shows a satisfied indicator with no connect button, Approve blocked when unconnected, Approve enabled when all connected; screenshots captured
- [ ] T042 [US2] Update frontend API types in `web/src/types/consent.ts`: replace `delegated_oauth2_tokens` with `granted_permission_sets: Record<string, string[]>` (map of PS ID → included service IDs) in `UserGrant`, update `CreateOrUpdateGrantRequest`, update `useToggleGrant` hook and `AgentGrantDetailPage` to use new structured field

**Checkpoint**: User Story 2 complete — consent screen shows PSets first, active sessions show satisfied indicators with no connect buttons, Playwright tests green, E2E US2 scenarios green

---

## Phase 5: User Story 3 — Agent Declares Permission Sets at Agent Level (Priority: P2)

**Goal**: Agent registration/update accepts and validates a `permission_sets` list (min 1 entry, all IDs must exist). Consent-info returns resolved permission set objects with `requirement_type` in declaration order.

**Independent Test**: Register an agent with `permission_sets: [{permission_set_id: <valid>, requirement_type: mandatory}, {permission_set_id: <valid>, requirement_type: optional}]` and `service_requirements` covering a subset of PS services. Verify 201 response. Attempt registration with non-existent ID — verify 422. Attempt registration where SR references a service not covered by any declared PS — verify 422 (FR-019). Call consent-info for the agent — verify resolved permission_sets appear with name, description, service_scopes, requirement_type.

### Tests for User Story 3 [MANDATORY — Principle VIII]

- [ ] T043 [P] [US3] Write unit tests (red phase) for agent `permission_sets` validation in `internal/adapters/http/handlers/admin/agents_handler_test.go` using `testify/mock` mock of `*permissionset.PermissionSetService` — cover: empty list → 422; non-existent `permission_set_id` (`psService.ValidateIDs` returns error) → 422; valid list → 201; list preserved in declaration order; **FR-019 coverage invariant**: every `service_requirements[].service_id` must be covered by at least one PS `ServiceScope` — agent with SR referencing a service not covered by any declared PS → 422 with descriptive error listing uncovered service IDs

### Implementation for User Story 3

- [ ] T044 [US3] Extend `internal/adapters/http/handlers/admin/agents_handler.go`: add `psService *permissionset.PermissionSetService` field to `AgentsHandler`; update `NewAgentsHandler` constructor to accept it; update `builder.go` to pass the `*permissionset.PermissionSetService` instance (created in T031) when constructing `AgentsHandler`; on agent create/update, reject empty `permission_sets` list (422), call `psService.ValidateIDs(ctx, ids)` and return 422 listing invalid IDs if any are not found; **FR-019 coverage invariant enforcement**: resolve all declared PS via `psService.GetByIDs`, collect union of all `ServiceScope.service_id` values, verify every `service_requirements[].service_id` is present in that union — return 422 with uncovered service IDs if not; persist validated list in declaration order via JSONB serialization
- [ ] T045 [US3] Integration checkpoint: call `GET /api/consent/agents/{id}/consent-info` for an agent with a `permission_sets` list — verify response includes `permission_sets` array with resolved objects and `requirement_type` per entry (exercised by E2E US3.S3 scenario written in T010)

**Checkpoint**: User Story 3 complete — agent validation enforces PSets list; consent-info returns resolved PSets; E2E US3 scenarios green

---

## Phase 6: User Story 4 — User Grant Stores Granted Permission Sets (Priority: P2)

**Goal**: Consent submission stores structured `granted_permission_sets` (map of PS ID → included service IDs) in `UserGrant`. Token exchange resolves effective scopes via in-process cache (≈60s TTL), applies SR scope ceiling intersection (FR-013), validates `UserSession` coverage, and returns `granted_permission_sets` in the response.

**Independent Test**: Submit consent grant with `granted_permission_sets: {<mandatory-ps-id>: [<svc-a>, <svc-b>], <optional-ps-id>: [<svc-a>]}`. Verify stored `UserGrant` contains structured map. Request token exchange — verify response includes `granted_permission_sets` and effective scopes reflect SR scope ceiling intersection (FR-013). Submit grant again with different optional selection — verify UserGrant upserted. Configure two PSets covering the same service with different scopes — verify token exchange returns the scope union intersected with SR scopes.

### Tests for User Story 4 [MANDATORY — Principle VIII]

- [ ] T046 [P] [US4] Write unit tests (red phase) in `internal/domain/tokenexchange/service_test.go` using hand-rolled mock of `*permissionset.PermissionSetService` — cover: scope union for two PSets covering the same service (computed from `psService.GetByIDs()` result); **SR scope ceiling intersection** (FR-013): PS scopes intersected with agent's `service_requirements[].scopes` per service — only scopes in both PS and SR are effective; `UserSession` insufficient scope → descriptive error (FR-018); response includes `granted_permission_sets`; structured grant with per-service inclusion respected (only included services contribute scopes); (cache TTL behaviour is covered by T021a — `TokenExchangeService` delegates PS resolution to `PermissionSetService` and has no own cache)

### Implementation for User Story 4

- [ ] T047 [US4] Extend consent grant HTTP handler (end-user server): accept `granted_permission_sets: map[uuid.UUID][]uuid.UUID` (PS ID → included service IDs) in request body; validate (1) map is non-empty — an empty map is always a 422 (FR-014); (2) all mandatory `permission_set_ids` for the agent are present as keys — any missing mandatory ID is a 422; (3) for each PS entry, `included_service_ids` must be a non-empty subset of that PS's `ServiceScope` service IDs intersected with agent's SR — unknown service IDs → 422; pass validated structured grant through to `UserGrant.GrantedPermissionSets`
- [ ] T048 [US4] Update `internal/adapters/storage/memory/user_grant_repository.go`: store and retrieve `GrantedPermissionSets map[id.PermissionSetID][]id.ServiceID` field (was `DelegatedOAuth2Tokens`); update deep-copy logic to deep-copy map values (slices)
- [ ] T049 [US4] Update `internal/adapters/storage/postgres/user_grant_repository.go`: bind `granted_permission_sets JSONB` column (map of PS ID → service ID arrays); remove `delegated_oauth2_tokens` JSONB binding; update all queries (insert, update, select); use `pgx` JSONB scanning
- [ ] T049a [US4] Update `CountAgentsByServiceID` and `ListByServiceID` in both storage adapters — these currently query the removed `delegated_oauth2_tokens` column and will fail post-migration; update port interface doc comment to "counts/lists agents whose granted permission sets include the given service in their included_service_ids"; update postgres adapter to query JSONB: extract all `included_service_ids` arrays from `granted_permission_sets` JSONB values and check for service ID membership; update memory adapter `UserGrantRepository` to iterate `GrantedPermissionSets` map values checking for service ID inclusion; update `internal/adapters/storage/factory.go` accordingly
- [ ] T050 [US4] Extend `internal/domain/tokenexchange/service.go`: inject `*permissionset.PermissionSetService` (replaces `ports.PermissionSetRepository`); update `NewTokenExchangeService` constructor accordingly; update `builder.go` wiring to pass the shared `*permissionset.PermissionSetService` instance; remove any in-service cache — `TokenExchangeService` delegates PS resolution entirely to `PermissionSetService` which owns the TTL cache
- [ ] T051 [US4] Extend `TokenExchangeService.ExchangeToken`: load `grant.GrantedPermissionSets` (structured map); call `psService.GetByIDs(ctx, ps_ids)` to resolve permission sets (TTL caching transparent); for each PS entry, filter `ServiceScope` to only `included_service_ids`; compute per-service scope union across all resolved PSets; **apply SR scope ceiling** (FR-013): intersect computed scopes with `agent.ServiceRequirements[service_id].Scopes` — only scopes present in both are effective; for each service validate `UserSession` token covers effective scopes — fail with descriptive error if not (FR-018); include `granted_permission_sets` in token exchange response (FR-012)
- [ ] T052 [US4] Merge grants + token exchange changes into `/api/enduser/openapi.yaml`: update `CreateGrantRequest` (remove `delegated_oauth2_tokens`, add `granted_permission_sets: object` — map of PS ID to included service ID arrays), `GrantResponse`, `TokenResponse` (add `granted_permission_sets`) per `contracts/enduser-permission-sets.yaml`

**Checkpoint**: User Story 4 complete — grants store structured PSets with per-service inclusion, token exchange resolves scope union with SR ceiling intersection and caching, insufficient scope fails closed, E2E US4 + edge case scenarios green

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY]

**Purpose**: Verify all constitution requirements are met before the feature is considered done.

### Design Phase Verification [MANDATORY]

- [X] T053 Verify `ARCHITECTURE.md` Glossary contains all four new terms: `PermissionSet`, `ServiceScope`, `AgentPermissionSetEntry`, `granted_permission_sets` (Principle V)
- [X] T054 [P] Verify no new config parameters added — Helm chart update not required (Principle VII)
- [X] T055 [P] Verify API-005 stakeholder confirmation documented in PR (Principle X)
- [X] T056 [P] Verify migration files 008/009/010 exist in `/migrations/` with correct `NNN_description.{up,down}.sql` naming (Principle IX)
- [ ] T057 Verify design system review was completed — `DECISION_TREES.md` consulted, only design system primitives used in `PermissionSetCard.tsx` (Principle XI)
- [X] T058 Verify `tests/e2e/permission_sets_test.go` exists with 24 `It()` blocks mapping 1:1 to spec.md scenarios (Principle XIII)
- [ ] T059 Verify all 24 E2E tests initially FAILED in red phase — no placeholder assertions, no `XIt`/`PIt`/`Skip()`, no "red phase" comments (Principle XIII)
- [ ] T060 [P] Verify `tests/e2e/frontend/permission_sets_frontend_test.go` exists with 8 Playwright UI scenarios (Principle XIII)
- [ ] T061 [P] Verify screenshots saved to `tests/e2e/screenshots/consent_permission_sets_above_services.png`, `consent_mandatory_ps_locked.png`, `consent_optional_ps_togglable.png`, `consent_per_service_toggle.png`, `consent_dynamic_connections_ps_toggle.png`, `consent_service_already_connected.png`, `consent_approve_blocked_unconnected.png`, `consent_approve_enabled_all_connected.png` (Principle XIII)

### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):

- [X] T062 [P] Verify `/api/admin/openapi.yaml` contains permission sets CRUD paths + agent schema `permission_sets` field
- [ ] T063 [P] Verify `/api/enduser/openapi.yaml` contains updated `AgentConsentInfoResponse`, `CreateGrantRequest` (with `granted_permission_sets` as object/map schema), `GrantResponse`, and `TokenResponse` schemas
- [ ] T064 Verify API implementations match confirmed OpenAPI specifications exactly (response shapes, status codes, error formats)

**Architecture & Documentation** (Principle II):

- [X] T065 Verify `ARCHITECTURE.md` updated with `PermissionSet` domain description (aggregate, invariants) alongside glossary terms
- [X] T066 [P] Confirm no new ADR required — all decisions follow existing ADRs (004 storage, 013 typed IDs) — document in PR

**Database & Persistence** (Principle IX):

- [ ] T067 [P] Verify migrations 008/009/010 pass apply → rollback → apply without data loss in integration tests
- [ ] T068 [P] Verify both `memory/permission_set_repository.go` and `postgres/permission_set_repository.go` pass identical `PermissionSetRepository` test suite (SC-006)
- [X] T069 [P] Verify deep-copy pattern used in memory adapter (no shared pointer escapes)
- [ ] T070 Verify `user_grant` postgres adapter correctly handles `granted_permission_sets JSONB` column with pgx — no `delegated_oauth2_tokens` references remain; structured map serialization/deserialization tested

**Security** (Principles I, III):

- [ ] T071 Verify permission set CRUD endpoints accessible only on port `:14000` — verify `:8000` returns 404 for any `/permission-sets` path (SR-001)
- [ ] T072 [P] Verify deletion protection: `DELETE /permission-sets/{id}` returns 409 when an agent references the ID (SR-002, FR-004) AND returns 409 when an active user grant references the ID (checked via `CountGrantsReferencingPermissionSet`)
- [ ] T073 [P] Verify `granted_permission_sets` in token response contains only UUIDs as keys and values — no `scopes` fields, no raw OAuth2 strings (SR-003); verify effective scopes reflect SR scope ceiling intersection
- [ ] T074 Verify structured audit log entries emitted on create, update, and delete of permission sets (SR-004) — check log output in integration test

**Architecture Patterns** (Principle VI):

- [X] T075 Verify `internal/domain/` never imports `internal/adapters/` — `go list -f '{{.Imports}}' ./internal/domain/...` must show no adapter imports

**Testing** (Principle VIII):

- [ ] T076 Verify unit tests for handler, consent service, and token exchange were written FIRST and initially FAILED (red-green TDD)
- [ ] T077 Verify memory and postgres adapters both tested with the same test suite method (identical test cases)

**E2E Acceptance Testing** (Principle XIII):

- [ ] T078 Run full E2E suite: `ginkgo -v ./tests/e2e/permission_sets_test.go` — all 24 tests MUST pass
- [ ] T079 Verify each `It()` block maps to exactly one acceptance scenario from `spec.md` (cross-reference plan.md scenario mapping table)
- [ ] T080 [P] Verify E2E tests follow `tests/e2e/README.md` patterns (bootstrap, BeforeEach/AfterEach, Gomega matchers)
- [ ] T081 [P] Verify E2E tests include comment references to spec scenario IDs (e.g., `// US1.S1`)

**Frontend** (Principle XI):

- [ ] T082 Run Playwright frontend E2E suite: `ginkgo -v ./tests/e2e/frontend/` — all 8 tests MUST pass with screenshots saved
- [ ] T083 [P] Verify `PermissionSetCard.tsx` and `PermissionSetsList.tsx` use only design system primitives from `web/src/design-system/` — no raw Tailwind `gray-*`, `navy-*`, `emerald-*` classes
- [ ] T084 [P] Verify WCAG 2.1 AA compliance: mandatory card has `aria-disabled="true"` + `aria-checked="true"`; optional card has `role="checkbox"` + keyboard toggle (Space key); service status communicated via ARIA label (not color alone)
- [ ] T085 [P] Verify no custom CSS bypassing design tokens in consent components

### Polish

- [X] T086 Run `just check` (fmt → vet → lint) and `just verify` — all checks MUST pass with zero issues
- [ ] T087 [P] Verify `PermissionSetService` cache TTL (covered by T021a unit tests): confirm cache re-fetches after TTL expiry and that `TokenExchangeService` and `ConsentService` both benefit from it transparently
- [ ] T088 [P] Verify `PermissionSetService` background eviction goroutine exits cleanly on server shutdown (no goroutine leak) — tested in T021a; confirm in E2E shutdown scenario
- [ ] T089 Run `just test` — no race conditions in `psCache` sync.Map operations or memory repository
- [ ] T090 Complete `quickstart.md` verification checklist — all items checked before opening PR
- [X] T091 Add `require_all_scopes` ServiceRequirement mode: validate mutual exclusion with `required_scopes`, resolve the granted Permission Set union at token exchange, disclose the assigned Permission Set union in consent service requirements, and cover the admin, domain, handler, and end-user contract paths.

---

## Dependencies & Execution Order

### Phase Dependencies

```
Phase 1 (Setup)
  └── Phase 2 (Design Preconditions) — BLOCKS ALL
        ├── 2a Domain Model (parallel)
        ├── 2b Configuration (parallel)
        ├── 2c API Design (parallel)
        ├── 2d Database Design (parallel)
        ├── 2e Frontend/Design (parallel)
        └── 2f E2E Tests RED Phase (parallel)
              └── Phase 2.5 (Foundational) — BLOCKS ALL USER STORIES
                    ├── Phase 3: US1 Admin CRUD (P1) ──────────────── MVP
                    ├── Phase 4: US2 Consent Screen (P1) ──────────── MVP (parallel with US1)
                    ├── Phase 5: US3 Agent Declaration (P2)
                    └── Phase 6: US4 Grant + Token Exchange (P2)
                          └── Phase N: Constitution Compliance & Polish
```

### User Story Dependencies

| Story | Depends On | Can Parallelize With |
|-------|-----------|----------------------|
| US1 (P1) | Phase 2 + Phase 2.5 | US2 |
| US2 (P1) | Phase 2 + Phase 2.5 + T021b (PermissionSetService) + T036 (consent service wired) | US1 |
| US3 (P2) | US1 (PermissionSetRepository must exist for ID validation) | — |
| US4 (P2) | US1 (PermissionSetRepository) + Phase 2.5 T016/T020 (UserGrant schema) | — |

### Within Each User Story

- Tests MUST be written and fail BEFORE implementation (Principle VIII)
- Storage adapters (T024/T025) before factories (T026/T027) before handler (T029)
- Domain service extension (T036) before HTTP handler extension (T037)
- React components (T038/T039) before ConsentScreen restructure (T040)

### Parallel Opportunities per Story

**US1**: T022 (unit tests) + T023 (integration tests) + T024 (memory adapter) + T025 (postgres adapter) can all start in parallel; T021a + T021b (PermissionSetService) must complete before T029
**US2**: T035 (consent service tests) + T038 (PermissionSetCard) + T039 (PermissionSetsList) can start in parallel after T036
**US3**: T043 (tests) runs before T044 (implementation)
**US4**: T046 (token exchange tests) + T047 (grant handler) + T048/T049 (storage adapters) can start in parallel

---

## Parallel Example: User Story 1

```bash
# After Phase 2.5 complete, launch in parallel:
Task T022: "Write unit tests for PermissionSetsHandler in permission_sets_handler_test.go"
Task T023: "Write integration tests for postgres PermissionSetRepository"
Task T024: "Create memory permission_set_repository.go"
Task T025: "Create postgres permission_set_repository.go"
# Then sequentially:
Task T026 → T027 → T028: "Extend storage factories"
Task T029: "Create permission_sets_handler.go"
Task T030 → T031 → T032: "Wire handler into app and routes"
```

---

## Implementation Strategy

### MVP First (US1 + US2 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Design Preconditions (2a–2f must all complete before 2.5)
3. Complete Phase 2.5: Foundational Infrastructure
4. Complete Phase 3 (US1) + Phase 4 (US2) in parallel
5. **STOP and VALIDATE**: E2E US1 + US2 scenarios pass, Playwright screenshots captured
6. Demo / deploy MVP

### Incremental Delivery

1. Setup → Design Preconditions → Foundational → **Foundation ready**
2. US1 + US2 → **MVP: admin manages PSets, consent screen updated**
3. US3 → **Agents declare PSets, consent-info resolved**
4. US4 → **Full flow: grants store PSets, token exchange validates scopes**
5. Constitution Compliance & Polish → **PR ready**

### Suggested Agent Assignments

- **golang-pro**: Phase 2.5 + US1 (storage adapters, admin handler) + US4 (token exchange cache)
- **react-specialist**: US2 frontend (PermissionSetCard, PermissionSetsList, ConsentScreen restructure)
- **typescript-pro**: US2 frontend tests + Vitest unit tests for React components
- **general-purpose**: Phase 2 verification tasks (T003–T012), constitution compliance (T053–T090)

---

## Notes

- `[P]` = task touches different files from other [P] tasks in same batch — safe to parallelize
- `[USN]` label maps task to specific user story for traceability
- All 24 E2E tests must be in red phase before any implementation task in Phase 3+ begins
- `just check` and `just verify` must pass after every logical group of tasks
- Commit after each completed phase or user story for clean git history
- The `delegated_oauth2_tokens` JSONB column is **removed** — search entire codebase for references before merge
- Recheck: `internal/domain/` import graph must remain clean (no adapter imports) after all changes
