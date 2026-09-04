# Implementation Plan: [FEATURE]

**Branch**: `[###-feature-name]` | **Date**: [DATE] | **Spec**: [link]

**Input**: Feature specification from `/specs/[###-feature-name]/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

[Extract from feature spec: primary requirement + technical approach from research]

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: [e.g., Python 3.11, Swift 5.9, Rust 1.75 or NEEDS CLARIFICATION]
**Primary Dependencies**: [e.g., FastAPI, UIKit, LLVM or NEEDS CLARIFICATION]
**Storage**: [if applicable, e.g., PostgreSQL, CoreData, files or N/A]
**Testing**: [e.g., pytest, XCTest, cargo test or NEEDS CLARIFICATION]
**Target Platform**: [e.g., Linux server, iOS 15+, WASM or NEEDS CLARIFICATION]
**Project Type**: [single/web/mobile - determines source structure]
**Performance Goals**: [domain-specific, e.g., 1000 req/s, 10k lines/sec, 60 fps or NEEDS CLARIFICATION]
**Constraints**: [domain-specific, e.g., <200ms p95, <100MB memory, offline-capable or NEEDS CLARIFICATION]
**Scale/Scope**: [domain-specific, e.g., 10k users, 1M LOC, 50 screens or NEEDS CLARIFICATION]

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [ ] **Domain Model**: Have entities, aggregates, value objects been identified and documented?
- [ ] **Domain Concepts**: Will new domain terms be added to ARCHITECTURE.md Glossary?
- [ ] **Entity IDs**: For each new domain entity with a UUID primary key, will a typed ID (`type XxxID uuid.UUID`) be added to `internal/domain/id/` via `gen_ids.go` and documented in `internal/domain/id/AGENTS.md`? (ADR 013)
- [ ] **Configuration Design**: Have all config requirements been identified with YAML examples?
- [ ] **Config Examples**: Will example YAML snippets be added to examples/config/?
- [ ] **Helm Deployment Contract**: If configuration changes affect a workload deployed by `charts/agentic-identity-broker/`, will its values, templates, and README be updated? Otherwise, is the standalone binary's accepted ADR boundary documented and kept out of the broker chart?
- [ ] **API Design First**: Will APIs be designed (OpenAPI spec) and confirmed BEFORE implementation?
- [ ] **API Documentation**: Will OpenAPI specs be created in `/api/enduser/` or `/api/admin/` as applicable?
- [ ] **API Changes**: Are all API changes confirmed by user/stakeholder (document in PR)?
- [ ] **Database Design**: Will all schema changes use go-migrate naming in `/migrations/`?
- [ ] **E2E Acceptance Tests**: Will E2E tests be written for ALL spec scenarios BEFORE implementation?
- [ ] **E2E Test Mapping**: Will each acceptance scenario map 1:1 to one It() block in tests/e2e/?
- [ ] **E2E Red Phase**: Will E2E tests FAIL initially with detailed, realistic expectations (not placeholder assertions)?
- [ ] **Frontend Playwright E2E**: If this feature changes the React UI, will Playwright E2E tests be added/amended in `tests/e2e/frontend/`?
- [ ] **Frontend Screenshots**: If this feature changes the React UI, will screenshots be captured to `tests/e2e/screenshots/` for each UI state under test?

**Implementation Considerations**:

- [ ] **Security-First**: Are security features enabled by default? No bypasses or optional security?
- [ ] **Architecture Docs**: Will ARCHITECTURE.md be updated if this touches architecture?
- [ ] **ADRs**: Does this require an ADR in adrs/ for major decisions?
- [ ] **Library-First Security**: Are we using vetted libraries for crypto/security (no custom implementations)?
- [ ] **Zalando Guidelines**: Will APIs follow Zalando RESTful API and Event Guidelines?
- [ ] **End-User Docs**: Will API documentation be rendered in `docs/api/` with examples?
- [ ] **Migration Testing**: Will migrations be tested (apply/rollback) in PostgreSQL integration tests?
- [ ] **Hexagonal Architecture**: Does domain logic use ports (interfaces) with clear adapter separation?
- [ ] **Persistence Patterns**: If adding persistence, will it follow specs/004-persistence-layer/quickstart.md?

*If any BLOCKING check fails, stop and clarify requirements. Implementation cannot begin until all preconditions complete.*

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->

```text
# [REMOVE IF UNUSED] Option 1: Single project (DEFAULT)
src/
├── models/
├── services/
├── cli/
└── lib/

tests/
├── contract/
├── integration/
└── unit/

# [REMOVE IF UNUSED] Option 2: Web application (when "frontend" + "backend" detected)
backend/
├── src/
│   ├── models/
│   ├── services/
│   └── api/
└── tests/

frontend/
├── src/
│   ├── components/
│   ├── pages/
│   └── services/
└── tests/

# [REMOVE IF UNUSED] Option 3: Mobile + API (when "iOS/Android" detected)
api/
└── [same as backend above]

ios/ or android/
└── [platform-specific structure: feature modules, UI flows, platform tests]
```

**Structure Decision**: [Document the selected structure and reference the real
directories captured above]

## Implementation Phase Overview

*Detailed task breakdown is in `tasks.md` (generated by `/speckit-tasks`). The table below reflects
the standard phase structure — include or omit optional phases based on this feature's needs.*

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring — isolate structural changes from feature work | Optional |
| **Phase 1** | Setup — project init, dependencies | Customizable |
| **Phase 2** | Design Preconditions (domain model, config, API, DB, E2E tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate — empty CRUD handlers & repositories, isolated from business logic | If new entities |
| **Phase 2.5** | Foundational Infrastructure — feature-specific foundation code | Customizable |
| **Phase 3+** | User Stories — business logic per priority | Customizable |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

**Review ergonomics rationale**: Phases 0 and 2.7 exist to keep PRs focused and reviewable:
- **Phase 0 PR** (if needed): contains only refactoring with no behavior change — reviewers approve
  structural changes first without needing to understand new feature intent
- **Phase 2.7 PR** (if new entities): contains only empty scaffolding (501 handlers, interface stubs)
  — reviewers approve skeleton quickly, then review business logic in sharp focus

*Document which phases apply to this feature and why any optional phases are included or skipped:*

- [ ] Phase 0 (refactoring): [include/skip — reason]
- [ ] Phase 2.7 (entity boilerplate): [include/skip — reason]

## Testing Strategy

<!--
  Per Constitution Principle XIII (End-to-End Acceptance Testing & Spec Traceability):
  All features MUST have E2E acceptance tests mapped 1:1 to spec scenarios.
  Frontend UI changes additionally require Playwright E2E tests and screenshots.

  This section documents HOW E2E tests will be structured and implemented for this feature.
-->

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/[feature]_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: Feature name (e.g., "OAuth2 Authorization Endpoint")
- **Nested Describe/Context**: Preconditions and scenarios (e.g., "when a valid request arrives" → "and no grant exists")
- **It blocks**: Individual acceptance scenarios (one It() per scenario from spec.md)

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| [User Story 1, Scenario 1] | `tests/e2e/[feature]_test.go:XX` | `It("should [behavior]", ...)` |
| [User Story 1, Scenario 2] | `tests/e2e/[feature]_test.go:YY` | `It("should [behavior]", ...)` |
| [User Story 2, Scenario 1] | `tests/e2e/[feature]_test.go:ZZ` | `It("should [behavior]", ...)` |

*Note: Populate this table during Phase 2f (E2E Acceptance Test Design) with actual line numbers and test descriptions*

**Red Phase Requirements**:
- E2E tests MUST compile and contain detailed, realistic expectations
- Assertions MUST target actual system output (e.g., `Expect(resp.StatusCode).To(Equal(200))`,
  `Expect(body).To(ContainSubstring("expected_field"))`)
- Placeholder always-fail assertions (e.g., `Expect(true).To(BeFalse())`) do NOT satisfy red phase —
  the failure must come from realistic assertions against the feature's expected behavior
- `XIt`, `PIt`, `XDescribe`, `PDescribe`, `XContext`, `PContext`, and `Skip()` are FORBIDDEN —
  all tests MUST run and fail
- Tests MUST NOT contain comments marking them as "in the red phase" — tests turn green naturally
  as implementation progresses and MUST NOT require annotation cleanup later

**Test Data Strategy**:
- Use fixtures from `tests/e2e/fixtures/` for stable, reusable test data
- Required fixtures: [list which fixtures are needed: agents, grants, principals, config, etc.]
- New fixture creation: [document any new fixtures that need to be created for this feature]

**Test Execution Flow**:
1. **Phase 2f (Design)**: Write E2E tests for all spec scenarios with detailed expectations
2. **Verify Red Phase**: Run `ginkgo -v ./tests/e2e/[feature]_test.go` — all tests must FAIL semantically
3. **Implementation**: Implement feature incrementally
4. **Verify Green Phase**: E2E tests turn GREEN as implementation satisfies acceptance criteria
5. **Minimal Changes**: Only fixture adjustments during implementation, not test logic

**Bootstrap Strategy**:
- Tests use production bootstrap code via `tests/e2e/bootstrap/` (app.Builder, HTTP server, routing)
- Fresh server and storage for each test (BeforeEach/AfterEach isolation)
- [Document any feature-specific bootstrap requirements]

**Helper Utilities**:
- Custom matchers needed: [list any new matchers required, or reference existing in tests/e2e/matchers/]
- HTTP helpers: [reference existing helpers in tests/e2e/helpers/ or document new ones]
- Mock services: [document any mock upstream services needed]

### Frontend Playwright E2E Tests (if UI changes)

<!--
  INCLUDE THIS SECTION if the feature changes any React UI in web/src/.
  Remove this section if the feature is purely backend with no UI changes.
-->

**Test Location**: `tests/e2e/frontend/[feature]_test.go`

**Framework**: Playwright via existing harness in `tests/e2e/frontend/` and page objects in `tests/e2e/pages/`

**Screenshot Location**: `tests/e2e/screenshots/`

**UI Scenario Mapping**:

| UI Scenario | Playwright Test Location | Screenshot Filename |
|-------------|--------------------------|---------------------|
| [Scenario description] | `tests/e2e/frontend/[feature]_test.go:XX` | `tests/e2e/screenshots/[descriptive_name].png` |

**Screenshot Naming Convention**:
- Use descriptive filenames that identify the UI state (e.g., `consent_page_with_github_scopes.png`,
  `revoke_dialog_open_from_detail_page.png`)
- One screenshot per meaningful UI state; capture before and after key interactions if relevant

**Test Execution**:
- Run frontend E2E suite: `ginkgo -v ./tests/e2e/frontend/`
- Screenshots are automatically saved during test runs to `tests/e2e/screenshots/`
- Frontend E2E tests MUST follow the same red-green discipline: write tests first with detailed
  element assertions, verify they fail before implementing the UI

### Unit & Integration Tests

**Unit Tests**:
- Location: `internal/[domain]/[entity]_test.go`
- Coverage: Domain logic, business rules, validation
- Strategy: TDD - write tests FIRST, verify they FAIL, then implement

**Integration Tests**:
- Location: `internal/adapters/[adapter]_test.go`
- Coverage: Database operations, external service integration
- Strategy: Real PostgreSQL via testcontainers, verify migrations

**Test Coverage Goals**:
- Unit test coverage: [target percentage or "critical paths only"]
- Integration test coverage: [specific adapters/repositories to test]
- E2E test coverage: 100% of acceptance scenarios from spec.md (mandatory per Principle XIII)
- Frontend E2E coverage: all UI acceptance scenarios (mandatory per Principle XIII if UI changes)

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
