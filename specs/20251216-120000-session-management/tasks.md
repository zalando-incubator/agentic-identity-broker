# Implementation Tasks: Request Principal Extraction

**Feature**: Request Principal Extraction (Session Management)
**Branch**: `20251216-120000-session-management`
**Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

## Overview

This document breaks down the implementation of request principal extraction into executable tasks organized by user story priority. Each user story represents an independently testable increment of functionality.

**Tech Stack**:
- Go 1.24.0
- chi/v5 v5.2.3 (HTTP router)
- viper v1.21.0 (configuration)
- testify v1.11.1 (test assertions)
- slog (structured logging)

**Testing Approach**: Automated tests using Go standard library testing, testify for assertions, httptest for HTTP handler testing, table-driven tests for validation scenarios.

## Implementation Strategy

**MVP Scope**: User Story 1 (P1) - Configure Principal Header
**Incremental Delivery**: Complete each user story independently before starting the next
**Parallel Opportunities**: Marked with [P] - tasks that can run concurrently on different files

---

## Phase 1: Setup

**Goal**: Prepare project structure and foundational packages for principal extraction.

### Tasks

- [x] T001 Create internal/domain/principal package directory
- [x] T002 [P] Add authentication configuration types to internal/config/schema.go

**Completion Criteria**:
- Directory `internal/domain/principal/` exists
- `AuthenticationConfig`, `PreauthConfig` structs defined in schema.go

---

## Phase 2: Foundational

**Goal**: Implement core context utilities and error types that all user stories depend on.

### Tasks

- [x] T003 [P] Implement context key and WithPrincipal function in internal/domain/principal/context.go
- [x] T004 [P] Implement FromContext and MustFromContext functions in internal/domain/principal/context.go
- [x] T005 [P] Implement MissingPrincipalError and InvalidPrincipalError types in internal/domain/principal/errors.go
- [x] T006 Write unit tests for context utilities in internal/domain/principal/context_test.go
- [x] T007 [P] Write unit tests for error types in internal/domain/principal/errors_test.go

**Completion Criteria**:
- All context utility functions implemented and tested
- Context key is unexported type (type safety)
- All unit tests pass with >90% coverage
- No external dependencies on user story code

**Blocking Dependencies**: None
**Blocks**: All user story implementations

---

## Phase 3: User Story 1 (P1) - Configure Principal Header

**Story**: A system administrator needs to configure the application to extract user principals from a specific HTTP header that their reverse proxy sets after authenticating users.

**Goal**: Enable configuration of principal header name with extensible structure supporting future JWT authentication.

**Independent Test**: Set configuration value and verify application loads it correctly at startup.

### Tasks

- [x] T008 [US1] Update ServerInstanceConfig with Authentication field in internal/config/schema.go
- [x] T009 [US1] Set default values for servers.*.authentication.preauth.principal_header_name in internal/config/loader.go
- [x] T010 [US1] Write unit tests for configuration defaults in internal/config/schema_test.go
- [x] T011 [US1] Write unit tests for configuration validation in internal/config/validator_test.go
- [x] T012 [US1] Add configuration validation for empty/invalid header names in internal/config/validator.go

**Acceptance Criteria**:
- Configuration loaded with custom header name: `servers.admin.authentication.preauth.principal_header_name: "X-Authenticated-User"` → application uses "X-Authenticated-User"
- Configuration loaded with no header name → application uses default "X-Remote-User"
- Configuration loaded with empty header name → application logs error and uses default "X-Remote-User"
- Environment variable override works: `SERVERS_API_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME="X-Auth-User"` → application uses "X-Auth-User"

**Completion Criteria**:
- ✅ Configuration schema updated with nested authentication structure
- ✅ Default values set for both admin and api servers
- ✅ Unit tests verify configuration loading
- ✅ Validation ensures non-empty header names
- ✅ Manual test: Update config.yaml and verify startup logs show correct header name

**Blocking Dependencies**: Phase 2 (Foundational)
**Independent**: Can be tested without middleware implementation

---

## Phase 4: User Story 2 (P2) - Extract Principal from Requests

**Story**: The application receives HTTP requests from a reverse proxy and needs to extract the authenticated user's principal (username) from the configured header to maintain user context throughout request processing.

**Goal**: Implement middleware to extract principals from headers and propagate via Go context.

**Independent Test**: Send HTTP requests with configured header and verify principal is extracted and available in request context.

### Tasks

- [x] T013 [P] [US2] Implement RequirePrincipalMiddleware factory function in internal/adapters/http/principal_middleware.go
- [x] T014 [P] [US2] Implement OptionalPrincipalMiddleware factory function in internal/adapters/http/principal_middleware.go
- [x] T015 [P] [US2] Implement writeErrorJSON helper function in internal/adapters/http/principal_middleware.go
- [x] T016 [US2] Write table-driven unit tests for RequirePrincipalMiddleware in internal/adapters/http/principal_middleware_test.go
- [x] T017 [US2] Write table-driven unit tests for OptionalPrincipalMiddleware in internal/adapters/http/principal_middleware_test.go
- [x] T018 [US2] Update server.go to apply RequirePrincipalMiddleware to protected routes in internal/adapters/http/server.go
- [x] T019 [US2] Update server.go to apply OptionalPrincipalMiddleware to optional routes in internal/adapters/http/server.go
- [x] T020 [US2] Write integration tests with chi router in tests/integration/principal_middleware_test.go

**Test Scenarios** (from spec.md):
- Valid principal ("alice@example.com") → extracted and available in context
- Route without extraction → principal extraction skipped
- Multiple header values → first value used
- Leading/trailing whitespace → trimmed
- Unicode/emojis → preserved (UTF-8)

**Acceptance Criteria**:
- Request with header `X-Remote-User: alice@example.com` to protected route → principal "alice@example.com" in context, handler invoked
- Request without header to protected route → rejected with 401
- Request to route without extraction → handler invoked without principal
- Request with header `X-Remote-User:   alice   ` → principal "alice" in context (trimmed)
- Request with header containing emoji → principal preserved exactly

**Completion Criteria**:
- ✅ RequirePrincipalMiddleware implemented with validation
- ✅ OptionalPrincipalMiddleware implemented
- ✅ Table-driven tests cover all scenarios (9+ test cases)
- ✅ Integration tests verify end-to-end extraction with chi router
- ✅ Manual test: curl request with principal header → logs show extracted principal

**Blocking Dependencies**: Phase 2 (Foundational), User Story 1 (configuration)
**Independent**: Can be tested with mock configuration without US3 validation logic

---

## Phase 5: User Story 3 (P3) - Handle Missing or Invalid Principals

**Story**: The application needs to gracefully handle requests that don't contain a valid principal by rejecting them when principal extraction is enabled for a route.

**Goal**: Add defensive validation for edge cases and security enforcement.

**Independent Test**: Send requests without principal header or with invalid values to routes with extraction enabled, verify rejection with 401/400.

### Tasks

- [x] T021 [P] [US3] Add missing principal validation (401 response) to RequirePrincipalMiddleware in internal/adapters/http/principal_middleware.go
- [x] T022 [P] [US3] Add empty principal validation (401 response) to RequirePrincipalMiddleware in internal/adapters/http/principal_middleware.go
- [x] T023 [P] [US3] Add whitespace-only principal validation (401 response) to RequirePrincipalMiddleware in internal/adapters/http/principal_middleware.go
- [x] T024 [P] [US3] Add max length validation (400 response) to RequirePrincipalMiddleware in internal/adapters/http/principal_middleware.go
- [x] T025 [US3] Write unit tests for missing principal scenarios in internal/adapters/http/principal_middleware_test.go
- [x] T026 [US3] Write unit tests for invalid principal scenarios in internal/adapters/http/principal_middleware_test.go
- [x] T027 [US3] Write integration tests for edge cases in tests/integration/principal_middleware_test.go

**Test Scenarios** (from spec.md):
- Missing header → 401 Unauthorized
- Empty value → 401 Unauthorized
- Whitespace only (`"   "`) → 401 Unauthorized
- Principal > 200 characters → 400 Bad Request
- Route without extraction enabled → request proceeds normally

**Acceptance Criteria**:
- Request without header to protected route → 401 with JSON `{"error": "missing or empty principal"}`
- Request with empty header value to protected route → 401 with JSON error
- Request with whitespace-only header to protected route → 401 with JSON error
- Request with 201-character principal to protected route → 400 with JSON `{"error": "principal exceeds maximum length of 200 characters"}`
- Request without header to public route → 200 OK (no validation)

**Completion Criteria**:
- ✅ All validation logic implemented (missing, empty, whitespace, max length)
- ✅ Error responses use consistent JSON structure
- ✅ Unit tests cover all edge cases
- ✅ Integration tests verify rejection behavior end-to-end
- ✅ Manual test: curl request with 201-char principal → 400 response

**Blocking Dependencies**: User Story 2 (middleware implementation)
**Independent**: Can be tested independently from other user stories

---

## Phase 6: Polish & Cross-Cutting Concerns

**Goal**: Complete documentation, performance validation, and final integration.

### Tasks

- [x] T028 [P] Update ARCHITECTURE.md with Principal and Request Context glossary entries in ARCHITECTURE.md
- [x] T029 [P] Document authentication.preauth.principal_header_name configuration in docs/configuration.md
- [x] T030 [P] Create middleware usage guide in docs/api/middleware.md
- [x] T031 [P] Write performance benchmarks for context operations in internal/domain/principal/context_bench_test.go
- [x] T032 [P] Write performance benchmarks for middleware in internal/adapters/http/principal_middleware_bench_test.go
- [x] T033 Run `just verify` and `just test-coverage`, and verify >90% coverage
- [x] T034 Run performance benchmarks and verify <1ms latency at p95
- [x] T035 Manual end-to-end test with reverse proxy configuration (nginx/Traefik)

**Completion Criteria**:
- ✅ ARCHITECTURE.md updated with domain concepts
- ✅ Configuration documentation complete with examples
- ✅ Middleware usage guide with code examples
- ✅ Benchmarks show <1ms added latency at p95
- ✅ All tests pass with >90% coverage
- ✅ Manual test with real reverse proxy configuration works

**Blocking Dependencies**: All user stories complete
**Deliverables**: Production-ready feature with complete documentation

---

## Dependency Graph

```
Phase 1: Setup (T001-T002)
    │
    ▼
Phase 2: Foundational (T003-T007) ─────────────────┐
    │                                               │
    ▼                                               │
US1 (P1): Configure Header (T008-T012)             │
    │                                               │
    ▼                                               │
US2 (P2): Extract Principal (T013-T020) ◄──────────┘
    │
    ▼
US3 (P3): Handle Invalid (T021-T027)
    │
    ▼
Phase 6: Polish (T028-T035)
```

**Critical Path**: T001 → T003-T005 → T008-T009 → T013-T015 → T018 → T021-T024

**Parallel Execution**:
- T003, T004, T005 can run in parallel (different functions)
- T006 and T007 can run in parallel (different test files)
- T013, T014, T015 can run in parallel (different functions)
- T021, T022, T023, T024 can run in parallel (validation logic)
- T028, T029, T030, T031, T032 can run in parallel (documentation and benchmarks)

---

## Execution Recommendations

### MVP (Minimum Viable Product)
Complete **User Story 1 (P1)** first:
- Configuration loading works
- Application reads principal header name at startup
- Testable without middleware implementation

### Incremental Delivery
1. **Sprint 1**: US1 (configuration) - Deliverable: Configurable system
2. **Sprint 2**: US2 (extraction) - Deliverable: Working middleware for valid principals
3. **Sprint 3**: US3 (validation) - Deliverable: Security-hardened system with edge case handling
4. **Sprint 4**: Polish - Deliverable: Production-ready with docs and benchmarks

### Parallel Opportunities

**Within US1** (3 tasks):
- T010 and T011 (tests) can run in parallel

**Within US2** (8 tasks):
- T013, T014, T015 (middleware functions) can run in parallel
- T016 and T017 (tests) can run in parallel after T013-T015

**Within US3** (7 tasks):
- T021, T022, T023, T024 (validation logic) can run in parallel
- T025 and T026 (tests) can run in parallel after validation logic

**Within Polish** (8 tasks):
- T028, T029, T030, T031, T032 (all documentation and benchmarks) can run in parallel

---

## Testing Summary

**Test Coverage Target**: >90% for all new code

**Test Types**:
1. **Unit Tests**: Context utilities, middleware functions, configuration loading
2. **Integration Tests**: End-to-end with chi router, multiple middleware combinations
3. **Table-Driven Tests**: Validation scenarios (missing, empty, invalid, valid principals)
4. **Benchmarks**: Performance validation (<1ms latency)

**Test Files**:
- `internal/domain/principal/context_test.go` (T006)
- `internal/domain/principal/errors_test.go` (T007)
- `internal/config/schema_test.go` (T010)
- `internal/config/validator_test.go` (T011)
- `internal/adapters/http/principal_middleware_test.go` (T016, T017, T025, T026)
- `tests/integration/principal_middleware_test.go` (T020, T027)
- `internal/domain/principal/context_bench_test.go` (T031)
- `internal/adapters/http/principal_middleware_bench_test.go` (T032)

**Manual Testing**:
- Configuration loading at startup (US1)
- Request with principal header (US2)
- Request without principal header (US3)
- Reverse proxy integration (Phase 6)

---

## Task Count Summary

| Phase | Task Count | Parallelizable | Story |
|-------|-----------|----------------|-------|
| Phase 1: Setup | 2 | 1 | - |
| Phase 2: Foundational | 5 | 4 | - |
| Phase 3: US1 (P1) | 5 | 0 | Configure |
| Phase 4: US2 (P2) | 8 | 3 | Extract |
| Phase 5: US3 (P3) | 7 | 4 | Validate |
| Phase 6: Polish | 8 | 5 | - |
| **Total** | **35** | **17** | - |

**Estimated Effort**: 2-3 days for experienced Go developer

---

## Success Criteria (from plan.md)

Implementation complete when:

1. ✅ Protected routes reject requests without valid principals (401)
2. ✅ Protected routes reject principals >200 chars (400)
3. ✅ Principal is accessible via context in all downstream handlers
4. ✅ Optional routes work with or without principals
5. ✅ Performance: <1ms latency added at p95 (validated via benchmarks)
6. ✅ Test coverage: >90% for middleware and context utilities
7. ✅ Documentation: Configuration, architecture, and API docs updated
8. ✅ Integration tests: End-to-end tests with chi router pass

---

## References

- **Feature Spec**: [spec.md](spec.md) - User stories and acceptance criteria
- **Implementation Plan**: [plan.md](plan.md) - Technical approach and architecture
- **Data Model**: [data-model.md](data-model.md) - Principal and Request Context entities
- **API Contracts**: [contracts/middleware.md](contracts/middleware.md) - Middleware behavior specification
- **Quickstart Guide**: [quickstart.md](quickstart.md) - Implementation patterns and examples
- **Research**: [research.md](research.md) - Chi router patterns and context propagation best practices
