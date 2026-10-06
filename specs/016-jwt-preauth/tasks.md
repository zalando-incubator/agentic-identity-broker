# Tasks: JWT Pre-Authentication & Principal Profile Enrichment

**Input**: Design documents from `/specs/016-jwt-preauth/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY for all features. Test tasks are included in each user story below and MUST be written before or alongside implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create directories and skeleton files for the JWT pre-auth feature

- [x] T001 Create domain directory `internal/domain/jwtauth/` and adapter directory `internal/adapters/jwtauth/`
- [x] T002 [P] Create configuration example file `examples/config/jwt-preauth.yaml` with signed, unsigned, and backward-compatible YAML examples from spec.md
- [x] T003 [P] Update `examples/config/README.md` to reference the new `jwt-preauth.yaml` configuration section

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [x] T004 Document domain model: PrincipalProfile value object, AuthResult value object, JWTAuthenticator port interface, JWTValidationFailed domain event — per `specs/016-jwt-preauth/data-model.md`
- [x] T004a [P] Add domain terms to ARCHITECTURE.md Glossary: PrincipalProfile, JWTAuthConfig, JWTAuthenticator, JWTVerificationMode, JWTValidationFailed
- [x] T004b [P] Document invariants: Principal non-empty ≤200 chars, DisplayName defaults to Principal, Email/PictureURL nil when absent, expiry always required

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [x] T005 Define `JWTConfig` and `JWTClaimExtractionConfig` types in `internal/ports/config.go` with mapstructure tags
- [x] T005a Extend `AuthenticationConfig` in `internal/ports/config.go` with `JWT *JWTConfig` pointer field
- [x] T005b [P] Set defaults for JWT config in `DefaultServerConfig()` in `internal/ports/config.go`: `HeaderName: "Authorization"`, `Verification: "jwks"`, `PrincipalExpression: "claims.sub"`
- [x] T005c [P] Verify `examples/config/jwt-preauth.yaml` committed (from T002) covers signed, unsigned, invalid-combo, and backward-compatible configs

**Checkpoint**: Configuration requirements designed with YAML examples

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

- [x] T006 Update `/api/enduser/openapi.yaml` to add optional `email` field (type: string, format: email, nullable: true) to `UserInfo` schema per `specs/016-jwt-preauth/contracts/openapi-diff.yaml`
- [x] T006a [P] Get user/stakeholder confirmation for the additive `email` field on `UserInfo` (document in PR)

**Checkpoint**: APIs designed and confirmed by user/stakeholder

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [x] T007 Confirm no database changes needed — profile enrichment is request-scoped (extracted from JWT per-request, not stored). Document in PR.

**Checkpoint**: Database schema confirmed (no changes)

### Phase 2e: Frontend/Design System Review [MANDATORY]

> **Historical visual scope**: The design checks below retain their original wording and completion states; the recorded `slate-600` choice is not a current semantic-token recommendation. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

- [x] T008 Review `web/src/design-system/docs/INDEX.md` for component selection — confirm Avatar primitive already supports profile pictures
- [x] T008a [P] Confirm semantic token usage: `trust-deep` for primary text (display name), `slate-600` for secondary text (email label)
- [x] T008b [P] Confirm no new universal components needed — existing Avatar and typography primitives handle all profile display cases

**Checkpoint**: Design system usage planned

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)

- [X] T009 Create E2E test helper `tests/e2e/helpers/mock_jwks_server.go` — mock JWKS server via `httptest.NewServer` serving test JWK set (reuse pattern from `tests/e2e/helpers/mock_upstream.go`)
- [X] T009a [P] Create E2E test helper `tests/e2e/helpers/jwt_helpers.go` — JWT builder functions for signed/unsigned/expired/wrong-audience/wrong-issuer JWTs
- [X] T009b [P] Create E2E test fixtures — add `SignedJWTConfig(jwksURL)`, `UnsignedJWTConfig()`, `NoJWTConfig()` fixture functions in `tests/e2e/fixtures/config.go` (extend existing file if present, create if not)
- [X] T010 Write E2E acceptance tests in `tests/e2e/jwt_preauth_test.go` for all 22 acceptance scenarios from spec.md (US1: 6, US2: 5, US3: 4, US4: 3 via Go Playwright page objects, US5: 3, Edge: 1).
- [X] T010a Map each acceptance scenario to one `It()` block with hierarchical structure: `Describe("JWT Pre-Authentication")` → `Describe("Signed JWT Authentication (US1)")` → `Context/It`
- [X] T010b Include comment references to spec scenarios in format: `// Scenario X.Y from specs/016-jwt-preauth/spec.md (User Story N)`
- [X] T010c Use Ginkgo/Gomega BDD framework following patterns in `tests/e2e/README.md`
- [X] T010d Use fixtures from `tests/e2e/fixtures/` and helpers from `tests/e2e/helpers/` — no hardcoded values
- [X] T010e Implement minimal stubs so E2E tests compile (empty handler, basic route registration, minimal types)
- [X] T010f Verify E2E tests compile AND FAIL semantically (red phase): `ginkgo -v ./tests/e2e/ --focus="JWT Pre-Authentication"`

**Checkpoint**: E2E acceptance tests written and verified to fail before implementation

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T011 Create `PrincipalProfile` value object in `internal/domain/principal/profile.go` with builder pattern: `NewProfile(principal).WithDisplayName(name).WithEmail(email).WithPictureURL(url)`
- [X] T012 [P] Add context functions `WithProfile(ctx, PrincipalProfile)` and `ProfileFromContext(ctx) (PrincipalProfile, bool)` in `internal/domain/principal/profile.go` using separate `profileContextKey` (co-located with PrincipalProfile type)
- [X] T013 Create `JWTAuthenticator` port interface in `internal/domain/jwtauth/authenticator.go` with `Authenticate(ctx, rawJWT) (*AuthResult, error)` method
- [X] T014 [P] Create `AuthResult` value object in `internal/domain/jwtauth/auth_result.go` with fields: Principal, DisplayName, Email, PictureURL, Claims
- [X] T015 [P] Create domain errors in `internal/domain/jwtauth/errors.go`: `ErrInvalidSignature`, `ErrTokenExpired`, `ErrAudienceMismatch`, `ErrIssuerMismatch`, `ErrClaimExtraction`, `ErrMalformedToken`, `ErrMissingExpiry`
- [X] T016 Create CEL evaluator in `internal/domain/jwtauth/cel_evaluator.go` following pattern from `internal/domain/tokenexchange/cel_evaluator.go` — compile four CEL programs (principal, display_name, email, picture_url) at construction, evaluate at runtime with `claims` variable of type `map(string, any)`
- [X] T017 Add JWT config validation in `internal/config/validator.go`: mutual exclusivity check (`verification: none` + `jwks_uri` → startup error), required `jwks_uri` when `verification: jwks`, CEL expression compilation validation at startup, JWKS URI HTTPS scheme enforcement per SR-004 (reject `http://` unless `skip_thirdparty_https_validation` is true)

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 — Accept Signed JWTs for Pre-Authentication (Priority: P1) 🎯 MVP

**Goal**: Operators can configure JWT-based pre-authentication with JWKS signature verification, and the broker authenticates users using signed JWTs.

**Independent Test**: Configure `authentication.jwt` with a JWKS endpoint and signed JWT header, make API request with valid/invalid JWTs, verify principal extraction and rejection behavior.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T018 [P] [US1] Unit tests for CEL evaluator in `internal/domain/jwtauth/cel_evaluator_test.go` — compile errors, string extraction, non-string handling, timeout, principal required
- [x] T019 [P] [US1] Unit tests for PrincipalProfile in `internal/domain/principal/profile_test.go` — construction, defaults (DisplayName falls back to Principal), context storage/retrieval, nil optional fields
- [x] T020 [P] [US1] Unit tests for jwx authenticator in `internal/adapters/jwtauth/jwx_authenticator_test.go` — valid signed JWT, invalid signature → error, expired JWT → error, wrong audience → error, wrong issuer → error, missing exp → error, malformed token → error
- [x] T021 [P] [US1] Unit tests for config validation in `internal/config/validator_test.go` — mutual exclusivity, required fields, defaults, CEL expression validation

### Implementation for User Story 1

- [x] T022 [US1] Implement jwx authenticator adapter in `internal/adapters/jwtauth/jwx_authenticator.go` — use `lestrrat-go/jwx/v3` for JWT parsing with `jwt.WithKeySet(keyset)` and `jwt.WithValidate(true)`, audience/issuer validation, Bearer prefix stripping for Authorization header, delegate claim extraction to CEL evaluator
- [x] T023 [US1] Extend `RequirePrincipalMiddleware` and `OptionalPrincipalMiddleware` in `internal/adapters/http/middleware/principal_middleware.go` — accept `JWTAuthenticator` parameter, check JWT header presence, authenticate via JWTAuthenticator, set both `WithPrincipal` and `WithProfile` in context, reject invalid JWT with 401 (fail-closed, no fallback)
- [x] T024 [US1] Wire JWT authenticator in `internal/app/builder.go` `Build()` method — conditionally create CEL evaluator, JWKS adapter, and jwx authenticator when `config.Server.Enduser.Authentication.JWT != nil`, pass to enduser route config
- [x] T025 [US1] Update `EnduserRouteConfig` in `internal/adapters/http/routing/enduser.go` — add optional `JWTAuthenticator` field, pass to middleware during route setup
- [x] T026 [US1] Add structured audit logging for JWT validation failures (invalid signature, expired, audience/issuer mismatch) per FR-020/SR-005 in `internal/adapters/jwtauth/jwx_authenticator.go`

**Checkpoint**: Signed JWT pre-authentication fully functional — E2E tests for US1 Scenarios 1-6 should turn GREEN

---

## Phase 4: User Story 5 — Backward-Compatible Configuration (Priority: P1)

**Goal**: Operators using plain-header pre-auth experience zero behavior changes. JWT is additive and opt-in.

**Independent Test**: Deploy with existing config (only `authentication.preauth.principal_header_name`), verify all endpoints work identically to current behavior.

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

- [X] T027 [P] [US5] Unit tests for middleware fallback logic in `internal/adapters/http/middleware/principal_middleware_test.go` — plain-header-only config works unchanged, JWT+header config prefers JWT when present, fallback to plain header when JWT absent, reject when JWT present but invalid (no silent fallback)

### Implementation for User Story 5

- [X] T028 [US5] Verify and test that middleware falls back to plain header when JWT header absent but plain header present in `internal/adapters/http/middleware/principal_middleware.go`
- [X] T029 [US5] Verify and test that builder creates no JWT authenticator when `JWT` config is nil in `internal/app/builder.go`
- [X] T030 [US5] Verify fail-closed behavior: when JWT header present but invalid, reject with 401 even when plain header also present (FR-013) in `internal/adapters/http/middleware/principal_middleware.go`

**Checkpoint**: Backward compatibility verified — E2E tests for US5 Scenarios 1-3 and edge case (invalid JWT no fallback) should turn GREEN

---

## Phase 5: User Story 2 — Accept Unsigned (Pre-Authenticated) JWTs (Priority: P2)

**Goal**: Operators in service mesh environments can use unsigned JWTs (alg: "none") when explicitly configured.

**Independent Test**: Configure `authentication.jwt` with `verification: none`, send unsigned JWT, verify principal extraction. Verify unsigned JWTs rejected when verification is `jwks`. Verify startup fails when `verification: none` and `jwks_uri` both present.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [X] T031 [P] [US2] Unit tests for unsigned JWT handling in `internal/adapters/jwtauth/jwx_authenticator_test.go` — unsigned JWT accepted when `verification: none`, unsigned JWT rejected when `verification: jwks`, signed JWT accepted without signature check when `verification: none`, expired JWT rejected regardless of verification mode
- [X] T032 [P] [US2] Unit tests for config validation in `internal/config/validator_test.go` — startup fails when `verification: none` + `jwks_uri` both present (mutual exclusivity)

### Implementation for User Story 2

- [X] T033 [US2] Add unsigned mode to jwx authenticator in `internal/adapters/jwtauth/jwx_authenticator.go` — use `jwt.Parse(rawToken, jwt.WithVerify(false))` + `jwt.WithValidate(true)` when `verification: none`; always enforce expiry regardless of mode
- [X] T034 [US2] Verify startup-time mutual exclusivity check (implemented in T017) in `internal/config/validator.go` produces clear error: `"authentication.jwt: verification 'none' and jwks_uri are mutually exclusive"` — verification only, no new code expected

**Checkpoint**: Unsigned JWT support fully functional — E2E tests for US2 Scenarios 1-5 should turn GREEN

---

## Phase 6: User Story 3 — Extract Profile Attributes from JWT Using CEL (Priority: P2)

**Goal**: Operators can configure CEL expressions for display_name, email, and picture_url extraction. Enriched profile surfaces through `/api/me`.

**Independent Test**: Configure CEL expressions, send JWT with matching claims, verify `/api/me` returns extracted values. Verify missing claims produce nil fields. Verify plain-header mode returns principal-only profile.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [X] T035 [P] [US3] Unit tests for enriched UserInfo handler in `internal/adapters/http/handlers/consent/user_info_handler_test.go` — enriched profile with all fields, partial profile (missing some claims), principal-only for plain header mode
- [X] T036 [P] [US3] Unit tests for UserInfo domain type in `internal/domain/consent/user_info_test.go` — verify Email field serialization with `json:"email,omitempty"`

### Implementation for User Story 3

- [X] T037 [US3] Add `Email *string` field to `UserInfo` struct in `internal/domain/consent/user_info.go` with `json:"email,omitempty"` tag
- [X] T038 [US3] Update `UserInfoHandler` in `internal/adapters/http/handlers/consent/user_info_handler.go` — read `PrincipalProfile` via `principal.ProfileFromContext(ctx)`, construct enriched `UserInfo` with display name, email, and picture URL from profile; fall back to `principal.FromContext(ctx)` for backward compatibility
- [X] T039 [US3] Verify CEL evaluator handles missing claims gracefully — non-string or absent CEL result for optional fields treated as nil (log warning), non-string principal result is authentication failure in `internal/domain/jwtauth/cel_evaluator.go`

**Checkpoint**: Profile extraction fully functional — E2E tests for US3 Scenarios 1-4 should turn GREEN

---

## Phase 7: User Story 4 — Display User Profile in Consent UI (Priority: P3)

**Goal**: Consent UI header shows display name, email, and profile picture when available, with graceful fallback.

**Independent Test**: Mock `/api/me` response with profile attributes, verify header renders avatar, display name (primary), and email (secondary). Mock without attributes, verify fallback to principal display.

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [ ] T040 [P] [US4] Go Playwright E2E tests for header display in `tests/e2e/jwt_preauth_test.go` — use page objects from `tests/e2e/pages/` to verify: with email + picture + display name, with principal only (fallback), with display name + email but no picture (initials avatar)

### Implementation for User Story 4

- [X] T041 [US4] Add `email?: string` to `UserInfo` interface in `web/src/types/consent.ts`
- [X] T042 [US4] Update header in `web/src/components/layout/AppLayout.tsx` — display email as secondary label when available (using `slate-600` token), show principal as fallback when no email; profile picture via existing Avatar primitive, initials avatar fallback from display name
- [X] T043 [US4] Ensure profile picture `alt` text set to display name for accessibility, email text readable by screen readers, keyboard navigation maintained

**Checkpoint**: Consent UI profile display fully functional — Go Playwright E2E tests pass

---

## 🔒 Phase 8: Constitution Compliance & Polish

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [X] T044 Verify domain model design is documented in ARCHITECTURE.md Glossary (Principle V)
- [X] T045 Verify configuration design YAML examples exist in `examples/config/jwt-preauth.yaml` (Principle VII)
- [X] T046 Verify configuration examples referenced in `examples/config/README.md` (Principle VII)
- [X] T047 Verify API design documented in `/api/enduser/openapi.yaml` with `email` field on `UserInfo` (Principles IV, X)
- [X] T048 Verify user/stakeholder confirmed API design (document reference in PR) (Principle X)
- [X] T049 Verify no database changes documented in PR (Principle IX)
- [X] T050 Verify design system review completed — Avatar primitive, semantic tokens confirmed (Principle XI)
- [X] T051 Verify E2E acceptance tests written in `tests/e2e/jwt_preauth_test.go` for all 22 spec scenarios including US4 Playwright frontend tests (Principle XIII)
- [X] T052 Verify E2E tests verified to FAIL before implementation (red phase) (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**API & Documentation** (Principles IV, X):
- [X] T053 [P] Verify `/api/enduser/openapi.yaml` updated with `email` field on `UserInfo` schema
- [X] T054 [P] Verify API implementation matches confirmed OpenAPI specification exactly
- [X] T055 Update `docs/configuration.md` with JWT pre-authentication section documenting all config options

**Architecture & Documentation** (Principle II):
- [X] T056 Update ARCHITECTURE.md with JWT pre-auth architectural description and glossary terms
- [X] T057 [P] Confirm no new ADR needed — reuses existing patterns (CEL: ADR 009, JWKS adapter: ADR 008, hexagonal architecture)

**Configuration** (Principle VII):
- [X] T058 [P] Verify configuration uses unified system configuration port in `internal/ports/config.go` (not custom loading)

**Database & Persistence** (Principle IX):
- [X] T059 [P] Confirm no migrations needed — profile enrichment is request-scoped

**Security** (Principles I, III):
- [X] T060 Verify security features enabled by default: `verification` defaults to `jwks`, unsigned requires explicit opt-in, fail-closed on all failures
- [X] T060a [P] Verify JWKS URI HTTPS scheme enforcement: `http://` JWKS URIs rejected at startup unless `skip_thirdparty_https_validation` is true (SR-004)
- [X] T061 [P] Verify no custom cryptography — only `lestrrat-go/jwx/v3` and `google/cel-go` used
- [X] T062 [P] Verify structured audit logging present for JWT validation failures (SR-005/FR-020)

**Architecture Patterns** (Principle VI):
- [X] T063 Verify `JWTAuthenticator` port interface in domain, jwx adapter in adapters — clean hexagonal separation

**Testing** (Principle VIII — Unit & Integration Tests):
- [X] T064 Verify unit tests written FIRST and failed before implementation (red-green TDD)
- [X] T065 Verify tests drive design (implementation emerged from test requirements)
- [X] T066 Verify tests changed minimally during implementation
- [X] T067 Verify automated tests included: unit tests for CEL evaluator, jwx authenticator, PrincipalProfile, config validation, middleware, handler

**E2E Acceptance Testing** (Principle XIII):
- [X] T068 Verify E2E tests exist in `tests/e2e/jwt_preauth_test.go` for all 22 acceptance scenarios (including US4 Playwright frontend tests)
- [X] T069 Verify each `It()` block maps to exactly ONE acceptance scenario from spec.md
- [X] T070 Verify E2E tests written BEFORE implementation and failed initially (red phase)
- [X] T071 Verify E2E tests changed minimally during implementation (fixture adjustments only)
- [X] T072 Verify E2E tests turned GREEN as implementation satisfied acceptance criteria
- [X] T073 Verify E2E tests use Ginkgo/Gomega framework following `tests/e2e/README.md` patterns
- [X] T074 Verify E2E test organization uses hierarchical structure (Describe → Context → It)
- [X] T075 Verify E2E tests include comment references to spec scenarios
- [X] T076 Run full E2E test suite: `ginkgo -v ./tests/e2e/ --focus="JWT Pre-Authentication"` (all tests must pass)

**Frontend** (Principle XI):
- [X] T077 Verify frontend components use design system Avatar primitive and semantic tokens
- [X] T078 Verify no custom CSS bypassing design tokens
- [X] T079 Verify WCAG 2.1 AA accessibility: profile picture `alt` text, email screen-reader friendly, keyboard navigation

**Builder & Routing** (Principle XII):
- [X] T080 Verify JWT authenticator instantiated in `internal/app/builder.go` `Build()` method
- [X] T081 Verify routing in `internal/adapters/http/routing/enduser.go` receives pre-wired authenticator, does NOT instantiate services

### Additional Polish

- [X] T082 Run `just check` (fmt, vet, lint) and `just verify` — all checks must pass
- [ ] T083 Run quickstart.md validation: manually verify signed JWT, unsigned JWT, and backward-compatible configurations per `specs/016-jwt-preauth/quickstart.md`
- [X] T084 Code cleanup: ensure consistent error messages, remove any TODOs or placeholder code

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Design Preconditions (Phase 2)**: Depends on Setup completion — BLOCKS all implementation
  - **CRITICAL**: Domain model must be designed and documented (2a)
  - **CRITICAL**: Configuration types must be defined with defaults (2b)
  - **CRITICAL**: API design must be updated and confirmed (2c)
  - **CRITICAL**: Database design confirmed (no changes) (2d)
  - **CRITICAL**: Frontend/design system review completed (2e)
  - **CRITICAL**: E2E acceptance tests must be written and verified to FAIL (2f)
  - Phases 2a–2e can proceed in parallel; 2f depends on 2a and 2b for types/fixtures
- **Foundational Infrastructure (Phase 2.5)**: Depends on ALL of Phase 2 — BLOCKS all user stories
- **User Story 1 (Phase 3, P1)**: Depends on Phase 2.5 — core signed JWT authentication
- **User Story 5 (Phase 4, P1)**: Depends on Phase 3 (US1) — verifies backward compatibility with US1 changes
- **User Story 2 (Phase 5, P2)**: Depends on Phase 3 (US1) — extends jwx authenticator with unsigned mode
- **User Story 3 (Phase 6, P2)**: Depends on Phase 3 (US1) — profile extraction via `/api/me`
- **User Story 4 (Phase 7, P3)**: Depends on Phase 6 (US3) — frontend display of enriched profile
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: First — establishes JWT authentication infrastructure
- **User Story 5 (P1)**: After US1 — validates backward compatibility with JWT changes
- **User Story 2 (P2)**: After US1 — extends authenticator with unsigned mode
- **User Story 3 (P2)**: After US1 — enriches `/api/me` endpoint with profile attributes
- **User Story 4 (P3)**: After US3 — displays enriched profile in consent UI

### Within Each User Story

- Tests MUST be written and FAIL before implementation (TDD red phase)
- Domain types/ports before adapters
- Adapters before middleware integration
- Middleware before handler/routing updates
- Story complete before moving to next priority

### Parallel Opportunities

**Phase 1 (Setup)**:
- T002 and T003 can run in parallel (different files)

**Phase 2 (Design)**:
- T004a, T004b can run in parallel with each other
- T005b, T005c can run in parallel
- T008, T008a, T008b can run in parallel
- T009, T009a, T009b can run in parallel (different helper/fixture files)

**Phase 2.5 (Foundation)**:
- T012, T014, T015 can run in parallel (different files in different packages)

**Phase 3 (US1 Tests)**:
- T018, T019, T020, T021 can ALL run in parallel (different test files)

**Phase 5 (US2 Tests)**:
- T031, T032 can run in parallel (different test files)

**Phase 6 (US3 Tests)**:
- T035, T036 can run in parallel (different test files)

---

## Parallel Example: User Story 1

```bash
# Launch all unit tests for US1 in parallel (different files):
Task T018: "Unit tests for CEL evaluator in internal/domain/jwtauth/cel_evaluator_test.go"
Task T019: "Unit tests for PrincipalProfile in internal/domain/principal/profile_test.go"
Task T020: "Unit tests for jwx authenticator in internal/adapters/jwtauth/jwx_authenticator_test.go"
Task T021: "Unit tests for config validation in internal/config/validator_test.go"

# Then implement sequentially (dependencies exist):
Task T022: "Implement jwx authenticator adapter" (depends on T016 CEL evaluator, T013 port interface)
Task T023: "Extend principal middleware" (depends on T022 authenticator, T011/T012 profile context)
Task T024: "Wire in builder.go" (depends on T022 authenticator, T023 middleware)
Task T025: "Update routing" (depends on T024 builder)
Task T026: "Add audit logging" (depends on T022 authenticator)
```

---

## Implementation Strategy

### MVP First (User Story 1 + User Story 5)

1. Complete Phase 1: Setup
2. Complete Phase 2: Design Preconditions (ALL sub-phases 2a–2f)
   - 2a: Domain model & glossary → ARCHITECTURE.md
   - 2b: Configuration types in `internal/ports/config.go` + validation in `internal/config/validator.go`
   - 2c: OpenAPI update (email field) + stakeholder confirmation
   - 2d: Confirm no DB changes
   - 2e: Frontend/design system review
   - 2f: E2E tests written and verified to FAIL (red phase)
3. Complete Phase 2.5: Foundational Infrastructure (PrincipalProfile, JWTAuthenticator port, CEL evaluator, domain errors, config validation)
4. Complete Phase 3: User Story 1 (signed JWT authentication)
5. Complete Phase 4: User Story 5 (backward compatibility)
6. **STOP and VALIDATE**: Signed JWT auth works, existing config unaffected
7. Deploy/demo MVP

### Incremental Delivery

1. Setup → Design Preconditions → Foundation → **Foundation ready**
2. Add User Story 1 (signed JWT) → Test independently → **MVP signed auth**
3. Add User Story 5 (backward compat) → Test independently → **MVP validated**
4. Add User Story 2 (unsigned JWT) → Test independently → **Service mesh support**
5. Add User Story 3 (profile extraction) → Test independently → **Enriched /api/me**
6. Add User Story 4 (consent UI) → Test independently → **Full feature complete**
7. Each story adds value without breaking previous stories
