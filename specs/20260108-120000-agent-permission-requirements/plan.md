# Implementation Plan: Agent Permission Requirements

**Branch**: `20260108-120000-agent-permission-requirements` | **Date**: 2026-01-08 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20260108-120000-agent-permission-requirements/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

This feature extends the Agent entity to capture service requirements (mandatory and optional third-party OAuth2 services with required scopes). The authorization flow validates that all mandatory requirements are met before proxying to the upstream OAuth2 server. The consent screen displays service requirements with clear mandatory/optional distinction, read-only scopes with descriptions, and supports seamless redirect after approval. The implementation follows hexagonal architecture with domain validation, PostgreSQL storage via migrations, and comprehensive E2E testing.

## Technical Context

**Language/Version**: Go 1.23.0+ (from go.mod)  
**Primary Dependencies**: 
- chi v5.2.3 (HTTP router)
- sqlx v1.3.5+ (PostgreSQL adapter) 
- pgx v5 (PostgreSQL driver)
- React 18+ (Consent frontend)
- Vite 5+ (Frontend build tool)

**Storage**: PostgreSQL 12+ (production), In-memory maps with sync.RWMutex (development/testing)  
**Testing**: Go testing + testify + Ginkgo/Gomega (E2E), Vitest (frontend)  
**Target Platform**: Linux server (backend), modern browsers (frontend)  
**Project Type**: Web application (Go backend + React frontend)  
**Performance Goals**: 
- Authorization validation < 200ms p95 (includes session checks for all requirements)
- Consent screen load < 2 seconds
- Service requirement validation at agent creation < 100ms

**Constraints**: 
- Service requirements stored as JSONB (backward compatible: NULL = no requirements)
- Authorization flow fail-closed: missing mandatory requirements redirect to consent screen
- Scope validation case-sensitive per SR-006
- redirect_uri strict same-origin checking per SR-001, SR-002

**Scale/Scope**: 
- Extend 1 entity (Agent with service_requirements JSONB column)
- Add 1 value object (ServiceRequirement)
- Add 1 enum (RequirementType)
- Extend 2 API surfaces (Admin API + End-user API)
- Extend 1 frontend component (Consent screen)
- Add 1 database migration (005_add_agent_service_requirements)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [../../.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Entities identified: Agent (extended), ServiceRequirement (value object), RequirementType (enum)
- [x] **Domain Concepts**: Will add to ARCHITECTURE.md Glossary: ServiceRequirement, RequirementType, Mandatory Service, Optional Service
- [x] **Configuration Design**: No new configuration required (uses existing system)
- [x] **Config Examples**: N/A - no feature-specific configuration
- [x] **API Design First**: APIs will be designed in OpenAPI spec per FR-006, API-001 through API-008
- [x] **API Documentation**: Will update `/api/admin/openapi.yaml` and `/api/enduser/openapi.yaml` per API-006
- [x] **API Changes**: All API changes documented in spec.md and will be confirmed before implementation
- [x] **Database Design**: Migration 005_add_agent_service_requirements will use go-migrate naming per DB-001
- [x] **E2E Acceptance Tests**: Will write E2E tests for all 6 user stories with 37 acceptance scenarios per Principle XIII
- [x] **E2E Test Mapping**: Each scenario will map 1:1 to It() blocks in tests/e2e/agent_permission_requirements_test.go
- [x] **E2E Red Phase**: E2E tests will FAIL initially before implementation begins

**Implementation Considerations**:

- [x] **Security-First**: Authorization fail-closed (FR-012), redirect_uri validation (SR-001, SR-002), scope validation (SR-006)
- [x] **Architecture Docs**: Will update ARCHITECTURE.md with extended Agent entity and service requirements concept
- [x] **ADRs**: No new ADR needed - follows ADR 20251215-120300 (Storage Layer Architecture) for JSONB storage
- [x] **Library-First Security**: Using standard library net/url for redirect_uri validation (no custom crypto)
- [x] **Zalando Guidelines**: APIs will follow Zalando RESTful API and Event Guidelines per API-008
- [x] **End-User Docs**: Will document new API endpoints in existing OpenAPI specs with examples
- [x] **Migration Testing**: Migration will be tested in PostgreSQL integration tests per DB-005
- [x] **Hexagonal Architecture**: Domain validation in Agent entity, storage via existing repository pattern
- [x] **Persistence Patterns**: Follows specs/20251215-120100-persistence-layer/quickstart.md - extending existing AgentRepository

**Status**: ✅ All preconditions complete - Phase 1 design finished

**Post-Design Re-evaluation**:
- [x] Domain Model: Completed in data-model.md (Agent extended, ServiceRequirement value object, RequirementType enum)
- [x] API Contracts: Completed in contracts/openapi-changes.md (Admin API + End-User API extensions)
- [x] Implementation Guide: Completed in quickstart.md (9 phases with code examples)
- [x] Testing Strategy: Defined in plan.md (37 E2E scenarios, unit tests, integration tests)
- [x] Agent Context: Updated via update-agent-context.sh (copilot-instructions.md created)

**Ready for Implementation**: All design artifacts complete, constitution checks pass. Next step is Phase 2 (implementation via /speckit.tasks command)

## Project Structure

### Documentation (this feature)

```text
specs/20260108-120000-agent-permission-requirements/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   ├── openapi.yaml     # OpenAPI spec for API changes
│   └── README.md        # API contract documentation
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── domain/
│   └── storage/
│       └── agent.go                              # EXTEND: Add service_requirements field
│
├── adapters/
│   ├── storage/
│   │   ├── memory/
│   │   │   └── agent_repository.go               # EXTEND: Handle service_requirements
│   │   └── postgres/
│   │       └── agent_repository.go               # EXTEND: Handle JSONB service_requirements
│   └── http/
│       ├── handlers/
│       │   ├── admin/
│       │   │   └── agent_handler.go              # EXTEND: Validate service_requirements
│       │   └── enduser/
│       │       ├── consent_handler.go            # EXTEND: Return service requirements
│       │       └── oauth2_handler.go             # EXTEND: Validate mandatory requirements
│       └── routing/
│           └── enduser.go                        # EXTEND: Add redirect_uri support

migrations/
└── 005_add_agent_service_requirements.{up,down}.sql  # NEW: Add service_requirements JSONB column

api/
├── admin/
│   └── openapi.yaml                              # EXTEND: Add service_requirements to Agent schema
└── enduser/
    └── openapi.yaml                              # EXTEND: Add service requirements endpoints

# Frontend (React)
web/src/
├── components/consent/
│   ├── ServiceRequirementCard.tsx                # NEW: Display service requirements
│   └── ServiceRequirementsList.tsx               # NEW: List mandatory/optional services
├── pages/
│   └── ConsentPage.tsx                           # EXTEND: Display requirements, handle redirect_uri
└── services/api/
    └── consent.ts                                # EXTEND: Fetch service requirements

# Tests
tests/e2e/
└── agent_permission_requirements_test.go         # NEW: E2E tests for all 37 scenarios

internal/domain/storage/
└── agent_test.go                                 # EXTEND: Test service_requirements validation

internal/adapters/storage/postgres/
└── agent_repository_test.go                      # EXTEND: Test JSONB storage
```

**Structure Decision**: Web application structure (Go backend + React frontend). This feature extends existing Agent entity backend logic and Consent frontend UI. No new services or repositories needed - extends existing AgentRepository with service_requirements field.

## Testing Strategy

<!--
  Per Constitution Principle XIII (End-to-End Acceptance Testing & Spec Traceability):
  All features MUST have E2E acceptance tests mapped 1:1 to spec scenarios.

  This section documents HOW E2E tests will be structured and implemented for this feature.
-->

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/agent_permission_requirements_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: "Agent Permission Requirements"
- **Nested Describe**: User Stories (e.g., "User Story 1: Administrator Configures Agent Service Requirements")
- **Context blocks**: Preconditions (e.g., "when creating agent with service requirements")
- **It blocks**: Individual acceptance scenarios (37 scenarios total from spec.md)

**Scenario Mapping**:

| Spec Scenario | Test Description |
|---------------|------------------|
| User Story 1, Scenario 1 | `It("should accept optional service_requirements array in POST /api/agents")` |
| User Story 1, Scenario 2 | `It("should require service_id, requirement_type, and required_scopes in each requirement")` |
| User Story 1, Scenario 3 | `It("should validate that all required_scopes exist in referenced service")` |
| User Story 1, Scenario 4 | `It("should return service_requirements with resolved display names in GET response")` |
| User Story 1, Scenario 5 | `It("should replace entire service requirements on PUT /api/agents/{id}")` |
| User Story 1, Scenario 6 | `It("should return HTTP 400 when service_id references non-existent service")` |
| User Story 1, Scenario 7 | `It("should return HTTP 400 when required_scopes contains invalid scope names")` |
| User Story 2, Scenario 1-7 | 7 scenarios for authorization endpoint validation |
| User Story 3, Scenario 1-6 | 6 scenarios for consent screen display |
| User Story 4, Scenario 1-5 | 5 scenarios for display-only scopes |
| User Story 5, Scenario 1-4 | 4 scenarios for simplified consent screen |
| User Story 6, Scenario 1-5 | 5 scenarios for redirect_uri handling |

*Note: Line numbers will be added during Phase 2f (E2E Test Design)*

**Test Data Strategy**:
- Use fixtures from `tests/e2e/fixtures/` for stable, reusable test data
- Required fixtures: 
  - Agents with various service_requirements configurations
  - ThirdPartyOAuth2Services with defined scopes
  - UserGrants for testing existing consent
  - ThirdPartyOAuth2Sessions for testing session validation
- New fixture creation: 
  - `agent_with_mandatory_service.json` (agent requiring GitHub with specific scopes)
  - `agent_with_optional_service.json` (agent with optional Slack integration)
  - `agent_with_multiple_requirements.json` (mix of mandatory and optional)

**Test Execution Flow**:
1. **Phase 2f (Design)**: Write E2E tests for all 37 scenarios
2. **Verify Red Phase**: Run `ginkgo -v ./tests/e2e/agent_permission_requirements_test.go` - all tests must FAIL
3. **Implementation**: Implement feature incrementally
4. **Verify Green Phase**: E2E tests turn GREEN as implementation satisfies acceptance criteria
5. **Minimal Changes**: Only fixture adjustments during implementation, not test logic

**Bootstrap Strategy**:
- Tests use production bootstrap code via `tests/e2e/bootstrap/` (app.Builder, HTTP server, routing)
- Fresh server and in-memory storage for each test (BeforeEach/AfterEach isolation)
- Mock upstream OAuth2 server for authorization proxy testing

**Helper Utilities**:
- Custom matchers: Use existing matchers in `tests/e2e/matchers/` (status code, JSON schema)
- HTTP helpers: Use existing helpers in `tests/e2e/helpers/` (HTTP client, request builders)
- Mock services: Mock upstream OAuth2 server for authorization endpoint testing

### Unit & Integration Tests

**Unit Tests**:
- Location: `internal/domain/storage/agent_test.go`
- Coverage: 
  - ServiceRequirement validation (service_id exists, scopes valid, no duplicates)
  - RequirementType enum validation
  - Agent entity validation with service_requirements
- Strategy: TDD - write tests FIRST, verify they FAIL, then implement
- Test count estimate: ~15 table-driven test cases

**Integration Tests**:
- Location: `internal/adapters/storage/postgres/agent_repository_test.go`
- Coverage: 
  - JSONB storage and retrieval of service_requirements
  - NULL handling (backward compatibility)
  - Migration 005 apply/rollback testing
- Strategy: Real PostgreSQL via testcontainers
- Test count estimate: ~10 integration test cases

**Frontend Tests**:
- Location: `web/src/components/consent/*.test.tsx`
- Coverage:
  - ServiceRequirementCard rendering (mandatory vs optional styling)
  - ServiceRequirementsList grouping and ordering
  - Scope display (read-only with descriptions)
- Framework: Vitest + React Testing Library
- Test count estimate: ~8 component test suites

**Test Coverage Goals**:
- Unit test coverage: 85%+ for domain validation logic
- Integration test coverage: 100% of repository methods touching service_requirements
- E2E test coverage: 100% of acceptance scenarios from spec.md (37 scenarios - mandatory per Principle XIII)
- Frontend test coverage: 80%+ for new components

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations - all Constitution checks pass. This feature follows established patterns:
- Extends existing Agent entity (no new entities)
- Uses existing repository pattern (AgentRepository)
- Follows ADR 20251215-120300 for storage (JSONB column with sqlx)
- No custom cryptography or security implementations
- Uses standard library for redirect_uri validation (net/url package)

**Integration Tests**:
- Location: `internal/adapters/[adapter]_test.go`
- Coverage: Database operations, external service integration
- Strategy: Real PostgreSQL via testcontainers, verify migrations

**Test Coverage Goals**:
- Unit test coverage: [target percentage or "critical paths only"]
- Integration test coverage: [specific adapters/repositories to test]
- E2E test coverage: 100% of acceptance scenarios from spec.md (mandatory per Principle XIII)

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
