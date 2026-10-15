# Tasks: Protected Resource Subresources on Third-Party Services API

**Input**: Design documents from `/specs/035-protected-resource-subresources/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/protected-resources.openapi.yaml`, `quickstart.md`

**Tests**: Mandatory. Constitution Principle VIII requires tests to compile and fail semantically before implementation; Principle XIII requires one Ginkgo `It` per specified acceptance scenario.

**Organization**: Tasks are grouped by independently testable user story. The normalized child-table and optimistic-concurrency work is foundational because every endpoint and the legacy full-service path must share it.

## Phase 1: Setup

**Purpose**: Establish the implementation baseline and source-level contracts without adding a competing architecture.

- [X] T001 Review existing third-party provider, HTTP, and token-resolution behavior in `internal/domain/thirdparty/service.go`, `internal/ports/thirdparty_provider.go`, `internal/adapters/storage/{memory,postgres}/thirdparty_provider.go`, `internal/adapters/http/handlers/admin/services_handler.go`, and `internal/domain/tokenexchange/service.go` before modifying interfaces or call sites.
- [X] T002 Review the canonical admin route and production wiring patterns in `internal/adapters/http/routing/admin.go`, `internal/app/handlers.go`, and `internal/app/builder.go` before adding the protected-resource handler.

---

## Phase 2: Design Preconditions

**Purpose**: Complete and approve the binding API, domain, persistence, and acceptance-test design before implementation. This phase blocks all code work.

### Domain Model & Glossary

- [X] T003 Reconcile the `ProtectedResource` URI-identity, normalized-set, and `Version` aggregate invariants in `specs/035-protected-resource-subresources/data-model.md` with `internal/domain/model/thirdparty_oauth2_provider.go`.
- [X] T004 Update the protected-resources glossary and storage description for the child-table uniqueness and ETag version model in `ARCHITECTURE.md`.

### Configuration Design

- [x] T005 Verify and record that this feature adds no runtime configuration, configuration examples, or Helm values; preserve `internal/ports/config.go` unchanged. Evidence: the Configuration Design, Config Examples, and Helm Chart entries in `specs/035-protected-resource-subresources/plan.md`.

### API Design & Approval

- [X] T006 During implementation, merge the five `/api/services/{service-id}/protected-resources` operations (GET, body-based POST add, member-addressed PUT add, PATCH, DELETE), request/response schemas, ETag headers, and `400`/`404`/`409` statuses from `specs/035-protected-resource-subresources/contracts/protected-resources.openapi.yaml` into `api/admin/openapi.yaml`.
- [X] T007 During implementation, merge the full-service `PUT /api/services/{service-id}` contract delta—optional `protected_resources`, required `If-Match` only for replacement, and `412`/`428` responses—from `specs/035-protected-resource-subresources/contracts/protected-resources.openapi.yaml` into `api/admin/openapi.yaml`.
- [x] T008 Record confirmation from this stakeholder conversation for the body-based POST add operation, the retained member-addressed PUT add operation, and full-service `PUT` behavior change, including compatibility impact, in `specs/035-protected-resource-subresources/plan.md`.
- [X] T009 Document the normalized child-table, global-uniqueness, and optimistic-concurrency decision with Context, Decision, Consequences, and Proposed status in `adrs/030-normalize-protected-resources.md`.
- [X] T010 Create `docs/api/protected-resource-subresources.md` with representative admin API usage and ETag retry examples for PUT/POST add, remove, rename, list, and full-set replacement; link it to the canonical `api/admin/openapi.yaml` contract.

### Database Design

- [X] T011 Finalize the migration preflight, backfill, rollback, `resource_uri` primary key, `service_id` index, and `version` semantics in `specs/035-protected-resource-subresources/data-model.md` before authoring SQL.

### E2E Acceptance-Test Design
- [X] T012 Write compiling, semantically failing Ginkgo acceptance coverage for every US1–US4 scenario, including both PUT and body-based POST add, FR-013 add/remove token-exchange reflection, and fully percent-encoded single-segment URI addressing for member routes, preserving one `It` per spec scenario and spec-reference comments, in `tests/e2e/service_protected_resources_test.go`.
- [X] T013 Verify the red E2E suite has no pending markers and fails for missing behavior rather than compilation errors using `tests/e2e/service_protected_resources_test.go` with `ginkgo -v --focus="Protected Resource Subresources" ./tests/e2e`.

**Checkpoint**: Stakeholder API approval is recorded. Do not implement until the remaining Phase 2 OpenAPI, ADR, data-model, and red-acceptance-test tasks are complete.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Replace the legacy array with the single authoritative child table, establish adapter parity, and make legacy full updates safe before user-story endpoints are implemented.

- [X] T014 Write real-PostgreSQL migration tests for clean apply, canonicality and collision aborts, rollback, and apply→rollback→apply data preservation in `tests/integration/infra/service_protected_resources_migration_test.go`.
- [X] T015 Create the fail-closed schema migration with `version`, `service_protected_resources`, canonicality/collision preflights, backfill, legacy-array removal, FK cascade, and indexes in `migrations/028_normalize_service_protected_resources.up.sql`.
- [X] T016 Create the inverse migration restoring the legacy array and GIN index from child rows without data loss in `migrations/028_normalize_service_protected_resources.down.sql`.
- [X] T017 Register migration `028` in bounded PostgreSQL adapter migration setup and extend the generic migration lifecycle coverage in `internal/adapters/storage/postgres/testhelpers_test.go` and `tests/integration/migrations/migrations_test.go`.
- [X] T018 Write table-driven unit coverage for per-URI absolute validation, trailing-slash normalization, and version-bearing entity validation in `internal/domain/model/thirdparty_oauth2_provider_test.go`.
- [X] T019 Add `Version` and reusable single-resource validation/normalization support while preserving full-set validation parity in `internal/domain/model/thirdparty_oauth2_provider.go`.
- [X] T020 Define an atomic protected-resource mutation result that returns the resulting resource state and new version/ETag, then extend the focused provider repository contract with atomic add, remove, rename, list, and version-aware full update operations in `internal/ports/thirdparty_provider.go`.
- [X] T021 Update every provider-repository test double for the expanded port—including the domain, token-exchange, consent, and admin handler mocks—in `internal/domain/{thirdparty,consent,tokenexchange}/*_test.go`, `internal/adapters/http/handlers/admin/services_handler_test.go`, and `internal/adapters/http/handlers/consent/agents_handler_test.go`.
- [X] T022 Write adapter behavior tests covering normalized ownership, idempotent add, conflicts, missing resources, version changes, full-update replacement semantics, same-URI claim races, and different-resource concurrency in `internal/adapters/storage/memory/thirdparty_provider_test.go` and `internal/adapters/storage/postgres/thirdparty_provider_test.go`.
- [X] T023 Implement the in-memory child-set representation, `resourceOwners` index, atomic mutation-result/version returns, CAS parity, and child-backed reads/writes in `internal/adapters/storage/memory/thirdparty_provider.go` and `internal/adapters/storage/memory/thirdparty_provider_record.go`.
- [X] T024 Implement PostgreSQL child-table reads/writes, transaction boundaries, unique-violation-to-conflict mapping, atomic mutation-result/version returns, version CAS, and child-table resolution in `internal/adapters/storage/postgres/thirdparty_provider.go` and `internal/adapters/storage/postgres/thirdparty_provider_record.go`.
- [X] T025 Extend encrypted-entity copy and record conversion tests for version and child-resource materialization in `internal/adapters/storage/memory/thirdparty_provider_record_test.go` and `internal/adapters/storage/postgres/thirdparty_provider_record_test.go`.
- [X] T025a Write failing admin create-service compatibility tests proving a complete `protected_resources` set is accepted, normalized, persisted through the child-table representation, returned by subsequent reads, and resolvable by token exchange in `internal/adapters/http/handlers/admin/services_handler_test.go` and `tests/e2e/token_exchange_test.go`.
- [X] T026 Write failing update-service tests for omitted resources, missing `If-Match`, stale `If-Match`, a successful replacement, emitted ETag, and secret-field isolation in `internal/adapters/http/handlers/admin/services_handler_test.go`.
- [X] T027 Implement optional protected-resource decoding, strong ETag parsing/emission, and `428`/`412` mapping for replacement PUTs in `internal/adapters/http/handlers/admin/services_handler.go`.
- [X] T028 Thread `expectedVersion` through the provider domain-service update path without allowing handlers to bypass the domain layer in `internal/domain/thirdparty/service.go`.
- [X] T029 Update authoritative resolver tests for child-table lookup and global uniqueness, including newly added resources resolving and removed resources failing new resolution, in `internal/domain/tokenexchange/service_test.go` and `tests/e2e/token_exchange_test.go`.
- [X] T030 Add the pre-wired protected-resource handler slot without constructing services in routing in `internal/app/handlers.go` and `internal/app/builder.go`.

**Checkpoint**: Migration, both adapters, the legacy full-service path, and authoritative token resolution converge on the child table; the stakeholder-confirmed API contract is enforced before sub-resource routes ship.

---

## Phase 3: User Story 1 — Add a Single Protected Resource (Priority: P1) 🎯 MVP

**Goal**: An administrator atomically adds one normalized, globally unique resource without sending or changing unrelated service fields.

**Independent Test**: Create a service, add one resource through both the body-based POST and percent-encoded-URI PUT operations, then GET the service and confirm exactly the normalized resource changed; independently validate idempotency, validation, conflict, and missing-service responses.

- [X] T031 [P] [US1] Add table-driven domain-service tests for valid add, normalized idempotent add, malformed URI, cross-service conflict, missing service, returned mutation version, and full mutation result in `internal/domain/thirdparty/service_test.go`.
- [X] T032 [P] [US1] Add HTTP handler tests for body-based `resource_uri` POST and percent-encoded-URI PUT add status/response-body/ETag behavior; POST missing-body, malformed-body, and malformed `resource_uri` rejection; identical idempotency/conflict semantics; client-secret isolation; required authenticated administrator; fully percent-encoded single-segment URI PUT addressing through `RawPath`/escaped-path routing (including `%2F`, `%3F`, `%23`, `%25`, and malformed escapes); and structured audit records for successful and rejected add requests, including `resource_uri` after successful validation and a null `resource_uri` with no raw input logged after validation failure, in `internal/adapters/http/handlers/admin/protected_resources_handler_test.go`.
- [X] T033 [US1] Implement domain-owned add validation, normalization, repository orchestration, and atomic mutation-result assembly in `internal/domain/thirdparty/service.go`.
- [X] T034 [US1] Implement body-based `resource_uri` POST add and retained PUT add. PUT extracts the escaped path from `RawPath` or equivalent escaped routing data that preserves exactly one percent-encoded resource segment, decodes it once before normalization, and rejects malformed escapes; POST decodes `resource_uri` from the JSON body. Both use the same domain operation, translate HTTP errors, return an ETag from the atomic mutation result, enforce the required principal, and emit a structured non-secret audit event with `resource_uri` only after successful validation (null with no raw input otherwise) in `internal/adapters/http/handlers/admin/protected_resources_handler.go`.
- [X] T035 [US1] Register the body-based POST collection route (`/api/services/{service-id}/protected-resources`) and retained member-addressed PUT route (`/api/services/{service-id}/protected-resources/{resource}`) using the pre-wired handler in `internal/adapters/http/routing/admin.go`.
- [X] T036 [US1] Run the US1 acceptance scenarios for both POST and retained PUT adds, including a newly added resource becoming resolvable to a new token-exchange request, and preserve their explicit spec traceability in `tests/e2e/service_protected_resources_test.go` with `ginkgo -v --focus="adds a new resource|POST|PUT|newly added resource|idempotent|cross-service|malformed|non-existent" ./tests/e2e`.

**Checkpoint**: US1 satisfies FR-001, FR-005–FR-008, FR-010–FR-014, FR-016–FR-017, and SC-004 independently.

---

## Phase 4: User Story 2 — Remove a Single Protected Resource (Priority: P1)

**Goal**: An administrator removes exactly one owned resource without replacing the set or altering unrelated service fields.

**Independent Test**: Add a resource, DELETE it by its percent-encoded-URI path segment, confirm it disappears and no longer resolves for a new token exchange while remaining fields stay unchanged.

- [X] T037 [P] [US2] Add domain-service tests for normalized removal, absent resource, missing service, last-resource removal, returned mutation version, and state in `internal/domain/thirdparty/service_test.go`.
- [X] T038 [P] [US2] Add HTTP handler tests for percent-encoded single-segment URI DELETE through `RawPath`/escaped-path routing (including `%2F`, `%3F`, `%23`, `%25`, and malformed escapes), `204` ETag responses, validation errors, `404` mapping, required authenticated administrator, and structured audit records for successful and rejected remove requests, including `resource_uri` after successful validation and a null `resource_uri` with no raw input logged after validation failure, in `internal/adapters/http/handlers/admin/protected_resources_handler_test.go`.
- [X] T039 [US2] Implement domain-owned remove validation, normalization, and repository orchestration in `internal/domain/thirdparty/service.go`.
- [X] T040 [US2] Implement DELETE escaped-path extraction from `RawPath` or equivalent escaped routing data that preserves exactly one percent-encoded resource segment, decodes it once before normalization, rejects malformed escapes, returns `204` with an ETag from the atomic mutation result, enforces the required principal, emits a structured non-secret audit event with `resource_uri` only after successful validation (null with no raw input otherwise), and maps domain errors in `internal/adapters/http/handlers/admin/protected_resources_handler.go`.
- [X] T041 [US2] Register the DELETE protected-resource route (`/api/services/{service-id}/protected-resources/{resource}`) in `internal/adapters/http/routing/admin.go`.
- [X] T042 [US2] Verify removal, empty-set validity, and immediate new-exchange resolution failure in `tests/e2e/service_protected_resources_test.go` and `tests/e2e/token_exchange_test.go` with `ginkgo -v --focus="removes an owned resource|removing a URI|last resource|stops new token-exchange" ./tests/e2e`.

**Checkpoint**: US2 satisfies FR-002, FR-005, FR-006, FR-009–FR-014, FR-016–FR-017 independently.

---

## Phase 5: User Story 3 — Modify a Protected Resource (Priority: P2)

**Goal**: An administrator atomically renames one owned URI with global conflict protection and no delete-then-add round trip.

**Independent Test**: Rename A to normalized B; assert A is absent and B is present, then independently cover target conflicts, missing source, and same-URI no-op behavior.

- [X] T043 [P] [US3] Add domain-service tests for rename success, normalization, owned-target conflict, missing source, same-URI no-op, and returned mutation version/state in `internal/domain/thirdparty/service_test.go`.
- [X] T044 [P] [US3] Add HTTP handler tests for PATCH rename (source URI from a fully percent-encoded single path segment through `RawPath`/escaped-path routing, including `%2F`, `%3F`, `%23`, `%25`, and malformed escapes; target from body) validation, result body, ETag, `404` and `409` mapping, required authenticated administrator, and structured audit records for successful and rejected rename requests, including `source_resource_uri` and `target_resource_uri` after validation and null field(s) with no raw input logged after validation failure, in `internal/adapters/http/handlers/admin/protected_resources_handler_test.go`.
- [X] T045 [US3] Implement domain-owned rename validation, normalization, no-op semantics, and repository orchestration in `internal/domain/thirdparty/service.go`.
- [X] T046 [US3] Implement PATCH rename escaped-path extraction from `RawPath` or equivalent escaped routing data that preserves exactly one percent-encoded source segment, decodes it once before normalization, rejects malformed escapes, decodes the target from the body, returns the mutation result and ETag, enforces the required principal, emits structured non-secret audit data with independently normalized `source_resource_uri` and `target_resource_uri` only after validation (null with no raw input otherwise), and translates domain errors in `internal/adapters/http/handlers/admin/protected_resources_handler.go`.
- [X] T047 [US3] Register the PATCH rename protected-resource route (`/api/services/{service-id}/protected-resources/{resource}`) in `internal/adapters/http/routing/admin.go`.
- [X] T048 [US3] Run the rename acceptance scenarios with their spec-reference comments in `tests/e2e/service_protected_resources_test.go` using `ginkgo -v --focus="renames a resource|renaming to|renaming a resource not|same URI" ./tests/e2e`.

**Checkpoint**: US3 satisfies FR-003, FR-005–FR-007, FR-009–FR-014, FR-016–FR-017 independently.

---

## Phase 6: User Story 4 — Retrieve a Service's Protected Resources (Priority: P3)

**Goal**: An administrator reads just the normalized resource set and its current ETag without retrieving credentials or the full service record.

**Independent Test**: Create a service with known resources, GET the sub-collection, and compare the response set and ETag; verify `404` for a missing service.

- [X] T049 [P] [US4] Add domain-service tests for list success, normalized resource results, current version, and missing service in `internal/domain/thirdparty/service_test.go`.
- [X] T050 [P] [US4] Add HTTP handler tests for GET response schema, ETag, and missing-service mapping in `internal/adapters/http/handlers/admin/protected_resources_handler_test.go`.
- [X] T051 [US4] Implement domain-owned protected-resource listing and current-version retrieval in `internal/domain/thirdparty/service.go`.
- [X] T052 [US4] Implement GET response formatting for `ProtectedResourceSet`, ETag emission, and error translation in `internal/adapters/http/handlers/admin/protected_resources_handler.go`.
- [X] T053 [US4] Register the GET protected-resource route in `internal/adapters/http/routing/admin.go`.
- [X] T054 [US4] Run the list and missing-service acceptance scenarios with their spec-reference comments in `tests/e2e/service_protected_resources_test.go` using `ginkgo -v --focus="returns all normalized resource URIs|listing resources" ./tests/e2e`.

**Checkpoint**: US4 satisfies FR-004, FR-010, FR-014, and FR-016 independently.

---

## Phase N: Constitution Compliance Verification

**Purpose**: Verify Phase 2 design preconditions and implemented behavior against every applicable binding constitution principle.

### Design Phase Verification

- [ ] T055 Verify all Phase 2 preconditions are complete: domain-model and glossary reconciliation; documented no-configuration decision; merged, stakeholder-confirmed OpenAPI contract; ADR; migration design; and compiling, semantically-red E2E scenarios. (Principles II, IV, VII, IX, X, XIII)

### Implementation Phase Verification

#### Architecture, API, and Domain Documentation

- [ ] T056 Verify implemented operations, schemas, ETag headers, and `400`/`404`/`409`/`412`/`428` responses exactly match the confirmed OpenAPI contract. (Principles IV, X)
- [ ] T057 Verify `ARCHITECTURE.md`, `adrs/030-normalize-protected-resources.md`, and `docs/api/third-party-services.md` consistently describe the shipped child-table and compatibility behavior. (Principles II, V)

#### Security, Configuration, and Architecture Boundaries

- [ ] T058 Verify security/audit controls remain enabled, no custom cryptography is introduced, and no configuration, Helm, frontend, or encryption changes were introduced beyond documented non-impacts. (Principles I, III, VII, XI)
- [ ] T059 Verify handlers, domain services, repository ports, adapters, Builder wiring, and route registration retain hexagonal boundaries and dependency-injection rules. (Principles VI, XII)

#### Automated Testing and Persistence

- [ ] T060 Run static checks for changed Go, migration, and documentation artifacts using `just check`. (Principle VIII)
- [ ] T061 Run unit and package tests for changed domain, handler, and storage packages using `just test`. (Principle VIII)
- [ ] T062 Run real-PostgreSQL migration, repository parity, uniqueness-race, and full-PUT CAS verification using `just test-integration-infra`. (Principle IX)
- [ ] T063 Run the complete backend E2E acceptance suite using `ginkgo -v ./tests/e2e`. (Principle XIII)
- [ ] T064 Verify every protected-resource E2E `It` maps 1:1 to a spec scenario, was introduced semantically red before implementation, has no pending marker, and runs through production DI bootstrap. (Principles VIII, XIII)
- [ ] T065 Run the quickstart validation for add/remove/rename/list, stale ETag rejection, missing-If-Match rejection, omitted-resource PUT behavior, and full-service create compatibility. (Principles IV, VIII, IX, X, XIII)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1** has no dependencies.
- **Phase 2** depends on Phase 1. Stakeholder confirmation T008 is complete; every remaining Phase 2 precondition still blocks implementation.
- **Phase 2.5** depends on every Phase 2 task. It blocks every user story because global uniqueness, child-table authority, adapter parity, and full-PUT convergence are shared invariants.
- **US1–US4** all depend on Phase 2.5. They can be assigned in parallel only after shared domain/repository/handler wiring is stable; shared-file edits must be coordinated.
- **Phase N** depends on all desired user stories.

### User Story Dependencies

```text
Phase 1 → Phase 2 → Phase 2.5
                                                   ├─→ US1 Add (P1)
                                                   ├─→ US2 Remove (P1)
                                                   ├─→ US3 Rename (P2)
                                                   └─→ US4 List (P3)
US1 + US2 + US3 + US4 → Phase N
```

- **US1**: No dependency on another user story after foundational work; MVP candidate.
- **US2**: No functional dependency on US1; its independent test may use setup through the existing full-service create API.
- **US3**: No functional dependency on US1 or US2; its independent test may use setup through the existing full-service create API.
- **US4**: No functional dependency on mutation endpoints; it reads the foundation's authoritative child set.

### Parallel Opportunities

- T006 and T009–T012 operate on separate artifacts after T003–T005; T008 is complete.
- T014 and T017 can proceed in parallel before their corresponding migration/model implementations.
- T021 and T022 can proceed in parallel once T019–T020 define the repository contract.
- Per story, the domain and handler test tasks are parallel only while they touch distinct files: T031/T032, T037/T038, T043/T044, and T049/T050.
- US story teams can prepare tests in parallel after Phase 2.5, but the shared `internal/domain/thirdparty/service.go`, `protected_resources_handler.go`, and `routing/admin.go` changes must be serialized or coordinated.

## Parallel Execution Examples

### User Story 1

```text
Task: T031 Add domain-service add tests in internal/domain/thirdparty/service_test.go
Task: T032 Add PUT add handler tests in internal/adapters/http/handlers/admin/protected_resources_handler_test.go
```

### User Story 2

```text
Task: T037 Add domain-service remove tests in internal/domain/thirdparty/service_test.go
Task: T038 Add DELETE handler tests in internal/adapters/http/handlers/admin/protected_resources_handler_test.go
```

### User Story 3

```text
Task: T043 Add domain-service rename tests in internal/domain/thirdparty/service_test.go
Task: T044 Add PATCH rename handler tests in internal/adapters/http/handlers/admin/protected_resources_handler_test.go
```

### User Story 4

```text
Task: T049 Add domain-service list tests in internal/domain/thirdparty/service_test.go
Task: T050 Add GET handler tests in internal/adapters/http/handlers/admin/protected_resources_handler_test.go
```

## Implementation Strategy

### MVP First

1. Complete Phase 1 and every Phase 2 precondition, including stakeholder confirmation in T008.
2. Complete foundational migration, adapter parity, child-table resolution, and full-PUT concurrency protection in Phase 2.5.
3. Deliver US1 through T036; run its independent E2E evidence before proceeding.
4. US2 is also P1 and should be delivered immediately after US1 for the complete safe-management MVP.

### Incremental Delivery

1. Deliver the shared persistence and full-update safety foundation.
2. Deliver US1 add, validate independently, then deliver US2 remove for the P1 lifecycle.
3. Deliver US3 rename, then US4 list; each has its own focused tests and acceptance criteria.
4. Finish Phase N only after the complete E2E and real-PostgreSQL evidence is green.

## Notes

- `[P]` means the task can run independently on a distinct file from its paired task; it does not waive the phase dependency.
- `[US#]` labels provide user-story traceability. Setup, precondition, foundation, and constitution-verification tasks intentionally have no story label.
- No frontend task is included: the specification limits the actor to an administrative API client and the plan explicitly excludes UI changes.
- No configuration or encryption task is included: the plan documents both as non-impacts.
