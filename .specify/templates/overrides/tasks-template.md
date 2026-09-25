---

description: "Task list template for feature implementation"
---

# Tasks: [FEATURE NAME]

**Input**: Design documents from `/specs/[###-feature-name]/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY for all features. Test tasks are included in each user story below and MUST be written before or alongside implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Template Structure

This template contains two types of sections:

### 🔒 MANDATORY SECTIONS (Constitution-Required)
These sections implement project constitution requirements and MUST be included in every tasks.md:
- **Phase 2: Design Preconditions** - Maps to constitution PRECONDITIONS checklist
- **Phase N: Constitution Compliance** - Maps to constitution Implementation Phase checklist

### 📝 CUSTOMIZABLE SECTIONS (Adapt to Your Feature)
These sections provide examples that should be replaced with feature-specific tasks:
- Phase 1: Setup
- Phase 2.5: Foundational Infrastructure
- Phase 3+: User Stories (adapt based on spec.md user stories)

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Single project**: `src/`, `tests/` at repository root
- **Web app**: `backend/src/`, `frontend/src/`
- **Mobile**: `api/src/`, `ios/src/` or `android/src/`
- Paths shown below assume single project - adjust based on plan.md structure

---

## Phase 0: Pre-implementation Refactoring [OPTIONAL]

**Purpose**: Refactor existing code to prepare for the new feature, keeping structural changes
isolated from feature work for clean, reviewable PRs.

**⚠️ WHEN TO INCLUDE**: Include this phase only when the feature requires significant changes to
existing code — rename, restructure, extract interfaces, split files, consolidate duplicates.
Skip entirely if the feature only adds new code without touching existing structures.

**📝 REVIEW DISCIPLINE**: Refactoring commits MUST be submitted as a SEPARATE PR (or isolated
commit batch) from feature implementation. This keeps reviewers focused: first approve structural
changes with no behavior change, then review new business logic separately.

<!--
  CUSTOMIZABLE SECTION: Replace T000 tasks with refactoring tasks specific to this feature.
  Examples: extract interface from concrete type, rename entity, split large file,
  move handler to new package, consolidate duplicate code.
  IMPORTANT: Refactoring MUST NOT change behavior — all existing tests must pass unchanged.
-->

- [ ] T000 Identify all existing code that requires refactoring before the new feature can be cleanly added
- [ ] T000a [P] [Refactor task 1] — e.g., "Extract StoragePort interface from concrete PostgresAdapter"
- [ ] T000b [P] [Refactor task 2] — e.g., "Rename UserToken → UserSession throughout codebase"
- [ ] T000c Verify all existing tests pass after refactoring (zero behavior changes)

**Checkpoint**: Refactoring complete, all existing tests pass, no behavior changes introduced

---

## Phase 1: Setup (Shared Infrastructure) [CUSTOMIZABLE]

**Purpose**: Project initialization and basic structure

<!--
  CUSTOMIZABLE SECTION: Replace T001-T003 with feature-specific setup tasks.
  These are examples only - adapt to your feature requirements.
-->

- [ ] T001 Create project structure per implementation plan
- [ ] T002 Initialize [language] project with [framework] dependencies
- [ ] T003 [P] Configure linting and formatting tools

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

**🔒 CONSTITUTION REQUIREMENT**: This phase maps directly to the constitution PRECONDITIONS checklist. All sub-phases (2a-2f) MUST be included in every tasks.md, though specific task details should be adapted to the feature.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

**Required Tasks** (adapt descriptions to your feature):
- [ ] T004 Identify all entities, aggregates, and value objects for this feature
- [ ] T004a Document domain model (entities, aggregates, value objects, domain events)
- [ ] T004b [P] Add all new domain terms to ARCHITECTURE.md Glossary section
- [ ] T004c Document invariants and consistency rules for key aggregates

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

**Required Tasks** (adapt descriptions to your feature):
- [ ] T005 Identify all configuration parameters needed for this feature
- [ ] T005a Create example YAML showing all new config options with defaults
- [ ] T005b [P] Commit configuration examples to `examples/config/[feature_name].yaml`
- [ ] T005c Update `examples/config/README.md` to reference new configuration section
- [ ] T005d [P] Update the deployment contract: update `charts/agentic-identity-broker/` values, templates, and README when it deploys the configured workload; otherwise document the accepted standalone deployment boundary and keep its settings out of the broker chart

**Checkpoint**: Configuration requirements designed with YAML examples and the applicable deployment contract documented

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

**Required Tasks** (adapt descriptions to your feature):
- [ ] T006 Design end-user API and document in `/api/enduser/openapi.yaml` (per Constitution Principle IV)
- [ ] T006a [P] Get user/stakeholder confirmation for end-user API design
- [ ] T007 [P] Design admin API in `/api/admin/openapi.yaml` if applicable (per Constitution Principle IV)
- [ ] T007a [P] Get user/stakeholder confirmation for admin API design (if applicable)

**Checkpoint**: APIs designed and confirmed by user/stakeholder

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

**Required Tasks** (adapt descriptions to your feature):
- [ ] T008 Design database schema and document migrations to create (or confirm no DB changes needed)
- [ ] T008a Create migration files: `[NNN]_[description].up.sql` and `.down.sql` (per go-migrate format)
- [ ] T008b [P] Document all schema changes (tables, indexes, constraints)

**Checkpoint**: Database schema designed, migrations documented

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

**Required Tasks** (if feature includes frontend components):
- [ ] T009 Review [web/src/design-system/docs/INDEX.md](../web/src/design-system/docs/INDEX.md) for component selection
- [ ] T009a [P] Identify which design system components to use (consult DECISION_TREES.md)
- [ ] T009b [P] Identify universal components that should be added to design system
- [ ] T009c Document semantic token usage (trust-deep, success-primary, neutral-*)

**Checkpoint**: Design system usage planned (if applicable)

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)

**Required Tasks** (adapt descriptions to your feature):
- [ ] T010 Write E2E acceptance tests in `tests/e2e/` for all acceptance scenarios from spec.md
- [ ] T010a Map each acceptance scenario from spec.md to one `It()` block in E2E tests
- [ ] T010b Use Ginkgo/Gomega framework following patterns in `tests/e2e/README.md`
- [ ] T010c Organize tests hierarchically: `Describe` (feature) → `Context` (preconditions) → `It` (scenario)
- [ ] T010d Use fixtures from `tests/e2e/fixtures/` (agents, grants, principals, config)
- [ ] T010e Add comment references to spec scenarios in E2E test files
- [ ] T010f Verify E2E tests FAIL semantically (red phase): detailed expectations present and failing,
      NOT placeholder always-fail assertions (e.g., `Expect(true).To(BeFalse())` is NOT acceptable);
      no `XIt`/`PIt`/`Skip()` pending markers; no "red phase" comments in test files
- [ ] T010g [IF FRONTEND] Add/amend Playwright E2E tests in `tests/e2e/frontend/` for all UI scenarios
- [ ] T010h [IF FRONTEND] Use existing page objects in `tests/e2e/pages/` and Playwright harness
- [ ] T010i [IF FRONTEND] Verify frontend Playwright tests capture screenshots to `tests/e2e/screenshots/`
      using descriptive filenames for each meaningful UI state

**Checkpoint**: E2E acceptance tests written and verified to fail semantically before implementation;
frontend Playwright tests added/amended and screenshots configured (if applicable)

---

## Phase 2.7: Entity Boilerplate (New Entities) [CUSTOMIZABLE - INCLUDE IF NEW ENTITIES]

**Purpose**: Create empty-but-compiling CRUD scaffolding for each new entity identified in Phase 2a
(domain model). Having this as a dedicated phase isolates structural additions from business logic,
making code review significantly easier.

**⚠️ WHEN TO INCLUDE**: Include when the feature introduces NEW domain entities requiring new ports,
storage adapters, and HTTP handlers. Skip if the feature only extends existing entities.

**📝 REVIEW DISCIPLINE**: Boilerplate commits SHOULD be in a SEPARATE PR (or isolated commit batch)
from business logic. Reviewers can quickly approve structural scaffolding (empty methods returning
501), then devote full attention to business logic in user story PRs.

<!--
  CUSTOMIZABLE SECTION: Duplicate the block below for each new entity from Phase 2a.
  Boilerplate is intentionally empty — business logic belongs in Phase 3+ user story phases.
  Empty handlers MUST compile and return 501 Not Implemented (not panic or fail to build).
-->

### Boilerplate: [EntityName] *(repeat per new entity)*

- [ ] TBPL001 Define typed ID (`type [Entity]ID uuid.UUID`) in `internal/domain/id/gen_ids.go` and
      document in `internal/domain/id/AGENTS.md` (ADR 013)
- [ ] TBPL002 Define domain struct for `[EntityName]` in `internal/domain/storage/` or appropriate
      domain package
- [ ] TBPL003 Define `[EntityName]Repository` interface in `internal/ports/storage.go` (ISP — 5-7
      methods max: Create, Get, Update, Delete, List)
- [ ] TBPL004 [P] Implement empty `[EntityName]Repository` on in-memory adapter in
      `internal/adapters/storage/memory/` (methods compile, return zero values / not-found)
- [ ] TBPL005 [P] Implement empty `[EntityName]Repository` on postgres adapter in
      `internal/adapters/storage/postgres/` (methods compile, return not-implemented error)
- [ ] TBPL006 Add `[EntityName]s() [EntityName]Repository` accessor to Adapter struct in both
      storage adapters
- [ ] TBPL007 [P] Create empty HTTP handler struct `[EntityName]Handler` in
      `internal/adapters/http/handlers/[entity]/handler.go` (CRUD methods return 501)
- [ ] TBPL008 Register empty routes for `[EntityName]` CRUD in the appropriate routing file
      (`internal/adapters/http/routing/enduser.go` or `admin.go`)
- [ ] TBPL009 Add `[EntityName]Handler` field to `app.EnduserHandlers` or `app.AdminHandlers`
      and wire it in `internal/app/builder.go`
- [ ] TBPL010 Verify project compiles with all new empty scaffolding in place (`just build`)

**Checkpoint**: All new entity scaffolding compiles, empty handlers return 501, no business logic
yet — ready for incremental user story implementation

---

## Phase 2.5: Foundational Infrastructure [CUSTOMIZABLE]

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

<!--
  CUSTOMIZABLE SECTION: Replace T009-T013 with feature-specific foundational tasks.
  These are examples only - adapt to your feature requirements.
-->

- [ ] T009 [P] Implement authentication/authorization framework
- [ ] T010 [P] Setup API routing and middleware structure
- [ ] T011 Create base models/entities that all stories depend on
- [ ] T012 Configure error handling and logging infrastructure
- [ ] T013 Setup environment configuration management

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

<!--
  ============================================================================
  CUSTOMIZABLE USER STORIES SECTION

  The sections below (Phase 3, 4, 5, etc.) are EXAMPLES for user story implementation.

  The /speckit-tasks command MUST:
  1. Replace these example user stories with actual user stories from spec.md
  2. Match user story priorities (P1, P2, P3...) from spec.md
  3. Generate feature-specific tasks based on plan.md and data-model.md
  4. Organize tasks so each story can be implemented and tested independently

  IMPORTANT: DO NOT delete Phase 2 (Design Preconditions) or Phase N (Constitution Compliance).
  Those sections are MANDATORY and must be retained in every generated tasks.md file.
  ============================================================================
-->

## Phase 3: User Story 1 - [Title] (Priority: P1) 🎯 MVP [CUSTOMIZABLE]

**Goal**: [Brief description of what this story delivers]

**Independent Test**: [How to verify this story works on its own]

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [ ] T014 [P] [US1] Unit tests for domain logic in tests/unit/[domain_entity]_test.go
- [ ] T015 [P] [US1] Integration tests for [endpoint/feature] in tests/integration/[feature]_test.go
- [ ] T016 [P] [US1] Contract tests for API endpoints (if applicable) in tests/contract/[endpoint]_test.go

### Implementation for User Story 1

- [ ] T017 [P] [US1] Create [Entity1] model in internal/domain/[entity1].go
- [ ] T018 [P] [US1] Create [Entity2] model in internal/domain/[entity2].go
- [ ] T019 [US1] Implement [Service] in internal/domain/services/[service].go (depends on T017, T018)
- [ ] T020 [US1] Implement [endpoint/feature] in internal/adapters/[location]/[file].go
- [ ] T021 [US1] Add validation and error handling per Constitution Principle I (security-first)
- [ ] T022 [US1] Add structured logging for user story 1 operations

**Checkpoint**: At this point, User Story 1 should be fully functional and testable independently

---

## Phase 4: User Story 2 - [Title] (Priority: P2) [CUSTOMIZABLE]

**Goal**: [Brief description of what this story delivers]

**Independent Test**: [How to verify this story works on its own]

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [ ] T023 [P] [US2] Unit tests for domain logic in tests/unit/[domain_entity]_test.go
- [ ] T024 [P] [US2] Integration tests for [endpoint/feature] in tests/integration/[feature]_test.go
- [ ] T025 [P] [US2] Contract tests for API endpoints (if applicable) in tests/contract/[endpoint]_test.go

### Implementation for User Story 2

- [ ] T026 [P] [US2] Create [Entity] model in internal/domain/[entity].go
- [ ] T027 [US2] Implement [Service] in internal/domain/services/[service].go
- [ ] T028 [US2] Implement [endpoint/feature] in internal/adapters/[location]/[file].go
- [ ] T029 [US2] Integrate with User Story 1 components (if needed)

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently

---

## Phase 5: User Story 3 - [Title] (Priority: P3) [CUSTOMIZABLE]

**Goal**: [Brief description of what this story delivers]

**Independent Test**: [How to verify this story works on its own]

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [ ] T030 [P] [US3] Unit tests for domain logic in tests/unit/[domain_entity]_test.go
- [ ] T031 [P] [US3] Integration tests for [endpoint/feature] in tests/integration/[feature]_test.go
- [ ] T032 [P] [US3] Contract tests for API endpoints (if applicable) in tests/contract/[endpoint]_test.go

### Implementation for User Story 3

- [ ] T033 [P] [US3] Create [Entity] model in internal/domain/[entity].go
- [ ] T034 [US3] Implement [Service] in internal/domain/services/[service].go
- [ ] T035 [US3] Implement [endpoint/feature] in internal/adapters/[location]/[file].go

**Checkpoint**: All user stories should now be independently functional

---

[Add more user story phases as needed, following the same pattern]

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and final polish

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

**These tasks MUST be included in every generated tasks.md file. They verify that all constitution principles have been followed.**

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist - verify Phase 2 tasks were completed correctly

- [ ] TXXX Verify domain model design is documented in ARCHITECTURE.md Glossary (Principle V)
- [ ] TXXX Verify configuration design YAML examples exist in `examples/config/` (Principle VII)
- [ ] TXXX Verify configuration examples referenced in `examples/config/README.md` (Principle VII)
- [ ] TXXX Verify API designs documented in `/api/enduser/openapi.yaml` or `/api/admin/openapi.yaml` (Principles IV, X)
- [ ] TXXX Verify user/stakeholder confirmed API designs (document reference in PR) (Principle X)
- [ ] TXXX Verify database schema design documented (migrations or confirmation of no DB changes) (Principle IX)
- [ ] TXXX [IF FRONTEND] Verify design system review completed and universal components identified (Principle XI)
- [ ] TXXX Verify E2E acceptance tests written in `tests/e2e/` for all spec scenarios (Principle XIII)
- [ ] TXXX Verify E2E tests verified to FAIL before implementation (red phase): detailed expectations
      written and failing; no placeholder always-fail assertions; no `XIt`/`PIt`/`Skip()` markers;
      no "red phase" comments in test files (Principle XIII)
- [ ] TXXX [IF FRONTEND] Verify Playwright E2E tests added/amended in `tests/e2e/frontend/` (Principle XIII)
- [ ] TXXX [IF FRONTEND] Verify screenshots captured to `tests/e2e/screenshots/` (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist - verify all principles followed during implementation

**API & Documentation** (Principles IV, X):
- [ ] TXXX [P] Update `/api/enduser/openapi.yaml` or `/api/admin/openapi.yaml` with implemented APIs
- [ ] TXXX [P] Verify API implementation matches confirmed OpenAPI specification exactly
- [ ] TXXX Update `docs/api/` with end-user API documentation and integration examples (if applicable)

**Architecture & Documentation** (Principle II):
- [ ] TXXX Update ARCHITECTURE.md with architectural changes and new domain concepts
- [ ] TXXX Update ARCHITECTURE.md Glossary with new domain terms (if not done in design phase)
- [ ] TXXX [P] Create/update ADR in adrs/ for major architectural decisions

**Configuration** (Principle VII):
- [ ] TXXX [P] Verify configuration uses the configuration system owned by its deployable application; standalone binaries require an accepted ADR for a separate configuration path
- [ ] TXXX [IF CONFIG CHANGED] Verify Helm values, templates, and README reflect all changed configuration for chart-managed workloads, or verify the accepted ADR and documented standalone deployment boundary (Principle VII)

**Database & Persistence** (Principle IX):
- [ ] TXXX [P] Create/verify database migrations in `/migrations/` follow sequential numbering (NNN format)
- [ ] TXXX [P] Write/update integration tests verifying migrations (apply, rollback, data integrity)
- [ ] TXXX [P] Verify all PostgreSQL-backed repositories tested in integration tests
- [ ] TXXX Verify persistence entities follow quickstart.md patterns (if applicable)

**Security** (Principles I, III):
- [ ] TXXX Verify security features are enabled by default (no optional bypasses)
- [ ] TXXX [P] Verify no custom cryptography (only vetted libraries used)
- [ ] TXXX [P] Add structured logging for security-critical operations

**Architecture Patterns** (Principle VI):
- [ ] TXXX Verify domain logic uses ports (interfaces) and adapters

**Testing** (Principle VIII - Unit & Integration Tests):
- [ ] TXXX Verify unit tests written FIRST and failed before implementation (red-green TDD)
- [ ] TXXX Verify tests drive design (implementation emerges from test requirements)
- [ ] TXXX Verify tests changed minimally during implementation
- [ ] TXXX Verify automated tests included (unit, integration, or both)
- [ ] TXXX Verify no Bash scripts used for code correctness validation

**E2E Acceptance Testing** (Principle XIII):
- [ ] TXXX Verify E2E tests exist in `tests/e2e/` for all acceptance scenarios from spec.md
- [ ] TXXX Verify each `It()` block in E2E tests maps to exactly ONE acceptance scenario from spec.md
- [ ] TXXX Verify E2E tests written BEFORE implementation and failed initially (red phase)
- [ ] TXXX Verify E2E tests changed minimally during implementation (fixture adjustments only)
- [ ] TXXX Verify E2E tests turned GREEN as implementation satisfied acceptance criteria
- [ ] TXXX Verify E2E tests use Ginkgo/Gomega framework following `tests/e2e/README.md` patterns
- [ ] TXXX Verify E2E test organization uses hierarchical structure (Describe → Context → It)
- [ ] TXXX Verify E2E tests include comment references to spec scenarios
- [ ] TXXX Run full E2E test suite: `ginkgo -v ./tests/e2e/` (all tests must pass)
- [ ] TXXX [IF FRONTEND] Verify Playwright E2E tests in `tests/e2e/frontend/` pass
- [ ] TXXX [IF FRONTEND] Verify screenshots saved to `tests/e2e/screenshots/` with descriptive filenames
- [ ] TXXX [IF FRONTEND] Run frontend E2E suite: `ginkgo -v ./tests/e2e/frontend/` (all tests must pass)

**Frontend** (Principle XI - if applicable):
- [ ] TXXX [IF FRONTEND] Verify frontend components use design system primitives and semantic tokens
- [ ] TXXX [IF FRONTEND] Verify universal components added to design system with Storybook stories
- [ ] TXXX [IF FRONTEND] Verify no custom CSS bypassing design tokens
- [ ] TXXX [IF FRONTEND] Verify WCAG 2.1 AA accessibility compliance (4.5:1 text, 3:1 UI contrast)

### Additional Polish [CUSTOMIZABLE]

<!--
  CUSTOMIZABLE SECTION: Add feature-specific polish tasks here.
  These are optional and can be adapted or removed based on feature needs.
-->

- [ ] TXXX Code cleanup and refactoring
- [ ] TXXX Performance optimization across all stories
- [ ] TXXX [P] Additional unit tests (if requested) in tests/unit/
- [ ] TXXX Run quickstart.md validation (if exists)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Pre-implementation Refactoring (Phase 0)**: OPTIONAL — can start immediately if needed;
  MUST be complete (all existing tests pass) before Phase 2 (Design Preconditions) begins
- **Setup (Phase 1)**: No dependencies — can start immediately (parallel with Phase 0 if Phase 0 is included)
- **Design Preconditions (Phase 2)**: Depends on Setup completion - BLOCKS all implementation
  - **CRITICAL**: Domain model must be designed and documented
  - **CRITICAL**: Configuration requirements must be designed with YAML examples
  - **CRITICAL**: APIs must be designed and confirmed by user/stakeholder BEFORE implementation
  - **CRITICAL**: Database schema must be designed and migrations planned (or confirmed no DB changes)
  - **CRITICAL**: E2E acceptance tests must be written with detailed expectations and verified to FAIL
    semantically (red phase); frontend Playwright tests added/amended if UI changes involved
  - **CRITICAL**: Helm deployment contract must be updated when configuration changes affect a chart-managed workload; otherwise the accepted standalone deployment boundary must be documented
  - Phase 2a, 2b, 2c, 2d, 2e, 2f can proceed in parallel, but all must complete before Phase 2.7 begins
- **Entity Boilerplate (Phase 2.7)**: OPTIONAL (only if new entities) — depends on completion of all Phase 2 design tasks (2a–2f);
  produces empty scaffolding BEFORE business logic; enables clean review
- **Foundational Infrastructure (Phase 2.5)**: Depends on ALL of Phase 2 + Phase 2.7 (if included) completion - BLOCKS all user stories
- **User Stories (Phase 3+)**: All depend on Phase 2 + Phase 2.5 + Phase 2.7 (if included) completion
  - User stories can then proceed in parallel (if staffed)
  - Or sequentially in priority order (P1 → P2 → P3)
- **Polish (Final Phase)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2 + Phase 2.5 completion - No dependencies on other stories
- **User Story 2 (P2)**: Can start after Phase 2 + Phase 2.5 completion - May integrate with US1 but should be independently testable
- **User Story 3 (P3)**: Can start after Phase 2 + Phase 2.5 completion - May integrate with US1/US2 but should be independently testable

### Within Each User Story

- Tests (if included) MUST be written and FAIL before implementation
- Models before services
- Services before endpoints
- Core implementation before integration
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tasks marked [P] can run in parallel (within Phase 2)
- Once Foundational phase completes, all user stories can start in parallel (if team capacity allows)
- All tests for a user story marked [P] can run in parallel
- Models within a story marked [P] can run in parallel
- Different user stories can be worked on in parallel by different team members

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together (if tests requested):
Task: "Contract test for [endpoint] in tests/contract/test_[name].py"
Task: "Integration test for [user journey] in tests/integration/test_[name].py"

# Launch all models for User Story 1 together:
Task: "Create [Entity1] model in src/models/[entity1].py"
Task: "Create [Entity2] model in src/models/[entity2].py"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. **(If needed) Complete Phase 0**: Pre-implementation refactoring — separate PR, all tests green
2. Complete Phase 1: Setup
3. Complete Phase 2: Design Preconditions (CRITICAL - blocks all stories)
   - 2a: Domain model & glossary
   - 2b: Configuration design with YAML examples
   - 2c: API design + user/stakeholder confirmation
   - 2d: Database schema design
   - 2e: Frontend/design system review (if applicable)
   - 2f: E2E acceptance test design (tests written and verified to FAIL)
4. **(If new entities) Complete Phase 2.7**: Entity boilerplate — separate PR, project compiles
5. Complete Phase 2.5: Foundational Infrastructure (CRITICAL - blocks all stories)
6. Complete Phase 3: User Story 1
7. **STOP and VALIDATE**: Test User Story 1 independently
8. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup → Design Preconditions (all 4 areas) → Foundational → Foundation ready
2. Add User Story 1 → Test independently → Deploy/Demo (MVP!)
3. Add User Story 2 → Test independently → Deploy/Demo
4. Add User Story 3 → Test independently → Deploy/Demo
5. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers or agents:

1. Parallel design work (Phase 2) - use specialized sub-agents:
   - **Phase 2a (Domain Model)**: Use architecture-reviewer or domain-architect specialist
   - **Phase 2b (Configuration Design)**: Use backend-developer or platform-engineer specialist
   - **Phase 2c (API Design)**: Use api-designer specialist (coordinates with user for confirmation)
   - **Phase 2d (Database Schema)**: Use database-administrator specialist
   - **Phase 2e (Frontend/Design System)**: Use frontend-developer or ui-designer specialist (if applicable)
   - **Phase 2f (E2E Acceptance Tests)**: Use qa-expert or test engineer specialist (writes failing tests)
2. Remaining team/agents complete Setup (Phase 1) while design in progress
3. Once ALL of Phase 2 complete and confirmed (especially API sign-off):
   - All team/agents complete Foundational Infrastructure (Phase 2.5) together
4. Once Phase 2.5 done, assign user stories to specialized agents:
   - **User Story 1**: golang-pro or feature-lead or frontend-developer
   - **User Story 2**: another agent like in User Story 1
   - **User Story 3**: another agent like in User Story 1
5. Stories complete and integrate independently

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence
