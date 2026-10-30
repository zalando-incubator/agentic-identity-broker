# Tasks: Provider Authorization Parameters

**Input**: Design documents from `/specs/20260720-120000-provider-auth-params/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/admin-service-authorization-params.md`, `quickstart.md`

**Tests**: Required by Constitution Principles VIII and XIII. Write tests first; each must compile and fail semantically before the matching implementation.

**Organization**: Tasks are grouped by user story. Phase 2 establishes the approved design and red acceptance coverage; Phase 2.5 contains the shared aggregate/storage foundation that blocks the stories.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the existing feature surfaces and test commands before changing behavior.

- [X] T001 Inspect existing service CRUD, OAuth2 session, memory/PostgreSQL repository, and E2E patterns in `internal/`, `tests/e2e/`, and `tests/integration/storage/infra/`
- [X] T002 Confirm the next migration sequence and existing JSONB migration conventions in `migrations/`

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Confirm the approved domain, API, database, and acceptance-test design before implementation.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [X] T003 Confirm `AuthorizationParams map[string]string` ownership, copy semantics, and invariants against `specs/20260720-120000-provider-auth-params/data-model.md`
- [X] T004 [P] Add the Provider Authorization Parameters glossary definition and ThirdpartyOAuth2Service relationship in `ARCHITECTURE.md`

### Phase 2b: Configuration Design [MANDATORY]

- [X] T005 Record that provider authorization parameters are persisted service configuration, requiring no runtime configuration, YAML example, or Helm change in `specs/20260720-120000-provider-auth-params/plan.md`

### Phase 2c: API Design [MANDATORY]

- [X] T006 Update the create, update, and response schemas for optional string-map `authorization_params` in `api/admin/openapi.yaml`
- [X] T007 Record the approved administrative API addition by retaining the confirmation reference in `specs/20260720-120000-provider-auth-params/spec.md`

### Phase 2d: Database Design [MANDATORY]

- [X] T008 Define the reversible `authorization_params JSONB NOT NULL DEFAULT '{}'::jsonb` change in `migrations/022_add_service_authorization_params.up.sql` and `migrations/022_add_service_authorization_params.down.sql`

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

No frontend change is in scope; the administrative API is the feature surface.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

- [X] T009 Create eight isolated Ginkgo `It` blocks, each with its exact scenario reference, for all acceptance scenarios in `tests/e2e/provider_authorization_params_test.go`
- [X] T010 Verify `tests/e2e/provider_authorization_params_test.go` compiles and fails semantically without `XIt`, `PIt`, `Skip()`, placeholder assertions, or red-phase comments

**Checkpoint**: Domain/API/database design is confirmed; all eight acceptance scenarios have red E2E coverage.

---

## Phase 2.5: Foundational Infrastructure (Blocking Prerequisites)

**Purpose**: Add the shared aggregate, persistence, and transport plumbing used by every story.

- [X] T011 Create the reversible JSONB migration with column comment in `migrations/022_add_service_authorization_params.up.sql` and `migrations/022_add_service_authorization_params.down.sql`
- [X] T012 [P] Add red table-driven validation and deep-copy isolation coverage in `internal/domain/model/thirdparty_oauth2_provider_test.go`
- [X] T013 [P] Add red JSONB record/entity conversion coverage in `internal/adapters/storage/postgres/thirdparty_provider_record_test.go`
- [X] T014 [P] Add red map copy and CRUD persistence coverage in `internal/adapters/storage/memory/thirdparty_provider_test.go`
- [X] T015 [P] Add red migration-backed create/get/update/list persistence coverage in `tests/integration/storage/infra/thirdparty_service_test.go`
- [X] T016 Extend `ThirdpartyOAuth2ProviderEntity` with validated, deep-copied `AuthorizationParams` in `internal/domain/model/thirdparty_oauth2_provider.go`
- [X] T017 Add JSONB map conversion and query mapping in `internal/adapters/storage/postgres/thirdparty_provider_record.go` and `internal/adapters/storage/postgres/thirdparty_provider.go`
- [X] T018 Preserve map isolation through memory repository create/get/update/list operations in `internal/adapters/storage/memory/thirdparty_provider.go`

**Checkpoint**: The field is safely represented and persisted by both storage adapters; user-story behavior can build on it.

---

## Phase 3: User Story 1 - Configure Provider Authorization Parameters (Priority: P1) 🎯 MVP

**Goal**: Administrators can create, read, list, update, preserve, and clear a service's provider authorization parameters.

**Independent Test**: Create a service with `business_partner_id: "12345"`; get and list return it; an update omitting the field preserves it and `{}` clears it.

### Tests for User Story 1

- [X] T019 [P] [US1] Add red create/get/list response and update omission/empty-map mapping coverage in `internal/adapters/http/handlers/admin/services_handler_test.go`

### Implementation for User Story 1

- [X] T020 [US1] Map `authorization_params` through admin create, get, list, and update request/response types in `internal/adapters/http/handlers/admin/services_handler.go`
- [X] T021 [US1] Preserve stored `AuthorizationParams` only when the decoded update map is nil in `internal/adapters/http/handlers/admin/services_handler.go`
- [X] T022 [US1] Document administrator configuration, omission preservation, clearing, and the Zalando `business_partner_id` example in `docs/guides/manage-agents-and-services.md`

**Checkpoint**: User Story 1 is independently functional through the admin API and its acceptance scenarios pass.

---

## Phase 4: User Story 2 - Authorize With Provider Configuration (Priority: P1)

**Goal**: An authorization redirect includes only the selected service's stored provider parameters alongside broker-owned fields.

**Independent Test**: Start authorization for a configured service and assert the upstream URL contains `business_partner_id=12345`, normal broker fields, and not a conflicting browser value.

### Tests for User Story 2

- [X] T023 [P] [US2] Add red upstream URL coverage for configured, unconfigured, and conflicting browser-query flows in `internal/domain/oauth2session/service_test.go`

### Implementation for User Story 2

- [X] T024 [US2] Append only stored provider `AuthorizationParams` after the broker-generated OAuth2 URL is built in `internal/domain/oauth2session/service.go`

**Checkpoint**: User Story 2 preserves broker field authority and passes its three E2E scenarios.

---

## Phase 5: User Story 3 - Prevent Unsafe Parameter Configuration (Priority: P1)

**Goal**: Invalid blank and broker-owned parameter names are rejected before persistence or authorization.

**Independent Test**: Create and update requests containing blank keys/values or every reserved key receive the existing validation error and leave configuration unchanged.

### Tests for User Story 3

- [X] T025 [P] [US3] Expand table-driven blank, whitespace, case-insensitive reserved-name, and update-no-mutation coverage in `internal/domain/model/thirdparty_oauth2_provider_test.go`
- [X] T026 [P] [US3] Add admin validation-error response coverage for invalid map create and update requests in `internal/adapters/http/handlers/admin/services_handler_test.go`

### Implementation for User Story 3

- [X] T027 [US3] Add shared create/update validation rejecting blank map keys or values and reserved OAuth2 names in `internal/domain/model/thirdparty_oauth2_provider.go`

**Checkpoint**: User Story 3 rejects unsafe configuration and passes its two E2E scenarios.

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify the implemented feature meets constitutional, security, migration, and documentation requirements.

### Design Phase Verification [MANDATORY]

- [X] T028 Verify Provider Authorization Parameters is documented in the glossary in `ARCHITECTURE.md` (Principles II and V)
- [X] T029 Verify no runtime configuration, config example, or Helm update is needed because the map is persisted service data in `specs/20260720-120000-provider-auth-params/plan.md` (Principle VII)
- [X] T030 Verify `authorization_params` matches the approved contract in `api/admin/openapi.yaml` and `specs/20260720-120000-provider-auth-params/contracts/admin-service-authorization-params.md` (Principles IV and X)
- [X] T031 Verify migration design and rollback are present in `migrations/022_add_service_authorization_params.up.sql` and `migrations/022_add_service_authorization_params.down.sql` (Principle IX)
- [X] T032 Verify all eight scenario-referenced E2E tests exist and had semantic red coverage in `tests/e2e/provider_authorization_params_test.go` (Principle XIII)

### Implementation Phase Verification [MANDATORY]

- [X] T033 [P] Verify all admin API paths match `authorization_params` schemas in `api/admin/openapi.yaml` (Principles IV and X)
- [X] T034 [P] Verify no end-user query parameter can override stored values and no configured values are logged in `internal/domain/oauth2session/service.go` (Principle I)
- [X] T035 [P] Verify map validation remains in the domain and storage behavior remains adapter-local in `internal/domain/model/thirdparty_oauth2_provider.go`, `internal/adapters/storage/memory/thirdparty_provider.go`, and `internal/adapters/storage/postgres/thirdparty_provider.go` (Principle VI)
- [X] T036 Run focused Go unit, handler, and storage suites from `internal/domain/model`, `internal/domain/oauth2session`, `internal/adapters/http/handlers/admin`, `internal/adapters/storage/memory`, and `internal/adapters/storage/postgres` (Principle VIII)
- [X] T037 Run backend acceptance tests with `just test-e2e-backend` for `tests/e2e/provider_authorization_params_test.go` (Principle XIII)
- [X] T038 Run PostgreSQL integration verification with `just test-integration-infra` for `tests/integration/storage/infra/thirdparty_service_test.go` (Principle IX)
- [X] T039 Run static checks with `just check` from `Justfile`
- [X] T040 Run quickstart validation against `specs/20260720-120000-provider-auth-params/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 can begin immediately.
- Phase 2 blocks all implementation: API and migration design must be complete, and E2E tests must be semantic red.
- Phase 2.5 depends on Phase 2 and blocks all user stories because it establishes the field and both persistence backends.
- US1, US2, and US3 can start after Phase 2.5. They touch related files, so execute them in order for the smallest conflict surface: **US1 → US2 → US3**.
- Constitution Compliance & Polish depends on all stories.

### User Story Dependencies

- **US1 (P1)**: Depends on Phase 2.5; delivers the administrative configuration MVP.
- **US2 (P1)**: Depends on Phase 2.5 and uses the stored field; can be implemented after US1 to reuse proven admin configuration.
- **US3 (P1)**: Depends on Phase 2.5; validates the same shared domain field and should follow US1 to avoid concurrent edits to the entity and handler tests.

### Parallel Opportunities

- T004 and T006 can proceed in parallel.
- T012–T015 are independent red tests and can proceed in parallel.
- T019 and T023 can proceed in parallel after the foundational field exists.
- T025 and T026 can proceed in parallel.
- T033–T035 can proceed in parallel.

## Parallel Example: Foundational Coverage

```text
Task: "T012 Add domain validation and copy tests in internal/domain/model/thirdparty_oauth2_provider_test.go"
Task: "T013 Add PostgreSQL record conversion tests in internal/adapters/storage/postgres/thirdparty_provider_record_test.go"
Task: "T014 Add memory storage tests in internal/adapters/storage/memory/thirdparty_provider_test.go"
Task: "T015 Add PostgreSQL integration tests in tests/integration/storage/infra/thirdparty_service_test.go"
```

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 and Phase 2, including semantic-red E2E coverage.
2. Complete Phase 2.5 so both storage adapters carry the field safely.
3. Complete US1 and independently verify admin create/get/list/update behavior.
4. Stop for review or demo; provider redirect behavior is intentionally deferred to US2.

### Incremental Delivery

1. Deliver US1 administrative configuration.
2. Deliver US2 stored-parameter authorization URL behavior without browser forwarding.
3. Deliver US3 domain rejection behavior and complete the verification gate.

---

## Phase 7: Remaining Authorization-Code Exchange Coverage

**Purpose**: Complete the specification's required server-side provider parameters on authorization-code exchange requests. Existing tasks cover the authorization URL only; this phase preserves them and adds the missing exchange behavior and proof.

- [X] T041 [P] [US2] Add unit coverage that authorization-code exchange sends stored `AuthorizationParams` on every retry in `internal/domain/oauth2session/service_test.go`
- [X] T042 [US2] Pass stored `AuthorizationParams` as `oauth2.AuthCodeOption` values on every authorization-code exchange retry in `internal/domain/oauth2session/service.go`
- [X] T043 [US2] Extend scenarios 2.1–2.3 to capture and assert the upstream authorization-code exchange form uses stored values, omits values for unconfigured services, and never forwards browser values in `tests/e2e/provider_authorization_params_test.go`
- [X] T044 [US2] Run focused session-service and provider-authorization acceptance tests with `go test ./internal/domain/oauth2session` and `just test-e2e-backend`

**Checkpoint**: Both upstream authorization and authorization-code exchange requests receive only the selected service's stored provider parameters.

---

## Phase 8: Refresh-Token Provider Parameters

**Purpose**: Extend the completed authorization and authorization-code behavior to session refreshes. The target Zalando Platform IdP requires `business_partner_id` on its refresh-token grant, so refresh must use the same stored service configuration without accepting browser input.

- [X] T045 [P] [US2] Add red unit coverage that refresh-token forms include stored `AuthorizationParams` and unconfigured services add none in `internal/domain/oauth2session/service_test.go`
- [X] T046 [US2] Add stored `AuthorizationParams` to the refresh-token form after broker-owned fields in `internal/domain/oauth2session/service.go`
- [X] T047 [US2] Extend scenarios 2.1–2.3 to capture and assert refresh-token forms use stored values, omit values for unconfigured services, and never use browser values in `tests/e2e/provider_authorization_params_test.go`
- [X] T048 [US2] Run session-service and provider-authorization acceptance coverage with `go test ./internal/domain/oauth2session` and `just test-e2e-backend`

**Checkpoint**: Upstream authorization, authorization-code exchange, and refresh-token requests receive only the selected service’s stored provider parameters.
