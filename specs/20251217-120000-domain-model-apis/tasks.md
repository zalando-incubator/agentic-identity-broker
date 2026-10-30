# Tasks: Domain Model and Consent APIs

**Feature**: 20251217-120000-domain-model-apis
**Branch**: `20251217-120000-domain-model-apis`
**Input**: Design documents from `/specs/20251217-120000-domain-model-apis/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/
**Total Estimated Time**: 28-37 hours

**Organization**: Tasks grouped by user story to enable independent implementation. Each user story can be completed, tested, and deployed independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: User story label (US1, US2, US3, US4)
- **File paths**: Absolute paths from repository root

---

## Phase 1: Setup (Project Structure & Dependencies)

**Purpose**: Initialize project structure and verify dependencies
**Time**: 1-2 hours
**Checkpoint**: Project structure ready for domain implementation

- [ ] T001 Create domain layer directory structure in `internal/domain/consent/` and `internal/domain/storage/`
- [ ] T002 [P] Create ports directory structure with new interfaces in `internal/ports/`
- [ ] T003 [P] Create adapter directories in `internal/adapters/storage/noop/` and `internal/adapters/storage/postgres/` extension
- [ ] T004 [P] Create HTTP handler directories in `internal/adapters/http/admin/` and `internal/adapters/http/consent/`
- [ ] T005 Create migrations directory in `migrations/`
- [ ] T006 Verify Go 1.24.0 environment and required dependencies (chi, sqlx, pgx v5, viper, slog)

---

## Phase 2: Foundational (Core Infrastructure)

**Purpose**: Blocking prerequisites that MUST complete before any user story work
**Time**: 8-10 hours
**⚠️ CRITICAL**: No user story implementation can begin until this phase is complete

### Domain Entities & Validation

- [ ] T007 [P] Define Agent entity struct in `internal/domain/storage/agent.go` with fields: id, client_id, external_id, display_name, description, governance_url, user_documentation_url, agent_interface_url, created_at, updated_at
- [ ] T008 [P] Implement Agent.Validate() method in `internal/domain/storage/agent.go` with validation rules for required fields, URL format checking, and length constraints
- [ ] T009 [P] Define ThirdpartyOAuth2Service entity struct in `internal/domain/storage/thirdparty_service.go` with fields for OAuth2 configuration, endpoints, and JSONB scopes array
- [ ] T010 [P] Implement ThirdpartyOAuth2Service.Validate() method in `internal/domain/storage/thirdparty_service.go` with HTTPS URL validation, scopes validation, at least one scope required
- [ ] T011 [P] Define OAuthScope value object struct in `internal/domain/storage/oauth_scope.go` with scope_value and description fields
- [ ] T012 [P] Define UserGrant entity struct in `internal/domain/storage/user_grant.go` with id, principal, agent_id, valid_until, delegated_oauth2_tokens (JSONB), created_at, updated_at
- [ ] T013 [P] Implement UserGrant.Validate() method in `internal/domain/storage/user_grant.go` with scope validation, future date validation for valid_until, principal/agent_id reference validation
- [ ] T014 [P] Define DelegatedOAuth2Token value object in `internal/domain/storage/user_grant.go` with thirdparty_oauth2_service_id and scopes array
- [ ] T015 Write table-driven validation tests for Agent.Validate() in `internal/domain/storage/agent_test.go`
- [ ] T016 Write table-driven validation tests for ThirdpartyOAuth2Service.Validate() in `internal/domain/storage/thirdparty_service_test.go`
- [ ] T017 Write table-driven validation tests for UserGrant.Validate() in `internal/domain/storage/user_grant_test.go`

### Encryption Port Interface

- [ ] T018 [P] Define EncryptionPort interface in `internal/ports/encryption.go` with Encrypt(plaintext, encryptionContext) and Decrypt(ciphertext, encryptionContext) methods
- [ ] T019 [P] Implement NoOpEncryption adapter in `internal/adapters/storage/noop/encryption.go` that stores plaintext (development only)
- [ ] T020 Write unit tests for NoOpEncryption in `internal/adapters/storage/noop/encryption_test.go`

### Repository Interfaces

- [ ] T021 [P] Define AgentRepository interface in `internal/ports/storage.go` with methods: Create, Get, Update, Delete, List
- [ ] T022 [P] Define ThirdpartyOAuth2ServiceRepository interface in `internal/ports/storage.go` with methods: Create, Get, Update, Delete, List, CountGrantsReferencingService
- [ ] T023 [P] Define UserGrantRepository interface in `internal/ports/storage.go` with methods: Create, Get, Update, Delete, ListByPrincipalAndAgent, FindByPrincipalAndAgent, DeleteByAgent
- [ ] T024 Extend Adapter struct in `internal/adapters/storage/memory/adapter.go` and `internal/adapters/storage/postgres/adapter.go` to include repository accessors

### Database Schema & Migrations

- [ ] T025 Write migration 001_create_agents up migration in `migrations/001_create_agents.up.sql` with agents table, UUID extension, indexes for client_id, external_id, created_at
- [ ] T026 Write migration 001_create_agents down migration in `migrations/001_create_agents.down.sql`
- [ ] T027 Write migration 002_create_thirdparty_services up migration in `migrations/002_create_thirdparty_services.up.sql` with table, client_secret BYTEA column, scopes JSONB, indexes
- [ ] T028 Write migration 002_create_thirdparty_services down migration in `migrations/002_create_thirdparty_services.down.sql`
- [ ] T029 Write migration 003_create_user_grants up migration in `migrations/003_create_user_grants.up.sql` with user_grants table, UNIQUE constraint on (principal, agent_id), indexes, foreign keys with CASCADE for agents
- [ ] T030 Write migration 003_create_user_grants down migration in `migrations/003_create_user_grants.down.sql`
- [ ] T031 Test migration execution (up and down) against PostgreSQL testcontainer

### Consent Service

- [ ] T032 Define ConsentService interface in `internal/domain/consent/service.go` with methods: GetAgentConsentInfo, GrantConsent, RevokeConsent, GetActiveGrants
- [ ] T033 Implement ConsentService.GetAgentConsentInfo() in `internal/domain/consent/service.go` to fetch agent and return ALL third-party services with requested_services field
- [ ] T034 Implement ConsentService.GrantConsent() in `internal/domain/consent/service.go` with scope validation, upsert semantics (one grant per user-agent pair), valid_until validation
- [ ] T035 Implement ConsentService.RevokeConsent() in `internal/domain/consent/service.go` to delete grant for principal/agent pair
- [ ] T036 Implement ConsentService.GetActiveGrants() in `internal/domain/consent/service.go` to filter expired grants (valid_until < now), return indefinite grants
- [ ] T037 Write unit tests for ConsentService with mock repositories in `internal/domain/consent/service_test.go`

**Checkpoint**: Foundation ready - all domain entities defined, ports established, database schema prepared, user story implementation can now begin

---

## Phase 3: User Story 1 - Administrator Manages Agent Registry (Priority: P1) 🎯 MVP

**Goal**: Enable administrators to register and configure AI agents in the system
**Independent Test**: Admin can CRUD agents via `/api/agents` endpoints
**Time**: 4-5 hours
**Dependency**: Phase 2 (Foundational) must be complete

### In-Memory Repository Implementation for US1

- [ ] T038 [P] [US1] Implement AgentRepository for in-memory adapter in `internal/adapters/storage/memory/agents.go` with Create, Get, Update, Delete, List methods using sync.RWMutex
- [ ] T039 [P] [US1] Write unit tests for in-memory AgentRepository in `internal/adapters/storage/memory/agents_test.go` with concurrent access tests

### PostgreSQL Repository Implementation for US1

- [ ] T040 [P] [US1] Implement AgentRepository for PostgreSQL adapter in `internal/adapters/storage/postgres/agents.go` with sqlx queries, error wrapping, context propagation
- [ ] T041 [P] [US1] Write integration tests for PostgreSQL AgentRepository in `internal/adapters/storage/postgres/agents_test.go` using testcontainers

### Admin API Handlers for US1

- [ ] T042 [US1] Create agents CRUD handler file in `internal/adapters/http/admin/agents_handler.go`
- [ ] T043 [US1] Implement POST /api/agents handler in `internal/adapters/http/admin/agents_handler.go` - create agent with validation, return 201 with agent record
- [ ] T044 [US1] Implement GET /api/agents/:agent-id handler in `internal/adapters/http/admin/agents_handler.go` - fetch and return agent, handle 404 not found
- [ ] T045 [US1] Implement PUT /api/agents/:agent-id handler in `internal/adapters/http/admin/agents_handler.go` - update agent with validation, return updated record
- [ ] T046 [US1] Implement DELETE /api/agents/:agent-id handler in `internal/adapters/http/admin/agents_handler.go` - delete agent, cascade delete grants, return 204
- [ ] T047 [US1] Implement error handling and validation responses in agents handler with detailed error messages
- [ ] T048 [US1] Write unit tests for admin agents handlers in `internal/adapters/http/admin/agents_handler_test.go`
- [ ] T049 [US1] Wire agents handlers to chi router in `internal/adapters/http/server.go` under /api/agents path for admin server (name == "admin")

**Checkpoint**: User Story 1 complete - Admin agent registry fully functional and independently testable

---

## Phase 4: User Story 2 - Administrator Configures Third-Party OAuth2 Services (Priority: P1)

**Goal**: Enable administrators to configure external OAuth2 providers with automatic endpoint discovery
**Independent Test**: Admin can CRUD OAuth2 services via `/api/third-party/oauth2/clients` endpoints, discovery works
**Time**: 6-8 hours
**Dependency**: Phase 2 (Foundational) must be complete. Can run in parallel with US1.

### OAuth2 Discovery Service

- [ ] T050 [P] [US2] Implement OAuth2 endpoint discovery function in `internal/domain/storage/discovery.go` to construct well-known metadata URL from issuer_uri
- [ ] T050b [P] [US2] Handle metadata_url override in discovery function for non-standard providers
- [ ] T050c [P] [US2] Implement HTTP client with timeout (10s), SSL certificate validation, metadata parsing
- [ ] T050d [P] [US2] Add error handling and fallback to manually configured endpoints
- [ ] T050e [US2] Write unit tests for OAuth2 discovery in `internal/domain/storage/discovery_test.go`

### In-Memory Repository Implementation for US2

- [ ] T051 [P] [US2] Implement ThirdpartyOAuth2ServiceRepository for in-memory adapter in `internal/adapters/storage/memory/thirdparty_services.go` with JSONB scopes handling
- [ ] T052 [P] [US2] Implement CountGrantsReferencingService() method for in-memory adapter to check if service has active grants
- [ ] T053 [P] [US2] Write unit tests for in-memory ThirdpartyOAuth2ServiceRepository in `internal/adapters/storage/memory/thirdparty_services_test.go`

### PostgreSQL Repository Implementation for US2

- [ ] T054 [P] [US2] Implement ThirdpartyOAuth2ServiceRepository for PostgreSQL adapter in `internal/adapters/storage/postgres/thirdparty_services.go` with JSONB support
- [ ] T055 [P] [US2] Implement client secret encryption/decryption in PostgreSQL adapter using EncryptionPort
- [ ] T056 [P] [US2] Implement CountGrantsReferencingService() for PostgreSQL using JSONB containment operators
- [ ] T057 [P] [US2] Implement JSONB marshaling/unmarshaling for scopes array in PostgreSQL adapter
- [ ] T058 [P] [US2] Write integration tests for PostgreSQL ThirdpartyOAuth2ServiceRepository in `internal/adapters/storage/postgres/thirdparty_services_test.go` with encryption tests

### Admin API Handlers for US2

- [ ] T059 [US2] Create OAuth2 services CRUD handler file in `internal/adapters/http/admin/services_handler.go`
- [ ] T060 [US2] Implement POST /api/third-party/oauth2/clients handler in `internal/adapters/http/admin/services_handler.go` - create service with discovery if enabled, validate config
- [ ] T061 [US2] Implement GET /api/third-party/oauth2/clients/:client-id handler in `internal/adapters/http/admin/services_handler.go` - return service with client_secret redacted
- [ ] T062 [US2] Implement PUT /api/third-party/oauth2/clients/:client-id handler in `internal/adapters/http/admin/services_handler.go` - update service, re-run discovery if enabled
- [ ] T063 [US2] Implement DELETE /api/third-party/oauth2/clients/:client-id handler in `internal/adapters/http/admin/services_handler.go` - block deletion if grants exist, return 409 Conflict with grant count
- [ ] T064 [US2] Implement client secret redaction in responses (mask as "***REDACTED***") in services handler
- [ ] T065 [US2] Implement validation for required fields, HTTPS URLs, at least one scope required
- [ ] T066 [US2] Write unit tests for admin services handlers in `internal/adapters/http/admin/services_handler_test.go`
- [ ] T067 [US2] Wire services handlers to chi router in `internal/adapters/http/server.go` under /api/third-party/oauth2/clients path for admin server

**Checkpoint**: User Story 2 complete - OAuth2 service configuration fully functional with discovery and independently testable

---

## Phase 5: User Story 3 - User Reviews Agent Information Before Granting Access (Priority: P2)

**Goal**: Provide endpoint for users to view agent details and available services/scopes before granting access
**Independent Test**: Client can GET agent consent info via `/api/consent/agent/:agent-id` and receives requested_services array
**Time**: 2-3 hours
**Dependency**: Phase 2 (Foundational), Phase 3 (US1), Phase 4 (US2) must be complete

### Consent API Handlers for US3

- [ ] T068 [US3] Create agent consent info handler file in `internal/adapters/http/consent/agent_info_handler.go`
- [ ] T069 [US3] Implement GET /api/consent/agent/:agent-id handler in `internal/adapters/http/consent/agent_info_handler.go` - fetch agent via ConsentService.GetAgentConsentInfo()
- [ ] T070 [US3] Return AgentConsentInfo response with agent metadata and requested_services array (all configured services)
- [ ] T071 [US3] Include scope descriptions in response (scope_value and description for each scope)
- [ ] T072 [US3] Implement error handling for missing agent/services (return 404)
- [ ] T073 [US3] Implement error handling for service configuration errors (return appropriate HTTP status)
- [ ] T074 [US3] Write unit tests for agent consent info handler in `internal/adapters/http/consent/agent_info_handler_test.go`
- [ ] T075 [US3] Wire agent info handler to chi router in `internal/adapters/http/server.go` under /api/consent path for enduser server (name == "enduser")

**Checkpoint**: User Story 3 complete - Users can view agent consent information independently

---

## Phase 6: User Story 4 - User Grants and Manages Permissions to Agents (Priority: P3)

**Goal**: Enable users to create, view, modify, and revoke grants delegating OAuth2 scopes to agents
**Independent Test**: User can create/read/modify/revoke grants via `/api/consent/agent/:agent-id/grants` endpoints
**Time**: 5-7 hours
**Dependency**: Phase 2 (Foundational), Phase 3 (US1), Phase 4 (US2), Phase 5 (US3) must be complete

### In-Memory Repository Implementation for US4

- [ ] T076 [P] [US4] Implement UserGrantRepository for in-memory adapter in `internal/adapters/storage/memory/user_grants.go` with Create, Get, Update, Delete, ListByPrincipalAndAgent, FindByPrincipalAndAgent, DeleteByAgent
- [ ] T077 [P] [US4] Implement upsert semantics (one grant per principal-agent pair) in memory adapter
- [ ] T078 [P] [US4] Implement cascade delete for agent deletion in memory adapter
- [ ] T079 [P] [US4] Implement expired grant filtering (valid_until < now) in memory adapter
- [ ] T080 [P] [US4] Write unit tests for in-memory UserGrantRepository in `internal/adapters/storage/memory/user_grants_test.go` with upsert, cascade, expiration tests

### PostgreSQL Repository Implementation for US4

- [ ] T081 [P] [US4] Implement UserGrantRepository for PostgreSQL adapter in `internal/adapters/storage/postgres/user_grants.go` with all required methods
- [ ] T082 [P] [US4] Implement JSONB handling for delegated_oauth2_tokens array in PostgreSQL adapter
- [ ] T083 [P] [US4] Implement upsert via ON CONFLICT clause in PostgreSQL adapter for one-grant-per-pair constraint
- [ ] T084 [P] [US4] Implement UNIQUE constraint (principal, agent_id) in queries and validation
- [ ] T085 [P] [US4] Implement expired grant filtering with `WHERE valid_until IS NULL OR valid_until > NOW()` in PostgreSQL adapter
- [ ] T086 [P] [US4] Write integration tests for PostgreSQL UserGrantRepository in `internal/adapters/storage/postgres/user_grants_test.go`

### User Grants API Handlers for US4

- [ ] T087 [US4] Create grants handler file in `internal/adapters/http/consent/grants_handler.go`
- [ ] T088 [US4] Implement GET /api/consent/agent/:agent-id/grants handler in `internal/adapters/http/consent/grants_handler.go` - call ConsentService.GetActiveGrants() with principal from session
- [ ] T089 [US4] Return array of UserGrant records including granted services, scopes, and valid_until (null for indefinite)
- [ ] T090 [US4] Implement POST /api/consent/agent/:agent-id/grants handler in `internal/adapters/http/consent/grants_handler.go` for grant creation/modification (upsert)
- [ ] T091 [US4] Accept GrantRequest with delegated_oauth2_tokens array, optional valid_until timestamp
- [ ] T092 [US4] Validate all requested scopes exist in service configurations (call ConsentService.GrantConsent())
- [ ] T093 [US4] Validate valid_until is in the future (if provided)
- [ ] T094 [US4] Support grant revocation via empty scopes array (POST with empty services/scopes array)
- [ ] T095 [US4] Implement principal extraction from session middleware context in handlers
- [ ] T096 [US4] Implement cross-user access prevention (return 403 if user tries to access another user's grants)
- [ ] T097 [US4] Implement error handling with validation error messages for invalid scopes, past dates, missing references
- [ ] T098 [US4] Implement structured audit logging for grant creation, modification, revocation
- [ ] T099 [US4] Write unit tests for grants handlers in `internal/adapters/http/consent/grants_handler_test.go` with principal/authorization tests
- [ ] T100 [US4] Wire grants handlers to chi router in `internal/adapters/http/server.go` under /api/consent/agent/:agent-id/grants path for enduser server

**Checkpoint**: User Story 4 complete - User grant management fully functional and independently testable

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, architecture compliance, and final improvements
**Time**: 2-3 hours
**Dependency**: All user stories must be complete

### Constitution Compliance (MANDATORY)

- [ ] T101 Update ARCHITECTURE.md Glossary with new domain terms: Agent, ThirdpartyOAuth2Service, UserGrant, OAuthScope, DelegatedOAuth2Token
- [ ] T102 [P] Update ARCHITECTURE.md with Consent subsystem architecture and component diagram
- [ ] T103 [P] Document EncryptionPort interface and no-op adapter in ARCHITECTURE.md security section
- [ ] T104 Verify all endpoints return JSON with Content-Type application/json headers
- [ ] T105 Verify admin APIs (port 14000) are protected via session middleware requiring admin role
- [ ] T106 Verify user APIs (port 8000) are protected via session middleware extracting principal
- [ ] T107 Verify client secrets are redacted in all GET responses
- [ ] T108 Verify principal validation prevents cross-user access on grant endpoints
- [ ] T109 Verify cascade delete works when agent is deleted (grants deleted)
- [ ] T110 Verify service deletion blocked when grants reference it (409 Conflict response)

### API Documentation

- [ ] T111 [P] Create admin API documentation in `docs/api/admin-apis.md` - agents and OAuth2 services endpoints
- [ ] T112 [P] Create user consent API documentation in `docs/api/consent-apis.md` - agent info and grants endpoints
- [ ] T113 [P] Generate API documentation from OpenAPI spec in `specs/20251217-120000-domain-model-apis/contracts/openapi.yaml`
- [ ] T114 Update main docs/configuration.md to include domain model configuration section for encryption and storage backends
- [ ] T115 Add code examples in API documentation with curl commands and JSON request/response samples

### Code Quality & Testing

- [ ] T116 [P] Run all unit tests and verify >90% code coverage for domain layer
- [ ] T117 [P] Run all integration tests with testcontainers PostgreSQL
- [ ] T118 Verify error handling returns appropriate HTTP status codes (201 created, 204 no content, 400 bad request, 404 not found, 409 conflict, 500 server error)
- [ ] T119 Verify structured audit logging for all user story operations (grant creation, agent CRUD, service CRUD)
- [ ] T120 Run gofmt, go vet, and golangci-lint checks on all new code

### End-to-End Validation

- [ ] T121 Test complete workflow: Create agent → Create service → Get consent info → Create grant → View grants → Modify grant → Revoke grant
- [ ] T122 Test service deletion blocking when grants exist (409 response)
- [ ] T123 Test grant expiration filtering (expired grants not returned in active grants)
- [ ] T124 Test agent deletion cascade deletes grants
- [ ] T125 Test principal isolation (user A cannot access user B's grants)
- [ ] T126 Test concurrent grant modifications by same user (upsert semantics work correctly)
- [ ] T127 Validate all response schemas match OpenAPI spec in contracts/openapi.yaml

**Checkpoint**: Feature complete - All user stories delivered, documented, and production-ready

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - start immediately
- **Foundational (Phase 2)**: Depends on Setup - BLOCKS all user stories
- **User Story 1 (Phase 3, P1)**: Depends on Foundational - MVP scope
- **User Story 2 (Phase 4, P1)**: Depends on Foundational - can run parallel with US1
- **User Story 3 (Phase 5, P2)**: Depends on Foundational, US1, US2
- **User Story 4 (Phase 6, P3)**: Depends on Foundational, US1, US2, US3
- **Polish (Phase 7)**: Depends on all desired user stories

### Parallel Opportunities

**After Phase 2 (Foundational) completion**:
- US1 and US2 can be worked on in parallel (different files, independent stories)
- Within US1: All in-memory tests → PostgreSQL tests → handlers can be pipelined
- Within US2: Discovery service → In-memory repo → PostgreSQL repo → handlers can be pipelined
- US3 cannot start until US1 and US2 are complete (needs both agents and services)
- US4 cannot start until US1, US2, and US3 are complete (needs all prerequisites)

**Parallel Example: Teams with Multiple Developers**

```
Setup Phase (1-2 hours): 1 person
  ↓
Foundational Phase (8-10 hours): Whole team together
  ↓
Once Foundational complete:
  Developer A: User Story 1 (4-5 hours)
  Developer B: User Story 2 (6-8 hours) - runs in parallel with A
  (When A+B done):
  Developer C: User Story 3 (2-3 hours)
  (When A+B+C done):
  Developer D: User Story 4 (5-7 hours)
  (When all done):
  Whole team: Polish & cross-cutting (2-3 hours)
```

---

## Implementation Strategy

### MVP First (Recommended for Initial Release)

1. **Complete Phase 1**: Setup (1-2 hours)
2. **Complete Phase 2**: Foundational (8-10 hours) ← **Critical milestone**
3. **Complete Phase 3**: User Story 1 - Agent Registry (4-5 hours)
4. **STOP and VALIDATE**: Test User Story 1 independently with admin endpoints
5. **Deploy MVP**: Agent management functionality ready
6. **Add Phase 4**: User Story 2 - OAuth2 Services (6-8 hours)
7. **Add Phase 5**: User Story 3 - Consent Info (2-3 hours)
8. **Add Phase 6**: User Story 4 - User Grants (5-7 hours)

### Incremental Delivery Timeline

- **Week 1**: Setup + Foundational (9-12 hours total)
- **Week 2**: User Story 1 + US2 in parallel (10-13 hours total)
- **Week 3**: User Story 3 + US4 in parallel (7-10 hours total)
- **Week 4**: Polish & cross-cutting concerns (2-3 hours)

### Work in Parallel (With Team)

With 4 developers:
- Developer 1 + 2: Complete Setup + Foundational together
- Developer 3: Start US1 when Foundational done
- Developer 4: Start US2 when Foundational done
- Once US1+US2 done: All continue with US3, then US4
- Final week: All hands on Polish phase

---

## Notes & Best Practices

- **Task Independence**: Each task should be completable without waiting for other tasks (except explicit dependencies)
- **File Isolation**: [P] tasks use different files to enable parallel execution
- **Test First**: Consider writing tests before implementation (TDD approach)
- **Story Checkpoints**: Stop and validate after each user story before moving to next
- **Commit Frequently**: One commit per task or logical group (~10 tasks per commit)
- **Verify Tests**: Run tests after each phase before proceeding to next phase
- **Error Prevention**: Avoid vague tasks, cross-file conflicts, or implicit dependencies
- **MVP Validation**: Complete at least through Phase 3 (US1) before declaring MVP ready

---

## Task Summary

| Phase | User Story | Tasks | Time | Status |
|-------|-----------|-------|------|--------|
| 1 | Setup | T001-T006 | 1-2h | Not started |
| 2 | Foundational | T007-T037 | 8-10h | Not started |
| 3 | US1 (Agent Registry) | T038-T049 | 4-5h | Not started |
| 4 | US2 (OAuth2 Services) | T050-T067 | 6-8h | Not started |
| 5 | US3 (Consent Info) | T068-T075 | 2-3h | Not started |
| 6 | US4 (User Grants) | T076-T100 | 5-7h | Not started |
| 7 | Polish | T101-T127 | 2-3h | Not started |
| | **TOTAL** | **T001-T127** | **28-37h** | **Not started** |

**Total Tasks**: 127
**Parallelizable Setup Tasks**: 6 out of 6
**Parallelizable Foundational Tasks**: 17 out of 31
**Parallelizable User Story Tasks**: Varies by story (see within-story dependencies)

---
