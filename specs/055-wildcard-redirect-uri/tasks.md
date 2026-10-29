# Tasks: Wildcard Redirect URI Hosts

**Input**: Design documents from `specs/055-wildcard-redirect-uri/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/redirect-uri.md`, and `quickstart.md`

**Tests**: Required. The spec maps every acceptance scenario to an E2E test, and the constitution requires test-first development.

**Organization**: Tasks are grouped by user story. Every task cites a feature requirement, acceptance scenario, design constraint, or binding constitution requirement.

## Feature Scope

- Implement only bounded HTTPS host wildcards for Agent-configured `redirect_uris`.
- Keep CIMD callbacks exact-match, private-use callbacks byte-for-byte exact, and loopback HTTP port matching unchanged.
- Do not add a runtime setting, API endpoint or field, persisted field, migration, frontend change, or external deployment configuration to this repository.
- The external test deployment must separately apply and reconcile the callback registration for existing and newly created test Agents before preview validation. This is a release dependency, not an AIB code task.

## Phase 1: Setup

- [X] T001 Verify `golang.org/x/net v0.60.0` in `go.mod` provides the `idna` and `publicsuffix` packages; do not add a dependency (SR-001, Principle III).

## Phase 2: Design Preconditions

### Phase 2a: Domain Model and Glossary

- [ ] T002 Review and finalize Agent callback invariants in `specs/055-wildcard-redirect-uri/data-model.md`, confirming that no new entity, identifier, or persisted field is needed (FR-001-FR-005).
- [ ] T003 [P] Add the Redirect URI Pattern domain term and its bounded-host definition to `ARCHITECTURE.md` (Principle V).
- [ ] T004 [P] Create `adrs/044-wildcard-web-redirect-uris.md` with context, decision, consequences, and proposed status for wildcard grammar, PSL boundary, and callback-source isolation (SR-001-SR-004, Principle II).

### Phase 2b: Configuration Design

- [ ] T005 Confirm in `specs/055-wildcard-redirect-uri/plan.md` that this feature adds no runtime configuration, YAML example, or Helm chart change (FR-011, SC-006, Principle VII).

### Phase 2c: API Design

- [ ] T006 [P] Document wildcard registration syntax, fixed URI components, and invalid-pattern behavior in the Agent response, create, and update schemas in `api/admin/openapi.yaml` (API-001, FR-001-FR-005).
- [ ] T007 [P] Document source-specific matching and unchanged error behavior in `api/enduser/openapi.yaml` and `docs/api/enduser-api.md` (API-001, FR-006-FR-010).
- [ ] T008 Review the existing Agent and authorization contract and record confirmation of the behavior change in `specs/055-wildcard-redirect-uri/contracts/redirect-uri.md` before implementation (API-002, Principle X).

### Phase 2d: Database Design

- [ ] T009 Confirm in `specs/055-wildcard-redirect-uri/data-model.md` that the existing string-list representation needs no migration or backfill (FR-001, Principle IX).

### Phase 2e: Frontend and Design System Review

Not applicable. This feature changes no React UI or frontend design-system component.

### Phase 2f: E2E Acceptance Test Design

- [ ] T010 Review the concrete journeys and verify all nine scenarios map to observable HTTP outcomes in `specs/055-wildcard-redirect-uri/spec.md` and `specs/055-wildcard-redirect-uri/plan.md` (Principles XIII-XIV).
- [ ] T010a Write all nine production-bootstrap Ginkgo acceptance tests in `tests/e2e/wildcard_redirect_uri_test.go`, one `It()` per scenario, using existing fixtures and scenario comments (Principle XIII).
- [ ] T010b Run the new E2E tests and verify they compile and fail semantically before any feature implementation; do not use pending tests or placeholder assertions (Principle XIII).

**Checkpoint**: Complete the design tasks before feature implementation. Confirm no configuration, database, or frontend work is required.

## Phase 2.5: Foundational URI Pattern Support

**Purpose**: Provide a tested, source-neutral HTTPS host-pattern validator and matcher for both user stories. Agent/CIMD source selection remains in the OAuth2 service.

- [ ] T011 [P] Add table-driven matcher tests for whole-label and partial-label wildcards, zero-character matches, and exact non-host components in `internal/domain/urivalidation/redirect_test.go` (FR-003, FR-005, SR-002).
- [ ] T012 [P] Add failing security tests for malformed labels, IDNA/PSL failures, wildcarded registrable domains or suffixes, and wildcard patterns whose probe hostname is an IP literal or single-label name in `internal/domain/urivalidation/redirect_wildcard_security_test.go` (FR-004, SR-001-SR-002).
- [ ] T012a [P] Add Agent create/update validation tests for valid and invalid wildcard registrations in `internal/domain/storage/agent_test.go` (FR-001-FR-004).
- [ ] T012b [P] Add OAuth2 service tests for opaque-Agent wildcard matching, concrete redirect preservation, exact CIMD/private-use matching, and malformed stored patterns in `internal/domain/oauth2/service_test.go` (FR-006-FR-010, SR-003-SR-004).
- [ ] T012c Run the focused domain tests and verify all new cases compile and fail semantically before implementation (Principle VIII).
- [ ] T013 Implement HTTPS host-pattern validation and label-bounded matching with `golang.org/x/net/idna` and `golang.org/x/net/publicsuffix` in `internal/domain/urivalidation/redirect.go`; reject unsafe patterns and preserve exact scheme, port, path, and query matching (FR-003-FR-005, SR-001-SR-002).

**Checkpoint**: `go test ./internal/domain/urivalidation` passes; pattern helpers fail closed and do not match across hostname labels.

## Phase 3: User Story 1 - Authorize Preview Callbacks (Priority: P1)

**Goal**: Accept valid registered preview-host patterns and complete authorization to each concrete callback.

**Independent Test**: Register a valid pattern and complete authorization through both PR preview hosts, the stable test host, sandbox, and staging; each response uses the exact requested callback and state.

### Implementation for User Story 1

- [ ] T014 [US1] Enable validated HTTPS host-wildcard registrations in Agent create/update validation in `internal/domain/storage/agent.go` (FR-001-FR-004).
- [ ] T015 [US1] Route opaque Agent callback checks through the wildcard matcher while keeping CIMD and private-use callback matching exact in `internal/domain/oauth2/service.go` (FR-006, FR-009-FR-010, SR-003-SR-004).

### Verification for User Story 1

- [ ] T016 [US1] Rerun the Agent and OAuth2 unit tests and the prewritten US1-S1 through US1-S5 E2E scenarios; verify every redirect uses the concrete callback and its own state (FR-001-FR-002, FR-006, SC-001-SC-002).

**Checkpoint**: US1-S1 through US1-S5 pass independently; every successful authorization redirects to the concrete request URI with its own state.

## Phase 4: User Story 2 - Reject Callbacks Outside the Trust Boundary (Priority: P1)

**Goal**: Reject wildcard patterns and callback requests outside the registered security boundary while preserving existing exact callback behavior.

**Independent Test**: Verify that unregistered hosts, changed schemes or paths, unsafe wildcard registrations, and malformed stored patterns cannot redirect; verify an existing exact callback still succeeds.

### Implementation for User Story 2

- [ ] T017 [US2] Ensure malformed stored Agent patterns and non-matching callbacks fail closed with the existing error response and no redirect in `internal/domain/oauth2/service.go` (FR-007, SR-001-SR-004).

### Verification for User Story 2

- [ ] T018 [US2] Rerun the prewritten Agent/OAuth2 unit tests and US2-S1 through US2-S4 E2E scenarios; verify invalid callbacks have no redirect and existing exact callbacks still work (FR-007-FR-010, SC-003-SC-005).

**Checkpoint**: US2-S1 through US2-S4 pass; invalid registrations are rejected, no unregistered callback receives a redirect, and exact/private-use/loopback behavior is unchanged.

## Phase 5: Polish and Cross-Cutting Verification

- [ ] T019 Reconcile the wildcard descriptions and examples across `api/admin/openapi.yaml`, `api/enduser/openapi.yaml`, and `docs/api/enduser-api.md` (API-001, FR-005-FR-007).
- [ ] T020 Review and set the final ADR status to Accepted when approved; ensure `adrs/044-wildcard-web-redirect-uris.md` matches the implemented trust boundary (SR-001-SR-004, Principle II).
- [ ] T021 Verify the Redirect URI Pattern glossary definition and architecture notes match the implementation in `ARCHITECTURE.md` (Principles II and V).
- [ ] T022 Run the focused domain tests from `specs/055-wildcard-redirect-uri/quickstart.md` and resolve failures (FR-001-FR-010, SR-001-SR-004).

## Phase N: Constitution Compliance Verification

### Design Phase Verification

- [ ] T023 Verify every task maps to a spec requirement, acceptance scenario, or binding constraint in `specs/055-wildcard-redirect-uri/spec.md` and `specs/055-wildcard-redirect-uri/tasks.md` (Feature Scope & Deferred Findings).
- [ ] T024 Verify the domain model and new glossary term are documented in `specs/055-wildcard-redirect-uri/data-model.md` and `ARCHITECTURE.md` (Principles II and V).
- [ ] T025 Verify API contracts and examples are documented and stakeholder confirmation is recorded in `api/admin/openapi.yaml`, `api/enduser/openapi.yaml`, `docs/api/enduser-api.md`, and `specs/055-wildcard-redirect-uri/contracts/redirect-uri.md` (Principles IV and X).
- [ ] T026 Verify the no-configuration, no-Helm, and no-database-change decisions are documented in `specs/055-wildcard-redirect-uri/plan.md` and `specs/055-wildcard-redirect-uri/data-model.md` (Principles VII and IX).
- [ ] T027 Verify the concrete journeys and all nine scenario-to-test mappings remain aligned in `specs/055-wildcard-redirect-uri/spec.md` and `specs/055-wildcard-redirect-uri/plan.md` (Principles XIII-XIV).

### Implementation Phase Verification

- [ ] T028 Review `internal/domain/urivalidation/redirect.go` and `internal/domain/oauth2/service.go` to confirm PSL-boundary enforcement, exact non-host components, source-specific matching, and fail-closed behavior (FR-003-FR-010, SR-001-SR-004).
- [ ] T029 Verify unit tests were written first, failed semantically, and then passed in `internal/domain/urivalidation/redirect_test.go`, `internal/domain/urivalidation/redirect_wildcard_security_test.go`, `internal/domain/storage/agent_test.go`, and `internal/domain/oauth2/service_test.go` (Principle VIII).
- [ ] T030 Verify all nine acceptance scenarios have exactly one Ginkgo `It()` with a spec reference and now pass in `tests/e2e/wildcard_redirect_uri_test.go` (Principle XIII).
- [ ] T031 Verify no frontend changes, unrelated remediations, or deployment-specific configuration were added; check the scope recorded in `specs/055-wildcard-redirect-uri/plan.md` (FR-011, SC-006).
- [ ] T032 Run `just check` from `justfile` and resolve formatting, vet, and lint failures (Principle VIII).
- [ ] T033 Run `just test` from `justfile` and resolve Go test failures (Principle VIII).
- [ ] T034 Run `just test-e2e-backend` from `justfile` and resolve backend E2E failures (Principle XIII).
- [ ] T035 Before external preview validation, confirm the separate test deployment has reconciled the callback registration for existing and newly created test Agents (SC-001).

## Dependencies and Execution Order

### Phase Dependencies

- Phase 1 setup completes before Phase 2 design confirmation.
- Phase 2a-2f are blocking design preconditions. Their independent documentation tasks can proceed in parallel where marked `[P]`.
- Phase 2f writes and verifies all nine E2E tests before implementation. Phase 2.5 writes and verifies all domain tests before T013.
- Phase 2.5 implementation depends on Phase 2f and its failing tests. It blocks both user stories because both depend on the URI-pattern validator and matcher.
- US1 and US2 are both P1. Their test code is complete before implementation; US2 verification follows the shared Agent-only matching work. Both stories are required before release.
- Phase 5 and Phase N follow both user stories. External test deployment reconciliation is required only before preview-environment validation, not for local broker tests.

### User Story Dependencies

- US1 can start after Phase 2.5 and is the smallest usable preview authorization increment.
- US2 shares the matcher and authorization service with US1; its verification should follow US1's implementation before release. Both P1 stories are required for a safe release.

### Parallel Opportunities

- In Phase 2, T003/T004 and T006/T007 work on separate files and can proceed independently after the feature scope is agreed.
- In Phase 2.5, T011, T012, T012a, and T012b target separate test files and can be authored in parallel; T012c verifies red before T013.
- T010a authors the full E2E suite and T010b verifies red before any Phase 2.5 implementation. After T013, the US1 implementation and US2 defensive checks can proceed, with T016/T018 validating their story-specific scenarios.

## Implementation Strategy

### MVP

Deliver Phase 2, Phase 2.5, and US1 to enable valid preview callbacks. Do not release that increment without US2's security-rejection tests and implementation; both P1 stories are required for production safety.

### Incremental Delivery

1. Complete design preconditions and shared validator/matcher with failing-first unit tests.
2. Implement US1 and verify the five valid preview hosts independently.
3. Implement and verify US2 rejection and exact-match regression behavior.
4. Complete cross-cutting gates and external test deployment reconciliation before preview-environment validation.
