# Tasks: Broker-Hosted Aggregated JWKS

**Input**: Design documents from `/specs/20260604-120000-aggregated-jwks/`
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓, quickstart.md ✓

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY. Test tasks are included in each user story phase below and MUST be written before or alongside implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [ ] T001 [P] Add domain terms `AggregatedKeySet`, `KeySource`, `JWKSPublisher` to ARCHITECTURE.md Glossary section
- [ ] T002 [P] Document value objects and invariants in data-model.md (already complete — verify accuracy against spec)

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [ ] T003 Confirm no new configuration parameters needed — behavior derives from existing `mode` + `upstream_issuer_uri` (document in PR description)

**Checkpoint**: Configuration requirements confirmed (no changes needed)

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

- [ ] T004 Update `/api/enduser/openapi.yaml` for `/oauth2/jwks.json` endpoint with mode-dependent content description, 503 response, and Cache-Control header per `specs/20260604-120000-aggregated-jwks/contracts/jwks-endpoint.yaml`
- [ ] T005 [P] Update `/api/enduser/openapi.yaml` for `/.well-known/oauth-authorization-server` to document `jwks_uri` present in all modes
- [ ] T006 Get user/stakeholder confirmation for API design changes (supersedes spec 20260511-120000 FR-012 and spec 20260328-120000 US6 Scenario 3)

**Checkpoint**: APIs designed and confirmed by user/stakeholder

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [ ] T007 Confirm no database changes required — feature operates entirely on in-memory key sets (document in PR description)

**Checkpoint**: Database schema confirmed (no changes needed)

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)

- [ ] T008 Write E2E acceptance tests in `tests/e2e/aggregated_jwks_test.go` with 21 `It()` blocks mapping 1:1 to spec acceptance scenarios
- [ ] T009 [P] Create test helper `helpers.NewMockUpstreamJWKS()` — mock HTTP server serving deterministic JWKS in `tests/e2e/helpers/`
- [ ] T010 [P] Create test helper `helpers.MintTestJWT()` — creates JWT signed with a test key in `tests/e2e/helpers/`
- [ ] T011 Organize E2E tests hierarchically: `Describe("Aggregated JWKS Endpoint")` → `Context` per mode → `It` per scenario
- [ ] T012 Verify E2E tests FAIL semantically (red phase): detailed expectations for HTTP status codes, JSON key counts, specific `kid` values, `Cache-Control` headers, and JWT verification — no placeholder always-fail assertions

**Checkpoint**: E2E acceptance tests written and verified to fail semantically before implementation

---

## Phase 2.5: Foundational Infrastructure [Port + Domain Service Skeleton]

**Purpose**: New port interface and domain service skeleton that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T013 Create `JWKSPublisherPort` interface in `internal/ports/jwks_publisher.go` with `PublishJWKS(ctx context.Context) (jwk.Set, error)` method
- [ ] T014 [P] Create domain service skeleton in `internal/domain/jwkspublisher/service.go` with constructor `NewService(mode, localKeys, upstreamKeys, logger)` and stub `PublishJWKS` method
- [ ] T015 [P] Define sentinel errors `ErrUpstreamUnavailable` and `ErrKidConflict` in `internal/domain/jwkspublisher/service.go`
- [ ] T016 Write unit tests (TDD red phase) in `internal/domain/jwkspublisher/service_test.go` — table-driven tests for all modes and error cases with hand-rolled mocks for `SigningKeyManager` and `JWKSPort`
- [ ] T017 Verify project compiles with new port and skeleton service (`just build`)

**Checkpoint**: Foundation ready — port interface defined, domain service skeleton compiles, unit tests written and failing

---

## Phase 3: User Story 1 — Unified Token Verification Surface (Priority: P1) 🎯 MVP

**Goal**: Clients fetch a single JWKS endpoint to verify any token the broker distributes, regardless of operating mode

**Independent Test**: Configure broker in each mode, fetch `/oauth2/jwks.json`, verify that tokens from the expected signing domains validate using only the broker-hosted JWKS response

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [ ] T018 [P] [US1] Unit tests for local-mode aggregation (returns only local keys) in `internal/domain/jwkspublisher/service_test.go`
- [ ] T019 [P] [US1] Unit tests for proxy-mode aggregation (returns only upstream keys) in `internal/domain/jwkspublisher/service_test.go`
- [ ] T020 [P] [US1] Unit tests for hybrid-mode aggregation (returns union of local + upstream keys) in `internal/domain/jwkspublisher/service_test.go`
- [ ] T021 [P] [US1] Unit test verifying private key material is never exposed in `internal/domain/jwkspublisher/service_test.go`

### Implementation for User Story 1

- [ ] T022 [US1] Implement `PublishJWKS` local-mode logic in `internal/domain/jwkspublisher/service.go` — return current local public keys (no upstream involvement)
- [ ] T023 [US1] Implement pre-computed snapshot rebuild logic in `internal/domain/jwkspublisher/service.go` — triggered on upstream cache refresh callback; fetches upstream keys, validates, atomically swaps the current snapshot
- [ ] T024 [US1] Implement `PublishJWKS` proxy/hybrid-mode logic in `internal/domain/jwkspublisher/service.go` — return current pre-computed snapshot; if snapshot is in error state, return appropriate error (ErrUpstreamUnavailable or ErrKidConflict)
- [ ] T025 [US1] Modify JWKS handler in `internal/adapters/http/handlers/enduser/jwks_handler.go` — change dependency from `ports.SigningKeyManager` to `ports.JWKSPublisherPort`, map errors to HTTP responses
- [ ] T026 [US1] Wire `JWKSPublisherService` in `internal/app/builder.go` — create with mode-appropriate sources, always set `jwksHandler` (no longer nil in proxy mode)
- [ ] T027 [US1] Update routing in `internal/adapters/http/routing/enduser.go` — remove conditional: always register `/oauth2/jwks.json`
- [ ] T028 [US1] Add `Cache-Control: public, max-age=300` header in JWKS handler response in `internal/adapters/http/handlers/enduser/jwks_handler.go`
- [ ] T029 [US1] Verify unit tests for US1 pass (green phase) — all mode-specific aggregation tests

**Checkpoint**: User Story 1 fully functional — JWKS endpoint serves correct keys per mode with caching headers

---

## Phase 4: User Story 2 — Discovery Advertises Broker-Hosted JWKS (Priority: P1)

**Goal**: Discovery document includes `jwks_uri` in all modes, giving clients a single predictable auto-configuration path

**Independent Test**: Fetch `/.well-known/oauth-authorization-server` in each mode, verify `jwks_uri` is present and resolves to the broker endpoint

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [ ] T030 [P] [US2] Unit test verifying metadata includes `jwks_uri` in local mode in existing OAuth2 service test file
- [ ] T031 [P] [US2] Unit test verifying metadata includes `jwks_uri` in proxy mode (new behavior) in existing OAuth2 service test file
- [ ] T032 [P] [US2] Unit test verifying metadata includes `jwks_uri` in hybrid mode in existing OAuth2 service test file

### Implementation for User Story 2

- [ ] T033 [US2] Modify discovery metadata in `internal/domain/oauth2/service.go` — set `metadata.JWKSURI` unconditionally (remove mode guard), keep `CodeChallengeMethodsSupported` conditional on local/hybrid
- [ ] T034 [US2] Update existing E2E tests in `tests/e2e/oauth2_discovery_e2e_test.go` that assert proxy-mode discovery returns 404 — update to expect 200 with `jwks_uri` present
- [ ] T035 [US2] Verify unit tests for US2 pass (green phase) — metadata includes `jwks_uri` in all modes

**Checkpoint**: User Story 2 fully functional — discovery advertises `jwks_uri` in all modes

---

## Phase 5: User Story 3 — Fail-Closed Upstream JWKS Availability (Priority: P2)

**Goal**: Broker starts degraded when upstream is unreachable, returns 503 while upstream key retrieval fails — never serves incomplete verification surface

**Independent Test**: Start broker with unreachable upstream URI and verify the JWKS endpoint returns 503; simulate upstream fetch failure after startup and verify 503 response

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [ ] T036 [P] [US3] Unit test: proxy mode returns `ErrUpstreamUnavailable` when upstream adapter errors in `internal/domain/jwkspublisher/service_test.go`
- [ ] T037 [P] [US3] Unit test: hybrid mode returns `ErrUpstreamUnavailable` when upstream adapter errors in `internal/domain/jwkspublisher/service_test.go`
- [ ] T038 [P] [US3] Unit test: local mode succeeds without upstream source in `internal/domain/jwkspublisher/service_test.go`

### Implementation for User Story 3

- [ ] T039 [US3] Add startup validation in `internal/app/builder.go` — eagerly call `GetKeySet(ctx)` with timeout on upstream adapter; if unreachable, log degraded startup state and allow `/oauth2/jwks.json` to return 503 until recovery
- [ ] T040 [US3] Implement upstream availability check in `PublishJWKS` in `internal/domain/jwkspublisher/service.go` — propagate adapter errors as `ErrUpstreamUnavailable`
- [ ] T041 [US3] Add upstream health tracking to publisher service in `internal/domain/jwkspublisher/service.go` — track last successful upstream fetch and expose degraded health when refreshes fail or become too old
- [ ] T042 [US3] Verify unit tests for US3 pass (green phase) — all fail-closed scenarios

**Checkpoint**: User Story 3 fully functional — broker fails closed on upstream unavailability

---

## Phase 6: User Story 4 — Duplicate Key ID Detection (Priority: P2)

**Goal**: Broker detects and rejects duplicate `kid` values across local and upstream key sets in hybrid mode — prevents key confusion attacks

**Independent Test**: Configure a local key with a `kid` matching an upstream key and verify the broker rejects aggregation

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [ ] T043 [P] [US4] Unit test: hybrid mode returns `ErrKidConflict` when local and upstream share a `kid` in `internal/domain/jwkspublisher/service_test.go`
- [ ] T044 [P] [US4] Unit test: hybrid mode publishes all keys without modification when no `kid` conflict in `internal/domain/jwkspublisher/service_test.go`
- [ ] T044b [P] [US4] Unit test: hybrid mode returns `ErrKidConflict` when upstream cache refresh introduces a conflicting `kid` at runtime in `internal/domain/jwkspublisher/service_test.go`

### Implementation for User Story 4

- [ ] T045 [US4] Implement kid conflict detection in `PublishJWKS` hybrid path in `internal/domain/jwkspublisher/service.go` — build `map[string]struct{}` from local kids, iterate upstream keys checking for collisions, return `ErrKidConflict` with conflicting kid logged
- [ ] T046 [US4] Add startup kid conflict check in `internal/app/builder.go` — after eager upstream fetch + local key check, compare kid values; conflict → degraded startup state with clear error identifying the conflicting key ID
- [ ] T046b [US4] Implement runtime kid conflict detection on upstream cache refresh in `internal/domain/jwkspublisher/service.go` — when pre-computed snapshot is rebuilt after cache refresh, detect new kid conflicts, set error state, log conflicting kid value; next JWKS request returns 503
- [ ] T047 [US4] Verify unit tests for US4 pass (green phase) — all kid conflict detection scenarios

**Checkpoint**: User Story 4 fully functional — duplicate kid detection prevents key confusion

---

## Phase 7: Observability — Structured Logging & Health Integration (Priority: P2)

**Goal**: Implement observability requirements OB-001, OB-002, OB-003 for operational monitoring of the aggregated JWKS feature

### Tests for Observability [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [ ] T072 [P] Unit test: upstream fetch success emits structured log event with key count in `internal/domain/jwkspublisher/service_test.go`
- [ ] T073 [P] Unit test: upstream fetch failure emits structured log event with error details in `internal/domain/jwkspublisher/service_test.go`
- [ ] T074 [P] Unit test: kid conflict detection emits structured log event with conflicting `kid` value in `internal/domain/jwkspublisher/service_test.go`
- [ ] T075 [P] Unit test: health state reports degraded when upstream JWKS expired in `internal/domain/jwkspublisher/service_test.go`

### Implementation for Observability

- [ ] T076 Add structured slog log events in `internal/domain/jwkspublisher/service.go` for: upstream fetch success (key count), upstream fetch failure (error), cache refresh, and kid conflict (conflicting kid value) — using `slog` structured attributes per ADR 20260301-120000
- [ ] T077 Expose `HealthState() string` method on JWKSPublisherService returning "healthy" or "degraded" based on upstream cache freshness
- [ ] T078 Integrate `HealthState()` into existing health endpoint in `internal/app/builder.go` or health handler — upstream JWKS degraded state surfaced in health response
- [ ] T079 Verify unit tests for observability pass (green phase)

**Checkpoint**: Observability complete — structured logs emitted, health endpoint reflects upstream JWKS state

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T048 Verify domain model design is documented in ARCHITECTURE.md Glossary — `AggregatedKeySet`, `KeySource`, `JWKSPublisher` present (Principle V)
- [ ] T049 Verify no new configuration parameters needed — no YAML examples required (Principle VII)
- [ ] T050 Verify API designs documented in `/api/enduser/openapi.yaml` for both modified endpoints (Principles IV, X)
- [ ] T051 Verify user/stakeholder confirmed API designs — supersedes documented in PR (Principle X)
- [ ] T052 Verify no database changes required — confirmed in PR description (Principle IX)
- [ ] T053 Verify E2E acceptance tests written in `tests/e2e/aggregated_jwks_test.go` for all 21 spec scenarios (Principle XIII)
- [ ] T054 Verify E2E tests FAIL before implementation (red phase): detailed expectations written and failing; no placeholder always-fail assertions; no `XIt`/`PIt`/`Skip()` markers; no "red phase" comments in test files (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):
- [ ] T055 [P] Verify `/api/enduser/openapi.yaml` updated with both modified endpoints (JWKS + discovery)
- [ ] T056 [P] Verify API implementation matches confirmed OpenAPI specification exactly

**Architecture & Documentation** (Principle II):
- [ ] T057 Verify ARCHITECTURE.md updated with new domain concepts in Glossary section

**Configuration** (Principle VII):
- [ ] T058 [P] Verify no new configuration loading — behavior derives from existing `mode` + `upstream_issuer_uri`

**Security** (Principles I, III):
- [ ] T059 Verify fail-closed behavior: incomplete verification surface never served (upstream unavailable → 503, kid conflict → 503, startup → failure)
- [ ] T060 [P] Verify no custom cryptography — only `lestrrat-go/jwx/v3` for JWK operations
- [ ] T061 [P] Verify private key material never exposed in JWKS response

**Architecture Patterns** (Principle VI):
- [ ] T062 Verify hexagonal architecture: `JWKSPublisherService` (domain) → `JWKSPublisherPort` (port) → handler uses port interface, never concrete service

**Testing** (Principle VIII - Unit & Integration Tests):
- [ ] T063 Verify unit tests written FIRST in `internal/domain/jwkspublisher/service_test.go` and failed before implementation (red-green TDD)
- [ ] T064 Verify tests drive design — implementation emerges from test requirements

**E2E Acceptance Testing** (Principle XIII):
- [ ] T065 Verify all 21 E2E tests in `tests/e2e/aggregated_jwks_test.go` map 1:1 to spec acceptance scenarios
- [ ] T066 Verify E2E tests written BEFORE implementation and failed initially (red phase)
- [ ] T067 Verify E2E tests turned GREEN as implementation satisfied acceptance criteria
- [ ] T068 Run full E2E test suite: `just test-e2e` (all tests must pass)

### Additional Polish

- [ ] T069 Run `just check` — static analysis clean (fmt → vet → lint)
- [ ] T070 Run `just verify` — full verification gate with E2E as final guard layer
- [ ] T071 Verify updated E2E discovery tests in `tests/e2e/oauth2_discovery_e2e_test.go` pass (proxy mode no longer 404)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Design Preconditions (Phase 2)**: No dependencies — can start immediately. BLOCKS all implementation.
  - **CRITICAL**: E2E acceptance tests must be written with detailed expectations and verified to FAIL
  - Phase 2a, 2b, 2c, 2d, 2f can proceed in parallel, but all must complete before Phase 2.5
- **Foundational Infrastructure (Phase 2.5)**: Depends on ALL of Phase 2 completion — BLOCKS all user stories
- **User Stories (Phase 3–6)**: All depend on Phase 2 + Phase 2.5 completion
  - US1 + US2 (both P1) can proceed in parallel after Phase 2.5 completes
  - US3 + US4 (both P2) can proceed in parallel after Phase 2.5 completes
  - US3 depends on US1 being complete (builds on proxy/hybrid handler logic)
  - US4 depends on US1 being complete (builds on hybrid aggregation logic)
- **Constitution Compliance (Phase N)**: Depends on all user stories and observability being complete
- **Observability (Phase 7)**: Depends on US3 completion (uses freshness tracking from T041). Can run in parallel with US4.

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2.5 — no dependencies on other stories
- **User Story 2 (P1)**: Can start after Phase 2.5 — independent of US1 (modifies different files: `oauth2/service.go` vs `jwkspublisher/service.go`)
- **User Story 3 (P2)**: Depends on US1 completion (adds error handling to the same publisher service)
- **User Story 4 (P2)**: Depends on US1 completion (adds kid conflict detection to hybrid aggregation path)

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Domain logic before adapter changes
- Service before handler
- Wiring (builder.go) after service + handler are ready
- Routing changes last

### Parallel Opportunities

- Phase 2a, 2b, 2c, 2d, 2f can all run in parallel
- T009 and T010 (test helpers) can run in parallel with T008 (test writing)
- T013 (port) and T014/T015 (service skeleton) can run in parallel
- US1 and US2 can be implemented in parallel (different source files)
- US3 and US4 can be implemented in parallel after US1 completes (different concerns in same service)
- All [P]-marked tasks within a phase can run simultaneously

---

## Parallel Example: User Story 1

```bash
# Launch all unit tests for User Story 1 together:
Task: T018 "Unit tests for local-mode aggregation in internal/domain/jwkspublisher/service_test.go"
Task: T019 "Unit tests for proxy-mode aggregation in internal/domain/jwkspublisher/service_test.go"
Task: T020 "Unit tests for hybrid-mode aggregation in internal/domain/jwkspublisher/service_test.go"
Task: T021 "Unit test verifying private key exclusion in internal/domain/jwkspublisher/service_test.go"

# Then implement sequentially (same file, dependencies):
Task: T022 "Implement local-mode logic in internal/domain/jwkspublisher/service.go"
Task: T023 "Implement proxy-mode logic in internal/domain/jwkspublisher/service.go"
Task: T024 "Implement hybrid-mode logic in internal/domain/jwkspublisher/service.go"

# Then adapter + wiring (different files, can parallel):
Task: T025 "Modify JWKS handler in internal/adapters/http/handlers/enduser/jwks_handler.go"
Task: T027 "Update routing in internal/adapters/http/routing/enduser.go"
# followed by:
Task: T026 "Wire JWKSPublisherService in internal/app/builder.go" (depends on T025, T027)
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2)

1. Complete Phase 2: Design Preconditions (all sub-phases in parallel)
   - 2a: Domain model glossary updates
   - 2b: Configuration confirmation (no changes)
   - 2c: OpenAPI updates + stakeholder confirmation
   - 2d: Database confirmation (no changes)
   - 2f: E2E acceptance tests (21 scenarios, red phase)
2. Complete Phase 2.5: Foundational Infrastructure (port + service skeleton)
3. Complete Phase 3: User Story 1 (core JWKS aggregation)
4. Complete Phase 4: User Story 2 (discovery metadata)
5. **STOP and VALIDATE**: Test US1 + US2 independently — `just test` + relevant E2E scenarios
6. Deploy/demo if ready (MVP delivers the core value proposition)

### Incremental Delivery

1. Phase 2 + 2.5 → Foundation ready
2. US1 + US2 (P1) → Core aggregation + discovery → Deploy/Demo (MVP!)
3. US3 (P2) → Fail-closed availability → Deploy/Demo (hardened)
4. US4 (P2) → Kid conflict detection → Deploy/Demo (complete)
5. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers or agents:

1. Phase 2 tasks in parallel:
   - **Agent A**: Phase 2a (glossary) + Phase 2c (OpenAPI updates)
   - **Agent B**: Phase 2f (E2E test writing — largest effort)
2. Phase 2.5 (single agent — small scope):
   - **Agent A**: Port + service skeleton + unit test shell
3. User Stories in parallel after Phase 2.5:
   - **Agent A**: US1 (core aggregation — most complex)
   - **Agent B**: US2 (discovery metadata — small, independent)
4. After US1 completes:
   - **Agent A**: US3 (fail-closed)
   - **Agent B**: US4 (kid conflict detection)
5. Phase N: Either agent runs verification

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story is independently completable and testable
- Verify tests fail before implementing (red phase mandatory)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- No Phase 0 (no refactoring needed) or Phase 1 (no project init needed) per plan.md
- No Phase 2.7 (no new persisted entities)
- No frontend tasks (no UI changes in this feature)
