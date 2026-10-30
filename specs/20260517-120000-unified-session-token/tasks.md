# Tasks: Unified Session Token State Transport

**Input**: Design documents from `/specs/20260517-120000-unified-session-token/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Per Constitution Principle VIII, automated tests are MANDATORY. Test tasks are included in each user story and MUST be written before implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II, V

- [X] T001 Confirm no new entities needed — existing `AuthorizationSessionClaims` reused with `CIMDMetadata=nil` for non-CIMD agents
- [X] T002 [P] Update ARCHITECTURE.md Glossary to note session tokens apply to ALL agent modes (not CIMD-only)

**Checkpoint**: Domain model confirmed (no changes), glossary updated

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII

- [X] T003 Confirm no new configuration parameters — feature reuses existing JWE key and TTL settings

**Checkpoint**: No config changes needed — N/A

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV, X

- [X] T004 Confirm no public API contract changes — consent URL parameter change is internal state transport, not a documented API

**Checkpoint**: No API changes — N/A

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX

- [X] T005 Confirm no database schema changes — session tokens remain stateless (JWE-sealed)

**Checkpoint**: No DB changes — N/A

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

- [X] T006 Confirm no frontend UI changes — consent page renders identically; only URL parameter name changes

**Checkpoint**: No frontend changes — N/A

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII

- [X] T007 Write E2E acceptance tests in `tests/e2e/unified_session_token_test.go` for all 7 spec scenarios
- [X] T007a Map each acceptance scenario to one `It()` block using Ginkgo/Gomega BDD framework
- [X] T007b Organize: `Describe("Unified Session Token")` → `Context("local agent")`, `Context("proxy agent")`, `Context("consent handlers")`
- [X] T007c Use existing fixtures from `tests/e2e/fixtures/` — create local agent fixture and proxy agent fixture (without CIMD metadata)
- [X] T007d Add comment references to spec scenarios (e.g., `// Scenario 1.1 from specs/20260517-120000-unified-session-token/spec.md`)
- [X] T007e Verify E2E tests FAIL semantically: assertions target HTTP redirect Location header (`session_token=` present, `redirect_uri=` absent), status codes (400/403 for rejections)

**Checkpoint**: E2E acceptance tests written and verified to fail semantically before implementation

---

## Phase 3: User Story 1 — Local Agent Consent Flow Uses Session Token (Priority: P1) 🎯 MVP

**Goal**: Local (non-CIMD, non-proxy) agents produce `session_token` in consent URLs instead of `redirect_uri`

**Independent Test**: Send authorize request for a local agent → verify redirect Location contains `session_token=` and NOT `redirect_uri=`

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [X] T008 [P] [US1] Unit tests for `buildConsentURL` with nil cimdMeta in `internal/domain/oauth2/service_test.go` — assert session_token output
- [X] T009 [P] [US1] Unit test verifying `NewAuthorizationSessionClaims` with nil CIMDMetadata produces valid claims with correct TTL in `internal/domain/oauth2/authorization_session_token_test.go`

### Implementation for User Story 1

- [X] T010 [US1] Modify `buildConsentURL` in `internal/domain/oauth2/service.go` — remove `if cimdMeta != nil` branch; always create session token for all agents
- [X] T011 [US1] Verify existing CIMD E2E tests still pass (regression check) by running `ginkgo -v ./tests/e2e/ --focus="CIMD"`

**Checkpoint**: Local agent authorize requests produce session_token; CIMD flow unchanged

---

## Phase 4: User Story 2 — Proxy Agent Consent Flow Uses Session Token (Priority: P1)

**Goal**: Verify proxy agents are covered by the US1 implementation (same code path)

**Independent Test**: Send authorize request for a proxy agent → verify redirect contains `session_token=`

**Note**: US1's change to `buildConsentURL` removes the CIMD conditional entirely, so proxy agents are automatically covered. This phase is verification-only.

- [X] T012 [US2] Add proxy agent fixture to `tests/e2e/fixtures/` if not already present
- [X] T013 [US2] Verify proxy agent E2E scenario (US2 Scenario 1) passes after US1 implementation

**Checkpoint**: Proxy agent coverage confirmed — no additional code changes needed

---

## Phase 5: User Story 3 — Consent Handlers Accept Session Token for All Agent Modes (Priority: P2)

**Goal**: Consent handlers require `session_token` exclusively — remove `redirect_uri` fallback

**Independent Test**: Submit consent request with `redirect_uri` but no `session_token` → verify 400 rejection

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [X] T014 [P] [US3] Unit tests for `agent_detail_handler` requiring session_token in `internal/adapters/http/handlers/consent/agent_detail_handler_test.go`
- [X] T015 [P] [US3] Unit tests for `grants_handler` requiring session_token in `internal/adapters/http/handlers/consent/grants_handler_test.go`
- [X] T016 [P] [US3] Unit test for principal mismatch rejection (403) in consent handlers
- [X] T017 [P] [US3] Unit test for expired token rejection (400) in consent handlers
- [X] T017a [P] [US3] Unit test for malformed JWE token (not valid compact serialization) returns 400 in `internal/adapters/http/handlers/consent/agent_detail_handler_test.go`
- [X] T017b [P] [US3] Unit test for agent ID mismatch (token agent ≠ URL path agent) returns 400 in `internal/adapters/http/handlers/consent/agent_detail_handler_test.go`

### Implementation for User Story 3

- [X] T018 [US3] Remove `redirect_uri` fallback from `internal/adapters/http/handlers/consent/agent_detail_handler.go` — return 400 if `session_token` absent
- [X] T019 [US3] Remove `redirect_uri` fallback from `internal/adapters/http/handlers/consent/grants_handler.go` — return 400 if `session_token` absent
- [X] T020 [US3] Verify all E2E tests pass: `ginkgo -v ./tests/e2e/unified_session_token_test.go`

**Checkpoint**: Consent handlers reject requests without session_token; all agent modes unified

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY]

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [X] T021 Verify ARCHITECTURE.md Glossary updated with session token scope change (Principle V)
- [X] T022 Verify no config changes needed — confirmed in Phase 2b (Principle VII)
- [X] T023 Verify no API changes needed — confirmed in Phase 2c (Principles IV, X)
- [X] T024 Verify no database changes needed — confirmed in Phase 2d (Principle IX)
- [X] T025 Verify E2E acceptance tests written in `tests/e2e/unified_session_token_test.go` for all 7 spec scenarios (Principle XIII)
- [X] T026 Verify E2E tests failed semantically before implementation (red phase) (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**Security** (Principles I, III):
- [X] T027 Verify security is strengthened (insecure fallback removed, fail-closed on missing token)
- [X] T028 [P] Verify no custom cryptography — uses existing `lestrrat-go/jwx` JWE (Principle III)

**Architecture Patterns** (Principle VI):
- [X] T029 Verify domain logic uses ports — `buildConsentURL` calls `CreateAuthorizationSessionToken` via domain service

**Testing** (Principle VIII):
- [X] T030 Verify unit tests written first and failed before implementation
- [X] T031 Verify tests changed minimally during implementation

**E2E Acceptance Testing** (Principle XIII):
- [X] T032 Verify all 7 spec scenarios have passing E2E tests
- [X] T033 Verify E2E tests changed minimally during implementation
- [X] T034 Run full E2E test suite: `ginkgo -v ./tests/e2e/` (all tests must pass)
- [X] T035 Run full verification gate: `just verify`

### Additional Polish

- [X] T036 Remove any dead code left by redirect_uri removal (unused imports, helper functions)
- [X] T037 Run `just check` (fmt → vet → lint) and `just verify` — all must pass

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 2 (Design Preconditions)**: No dependencies — can start immediately; BLOCKS all implementation
- **Phase 3 (US1)**: Depends on Phase 2f (E2E tests written and failing)
- **Phase 4 (US2)**: Depends on Phase 3 completion (US1 implementation covers proxy agents too)
- **Phase 5 (US3)**: Depends on Phase 3 completion (session tokens must be generated before handlers can require them)
- **Phase N (Compliance)**: Depends on all user stories complete

### User Story Dependencies

- **US1 (P1)**: Independent — modifies token generation in domain service
- **US2 (P1)**: Depends on US1 — same code path, validation only
- **US3 (P2)**: Depends on US1+US2 — handlers can only require session_token after all agents produce it

### Parallel Opportunities

- T001–T006 (Phase 2 confirmations) can all run in parallel
- T008, T009 (US1 unit tests) can run in parallel
- T014–T017 (US3 unit tests) can run in parallel
- T018, T019 (US3 handler changes) can run in parallel (different files)

---

## Implementation Strategy

1. Complete Phase 2 confirmations (T001–T006) — verify no model, config, API, DB, or frontend changes needed; update ARCHITECTURE.md glossary
2. Write E2E acceptance tests (T007) — all 7 scenarios red; assertions on `session_token=` present, `redirect_uri=` absent in redirect Location
3. Write unit tests for US1 `buildConsentURL` and `NewAuthorizationSessionClaims` (T008, T009) — red
4. Implement US1: modify `buildConsentURL` to always produce `session_token`, remove CIMD-only branch (T010)
5. Verify US1+US2 green: US1/US2 E2E scenarios pass; CIMD regression intact (T011–T013)
6. Write unit tests for consent handler session_token requirements (T014–T017b) — red
7. Implement US3: remove `redirect_uri` fallback from `agent_detail_handler` and `grants_handler` (T018, T019)
8. Verify all E2E green (T020, T032–T035)
9. Constitution compliance, dead-code removal, and `just check` + `just verify` (T021–T037)
