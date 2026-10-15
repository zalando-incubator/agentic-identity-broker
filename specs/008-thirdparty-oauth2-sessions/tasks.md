# Tasks: Third-Party OAuth2 Session Management

**Input**: Design documents from `/specs/008-thirdparty-oauth2-sessions/`  
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY for all features. Test tasks are included in each user story below and MUST be written before or alongside implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and dependency setup

- [x] T001 Add golang.org/x/oauth2 dependency to go.mod
- [x] T002 [P] Add github.com/lestrrat-go/jwx/v3 dependency to go.mod
- [x] T003 [P] Create directory structure: internal/domain/oauth2session/, internal/adapters/http/oauth2_sessions/

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [x] T004 Document domain model entities (UserSession, OAuth2StateTokenClaims) in data-model.md ✓ (already complete)
- [x] T005 [P] Add new domain terms to ARCHITECTURE.md Glossary: UserSession, OAuth2StateToken, OAuth2SessionService, Token Vault, Session Termination
- [x] T006 [P] Document invariants: one session per (principal, service_id), tokens always encrypted, PKCE mandatory

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [x] T007 Create example YAML in examples/config/third-party-oauth2.yaml with jwe_signing_key, state_token_ttl, pkce_verifier_length
- [x] T008 [P] Update examples/config/README.md to reference third-party-oauth2.yaml
- [x] T009 [P] Add ThirdPartyOAuth2Config struct to internal/config/schema.go

**Checkpoint**: Configuration requirements designed with YAML examples

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

- [x] T010 Verify contracts/oauth2-sessions.yaml covers all endpoints from spec.md requirements ✓ (already complete)
- [x] T011 [P] Merge contracts/oauth2-sessions.yaml into /api/enduser/openapi.yaml
- [x] T012 [P] Get user/stakeholder confirmation for end-user API design (document in PR)

**Checkpoint**: APIs designed and confirmed by user/stakeholder

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [x] T013 Create original migration migrations/004_create_user_sessions.up.sql. Preserve this migration and migration 037 unchanged. This preparation PR ships only migration 038 and prepared code, with default/max target 38 and API compatibility true. Fresh installs run direct UP-all; normal upgrades use the guarded wrapper. Final contributor table samples describe the following cleanup PR, not the phase-38 table. PR2 supplies 039 only after the PR1 rollout and all old work drain. PR3 supplies 040 only after SPA/external consumers upgrade and schema dependencies end. Compatibility defaults true in preparation/activation, supports explicit false, and defaults false in cleanup. Preserve rows and rejection through phase-compatible rollback. Inspect dirty metadata before deliberate repair, never automatic `Force` or row deletion.
- [x] T014 [P] Create migration migrations/004_create_user_sessions.down.sql
- [x] T015 [P] Document unique constraint (principal, service_id) and foreign key to thirdparty_oauth2_services

**Checkpoint**: Database schema designed, migrations documented

### Phase 2e: Frontend/Design System Review [MANDATORY]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

- [x] T016 Review web/src/design-system/docs/INDEX.md for session card component patterns
- [x] T017 [P] Identify design system components to use: Card, Button, Dialog, Badge for status indicators
- [x] T018 [P] Document semantic token usage: success-primary (Active), warning-primary (Access Token Expired), error-primary (Reauthentication required)

**Checkpoint**: Design system usage planned

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T019 Create UserSession in internal/domain/storage/user_session.go with RefreshTokenRejected, Validate(), RequiresReauthentication(), CanRefresh(), and HasValidAccessToken(). False rejection means no observed rejection, not proven validity.
- [x] T020 [P] Create EncryptionContext type in internal/domain/storage/user_session.go with Value()/Scan() for JSONB
- [x] T021 [P] Create UserSessionRepository interface in internal/ports/storage.go (Create, Get, FindByPrincipalAndService, ListByPrincipal, Delete, DeleteByPrincipalAndService, CountByService)
- [x] T021a [P] Extend UserGrantRepository interface with CountAgentsByServiceID(ctx, serviceID) method to query delegated_oauth2_tokens JSONB for agent count per FR-016
- [x] T021b [P] Add ListByServiceID(ctx, serviceID) method to UserGrantRepository interface to retrieve actual agent IDs instead of placeholders
- [x] T022 Create OAuth2StateTokenClaims value object in internal/domain/oauth2session/state_token.go with Validate(), IsExpired()
- [x] T023 [P] Create PKCE generation function GeneratePKCE() in internal/domain/oauth2session/pkce.go per RFC 7636
- [x] T024 [P] Create domain errors in internal/domain/oauth2session/errors.go (ErrStateTokenExpired, ErrPrincipalMismatch, ErrServiceNotFound, etc.)
- [x] T025 Create OAuth2SessionService struct in internal/domain/oauth2session/service.go with Config, dependencies (repos, encryption, jweKey, logger)
- [x] T026 [P] Create in-memory UserSessionRepository adapter in internal/adapters/storage/memory/user_session.go
- [x] T027 Create PostgreSQL UserSessionRepository in internal/adapters/storage/postgres/user_session.go. Authorization upsert replaces credentials and clears recorded rejection. Normal refresh cannot recover rejected sessions.

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - View Available Third-Party Sessions (Priority: P1) 🎯 MVP

**Goal**: Users can see retained sessions, including rejected sessions. Cards show initiation time, dependent-agent count, encryption status, access-token expiry status, and required reauthentication.

**Independent Test**: User navigates to "Third-party Sessions" page and sees list of services with established sessions showing status information.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [x] T028 [P] [US1] Cover RequiresReauthentication(), CanRefresh(), HasValidAccessToken(), and Validate(). Include rejected refresh with unexpired access and expired access without refresh credentials.
- [x] T029 [P] [US1] Cover UserSessionSummary with requires_reauthentication, independent access_token_expired, and ciphertext-based has_refresh_token. Include rejected credentials and the token-free PostgreSQL projection.
- [x] T030 [P] [US1] Integration tests for UserSessionRepository.ListByPrincipal() in tests/integration/user_sessions_test.go
- [x] T031 [P] [US1] Integration tests for GET /api/third-party/sessions endpoint in tests/integration/oauth2_sessions_api_test.go

### Implementation for User Story 1

- [x] T032 [US1] Create UserSessionSummary with NewUserSessionSummary() in internal/domain/storage/user_session.go. Do not expose a refresh-expiry field.
- [x] T033 [US1] Implement OAuth2SessionService.ListUserSessions() in internal/domain/oauth2session/service.go
- [x] T034 [US1] Implement dependent agent count query using UserGrantRepository (query delegated_oauth2_tokens JSONB)
- [x] T035 [P] [US1] Create HTTP handler Handler struct in internal/adapters/http/oauth2_sessions/handler.go
- [x] T036 [US1] Implement ListSessions handler GET /api/third-party/sessions in internal/adapters/http/oauth2_sessions/handler.go
- [x] T037 [P] [US1] Register routes with Chi router in internal/adapters/http/oauth2_sessions/handler.go RegisterRoutes()
- [x] T038 [P] [US1] Create frontend API client sessionsApi in web/src/services/api/sessions.ts
- [x] T039 [P] [US1] Create useSessions hook in web/src/hooks/useSessions.ts
- [x] T040 [US1] Implement SessionCard with Reauthentication required / Access Token Expired / Active precedence. Show Login for required reauthentication, Refresh only with available refresh authority, and Terminate for every retained row. Remove refresh-expiry dates and permanent-validity claims.
- [x] T041 [US1] Implement ThirdPartySessionsPage. After a refresh error, refetch before showing the saved message. Login completes the real authorize/callback flow and restores Active status.
- [x] T042 [US1] Add structured logging for session list operations

**Checkpoint**: User Story 1 fully functional - users can view their sessions

**Current recovery acceptance**: With an unexpired access token, manual Refresh that receives HTTP 400 `invalid_grant` retains the session and immediately shows Reauthentication required. Login is available, Terminate is optional, and later token acquisition returns no token. Successful authorize/callback clears rejection and returns the card to Active. A transient refresh error does not record rejection.

**Persistence contract**: Locked callback errors never persist changes. Terminal rejection commits through `(true, nil)` before the service returns the captured provider error. Rejection leaves token metadata unchanged. Sweep queries exclude recorded rejection, and stale rejected candidates are skipped.

---

## Phase 4: User Story 2 - Establish OAuth2 Session via Authorization Code Flow (Priority: P2)

**Goal**: Users can authenticate with third-party services through OAuth2 authorization code flow with PKCE, storing encrypted tokens.

**Independent Test**: Navigating to /api/third-party/{serviceId}/oauth2/authorize redirects to third-party, callback stores encrypted tokens, user sees established session.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [x] T043 [P] [US2] Unit tests for GeneratePKCE() in tests/unit/oauth2session/pkce_test.go (verifier length, challenge computation)
- [x] T044 [P] [US2] Unit tests for CreateStateToken(), ValidateStateToken() in tests/unit/oauth2session/state_token_test.go
- [x] T045 [P] [US2] Unit tests for OAuth2SessionService.InitiateOAuth2Flow() in tests/unit/oauth2session/service_test.go
- [x] T046 [P] [US2] Unit tests for OAuth2SessionService.HandleCallback() in tests/unit/oauth2session/service_test.go
- [x] T047 [P] [US2] Integration tests for authorize endpoint in tests/integration/oauth2_sessions_api_test.go
- [x] T048 [P] [US2] Integration tests for callback endpoint in tests/integration/oauth2_sessions_api_test.go

### Implementation for User Story 2

- [x] T049 [US2] Implement JWE key loading from config in internal/domain/oauth2session/service.go (loadJWEKey)
- [x] T050 [US2] Implement CreateStateToken() JWE encryption in internal/domain/oauth2session/service.go using jwx/v3
- [x] T051 [US2] Implement ValidateStateToken() JWE decryption with principal/expiration validation in internal/domain/oauth2session/service.go
- [x] T052 [US2] Implement buildOAuth2Config() helper in internal/domain/oauth2session/service.go
- [x] T053 [US2] Implement OAuth2SessionService.InitiateOAuth2Flow() in internal/domain/oauth2session/service.go
- [x] T054 [US2] Implement redirect URI same-origin validation in handler
- [x] T055 [US2] Implement exchangeCodeWithRetry() with exponential backoff (1s, 2s, 4s) in internal/domain/oauth2session/service.go
- [x] T056 [US2] Implement token encryption using EncryptionPort in createSession() helper
- [x] T057 [US2] Implement OAuth2SessionService.HandleCallback() in internal/domain/oauth2session/service.go. Successful authorization supplies RefreshTokenRejected=false through the existing session upsert.
- [x] T058 [P] [US2] Implement InitiateFlow handler GET /api/third-party/{serviceId}/oauth2/authorize in internal/adapters/http/oauth2_sessions/handler.go
- [x] T059 [US2] Implement HandleCallback handler GET /api/third-party/{serviceId}/oauth2/callback in internal/adapters/http/oauth2_sessions/handler.go
- [x] T060 [US2] Handle OAuth2 error responses (access_denied, invalid_scope) in callback with user-friendly redirect
- [x] T061 [P] [US2] Add frontend success/error handling after OAuth2 callback redirect in ThirdPartySessionsPage.tsx
- [x] T062 [US2] Add structured audit logging: session establishment, failed state validation, failed PKCE validation
- [x] T062a [P] [US2] Unit test verifying audit log emitted on failed PKCE validation (SR-009 compliance)

**Checkpoint**: User Story 2 fully functional - users can establish sessions via OAuth2 flow

---

## Phase 5: User Story 3 - Terminate Third-Party Session with Warnings (Priority: P3)

**Goal**: Users can revoke sessions with warning about affected agents before deletion.

**Independent Test**: User clicks "Terminate Session", sees warning dialog listing affected agents, confirms, session and tokens deleted.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [x] T063 [P] [US3] Unit tests for OAuth2SessionService.TerminateSession() in tests/unit/oauth2session/service_test.go
- [x] T064 [P] [US3] Unit tests for GetSessionDetails() (dependent agents) in tests/unit/oauth2session/service_test.go
- [x] T065 [P] [US3] Integration tests for DELETE /api/third-party/{serviceId}/session in tests/integration/oauth2_sessions_api_test.go
- [x] T066 [P] [US3] Integration tests for GET /api/third-party/{serviceId}/session in tests/integration/oauth2_sessions_api_test.go

### Implementation for User Story 3

- [x] T067 [US3] Implement OAuth2SessionService.GetSessionDetails() with dependent agent list in internal/domain/oauth2session/service.go
- [x] T068 [US3] Implement OAuth2SessionService.TerminateSession() with token deletion in internal/domain/oauth2session/service.go
- [x] T069 [US3] Implement GetSessionDetails handler GET /api/third-party/{serviceId}/session in internal/adapters/http/oauth2_sessions/handler.go
- [x] T070 [US3] Implement TerminateSession handler DELETE /api/third-party/{serviceId}/session in internal/adapters/http/oauth2_sessions/handler.go
- [x] T071 [P] [US3] Create TerminationDialog component in web/src/components/sessions/TerminationDialog.tsx
- [x] T072 [US3] Integrate TerminationDialog with SessionCard in ThirdPartySessionsPage
- [x] T073 [P] [US3] Add terminateSession() method to sessionsApi in web/src/services/api/sessions.ts
- [x] T074 [US3] Add structured audit logging: session termination

**Checkpoint**: User Story 3 fully functional - users can terminate sessions with warnings

---

## Phase 6: User Story 4 - Secure State Token Management (Priority: P2)

**Goal**: JWE state tokens securely bind OAuth2 flows to users with CSRF protection and tamper detection.

**Independent Test**: State tokens contain all required claims, validation rejects expired/tampered/mismatched tokens.

*Note: This story is implemented alongside US2 (InitiateOAuth2Flow and HandleCallback). These tasks focus on security-specific testing and validation.*

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [x] T075 [P] [US4] Security tests for state token expiration rejection in tests/unit/oauth2session/state_token_security_test.go ✓
- [x] T076 [P] [US4] Security tests for principal mismatch rejection (CSRF) in tests/unit/oauth2session/state_token_security_test.go ✓
- [x] T077 [P] [US4] Security tests for service_id mismatch rejection in tests/unit/oauth2session/state_token_security_test.go ✓
- [x] T078 [P] [US4] Security tests for tampered token rejection in tests/unit/oauth2session/state_token_security_test.go ✓

### Implementation for User Story 4

- [x] T079 [US4] Verify JWE uses authenticated encryption A256GCMKW + A256GCM (code review task) ✓
- [x] T080 [US4] Verify state token TTL <= 15 minutes is enforced in config validation ✓
- [x] T081 [US4] Verify principal mismatch returns 403 Forbidden with security log ✓
- [x] T082 [US4] Verify service_id mismatch returns 400 Bad Request ✓

**Checkpoint**: User Story 4 complete - state tokens are secure ✓

---

## 🔒 Phase 7: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [x] T083 Verify domain model documented in ARCHITECTURE.md Glossary (Principle V) ✓
- [x] T084 Verify examples/config/third-party-oauth2.yaml exists (Principle VII) ✓
- [x] T085 [P] Verify examples/config/README.md references third-party-oauth2.yaml (Principle VII) ✓
- [x] T086 Verify /api/enduser/openapi.yaml includes all session endpoints (Principles IV, X) ✓
- [x] T087 Verify user/stakeholder confirmed API designs (document reference in PR) (Principle X) ✓
- [x] T088 Verify migrations 004_create_user_sessions exist and tested (Principle IX) ✓

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):
- [x] T089 [P] Verify API implementation matches OpenAPI specification exactly ✓
- [x] T090 Update docs/api/ with end-user OAuth2 session documentation and examples ✓

**Architecture & Documentation** (Principle II):
- [x] T091 Update ARCHITECTURE.md with OAuth2 session domain service and flow diagram ✓
- [x] T092 [P] Verify ARCHITECTURE.md Glossary has all new domain terms ✓

**Database & Persistence** (Principle IX):
- [x] T093 [P] Verify migrations follow sequential numbering (004) ✓
- [x] T094 [P] Verify migration integration tests (apply, rollback, data integrity) ✓
- [x] T095 [P] Verify PostgreSQL UserSessionRepository tested in integration tests ✓
- [x] T096 Verify persistence follows quickstart.md patterns (StorageError wrapping) ✓

**Security** (Principles I, III):
- [x] T097 Verify PKCE is mandatory (no bypass) ✓
- [x] T098 [P] Verify JWE encryption for state tokens (no custom crypto) ✓
- [x] T099 [P] Verify token encryption at rest using EncryptionPort (no custom crypto) ✓
- [x] T100 [P] Verify structured audit logging for security-critical operations ✓

**Architecture Patterns** (Principle VI):
- [x] T101 Verify OAuth2SessionService uses ports (ThirdpartyOAuth2ServiceRepository, UserSessionRepository, EncryptionPort) ✓

**Testing** (Principle VIII):
- [x] T102 Verify unit tests for domain logic ✓
- [x] T103 [P] Verify integration tests for HTTP handlers ✓
- [x] T104 [P] Verify integration tests for PostgreSQL repository ✓

**Frontend** (Principle XI):
- [x] T105 Verify SessionCard uses design system Card component ✓
- [x] T106 [P] Verify TerminationDialog uses design system Dialog component ✓
- [x] T107 [P] Verify semantic tokens for status colors (success/warning/error) ✓
- [x] T108 [P] Verify WCAG 2.1 AA accessibility (contrast ratios) ✓

### Additional Polish

- [x] T109 Code cleanup and refactoring ✓
- [x] T110 [P] Run go vet and go lint ✓
- [x] T111 [P] Run frontend eslint and prettier ✓
- [x] T112 Run quickstart.md validation steps ✓

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Design Preconditions (Phase 2)**: Depends on Setup completion - BLOCKS all implementation
  - Phase 2a, 2b, 2c, 2d, 2e can proceed in parallel
- **Foundational Infrastructure (Phase 2.5)**: Depends on ALL of Phase 2 completion - BLOCKS all user stories
- **User Stories (Phase 3-6)**: All depend on Phase 2 + Phase 2.5 completion
  - US1 (P1), US2 (P2), US4 (P2) can proceed in parallel
  - US3 (P3) can also proceed in parallel but is lower priority
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2.5 - MVP, no dependencies on other stories
- **User Story 2 (P2)**: Can start after Phase 2.5 - Core OAuth2 flow
- **User Story 4 (P2)**: Implements security for US2 - Run tests after US2 implementation
- **User Story 3 (P3)**: Can start after Phase 2.5 - Depends on session existing (US2 for full testing)

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Domain logic before HTTP handlers
- Backend before frontend
- Story complete before moving to next priority

### Parallel Opportunities

**Phase 1 (all parallel)**:
- T001, T002, T003 can run simultaneously

**Phase 2 (parallel within sub-phases)**:
- T005, T006 (2a) in parallel
- T007, T008, T009 (2b) in parallel
- T011, T012 (2c) in parallel
- T013, T014, T015 (2d) in parallel
- T016, T017, T018 (2e) in parallel

**Phase 2.5 (partial parallel)**:
- T020, T021, T023, T024, T026 can run in parallel
- T019, T022, T025, T027 are blocking

**User Story Tests (parallel within story)**:
- All [P] marked test tasks within a story can run in parallel

**Different Stories (parallel if team capacity)**:
- US1, US2, US3, US4 can be worked on by different developers after Phase 2.5

---

## Parallel Example: User Story 2 Tests

```bash
# Launch all US2 tests together:
Task T043: "Unit tests for GeneratePKCE()"
Task T044: "Unit tests for CreateStateToken(), ValidateStateToken()"
Task T045: "Unit tests for InitiateOAuth2Flow()"
Task T046: "Unit tests for HandleCallback()"
Task T047: "Integration tests for authorize endpoint"
Task T048: "Integration tests for callback endpoint"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Design Preconditions (T004-T018)
3. Complete Phase 2.5: Foundational Infrastructure (T019-T027)
4. Complete Phase 3: User Story 1 (T028-T042)
5. **STOP and VALIDATE**: Test User Story 1 independently
6. Deploy/demo session viewing capability

### Incremental Delivery

1. Setup → Design → Foundation ready
2. Add User Story 1 → MVP: Session viewing ✓
3. Add User Story 2 + 4 → OAuth2 flow with security ✓
4. Add User Story 3 → Session termination ✓
5. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

**Developer 1 (Backend)**:
- Phase 2.5: Domain entities and repositories
- US1: Service and handler implementation
- US2: OAuth2 flow implementation

**Developer 2 (Frontend)**:
- Phase 2e: Design system review
- US1: React components after backend ready
- US3: TerminationDialog after US1 complete

**Developer 3 (Security/Testing)**:
- All test tasks across user stories
- US4: Security verification
- Phase 7: Compliance verification

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence
