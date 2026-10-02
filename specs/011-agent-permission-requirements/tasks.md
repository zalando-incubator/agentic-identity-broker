# Tasks: Agent Permission Requirements

**Feature**: 011-agent-permission-requirements  
**Input**: Design documents from `/specs/011-agent-permission-requirements/`  
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/openapi-changes.md ✅

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing) and Principle XIII (End-to-End Acceptance Testing), automated tests are MANDATORY. E2E tests for all 34 acceptance scenarios MUST be written BEFORE implementation and verified to FAIL (red phase).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `- [ ] [ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4, US5, US6)
- Include exact file paths in descriptions

## Path Conventions

- Backend: `internal/domain/`, `internal/adapters/`, `migrations/`
- Frontend: `web/src/components/`, `web/src/pages/`
- Tests: `tests/e2e/`, `tests/integration/`, `internal/domain/storage/` (unit tests)
- APIs: `api/admin/openapi.yaml`, `api/enduser/openapi.yaml`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and validation

- [x] T001 Verify Go 1.23.0+ installed and repository cloned at `/home/runner/work/agentic-identity-broker/agentic-identity-broker`
- [x] T002 [P] Run `just deps` to ensure all dependencies are up to date
- [x] T003 [P] Run `just verify` to verify existing tests pass and establish baseline

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

**🔒 CONSTITUTION REQUIREMENT**: This phase maps directly to the constitution PRECONDITIONS checklist. All sub-phases (2a-2f) MUST be included in every tasks.md.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [x] T004 Review domain model design in `specs/011-agent-permission-requirements/data-model.md`
- [x] T004a Verify entities documented: Agent (extended), ServiceRequirement (value object), RequirementType (enum)
- [x] T004b Add new domain terms to `ARCHITECTURE.md` Glossary section: ServiceRequirement, RequirementType, Mandatory Service, Optional Service
- [x] T004c Document invariants: No duplicate service_id in service_requirements array, case-sensitive scope validation

**Checkpoint**: ✅ Domain model complete and documented in ARCHITECTURE.md

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [x] T005 Review configuration requirements - no new configuration needed (uses existing system)
- [x] T005a Verify no feature-specific configuration parameters required
- [x] T005b Document that feature uses existing database connection and HTTP server configuration

**Checkpoint**: ✅ Configuration requirements reviewed (no changes needed)

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

- [x] T006 Review Admin API design in `specs/011-agent-permission-requirements/contracts/openapi-changes.md`
- [x] T006a Verify Admin API extensions: POST/PUT/GET `/api/agents` with service_requirements field
- [x] T006b Review End-User API design in contracts: GET `/api/consent/agent/{agent-id}` with service requirements
- [x] T006c Verify redirect_uri parameter support in POST `/api/consent/agent/{agent-id}/approve`
- [x] T006d Confirm API designs align with spec.md user stories and acceptance criteria

**Checkpoint**: ✅ APIs designed and documented in contracts/openapi-changes.md

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [x] T007 Review database schema design in `specs/011-agent-permission-requirements/data-model.md`
- [x] T007a Verify migration 005_add_agent_service_requirements adds JSONB column to agents table
- [x] T007b Verify migration includes GIN index for JSONB queries
- [x] T007c Verify migration files follow go-migrate naming: `005_add_agent_service_requirements.{up,down}.sql`
- [x] T007d Confirm backward compatibility: NULL = no requirements

**Checkpoint**: ✅ Database schema designed, migrations documented

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

> **Historical visual scope**: The visual choices and design checks in this task record retain their original wording and completion states, not renewed aesthetic requirements. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

- [x] T008 Review design system at `web/src/design-system/docs/INDEX.md` for component selection
- [x] T008a Identify design system components: Card, CardHeader, CardBody, Badge, Button
- [x] T008b Plan semantic token usage: trust-deep (required), neutral (optional), success-primary (active session)
- [x] T008c Plan new components: ServiceRequirementCard, ServiceRequirementsList (as universal components)

**Checkpoint**: ✅ Design system usage planned

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)

- [x] T009 Create E2E test file `tests/e2e/agent_permission_requirements_test.go` with Ginkgo/Gomega structure
- [x] T009a Map User Story 1 scenarios (7 scenarios) to `It()` blocks in E2E tests
- [x] T009b Map User Story 2 scenarios (7 scenarios) to `It()` blocks in E2E tests
- [x] T009c Map User Story 3 scenarios (6 scenarios) to `It()` blocks in E2E tests
- [x] T009d Map User Story 4 scenarios (5 scenarios) to `It()` blocks in E2E tests
- [x] T009e Map User Story 5 scenarios (4 scenarios) to `It()` blocks in E2E tests
- [x] T009f Map User Story 6 scenarios (5 scenarios) to `It()` blocks in E2E tests
- [x] T009g Map edge cases to `It()` blocks in E2E tests (edge cases are additional scenarios beyond the 34 numbered acceptance scenarios)
- [x] T009h Create test fixtures in `tests/e2e/fixtures/`: agent_with_mandatory_service.json, agent_with_optional_service.json, agent_with_multiple_requirements.json
- [x] T009i Add comment references in E2E test file mapping to spec.md scenario numbers
- [x] T009j Run `ginkgo -v ./tests/e2e/agent_permission_requirements_test.go` and verify ALL tests FAIL (red phase)

**Checkpoint**: ✅ E2E acceptance tests written (34 scenarios + edge cases) and verified to fail before implementation

---

## Phase 2.5: Foundational Infrastructure [Database Migration]

**Purpose**: Database schema changes that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T010 Create migration file `migrations/005_add_agent_service_requirements.up.sql` (DB-001, DB-002)
- [x] T011 Create rollback migration file `migrations/005_add_agent_service_requirements.down.sql` (DB-001, DB-005)
- [x] T012 Write integration test in `tests/integration/infra/agent_service_requirements_migration_test.go` to verify migration applies cleanly using existing testcontainer setup (DB-005)
- [x] T013 Write integration test to verify migration rollback works without data loss (DB-005)
- [x] T014 Apply migration to test database and verify agents table has service_requirements JSONB column (DB-002, DB-003)
- [x] T015 Apply migration to test database and verify idx_agents_service_requirements GIN index created (DB-004)
- [x] T016 Verify existing agents in test data have NULL service_requirements (backward compatibility) (DB-002)

**Checkpoint**: ✅ Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Administrator Configures Agent Service Requirements (Priority: P1) 🎯 MVP

**Goal**: Enable administrators to define mandatory and optional third-party service requirements for agents through the Admin API. Each requirement specifies which OAuth2 scopes must be granted for the service.

**Independent Test**: Administrator creates/updates an agent via Admin API with service_requirements array. System validates configuration, stores it, and returns complete agent with service requirements including resolved service names.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T017 [P] [US1] Write unit tests in `internal/domain/storage/requirement_type_test.go` for RequirementType validation
- [x] T018 [P] [US1] Write unit tests in `internal/domain/storage/service_requirement_test.go` for ServiceRequirement structure validation
- [x] T019 [P] [US1] Write unit tests in `internal/domain/storage/agent_test.go` for Agent.ValidateServiceRequirements() including duplicate detection
- [x] T020 [P] [US1] Write integration tests in `tests/integration/agent_repository_service_requirements_memory_test.go` and `tests/integration/infra/agent_repository_service_requirements_test.go` for JSONB serialization/deserialization using existing self-contained and infra-backed setups
- [x] T021 [P] [US1] Write unit tests in `internal/adapters/http/handlers/admin/agent_handler_test.go` for validateServiceRequirements()
- [x] T022 [US1] Run tests and verify they FAIL (no implementation exists yet)

### Implementation for User Story 1

- [x] T023 [P] [US1] Create RequirementType enum in `internal/domain/storage/requirement_type.go` (FR-002)
- [x] T024 [P] [US1] Create ServiceRequirement value object in `internal/domain/storage/service_requirement.go` with Validate() method (FR-002, FR-003, FR-004)
- [x] T025 [US1] Extend Agent entity in `internal/domain/storage/agent.go` with ServiceRequirements field and ValidateServiceRequirements() method (FR-001, FR-003a)
- [x] T026 [US1] Extend memory repository in `internal/adapters/storage/memory/agent_repository.go` Create/Update/GetByID methods to handle ServiceRequirements (FR-001)
- [x] T027 [US1] Extend PostgreSQL repository in `internal/adapters/storage/postgres/agent_repository.go` to serialize/deserialize ServiceRequirements as JSONB (FR-001, DB-002, DB-003)
- [x] T028 [US1] Add validateServiceRequirements() method to `internal/adapters/http/handlers/admin/agent_handler.go` for referential integrity checks (FR-003, FR-004, FR-008)
- [x] T029 [US1] Extend CreateAgent handler in `internal/adapters/http/handlers/admin/agent_handler.go` to validate and store service_requirements (FR-005, API-001)
- [x] T030 [US1] Extend UpdateAgent handler in `internal/adapters/http/handlers/admin/agent_handler.go` to validate and update service_requirements (FR-006, API-002)
- [x] T031 [US1] Extend GetAgent handler in `internal/adapters/http/handlers/admin/agent_handler.go` to resolve service names in response (FR-007, API-003)
- [x] T032 [US1] Update Admin API OpenAPI spec in `api/admin/openapi.yaml` with ServiceRequirement schema and Agent extension (API-006, API-008)
- [x] T033 [US1] Add structured logging for service requirement validation failures per SR-004 (SR-004, API-007)
- [x] T034 [US1] Run unit tests and verify they PASS
- [x] T035 [US1] Run integration tests and verify they PASS
- [x] T036 [US1] Run E2E tests for User Story 1 scenarios and verify they PASS

**Checkpoint**: ✅ At this point, User Story 1 should be fully functional and testable independently. Administrators can configure agent service requirements via Admin API.

---

## Phase 4: User Story 2 - Authorization Endpoint Validates Service Requirements (Priority: P1) 🎯 MVP

**Goal**: Extend OAuth2 authorization endpoint to validate that users have active sessions for all mandatory services with required scopes before proxying to upstream OAuth2 server.

**Independent Test**: Initiate OAuth2 authorization request for agent with mandatory service requirements. If user lacks required sessions/scopes, system redirects to consent screen instead of proxying to upstream server.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T037 [P] [US2] Write unit tests in `internal/adapters/http/handlers/enduser/oauth2_handler_test.go` for hasRequiredScopes() function
- [x] T038 [P] [US2] Write unit tests in `internal/adapters/http/handlers/enduser/oauth2_handler_test.go` for validateMandatoryRequirements() method
- [x] T039 [US2] Run tests and verify they FAIL (no implementation exists yet)

### Implementation for User Story 2

- [x] T040 [P] [US2] Implement hasRequiredScopes() helper function in `internal/adapters/http/handlers/enduser/oauth2_handler.go` (case-sensitive superset check) (FR-011, SR-006)
- [x] T041 [US2] Implement validateMandatoryRequirements() method in `internal/adapters/http/handlers/enduser/oauth2_handler.go` (checks sessions and scopes) (FR-009, FR-010, FR-011)
- [x] T042 [US2] Extend Authorize() handler in `internal/adapters/http/handlers/enduser/oauth2_handler.go` to call validateMandatoryRequirements() after consent check (FR-009, FR-013)
- [x] T043 [US2] Add redirect to consent screen in Authorize() handler when mandatory requirements not satisfied (FR-012)
- [x] T044 [US2] Ensure optional requirements do not block authorization flow (FR-014, FR-015)
- [x] T045 [US2] Add structured logging when mandatory requirements not met for security audit (SR-004, API-007)
- [x] T046 [US2] Update End-User API OpenAPI spec in `api/enduser/openapi.yaml` documenting new authorization behavior (API-006, API-008)
- [x] T047 [US2] Run unit tests and verify they PASS
- [x] T048 [US2] Run E2E tests for User Story 2 scenarios and verify they PASS

**Checkpoint**: ✅ At this point, User Stories 1 AND 2 should both work independently. Authorization endpoint enforces mandatory service requirements.

---

## Phase 5: User Story 6 - Redirect URL for Seamless Flow Continuation (Priority: P1) 🎯 MVP

**Goal**: Support redirect_uri parameter in consent approval flow. After user approves consent, backend redirects to the URL allowing OAuth2 flow to continue seamlessly.

**Independent Test**: Initiate OAuth2 authorization that redirects to consent screen with redirect_uri parameter. After approval, browser automatically redirects back to authorization URL.

### Tests for User Story 6 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T049 [P] [US6] Write unit tests in `internal/adapters/http/handlers/enduser/consent_handler_test.go` for validateRedirectURI() function
- [x] T050 [US6] Run tests and verify they FAIL (no implementation exists yet)

### Implementation for User Story 6

- [x] T051 [P] [US6] Implement validateRedirectURI() function in `internal/adapters/http/handlers/enduser/consent_handler.go` using net/url package (FR-026, FR-027)
- [x] T052 [US6] Add same-origin validation: check scheme, host, port match request origin (SR-001, SR-002, FR-026)
- [x] T053 [US6] Allow relative URLs (no scheme/host) in validateRedirectURI() (FR-026)
- [x] T054 [US6] Reject external domains with HTTP 400 error (FR-027)
- [x] T055 [US6] Extend ApproveConsent() handler in `internal/adapters/http/handlers/enduser/consent_handler.go` to check redirect_uri parameter (FR-025, API-005)
- [x] T056 [US6] Issue HTTP 302/303 redirect when redirect_uri is valid (FR-025)
- [x] T057 [US6] Display success confirmation when redirect_uri not present (FR-028)
- [x] T058 [US6] Preserve query parameters in redirect_uri when redirecting (FR-029)
- [x] T059 [US6] Update End-User API OpenAPI spec in `api/enduser/openapi.yaml` with redirect_uri parameter documentation (API-005, API-006, API-008)
- [x] T060 [US6] Run unit tests and verify they PASS
- [x] T061 [US6] Run E2E tests for User Story 6 scenarios and verify they PASS

**Checkpoint**: ✅ At this point, User Stories 1, 2, AND 6 should all work independently. OAuth2 flow seamlessly continues after consent approval. **MVP COMPLETE** - P1 user stories functional.

---

## Phase 6: User Story 3 - Consent Screen Displays Required Services (Priority: P2)

**Goal**: Enhance consent screen UI to display third-party service requirements with clear mandatory/optional distinction and connection status.

**Independent Test**: Navigate to consent screen for agent with both mandatory and optional service requirements. UI displays services grouped by type with appropriate visual indicators.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T062 [P] [US3] Write unit tests in `internal/adapters/http/handlers/enduser/consent_handler_test.go` for buildServiceRequirementsForUser() method
- [x] T063 [P] [US3] Write component tests in `web/src/components/consent/ServiceRequirementCard.test.tsx` for ServiceRequirementCard rendering
- [x] T064 [US3] Run tests and verify they FAIL (no implementation exists yet)

### Implementation for User Story 3

- [x] T065 [P] [US3] Implement buildServiceRequirementsForUser() method in `internal/adapters/http/handlers/enduser/consent_handler.go` to enrich requirements with session status (FR-016, FR-019)
- [x] T066 [US3] Extend GetAgentForConsent() handler in `internal/adapters/http/handlers/enduser/consent_handler.go` to return service_requirements with user session status (FR-016, API-004)
- [x] T067 [US3] Resolve service display names and scope descriptions in buildServiceRequirementsForUser() (FR-019, FR-022)
- [x] T068 [US3] Update End-User API OpenAPI spec in `api/enduser/openapi.yaml` with ServiceRequirementForUser and ScopeWithDescription schemas (API-006)
- [x] T069 [P] [US3] Create ServiceRequirementCard component in `web/src/components/consent/ServiceRequirementCard.tsx` (FR-017)
- [x] T070 [P] [US3] Create ServiceRequirementsList component in `web/src/components/consent/ServiceRequirementsList.tsx` (FR-016, FR-018)
- [x] T071 [US3] Implement mandatory service visual highlighting in ServiceRequirementCard (Badge with "Required" label, trust-deep color) (FR-017)
- [x] T072 [US3] Implement optional service visual styling in ServiceRequirementCard (Badge with "Optional" label, neutral color) (FR-017)
- [x] T073 [US3] Display connection status in ServiceRequirementCard ("Active Session" or "Login" button) (FR-019, FR-020, FR-016a)
- [x] T074 [US3] Extend ConsentPage in `web/src/pages/ConsentPage.tsx` to fetch and display service_requirements (FR-016)
- [x] T075 [US3] Group services by requirement_type in ConsentPage: mandatory services first, then optional (FR-018)
- [x] T076 [US3] Implement handleLogin() function to redirect to `/api/third-party/{serviceId}/oauth2/authorize` with consent screen as redirect_uri (FR-020, FR-020a)
- [x] T077 [US3] Run unit tests and verify they PASS
- [x] T078 [US3] Run component tests and verify they PASS
- [x] T079 [US3] Run E2E tests for User Story 3 scenarios and verify they PASS
- [x] T080 [US3] Test consent screen manually and take screenshot showing mandatory/optional service distinction

**Checkpoint**: ✅ At this point, User Stories 1, 2, 3, AND 6 should all work independently. Consent screen clearly displays service requirements.

---

## Phase 7: User Story 4 - Display-Only Scopes with Descriptions (Priority: P2)

**Goal**: Display required scopes as read-only text with descriptions. Remove scope selection UI - users no longer select individual scopes.

**Independent Test**: View consent screen for agent with service requirements. Each service shows required scopes as non-editable text with descriptions. No checkboxes or toggles present.

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T081 [P] [US4] Write component tests in `web/src/components/consent/ServiceRequirementCard.test.tsx` for read-only scope display
- [x] T082 [US4] Run tests and verify they FAIL (no implementation exists yet)

### Implementation for User Story 4

- [x] T083 [P] [US4] Implement read-only scope list in ServiceRequirementCard component (no checkboxes, no toggles) (FR-021)
- [x] T084 [US4] Display scope names in `<code>` tags with trust-deep color (FR-021)
- [x] T085 [US4] Display scope descriptions as helper text below scope names (when available) (FR-022)
- [x] T086 [US4] Handle scopes without descriptions (display name only) (FR-022)
- [x] T087 [US4] Remove any existing scope selection UI from ConsentPage if present (FR-021)
- [x] T088 [US4] Run component tests and verify they PASS
- [x] T089 [US4] Run E2E tests for User Story 4 scenarios and verify they PASS
- [x] T090 [US4] Test consent screen manually and take screenshot showing read-only scopes with descriptions

**Checkpoint**: ✅ All user stories should now be independently functional. Scope display is simplified and read-only.

---

## Phase 8: User Story 5 - Simplified Consent Screen Without Edit Mode (Priority: P2)

**Goal**: Remove edit mode toggle from consent screen. Service connection actions (Login/Disconnect) are always visible without needing to enter edit mode.

**Independent Test**: Navigate to consent screen. Login/Disconnect buttons are immediately visible without clicking any Edit button.

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T091 [P] [US5] Write component tests in `web/src/pages/AgentGrantDetailPage.test.tsx` verifying no edit mode toggle present
- [x] T092 [US5] Run tests and verify they FAIL (no implementation exists yet)

### Implementation for User Story 5

- [x] T093 [P] [US5] Remove edit mode state/toggle from AgentGrantDetailPage component in `web/src/pages/AgentGrantDetailPage.tsx` if present (FR-023)
- [x] T094 [US5] Ensure Login buttons are always visible for services without active sessions (FR-023, FR-016a)
- [x] T095 [US5] Ensure Disconnect actions are always visible for services with active sessions (FR-023)
- [x] T096 [US5] Disable Approve button when mandatory services lack active sessions (FR-024)
- [x] T097 [US5] Run component tests and verify they PASS
- [x] T098 [US5] Run E2E tests for User Story 5 scenarios and verify they PASS
- [x] T099 [US5] Test consent screen manually and take screenshot showing single-mode UI

**Checkpoint**: ✅ All user stories complete. Consent screen has simplified single-mode interface.

---

## 🔒 Phase 9: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and final polish

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

**These tasks MUST be included in every generated tasks.md file. They verify that all constitution principles have been followed.**

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist - verify Phase 2 tasks were completed correctly

- [x] T100 Verify domain model design documented in `ARCHITECTURE.md` Glossary (Principle V) - ServiceRequirement, RequirementType, Mandatory Service, Optional Service
- [x] T101 Verify configuration design reviewed (no new config needed) - uses existing system (Principle VII)
- [x] T102 Verify API designs documented in `api/admin/openapi.yaml` and `api/enduser/openapi.yaml` (Principles IV, X)
- [x] T103 Verify database schema design documented - migration 005_add_agent_service_requirements (Principle IX)
- [x] T104 Verify design system review completed - Card, Badge, Button components identified (Principle XI)
- [x] T105 Verify E2E acceptance tests written in `tests/e2e/agent_permission_requirements_test.go` for all 34 spec scenarios plus edge cases (Principle XIII)
- [x] T106 Verify E2E tests verified to FAIL before implementation (red phase documented) (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist - verify all principles followed during implementation

**API & Documentation** (Principles IV, X):
- [x] T107 [P] Update `api/admin/openapi.yaml` with ServiceRequirement schema and Agent extension (completed in Phase 3)
- [x] T108 [P] Update `api/enduser/openapi.yaml` with ServiceRequirementForUser and redirect_uri parameter (completed in Phases 5-6)
- [x] T109 [P] Verify API implementation matches OpenAPI specifications exactly

**Architecture & Documentation** (Principle II):
- [x] T110 Update `ARCHITECTURE.md` with service requirements concept and extended Agent entity
- [x] T111 Update `ARCHITECTURE.md` Glossary with domain terms: ServiceRequirement, RequirementType, Mandatory Service, Optional Service
- [x] T112 [P] Verify no new ADR needed (follows ADR 004 for JSONB storage)

**Configuration** (Principle VII):
- [x] T113 [P] Verify configuration uses existing unified system configuration (no custom loading)

**Database & Persistence** (Principle IX):
- [x] T114 [P] Verify migration files in `migrations/` follow sequential numbering: 005_add_agent_service_requirements.{up,down}.sql
- [x] T115 [P] Verify integration tests test migrations (apply, rollback, data integrity) in `tests/integration/infra/agent_service_requirements_migration_test.go`
- [x] T116 [P] Verify PostgreSQL repository integration tests pass in `tests/integration/infra/agent_repository_service_requirements_test.go`
- [x] T117 Verify persistence entities follow `specs/004-persistence-layer/quickstart.md` patterns (JSONB with sqlx)

**Security** (Principles I, III):
- [x] T118 Verify security features enabled by default: authorization fail-closed (FR-012), redirect_uri validation (SR-001, SR-002)
- [x] T119 [P] Verify no custom cryptography - uses net/url standard library for redirect_uri validation
- [x] T120 [P] Verify structured logging added for security-critical operations (SR-004)

**Architecture Patterns** (Principle VI):
- [x] T121 Verify domain logic uses ports and adapters: Agent validation in domain layer, HTTP handlers in adapters layer

**Testing** (Principle VIII - Unit & Integration Tests):
- [x] T122 Verify unit tests written FIRST and failed before implementation (documented in Phase 3-8 checkpoints)
- [x] T123 Verify tests drive design (implementation emerged from test requirements)
- [x] T124 Verify tests changed minimally during implementation (fixture adjustments only)
- [x] T125 Verify automated tests included: unit tests in `internal/domain/storage/`, integration tests in `tests/integration/`
- [x] T126 Verify no Bash scripts used for code correctness validation

**E2E Acceptance Testing** (Principle XIII):
- [x] T127 Verify E2E tests exist in `tests/e2e/agent_permission_requirements_test.go` for all 34 acceptance scenarios from spec.md plus edge cases
- [x] T128 Verify each `It()` block maps to exactly ONE acceptance scenario from spec.md (scenario mapping documented)
- [x] T129 Verify E2E tests written BEFORE implementation and failed initially (red phase)
- [x] T130 Verify E2E tests changed minimally during implementation (fixture adjustments only)
- [x] T131 Verify E2E tests turned GREEN as implementation satisfied acceptance criteria
- [x] T132 Verify E2E tests use Ginkgo/Gomega framework following `tests/e2e/README.md` patterns
- [x] T133 Verify E2E test organization uses hierarchical structure: Describe (feature) → Context (preconditions) → It (scenario)
- [x] T134 Verify E2E tests include comment references to spec.md scenarios
- [x] T135 Run full E2E test suite: `ginkgo -v ./tests/e2e/` (all tests must pass)

**Frontend** (Principle XI):
- [x] T136 Verify frontend components use design system primitives: Card, CardHeader, CardBody, Badge, Button
- [x] T137 Verify semantic tokens used: trust-deep (required), neutral (optional), success-primary (active session)
- [x] T138 Verify ServiceRequirementCard and ServiceRequirementsList added as universal components to design system (if applicable)
- [x] T139 Verify no custom CSS bypassing design tokens
- [x] T140 Verify WCAG 2.1 AA accessibility compliance: 4.5:1 text contrast, 3:1 UI component contrast

### Additional Polish

- [x] T141 Run `just fmt` to format all Go code
- [x] T142 Run `just vet` for static analysis
- [x] T143 Run `just lint` to check code quality
- [x] T144 Run `just verify` to execute the full verification gate
- [x] T145 Run `just check` (fmt, vet, lint) and verify static checks pass
- [x] T146 [P] Build frontend with `just web-build` and verify no errors
- [x] T147 [P] Run frontend tests with `cd web && npm test` and verify all pass
- [x] T148 Code cleanup and refactoring for readability
- [x] T149 Performance review: verify authorization validation < 200ms p95
- [x] T150 Run quickstart.md validation steps from `specs/011-agent-permission-requirements/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Design Preconditions (Phase 2)**: Depends on Setup completion - BLOCKS all implementation
  - **CRITICAL**: Domain model must be designed and documented ✅ (already complete in data-model.md)
  - **CRITICAL**: Configuration requirements must be reviewed ✅ (no changes needed)
  - **CRITICAL**: APIs must be designed and documented ✅ (already complete in contracts/openapi-changes.md)
  - **CRITICAL**: Database schema must be designed ✅ (migration 005 documented in data-model.md)
  - **CRITICAL**: Design system reviewed ✅ (components identified in quickstart.md)
  - **CRITICAL**: E2E acceptance tests must be written for all 37 spec scenarios and verified to FAIL (T009-T009j)
  - Phase 2a-2f tasks verify design artifacts that already exist - can proceed in parallel
- **Foundational Infrastructure (Phase 2.5)**: Depends on ALL of Phase 2 completion - BLOCKS all user stories
  - Database migration MUST be applied before any user story work
- **User Stories (Phase 3-8)**: All depend on Phase 2 + Phase 2.5 completion
  - Phase 3 (US1 - P1): Can start after Phase 2.5 - No dependencies on other stories
  - Phase 4 (US2 - P1): Can start after Phase 2.5 - No dependencies on other stories
  - Phase 5 (US6 - P1): Can start after Phase 2.5 - No dependencies on other stories
  - Phase 6 (US3 - P2): Depends on Phase 4 (US2) for backend service requirements endpoint
  - Phase 7 (US4 - P2): Depends on Phase 6 (US3) for ServiceRequirementCard component
  - Phase 8 (US5 - P2): Depends on Phase 6 (US3) for ConsentPage component
  - **MVP Scope**: Phases 3, 4, 5 (US1, US2, US6 - all P1 stories) constitute the MVP
- **Polish (Phase 9)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1 - Phase 3)**: Administrator configuration - INDEPENDENT
- **User Story 2 (P1 - Phase 4)**: Authorization validation - INDEPENDENT (requires US1 data but testable independently)
- **User Story 6 (P1 - Phase 5)**: Redirect URL support - INDEPENDENT
- **User Story 3 (P2 - Phase 6)**: Consent screen display - Depends on US2 backend endpoint
- **User Story 4 (P2 - Phase 7)**: Read-only scopes - Depends on US3 component structure
- **User Story 5 (P2 - Phase 8)**: Simplified UI - Depends on US3 component structure

### Within Each User Story

- Tests MUST be written and FAIL before implementation (TDD)
- Domain layer before adapters
- Backend before frontend (for US3-5)
- Unit tests before integration tests
- E2E tests verify story completion

### Parallel Opportunities

**Phase 1 (Setup)**: All tasks can run in parallel

**Phase 2 (Design Preconditions)**: All verification tasks (T004-T009) can run in parallel - verifying existing design artifacts

**Phase 2.5 (Foundational)**: T010-T011 (migration files) can be created in parallel

**Phase 3 (US1)**: 
- T017-T021 (all test files) can run in parallel
- T023-T024 (RequirementType, ServiceRequirement) can run in parallel
- T026-T027 (memory and PostgreSQL repositories) can run in parallel
- T032-T033 (OpenAPI spec, logging) can run in parallel

**Phase 4 (US2)**:
- T037-T038 (test files) can run in parallel
- T040-T041 (helper functions) can run in parallel

**Phase 5 (US6)**:
- T049-T050 can run sequentially (tests first)
- T051-T054 (validation function) can run in parallel after tests

**Phase 6 (US3)**:
- T062-T063 (backend and frontend tests) can run in parallel
- T065-T068 (backend implementation) and T069-T070 (frontend components) can run in parallel

**Phase 7 (US4)**:
- T083-T087 (scope display updates) can run in parallel within frontend

**Phase 9 (Polish)**:
- T107-T108 (OpenAPI updates) can run in parallel
- T110-T112 (architecture docs) can run in parallel
- T114-T117 (database verification) can run in parallel
- T118-T120 (security verification) can run in parallel
- T141-T145 (code quality checks + verification) should run sequentially (`just verify` depends on a clean codebase)
- T146-T147 (frontend build and tests) can run in parallel

---

## Parallel Example: User Story 1 (P1 - MVP)

```bash
# Launch all test files for User Story 1 together:
Task T017: "Write unit tests in internal/domain/storage/requirement_type_test.go"
Task T018: "Write unit tests in internal/domain/storage/service_requirement_test.go"
Task T019: "Write unit tests in internal/domain/storage/agent_test.go"
Task T020: "Write integration tests in tests/integration/agent_repository_service_requirements_memory_test.go and tests/integration/infra/agent_repository_service_requirements_test.go"
Task T021: "Write unit tests in internal/adapters/http/handlers/admin/agent_handler_test.go"

# After tests written, launch domain layer components together:
Task T023: "Create RequirementType enum in internal/domain/storage/requirement_type.go"
Task T024: "Create ServiceRequirement value object in internal/domain/storage/service_requirement.go"

# After domain layer, launch repository adapters together:
Task T026: "Extend memory repository in internal/adapters/storage/memory/agent_repository.go"
Task T027: "Extend PostgreSQL repository in internal/adapters/storage/postgres/agent_repository.go"
```

---

## Implementation Strategy

### MVP First (P1 User Stories: US1, US2, US6)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Design Preconditions (T004-T009j) - Verify existing design + write E2E tests
3. Complete Phase 2.5: Foundational Infrastructure (T010-T016) - Database migration
4. Complete Phase 3: User Story 1 - Admin configuration (T017-T036)
5. Complete Phase 4: User Story 2 - Authorization validation (T037-T048)
6. Complete Phase 5: User Story 6 - Redirect URL (T049-T061)
7. **STOP and VALIDATE**: Test all P1 stories independently - MVP functional
8. Deploy/demo MVP if ready

### Incremental Delivery Beyond MVP (P2 User Stories: US3, US4, US5)

After MVP validation:

1. Add Phase 6: User Story 3 - Consent screen display (T062-T080)
2. Test independently → Deploy/Demo
3. Add Phase 7: User Story 4 - Read-only scopes (T081-T090)
4. Test independently → Deploy/Demo
5. Add Phase 8: User Story 5 - Simplified UI (T091-T099)
6. Test independently → Deploy/Demo
7. Complete Phase 9: Constitution compliance and polish (T100-T150)

### Parallel Team Strategy

With multiple developers or agents:

1. **Phase 2 (Design Preconditions)** - Verification in parallel:
   - Agent 1: Verify domain model and update ARCHITECTURE.md (T004-T004c)
   - Agent 2: Verify configuration and database design (T005-T007d)
   - Agent 3: Verify API contracts and design system (T008-T008c)
   - Agent 4: Write E2E tests for all 37 scenarios (T009-T009j) - **CRITICAL PATH**

2. **Phase 2.5 (Foundational)** - All agents collaborate:
   - Create migration files together (T010-T016)

3. **Phase 3-5 (MVP - P1 Stories)** - Parallel story development:
   - Agent 1 (Backend specialist): User Story 1 (T017-T036) - Admin API
   - Agent 2 (Backend specialist): User Story 2 (T037-T048) - Authorization
   - Agent 3 (Backend specialist): User Story 6 (T049-T061) - Redirect URL

4. **Phase 6-8 (P2 Stories)** - Parallel frontend development:
   - Agent 1 (Full-stack): User Story 3 (T062-T080) - Backend + Frontend display
   - Agent 2 (Frontend specialist): User Story 4 (T081-T090) - Read-only scopes
   - Agent 3 (Frontend specialist): User Story 5 (T091-T099) - Simplified UI

5. **Phase 9 (Compliance)** - All agents verify constitution requirements together

---

## Notes

- [P] tasks = different files, no dependencies within same user story
- [Story] label (US1-US6) maps task to specific user story for traceability
- Each user story should be independently completable and testable
- E2E tests with 34 scenarios MUST be written BEFORE implementation (T009-T009j)
- Verify tests fail (red phase) before implementing each story
- MVP = User Stories 1, 2, 6 (all P1 stories) - complete authorization flow
- P2 stories (US3, US4, US5) enhance consent screen UX incrementally
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Constitution compliance verification (Phase 9) is MANDATORY before feature completion

---

## Summary

**Total Tasks**: 150 tasks across 9 phases  
**MVP Scope**: Phases 1-5 (61 tasks) - US1, US2, US6 (P1 stories)  
**Full Feature**: All phases (150 tasks) - All 6 user stories  
**E2E Test Coverage**: 34 acceptance scenarios from spec.md (plus edge cases)  
**Parallel Opportunities**: 40+ tasks can run in parallel with proper team coordination  
**Independent Test Criteria**: Each user story has clear acceptance criteria and can be tested in isolation
