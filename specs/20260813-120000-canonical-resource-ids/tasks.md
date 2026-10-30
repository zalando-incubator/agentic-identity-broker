# Tasks: Canonical Resource IDs

**Input**: Design documents from `/specs/20260813-120000-canonical-resource-ids/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `quickstart.md`, and `contracts/admin-canonical-ids.md`

**Tests**: Mandatory. Follow the red-green-refactor cycle: every new test must compile and fail semantically before the production behavior it covers is implemented.

**Organization**: Tasks are grouped by user story so that each increment has an independently executable HTTP acceptance check.

## Phase 1: Setup

**Purpose**: Establish the exact feature change set and test entry points without introducing a competing identifier model.

- [X] T001 Confirm migration `029_add_canonical_ids` is sequential and the ten-scenario E2E mapping agrees across `specs/20260813-120000-canonical-resource-ids/plan.md`, `specs/20260813-120000-canonical-resource-ids/data-model.md`, and `migrations/`

---

## Phase 2: Design Preconditions

**Purpose**: Complete the constitution-required domain, API, persistence, and acceptance-test design before behavior changes begin.

### Phase 2a: Domain Model & Glossary

- [X] T002 Add Canonical ID and Stable Identifier glossary definitions and amend affected resource definitions in `ARCHITECTURE.md`
- [X] T003 Record the canonical-ID invariants, UUID-only persistence rule, and per-resource uniqueness decision in `specs/20260813-120000-canonical-resource-ids/data-model.md`

### Phase 2b: Configuration Design

- [X] T004 Verify no runtime configuration, configuration examples, or Helm values change by tracing `internal/ports/config.go` and `examples/config/README.md`

### Phase 2c: API Design

- [X] T005 Apply the confirmed canonical-ID contract to paths, requests, responses, headers, examples, and errors in `api/admin/openapi.yaml`

### Phase 2d: Database Design

- [X] T006 Write semantically failing real-PostgreSQL migration apply/rollback tests in `internal/adapters/storage/postgres/agent_repository_test.go`, `internal/adapters/storage/postgres/permission_set_repository_test.go`, and `internal/adapters/storage/postgres/thirdparty_provider_test.go`, then define reversible nullable canonical-ID columns and type-scoped partial unique indexes in `migrations/029_add_canonical_ids.up.sql` and `migrations/029_add_canonical_ids.down.sql`

### Phase 2f: E2E Acceptance Test Design

- [X] T007 Write ten production-bootstrap Ginkgo acceptance tests with one `It` and scenario comment per scenario in `tests/e2e/canonical_resource_ids_test.go`
- [X] T008 Run `ginkgo -v ./tests/e2e/canonical_resource_ids_test.go` and confirm `tests/e2e/canonical_resource_ids_test.go` fails semantically without skipped or placeholder assertions

**Checkpoint**: The API contract is confirmed in `spec.md`; the glossary, migration design, and all ten E2E scenarios are specified before implementation.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Add the shared validation, storage, and lookup capabilities required by every user story while retaining UUID-backed typed IDs and UUID-only stored relationships.

- [X] T009 Write table-driven validation tests for absent, empty, UUID-shaped, invalid-character, 128-character valid, and 129-character invalid canonical IDs in `internal/domain/storage/canonical_id_test.go`
- [X] T010 Implement the optional canonical-ID value validation used by all managed resources in `internal/domain/storage/canonical_id.go`
- [X] T011 Write red copy and validation tests for canonical metadata in `internal/domain/storage/agent_test.go`, `internal/domain/storage/permission_set_test.go`, and `internal/domain/model/thirdparty_oauth2_provider_test.go`, then extend Agent, PermissionSet, and ThirdpartyOAuth2ProviderEntity metadata and copy/validation behavior in their corresponding source files
- [X] T012 Extend type-scoped canonical single and batch lookup contracts without weakening ISP in `internal/ports/storage.go` and `internal/ports/thirdparty_provider.go`
- [X] T013 Prove in-memory canonical lookup, same-type conflict, update replacement/removal, delete reuse, and write-lock atomicity in `internal/adapters/storage/memory/agent_repository_test.go`, `internal/adapters/storage/memory/permission_set_repository_test.go`, and `internal/adapters/storage/memory/thirdparty_provider_test.go`
- [X] T014 Prove PostgreSQL partial-index conflict mapping, canonical lookup, cross-type reuse, and UUID relationship persistence in `internal/adapters/storage/postgres/agent_repository_test.go`, `internal/adapters/storage/postgres/permission_set_repository_test.go`, and `internal/adapters/storage/postgres/thirdparty_provider_test.go`
- [X] T015 Add atomic canonical indexes, collision checks, lookup, replacement, removal, and deletion release to `internal/adapters/storage/memory/agent_repository.go`, `internal/adapters/storage/memory/permission_set_repository.go`, and `internal/adapters/storage/memory/thirdparty_provider.go`
- [X] T016 Persist canonical metadata, partial-index conflict mapping, canonical lookups, and batch canonical reads in `internal/adapters/storage/postgres/agent_repository.go`, `internal/adapters/storage/postgres/permission_set_repository.go`, and `internal/adapters/storage/postgres/thirdparty_provider.go`

**Checkpoint**: All repositories provide matching, type-scoped canonical behavior; canonical text never enters UUID typed fields, foreign keys, JSONB references, or encryption contexts.

---

## Phase 3: User Story 1 - Manage Resources by Canonical ID (Priority: P1) MVP

**Goal**: An administrator can create resources with optional canonical IDs and operate on every direct in-scope resource path using either its canonical ID or UUID.

**Independent Test**: Create each managed resource with a canonical ID, then GET, PUT, and DELETE it via canonical and UUID paths; create one without a canonical ID and verify UUID-only management remains unchanged.

### Tests for User Story 1

- [X] T017 [P] [US1] Add canonical create/update validation, duplicate-conflict, UUID fallback, and direct-path handler tests in `internal/adapters/http/handlers/admin/agents_handler_test.go`, `internal/adapters/http/handlers/admin/services_handler_test.go`, and `internal/adapters/http/handlers/admin/permission_sets_handler_test.go`
- [X] T018 [P] [US1] Add canonical agent-path resolution tests for credential generation, metadata retrieval, and revocation in `internal/adapters/http/handlers/admin/client_credentials_handler_test.go`
- [X] T019 [P] [US1] Add UUID-first, canonical-fallback, and unresolved identifier resolver tests in `internal/domain/agents/service_test.go`, `internal/domain/thirdparty/service_test.go`, and `internal/domain/permissionset/service_test.go`

### Implementation for User Story 1

- [X] T020 [US1] Add UUID-first, type-scoped canonical fallback resolver methods to `internal/domain/agents/service.go`, `internal/domain/thirdparty/service.go`, and `internal/domain/permissionset/service.go`
- [X] T021 [US1] Preserve create/update canonical-ID presence semantics and map invalid and duplicate values to 400 and 409 in `internal/adapters/http/handlers/admin/agents_handler.go`, `internal/adapters/http/handlers/admin/services_handler.go`, and `internal/adapters/http/handlers/admin/permission_sets_handler.go`
- [X] T022 [US1] Resolve canonical or UUID direct agent, service, permission-set, and service-filter identifiers after authentication and before the domain call in `internal/adapters/http/handlers/admin/agents_handler.go`, `internal/adapters/http/handlers/admin/services_handler.go`, and `internal/adapters/http/handlers/admin/permission_sets_handler.go`
- [X] T023 [US1] Inject agent identifier resolution into client-credential operations while retaining UUID credential IDs in `internal/adapters/http/handlers/admin/client_credentials_handler.go` and `internal/app/builder.go`

**Checkpoint**: User Story 1 passes its direct-path acceptance scenarios and preserves UUID-only clients.

---

## Phase 4: User Story 2 - Reference Resources by Either Identifier (Priority: P1)

**Goal**: Every supported agent and permission-set write reference accepts a target UUID or canonical ID but persists only the resolved UUID.

**Independent Test**: Submit each supported relationship once with the target canonical ID and once with its UUID, then prove both stored relationships target the same UUID; submit an unresolved target and observe no partial update.

### Tests for User Story 2

- [X] T024 [P] [US2] Add service-requirement and permission-set reference-resolution tests covering UUID, canonical, wrong type, and no-partial-update cases in `internal/adapters/http/handlers/admin/agents_handler_test.go`
- [X] T025 [P] [US2] Add permission-set service-scope UUID/canonical resolution and unresolved-reference tests in `internal/adapters/http/handlers/admin/permission_sets_handler_test.go`
- [X] T026 [P] [US2] Add transaction-safe service-target and foreign-key reference validation integration coverage in `internal/adapters/storage/postgres/agent_repository_test.go` and `internal/adapters/storage/postgres/permission_set_repository_test.go`

### Implementation for User Story 2

- [X] T027 [US2] Resolve `service_requirements[].service_id` to a ServiceID before agent validation and preserve all-or-nothing writes in `internal/domain/agents/service.go` and `internal/adapters/http/handlers/admin/agents_handler.go`
- [X] T028 [US2] Resolve `permission_sets[].permission_set_id` to a PermissionSetID before agent validation and preserve all-or-nothing writes in `internal/domain/agents/service.go` and `internal/adapters/http/handlers/admin/agents_handler.go`
- [X] T029 [US2] Resolve `service_scopes[].service_id` to a ServiceID before permission-set validation and persistence in `internal/domain/permissionset/service.go` and `internal/adapters/http/handlers/admin/permission_sets_handler.go`
- [X] T030 [US2] Revalidate resolved service references inside the repository mutation transaction so a concurrent deletion cannot leave JSONB or relational dangling references in `internal/adapters/storage/postgres/agent_repository.go` and `internal/adapters/storage/postgres/permission_set_repository.go`

**Checkpoint**: User Story 2 accepts UUID and canonical references identically and rejects an unresolved reference without mutation.

---

## Phase 5: User Story 3 - Receive Readable Resource Representations (Priority: P2)

**Goal**: Reads expose nullable top-level `canonical_id` while default nested references remain UUIDs; an honored canonical preference changes only nested reference presentation and response headers.

**Independent Test**: Read and list each resource with and without canonical IDs, then repeat relationship reads with `Prefer: reference-id=canonical` and verify only nested references with a canonical target change, plus `Preference-Applied` and `Vary`.

### Tests for User Story 3

- [X] T031 [US3] Add default stable UUID and nullable top-level canonical-ID read/list response tests in `internal/adapters/http/handlers/admin/agents_handler_test.go`, `internal/adapters/http/handlers/admin/services_handler_test.go`, and `internal/adapters/http/handlers/admin/permission_sets_handler_test.go`
- [X] T032 [P] [US3] Add table-driven absent, `reference-id=uuid`, `reference-id=canonical`, and unrecognized Prefer-header parser tests in `internal/adapters/http/handlers/admin/canonical_presentation_test.go`
- [X] T033 [US3] Add `reference-id=canonical` nested rendering, fallback UUID, `Preference-Applied: reference-id=canonical`, and `Vary: Prefer` response tests in `internal/adapters/http/handlers/admin/agents_handler_test.go` and `internal/adapters/http/handlers/admin/permission_sets_handler_test.go`

### Implementation for User Story 3

- [X] T034 [US3] Add nullable `canonical_id` to all top-level agent, service, and permission-set response DTOs and conversion functions in `internal/adapters/http/handlers/admin/agents_handler.go`, `internal/adapters/http/handlers/admin/services_handler.go`, and `internal/adapters/http/handlers/admin/permission_sets_handler.go`
- [X] T035 [US3] Parse only `Prefer: reference-id=canonical` and consistently apply `Vary: Prefer` to all in-scope list and single-resource reads in `internal/adapters/http/handlers/admin/canonical_presentation.go`
- [X] T036 [US3] Batch-load canonical metadata and render agent nested service and permission-set references canonically only for an honored `reference-id=canonical` preference in `internal/adapters/http/handlers/admin/agents_handler.go`
- [X] T037 [US3] Batch-load canonical metadata and render permission-set service-scope references canonically only for an honored `reference-id=canonical` preference in `internal/adapters/http/handlers/admin/permission_sets_handler.go`

**Checkpoint**: User Story 3 keeps every top-level `id` stable as UUID, retains the pre-feature nested UUID default, and produces cache-safe opt-in canonical presentation.

---

## Phase 6: Constitution Compliance & Polish

**Purpose**: Verify API, storage, security, TDD, and E2E commitments against the implemented feature.

- [X] T038 Verify `api/admin/openapi.yaml` exactly matches the confirmed contract in `specs/20260813-120000-canonical-resource-ids/contracts/admin-canonical-ids.md`
- [X] T039 Verify Canonical ID and Stable Identifier definitions and affected entity glossary entries in `ARCHITECTURE.md`
- [X] T040 Verify `migrations/029_add_canonical_ids.up.sql` and `migrations/029_add_canonical_ids.down.sql` are sequential, reversible, and leave existing UUID data intact
- [X] T041 Run focused memory and PostgreSQL repository tests covering `internal/adapters/storage/memory/` and `internal/adapters/storage/postgres/`
- [X] T042 Run `ginkgo -v ./tests/e2e/canonical_resource_ids_test.go` and verify every scenario comment in `tests/e2e/canonical_resource_ids_test.go` maps to exactly one `spec.md` acceptance scenario
- [X] T043 Run the feature's PostgreSQL integration suite for `internal/adapters/storage/postgres/` and verify migration rollback, uniqueness, conflict, and transactional-reference behavior
- [X] T044 Run `just check` from `justfile` and resolve format, vet, and lint failures without suppressing diagnostics
- [X] T045 Run the validation sequence in `specs/20260813-120000-canonical-resource-ids/quickstart.md`

---

## Dependencies & Execution Order

```text
Phase 1 → Phase 2 → Phase 2.5 → {US1, US2, US3} → Phase 6
                                 US1 → US2 → US3 (recommended incremental order)
```

- **Phase 1** establishes the migration number and file map.
- **Phase 2** blocks behavior changes: glossary, confirmed OpenAPI contract, reversible migration design, and semantically failing acceptance coverage are required first.
- **Phase 2.5** blocks every story: it adds the shared canonical validation, optional metadata, repository contracts, matching adapters, and storage tests.
- **US1** needs Phase 2.5. It delivers direct canonical addressing and is the MVP.
- **US2** needs Phase 2.5 and the resolver methods introduced in US1; it can be developed in parallel only if those resolver methods are committed first.
- **US3** needs Phase 2.5 and can begin after response DTO metadata is available; recommended after US1 and US2 because it renders their resolved relationships.
- **Phase 6** begins after all desired user stories are green.
- **Phase 8** reuses the US1 `ThirdpartyOAuth2ProviderService.ResolveID` resolver to extend canonical addressing to the service protected-resource sub-resource paths; it depends only on that resolver and may run after US1.

## Parallel Execution Examples

### User Story 1

```text
Task: "T017 Add direct-path handler tests in internal/adapters/http/handlers/admin/*_handler_test.go"
Task: "T018 Add client-credential path tests in internal/adapters/http/handlers/admin/client_credentials_handler_test.go"
Task: "T019 Add domain resolver tests in internal/domain/{agents,thirdparty,permissionset}/service_test.go"
```

### User Story 2

```text
Task: "T024 Add agent relationship resolver tests in internal/adapters/http/handlers/admin/agents_handler_test.go"
Task: "T025 Add permission-set service-scope tests in internal/adapters/http/handlers/admin/permission_sets_handler_test.go"
Task: "T026 Add PostgreSQL transaction tests in internal/adapters/storage/postgres/*_repository_test.go"
```

### User Story 3

```text
Task: "T031 Add default representation tests in internal/adapters/http/handlers/admin/*_handler_test.go"
Task: "T032 Add Prefer parser tests in internal/adapters/http/handlers/admin/canonical_presentation_test.go"
Task: "T034 Add top-level response metadata in internal/adapters/http/handlers/admin/{agents,services,permission_sets}_handler.go"
```

## Implementation Strategy

### MVP First

1. Complete Phases 1, 2, and 2.5, including semantic red-phase proof.
2. Complete US1 and run its direct-path acceptance scenarios. This is the MVP: optional canonical IDs and direct canonical resource management.
3. Add US2 to accept canonical write references while storing resolved UUIDs.
4. Add US3 for readable, cache-safe response representations.
5. Complete Phase 6 only after the targeted tests, E2E suite, PostgreSQL integration suite, and `just check` pass.

### Incremental Delivery

- **US1**: Canonical resource lifecycle with UUID compatibility.
- **US2**: Canonical relationship input with UUID-only persistence.
- **US3**: Canonical metadata reads and opt-in nested-reference presentation.

## Notes

- `[P]` means different files with no incomplete task dependency; schedule those tasks concurrently only when their listed prerequisite phase is complete.
- Story labels provide spec traceability. Setup, design preconditions, foundations, and compliance deliberately have no story label.
- No frontend tasks exist: the contract and plan establish an admin API-only feature with no admin SPA.
- No configuration or Helm tasks exist beyond verification because this feature adds no runtime configuration.

## Phase 7: Convergence

- [X] T046 CRITICAL Document the `Prefer: reference-id=canonical`, `Preference-Applied`, and `Vary: Prefer` contract in `api/admin/openapi.yaml` per Constitution IV (missing)
- [X] T047 CRITICAL Add ten production-bootstrap Ginkgo acceptance scenarios with 1:1 `spec.md` traceability in `tests/e2e/canonical_resource_ids_test.go` per Constitution XIII (missing)
- [X] T048 CRITICAL Add real-PostgreSQL migration apply/rollback and canonical-index integrity coverage for migration `029` per Constitution IX (missing)
- [X] T049 Map `uq_agents_canonical_id` conflicts to `StorageError` conflict results in PostgreSQL agent create and update operations per FR-004 (partial)
- [X] T050 Propagate explicit `canonical_id: null` clear state through agent update validation and both storage adapters, with a regression test, per FR-012 (partial)
- [X] T051 Honor `Prefer: reference-id=canonical` by batch-loading nested target canonical IDs, preserving UUID fallbacks, and setting `Preference-Applied` on in-scope reads per FR-011 (missing)
- [X] T052 Revalidate agent service-requirement targets inside the PostgreSQL mutation transaction to prevent concurrent deletion from persisting dangling JSONB references per FR-007 (partial)
- [X] T053 Add focused canonical-ID model, repository, resolver, direct-path, cross-reference, and representation regression tests per plan: Testing Strategy (missing)
- [X] T054 Inject the agent identifier resolver into client-credential handling instead of performing optional repository type-assertion resolution per plan: resolution boundary (partial)

---

## Phase 8: Protected-Resource Path Canonical Coverage

**Purpose**: Close the FR-005 gap where the service protected-resource sub-resource paths resolve `{service-id}` UUID-only, contradicting the shared `ServiceId` OpenAPI parameter that already advertises canonical support. Cross-check confirmed every other in-scope admin handler resolves canonical IDs via `ResolveID`; only `ProtectedResourcesHandler.serviceID` still calls UUID-only `id.ParseServiceID`, so canonical service IDs return 400 on `/api/services/{service-id}/protected-resources` and `/api/services/{service-id}/protected-resources/{resource}` (List, Create, Add, Rename, Remove).

**Independent Test**: Address a canonical-ID service's protected-resource collection and member operations by that canonical ID and by its UUID, and confirm both resolve to the same service; address an unresolved canonical ID and confirm 404.

- [X] T055 [P] Add red tests proving canonical service-ID resolution for `List`, `Create`, `Add`, `Rename`, and `Remove`, plus a 404 for an unresolved canonical service ID, in `internal/adapters/http/handlers/admin/protected_resources_handler_test.go`
- [X] T056 Resolve the `service-id` path segment through `thirdparty.ThirdpartyOAuth2ProviderService.ResolveID` (UUID-first, type-scoped canonical fallback) instead of UUID-only `id.ParseServiceID`, mapping an unresolved identifier to 404 via the existing `storageError` helper, in `internal/adapters/http/handlers/admin/protected_resources_handler.go`
- [X] T057 Run `just check` and the focused `internal/adapters/http/handlers/admin/` handler suite to confirm canonical and UUID protected-resource addressing behave identically

**Checkpoint**: All five protected-resource operations accept a service UUID or canonical ID identically, satisfying FR-005 for the service protected-resource sub-resource paths.
