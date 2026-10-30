# Tasks: OAuth2 Authorization Server Proxy

**Input**: Design documents from `/specs/20251222-120100-oauth2-auth-server/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/openapi.yaml

**Tests**: Test tasks are included per Constitution Principle VIII (TDD is mandatory for all features)

**Organization**: Tasks are grouped by user story with tests BEFORE implementation to enable TDD workflow

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Configuration schema and examples

- [X] T001 Add OAuth2AuthServerConfig to internal/config/schema.go with upstream_issuer_uri, upstream_authorize_endpoint, upstream_token_endpoint, supported_response_types, supported_grant_types, upstream_timeout, mode fields (public URL is sourced from server.enduser.public_url)
- [X] T002 [P] Implement Validate() method for OAuth2AuthServerConfig in internal/config/schema.go validating required fields and HTTPS URLs
- [X] T003 [P] Unit test for OAuth2AuthServerConfig.Validate() in internal/config/schema_test.go with table-driven tests covering valid config, missing required fields, non-HTTPS URLs, and default value assignment
- [X] T004 [P] Create examples/config/oauth2-authorization-server.yaml with complete configuration example
- [X] T005 [P] Update examples/config/README.md to reference oauth2-authorization-server.yaml configuration section

**Checkpoint**: Configuration schema and examples complete

---

## Phase 2: Foundational Infrastructure (Blocking Prerequisites)

**Purpose**: Core OAuth2 domain interfaces and utilities that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T006 Create internal/ports/oauth2.go defining OAuth2Service interface with HandleAuthorization and GenerateMetadata methods
- [X] T007 [P] Define AuthorizationRequest struct in internal/ports/oauth2.go with ClientID, RedirectURI, Scope, State, ResponseType, CodeChallenge, CodeChallengeMethod, OriginalURL fields
- [X] T008 [P] Define AuthorizationDecision struct in internal/ports/oauth2.go with Action, RedirectURL, ErrorCode, ErrorDesc fields
- [X] T009 [P] Define MetadataResponse struct in internal/ports/oauth2.go with Issuer, AuthorizationEndpoint, TokenEndpoint, ResponseTypesSupported, GrantTypesSupported, TokenEndpointAuthMethodsSupported fields
- [X] T010 [P] Create internal/domain/oauth2/errors.go with OAuth2-specific error types and helper functions for building error redirect URLs
- [X] T011 [P] Unit test for buildErrorRedirectURL in internal/domain/oauth2/errors_test.go with table-driven tests covering error codes, descriptions, state parameter preservation, and URL encoding
- [X] T012 Create internal/adapters/http/upstream/oauth2_client.go with NewSecureUpstreamClient function implementing TLS validation using x509.SystemCertPool with MinVersion TLS12
- [X] T013 [P] Unit test for NewSecureUpstreamClient in internal/adapters/http/upstream/oauth2_client_test.go verifying TLS configuration (MinVersion TLS12, SystemCertPool, no InsecureSkipVerify)

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel ✅

---

## Phase 3: User Story 1 - Client Application Initiates OAuth2 Authorization Flow (Priority: P1) 🎯 MVP

**Goal**: Implement /oauth2/authorize endpoint that validates client_id, checks consent, and redirects to either consent UI or upstream OAuth2 server

**Independent Test**: Make GET request to /oauth2/authorize with valid client_id and verify system validates agent, checks grant status, and redirects appropriately (consent UI if no grant, upstream if grant exists)

### Tests for User Story 1 (TDD - Write Tests FIRST) ⚠️

> **CRITICAL**: Write these tests FIRST, ensure they FAIL before implementation

- [X] T014 [P] [US1] Unit test for HandleAuthorization in internal/domain/oauth2/service_test.go with table-driven tests covering: invalid client_id (returns error redirect), valid client_id with no grant (redirect to consent UI), valid client_id with active grant (redirect to upstream), expired grant (redirect to consent UI)
- [X] T015 [P] [US1] Unit test for buildUpstreamAuthorizeURL in internal/domain/oauth2/service_test.go verifying all OAuth2 parameters preserved (client_id, redirect_uri, scope, state, response_type, code_challenge, code_challenge_method)
- [X] T016 [P] [US1] Unit test for OAuth2AuthorizeHandler.ServeHTTP in internal/adapters/http/enduser/oauth2_authorize_test.go with mocked OAuth2Service covering: missing principal (should panic with MustFromContext), missing required parameters (400 error), service error (500 error), successful redirect
- [X] T017 [P] [US1] Unit test for OAuth2AuditMiddleware in internal/adapters/http/middleware/oauth2_audit_test.go verifying request ID generation, principal extraction via FromContext, structured logging with slog, and request/response logging
- [X] T018 [P] [US1] Integration test for authorization endpoint in tests/integration/oauth2_authorize_test.go with real AgentRepository and GrantRepository (in-memory), verifying complete flow from HTTP request to redirect response

### Implementation for User Story 1

- [X] T019 [P] [US1] Create internal/domain/oauth2/service.go with service struct containing agentRepo, grantRepo, config fields and NewService constructor
- [X] T020 [P] [US1] Define OAuth2Config struct in internal/domain/oauth2/service.go with UpstreamAuthorizeEndpoint, PublicBaseURL, SupportedResponseTypes, SupportedGrantTypes fields
- [X] T021 [US1] Implement HandleAuthorization method in internal/domain/oauth2/service.go validating client_id against AgentRepository and checking grant via GrantRepository (ensure T014 tests pass)
- [X] T022 [US1] Add buildUpstreamAuthorizeURL helper function in internal/domain/oauth2/service.go preserving all OAuth2 query parameters (ensure T015 tests pass)
- [X] T023 [P] [US1] Create internal/adapters/http/enduser/oauth2_authorize.go with OAuth2AuthorizeHandler struct and ServeHTTP method
- [X] T024 [US1] Implement OAuth2AuthorizeHandler.ServeHTTP in internal/adapters/http/enduser/oauth2_authorize.go extracting principal via principal.MustFromContext, parsing request, calling OAuth2Service, and executing redirect (ensure T016 tests pass)
- [X] T025 [US1] Add authorization endpoint route in enduser router (internal/adapters/http/server.go or equivalent) mounting OAuth2AuthorizeHandler at GET /oauth2/authorize protected by RequirePrincipalMiddleware
- [X] T026 [P] [US1] Create internal/adapters/http/middleware/oauth2_audit.go with OAuth2AuditMiddleware for structured logging with request ID generation, principal extraction, and slog (ensure T017 tests pass)
- [X] T027 [US1] Add OAuth2AuditMiddleware to authorization endpoint route in enduser router applying before handler (ensure T018 integration test passes)

**Checkpoint**: User Story 1 complete - Authorization endpoint validates agents, checks consent, redirects appropriately, all tests passing

---

## Phase 4: User Story 2 - Client Application Exchanges Authorization Code for Access Token (Priority: P2)

**Goal**: Implement /oauth2/token endpoint that proxies server-to-server token requests to upstream OAuth2 server

**Independent Test**: Make POST request to /oauth2/token with authorization_code grant and verify request is proxied to upstream with response returned unmodified

### Tests for User Story 2 (TDD - Write Tests FIRST) ⚠️

> **CRITICAL**: Write these tests FIRST, ensure they FAIL before implementation

- [X] T028 [P] [US2] Unit test for OAuth2TokenHandler.ServeHTTP in internal/adapters/http/enduser/oauth2_token_test.go with mocked upstreamClient covering: Content-Type validation (reject invalid types), header filtering (hop-by-hop headers excluded), successful proxy, upstream errors proxied
- [X] T029 [P] [US2] Unit test for isHopByHopHeader in internal/adapters/http/enduser/oauth2_token_test.go with table-driven tests for all hop-by-hop headers (Connection, Keep-Alive, Proxy-Authenticate, Proxy-Authorization, Te, Trailers, Transfer-Encoding, Upgrade) and non-hop-by-hop headers
- [X] T030 [P] [US2] Integration test for token endpoint in tests/integration/oauth2_token_test.go with httptest mock upstream server verifying: request proxying, response streaming, status code preservation, header copying

### Implementation for User Story 2

- [X] T031 [P] [US2] Create internal/adapters/http/enduser/oauth2_token.go with OAuth2TokenHandler struct containing upstreamClient, upstreamTokenURL, logger fields
- [X] T032 [US2] Implement OAuth2TokenHandler.ServeHTTP in internal/adapters/http/enduser/oauth2_token.go validating Content-Type, creating upstream request, copying headers (excluding hop-by-hop), proxying request, and streaming response (ensure T028 tests pass)
- [X] T033 [US2] Add isHopByHopHeader helper function in internal/adapters/http/enduser/oauth2_token.go filtering Connection, Keep-Alive, Proxy-Authenticate, Proxy-Authorization, Te, Trailers, Transfer-Encoding, Upgrade headers (ensure T029 tests pass)
- [X] T034 [US2] Add token endpoint route in enduser router mounting OAuth2TokenHandler at POST /oauth2/token without RequirePrincipalMiddleware (ensure T030 integration test passes)

**Checkpoint**: User Story 2 complete - Token endpoint proxies requests to upstream OAuth2 server, all tests passing

---

## Phase 5: User Story 3 - OAuth2 Clients Discover Authorization Server Metadata (Priority: P3)

**Goal**: Implement /.well-known/oauth-authorization-server endpoint that returns OAuth2 metadata for auto-discovery

**Independent Test**: Make GET request to /.well-known/oauth-authorization-server and verify valid RFC 8414 compliant metadata document is returned with broker endpoints and capabilities

### Tests for User Story 3 (TDD - Write Tests FIRST) ⚠️

> **CRITICAL**: Write these tests FIRST, ensure they FAIL before implementation

- [X] T035 [P] [US3] Unit test for GenerateMetadata in internal/domain/oauth2/service_test.go verifying RFC 8414 compliance: issuer matches server.enduser.public_url, authorization_endpoint correct path, token_endpoint correct path, response_types_supported includes "code", grant_types_supported includes configured types
- [X] T036 [P] [US3] Unit test for OAuth2MetadataHandler.ServeHTTP in internal/adapters/http/enduser/oauth2_metadata_test.go with mocked OAuth2Service covering: successful metadata generation, service error handling, JSON encoding, Content-Type header
- [X] T037 [P] [US3] Integration test for metadata endpoint in tests/integration/oauth2_metadata_test.go verifying: HTTP 200 response, valid JSON, RFC 8414 schema compliance, correct endpoint URLs

### Implementation for User Story 3

- [X] T038 [P] [US3] Implement GenerateMetadata method in internal/domain/oauth2/service.go returning MetadataResponse with broker's issuer, endpoints, and supported features (ensure T035 tests pass)
- [X] T039 [P] [US3] Create internal/adapters/http/enduser/oauth2_metadata.go with OAuth2MetadataHandler struct and ServeHTTP method
- [X] T040 [US3] Implement OAuth2MetadataHandler.ServeHTTP in internal/adapters/http/enduser/oauth2_metadata.go calling OAuth2Service.GenerateMetadata and returning JSON response (ensure T036 tests pass)
- [X] T041 [US3] Add metadata endpoint route in enduser router mounting OAuth2MetadataHandler at GET /.well-known/oauth-authorization-server without authentication (ensure T037 integration test passes)

**Checkpoint**: User Story 3 complete - Metadata endpoint returns OAuth2 server capabilities, all tests passing

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, API specs, security verification, and end-to-end testing

### End-to-End Integration Tests (TDD) ⚠️

- [X] T042 [P] Integration test for complete OAuth2 authorization code flow in tests/integration/oauth2_e2e_test.go with mock upstream OAuth2 server (httptest.NewTLSServer) verifying: authorization request → no consent → redirect to consent UI → consent granted → authorization request → redirect to upstream → token exchange → access token received
- [X] T043 [P] Integration test for error scenarios in tests/integration/oauth2_error_handling_test.go covering: upstream unreachable (returns OAuth2 error), invalid TLS certificate (connection rejected), malformed upstream responses (handled gracefully), expired grants (redirect to consent)
- [X] T044 [P] Security verification test in tests/integration/oauth2_security_test.go verifying: no sensitive data logged (scan logs for access_token, refresh_token, authorization code, client_secret), TLS validation enabled, fail-closed on errors, HTTPS-only configuration enforced

### API Documentation (MANDATORY)

- [X] T045 [P] Merge specs/20251222-120100-oauth2-auth-server/contracts/openapi.yaml OAuth2 endpoint specifications into api/enduser/openapi.yaml
- [X] T046 [P] Verify merged OpenAPI spec includes all three endpoints (authorize, token, metadata) with complete request/response schemas and examples

### Architecture Documentation (MANDATORY)

- [X] T047 [P] Update ARCHITECTURE.md adding OAuth2 proxy flow diagram showing broker → consent UI and broker → upstream OAuth2 server interactions
- [X] T048 [P] Add OAuth2 terminology to ARCHITECTURE.md Glossary section including Authorization Code, Access Token, Refresh Token, OAuth2 Metadata, Upstream OAuth2 Server definitions from data-model.md

### End-User Documentation (MANDATORY)

- [X] T049 Create docs/oauth2-configuration.md documenting OAuth2 configuration parameters, example flows, and integration instructions
- [X] T050 [P] Add OAuth2 flow examples to docs/oauth2-configuration.md showing authorization flow with consent check and token exchange
- [X] T051 [P] Create docs/api/oauth2-integration-guide.md with client integration examples and common use cases

### Configuration Validation

- [X] T052 Add OAuth2AuthServer configuration block to main Config struct in internal/config/schema.go and wire into configuration loading
- [X] T053 Call OAuth2AuthServerConfig.Validate() during application startup in main.go or config loader ensuring upstream URLs use HTTPS

### Router Integration Wiring

- [X] T054 Wire OAuth2Service initialization in main.go or server setup creating service instance with agentRepo, grantRepo, and config
- [X] T055 Wire upstream HTTP client creation in main.go or server setup calling NewSecureUpstreamClient with configured timeout
- [X] T056 Wire all three OAuth2 handlers in enduser router setup passing oauth2Service, upstreamClient, and logger dependencies

### Security Verification (Manual Review)

- [X] T057 [P] Verify TLS certificate validation is enabled in NewSecureUpstreamClient with no InsecureSkipVerify flag (review code and T013 test)
- [X] T058 [P] Verify OAuth2 error responses use fail-closed pattern returning errors when upstream unreachable (review code and T043 test)
- [X] T059 [P] Verify no sensitive data logged in any OAuth2 handlers or middleware (review code and T044 test results)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational Infrastructure (Phase 2)**: Depends on Phase 1 completion - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Phase 2 completion - Tests BEFORE implementation
- **User Story 2 (Phase 4)**: Depends on Phase 2 completion - Tests BEFORE implementation, independent of US1
- **User Story 3 (Phase 5)**: Depends on Phase 2 completion - Tests BEFORE implementation, independent of US1/US2
- **Polish (Phase 6)**: Depends on completion of desired user stories

### User Story Dependencies

- **User Story 1 (P1)**: No dependencies on other user stories - Can start after Phase 2
- **User Story 2 (P2)**: No dependencies on US1 - Can run in parallel with US1 after Phase 2
- **User Story 3 (P3)**: No dependencies on US1/US2 - Can run in parallel with US1/US2 after Phase 2

**Key Insight**: All three user stories are fully independent and can be implemented in parallel once foundational infrastructure (Phase 2) is complete.

### TDD Workflow Within Each User Story

1. **Write tests FIRST** (T014-T018 for US1, T028-T030 for US2, T035-T037 for US3)
2. **Run tests** - verify they FAIL (no implementation yet)
3. **Implement code** (T019-T027 for US1, T031-T034 for US2, T038-T041 for US3)
4. **Run tests** - verify they PASS
5. **Refactor** if needed while keeping tests green

### Parallel Opportunities

**Phase 1 (Setup)**: Tasks T003, T004, T005 can run in parallel after T002 completes

**Phase 2 (Foundation)**: Tasks T007, T008, T009, T010, T011, T013 can all run in parallel after T006 completes

**Phase 3 (US1) Tests**: Tasks T014, T015, T016, T017, T018 can all run in parallel

**Phase 3 (US1) Implementation**: Tasks T019, T020, T023, T026 can run in parallel; T021, T022, T024 sequential; T025, T027 sequential after handlers ready

**Phase 4 (US2) Tests**: Tasks T028, T029, T030 can all run in parallel

**Phase 4 (US2) Implementation**: Tasks T031, T033 can run in parallel; T032, T034 sequential

**Phase 5 (US3) Tests**: Tasks T035, T036, T037 can all run in parallel

**Phase 5 (US3) Implementation**: Tasks T038, T039 can run in parallel; T040, T041 sequential

**Phase 6 (Polish)**: Tasks T042, T043, T044, T045, T046, T047, T048, T049, T050, T051, T057, T058, T059 can all run in parallel

**Cross-Story Parallelism**: Once Phase 2 completes, all of Phase 3, Phase 4, and Phase 5 can run in parallel with different team members/agents

---

## Parallel Example: TDD Workflow for User Story 1

```bash
# Step 1: Write all tests in parallel
Task T014: "Unit test for HandleAuthorization with table-driven tests"
Task T015: "Unit test for buildUpstreamAuthorizeURL"
Task T016: "Unit test for OAuth2AuthorizeHandler.ServeHTTP"
Task T017: "Unit test for OAuth2AuditMiddleware"
Task T018: "Integration test for authorization endpoint"

# Step 2: Run tests - verify they FAIL
go test ./internal/domain/oauth2/... -v  # Should fail (no implementation)
go test ./internal/adapters/http/enduser/... -v  # Should fail

# Step 3: Implement code (some in parallel)
Task T019: "Create service.go with service struct"
Task T020: "Define OAuth2Config struct"
Task T023: "Create oauth2_authorize.go handler"
Task T026: "Create oauth2_audit.go middleware"

# Step 4: Complete implementation sequentially
Task T021: "Implement HandleAuthorization method"
Task T022: "Add buildUpstreamAuthorizeURL helper"
Task T024: "Implement ServeHTTP handler"
Task T025: "Add route to router"
Task T027: "Wire middleware"

# Step 5: Run tests - verify they PASS
go test ./internal/domain/oauth2/... -v  # Should pass
go test ./internal/adapters/http/enduser/... -v  # Should pass
just test-integration  # Should pass
```

---

## Implementation Strategy

### MVP First (User Story 1 with TDD)

1. Complete Phase 1: Setup (T001-T005) including config validation test
2. Complete Phase 2: Foundational Infrastructure (T006-T013) including TLS client test
3. Complete Phase 3: User Story 1 (T014-T027) **following TDD workflow**:
   - Write all tests (T014-T018)
   - Verify tests fail
   - Implement code (T019-T027)
   - Verify tests pass
4. **STOP and VALIDATE**: Run full test suite and manual testing
5. Complete essential Polish tasks (T045-T056 for deployment)

**MVP Delivers**: OAuth2 authorization endpoint with agent validation and consent checking, **fully tested** with unit and integration tests

### Incremental Delivery (TDD for Each Story)

After MVP (US1), add stories incrementally with TDD:

1. **US1 → Deploy**: Authorization endpoint functional and tested
2. **Add US2 with TDD** (T028-T034): Write token tests → Implement → Verify → Deploy
3. **Add US3 with TDD** (T035-T041): Write metadata tests → Implement → Verify → Deploy
4. **Add E2E tests** (T042-T044): Complete end-to-end validation

### Parallel Team Strategy with TDD

With multiple developers or agents:

1. **Phase 1-2 (Setup + Foundation)**: Assign to golang-pro to establish baseline with tests (T001-T013)
2. **Once Phase 2 complete, split user stories with TDD**:
   - **US1 (Authorization)**: golang-pro agent
     - First: Write tests T014-T018
     - Then: Implement T019-T027
   - **US2 (Token)**: backend-developer agent
     - First: Write tests T028-T030
     - Then: Implement T031-T034
   - **US3 (Metadata)**: api-designer agent
     - First: Write tests T035-T037
     - Then: Implement T038-T041
3. **Polish Phase**: Add E2E tests (qa-expert), documentation (technical-writer), security review (security-engineer)

---

## Task Summary

**Total Tasks**: 59 (was 41 without tests - added 18 test tasks)

**By Phase**:
- Phase 1 (Setup): 5 tasks (includes config validation test)
- Phase 2 (Foundation): 8 tasks (includes error and TLS client tests)
- Phase 3 (US1): 14 tasks (5 tests + 9 implementation)
- Phase 4 (US2): 7 tasks (3 tests + 4 implementation)
- Phase 5 (US3): 7 tasks (3 tests + 4 implementation)
- Phase 6 (Polish): 18 tasks (3 E2E tests + 15 documentation/wiring)

**By Category**:
- **Test tasks**: 18 (unit tests, integration tests, E2E tests, security tests)
- **Implementation tasks**: 26 (domain logic, handlers, routing, middleware)
- **Infrastructure tasks**: 15 (config, ports, wiring, documentation)

**By User Story**:
- User Story 1 (P1): 5 test tasks + 9 implementation tasks = 14 total
- User Story 2 (P2): 3 test tasks + 4 implementation tasks = 7 total
- User Story 3 (P3): 3 test tasks + 4 implementation tasks = 7 total
- Shared infrastructure: 13 tasks (Setup + Foundation with tests)
- Cross-cutting polish: 18 tasks (includes E2E and security tests)

**Test Coverage**:
- Unit tests: 11 tasks (domain logic, handlers, middleware, helpers)
- Integration tests: 4 tasks (per-endpoint integration tests)
- End-to-end tests: 3 tasks (complete flows, error scenarios, security)

**Parallel Opportunities**: 36 tasks marked [P] can run in parallel within their phase

**MVP Scope**: 27 tasks (Setup + Foundation + US1 with tests) delivers fully tested OAuth2 authorization endpoint

**Test-to-Implementation Ratio**: 18 test tasks for 26 implementation tasks ≈ **69% test coverage by task count** ✅

---

## Notes

- **[P] tasks** = different files, no dependencies, safe to parallelize
- **[Story] label** maps task to specific user story (US1, US2, US3) for traceability
- **⚠️ TDD CRITICAL**: Tests MUST be written BEFORE implementation per Constitution Principle VIII
- **Test verification**: After writing tests, run them to verify they FAIL before implementing code
- Each user story is independently completable and testable
- **US1 is MVP** - provides core authorization flow with consent checking, fully tested
- **US2 completes the flow** - adds token exchange capability with tests
- **US3 enhances DX** - adds metadata discovery with tests
- Run the verification gate at each checkpoint: `just verify`
- Verify configuration validation, TLS security, and error handling at each checkpoint
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently with automated tests

---

## Constitution Compliance ✅

This task list satisfies Constitution Principle VIII (Test-Driven Development):

✅ **Automated tests included** - 18 test tasks covering unit, integration, and E2E scenarios
✅ **TDD workflow enforced** - Tests written BEFORE implementation in each user story phase
✅ **Happy paths covered** - Tests verify successful flows (authorization, token, metadata)
✅ **Error cases covered** - Tests verify error handling (invalid client, upstream failures, malformed requests)
✅ **Edge cases covered** - Tests verify expired grants, missing parameters, TLS validation
✅ **Table-driven tests specified** - T003, T011, T014, T029 use table-driven test pattern
✅ **Integration tests included** - T018, T030, T037, T042, T043, T044 test real behavior end-to-end
✅ **Maintainable tests** - Clear test descriptions with file paths and specific scenarios

**Test Coverage Goals**:
- Domain logic: 100% (all service methods have unit tests)
- HTTP handlers: 100% (all handlers have unit tests)
- Integration: Complete flows tested end-to-end
- Security: Explicit security verification tests (T044, T057-T059)
