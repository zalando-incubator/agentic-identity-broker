# Implementation Plan: JWT Pre-Authentication & Principal Profile Enrichment

**Branch**: `016-jwt-preauth` | **Date**: 2026-02-27 | **Spec**: [spec.md](specs/016-jwt-preauth/spec.md)
**Input**: Feature specification from `/specs/016-jwt-preauth/spec.md`

## Summary

Extend the identity broker's pre-authentication layer to accept signed and unsigned JWTs alongside the existing plain-header mode. JWT claims are extracted via configurable CEL expressions to derive the principal identifier and optional profile attributes (display name, email, picture URL). The enriched profile is surfaced through the `/api/me` endpoint and rendered in the consent UI header. Implementation uses `lestrrat-go/jwx/v3` (already a dependency at v3.0.13) for JWT parsing, signature verification, and JWKS caching, and `google/cel-go` (already at v0.27.0) for claim extraction expressions. No new database tables are required; changes are confined to configuration, authentication middleware, the `/api/me` handler, and the consent UI header.

## Technical Context

**Language/Version**: Go 1.23.0+ (backend), TypeScript 5.3+ / React 18.2+ (frontend)  
**Primary Dependencies**: `lestrrat-go/jwx/v3` v3.0.13 (JWT/JWKS), `google/cel-go` v0.27.0 (CEL expressions), Chi (HTTP router), Viper/Cobra (config), Tailwind CSS v4 / Headless UI (frontend)  
**Storage**: N/A — no new persistence entities or migrations; enriched profile is request-scoped  
**Testing**: Go `testing` + Ginkgo/Gomega (E2E), Go Playwright (frontend E2E), table-driven tests for validation  
**Target Platform**: Linux server (Docker/Kubernetes), browser SPA  
**Project Type**: web (Go backend + React SPA frontend)  
**Performance Goals**: JWT validation < 50ms p95 (per SC-004); JWKS cache refresh handled by `jwx` library background goroutine  
**Constraints**: Zero clock skew tolerance for JWT expiry; JWKS must be reachable at startup (fail-closed); unsigned JWTs rejected by default  
**Scale/Scope**: Adds ~6 new Go source files (JWT authenticator, CEL evaluator, config types, middleware integration), ~2 frontend file changes (AppLayout header, TypeScript types), OpenAPI schema update, E2E test file

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Have entities, aggregates, value objects been identified and documented?
  - Value objects: `PrincipalProfile` (enriched user identity), `JWTValidationResult` (parsed/validated claims). No new entities — enriches existing Principal concept. Documented in spec § Domain Model.
- [x] **Domain Concepts**: Will new domain terms be added to ARCHITECTURE.md Glossary?
  - New terms: PrincipalProfile, JWTAuthConfig, JWTValidationResult, JWTVerificationMode. Will be added during implementation.
- [x] **Configuration Design**: Have all config requirements been identified with YAML examples?
  - Full `authentication.jwt` block designed: `header_name`, `verification`, `jwks_uri`, `expected_audience`, `expected_issuer`, `claim_extraction.*`. Four YAML examples in spec (signed, unsigned, invalid combo, backward-compatible).
- [x] **Config Examples**: Will example YAML snippets be added to examples/config/?
  - `examples/config/jwt-preauth.yaml` will be created with all three valid configurations (signed, unsigned, header-only).
- [x] **API Design First**: Will APIs be designed (OpenAPI spec) and confirmed BEFORE implementation?
  - Only change: additive `email` field on `UserInfo` schema in `/api/enduser/openapi.yaml`. Non-breaking. OpenAPI updated before implementation.
- [x] **API Documentation**: Will OpenAPI specs be created in `/api/enduser/` or `/api/admin/` as applicable?
  - Update to existing `/api/enduser/openapi.yaml` — add `email` field to `UserInfo` schema.
- [x] **API Changes**: Are all API changes confirmed by user/stakeholder (document in PR)?
  - API change is additive only (new optional `email` field). Will be documented in PR for confirmation.
- [x] **Database Design**: Will all schema changes use go-migrate naming in `/migrations/`?
  - N/A — no database changes. Profile enrichment is request-scoped (extracted from JWT per-request, not stored).
- [x] **E2E Acceptance Tests**: Will E2E tests be written for ALL spec scenarios BEFORE implementation?
  - Yes. `tests/e2e/jwt_preauth_test.go` will cover 19 backend acceptance scenarios (US1-3, US5, edge cases). US4 (3 frontend scenarios) covered by Go Playwright E2E tests in `tests/e2e/jwt_preauth_test.go` using page objects from `tests/e2e/pages/`.
- [x] **E2E Test Mapping**: Will each acceptance scenario map 1:1 to one It() block in tests/e2e/?
  - Yes. Each scenario maps to one `It()` with spec reference comment.
- [x] **E2E Red Phase**: Will E2E tests FAIL initially, proving they test actual functionality?
  - Yes. Tests will compile against minimal stubs but fail semantically until JWT auth is implemented.

**Implementation Considerations**:

- [x] **Security-First**: Are security features enabled by default? No bypasses or optional security?
  - JWT signature verification defaults to `jwks` (signed). Unsigned JWTs require explicit `verification: none`. Fail-closed: invalid JWT → 401. No fallback from invalid JWT to plain header.
- [x] **Architecture Docs**: Will ARCHITECTURE.md be updated if this touches architecture?
  - Yes — Glossary additions for PrincipalProfile, JWTAuthConfig. Configuration subsystem section update to document JWT auth.
- [ ] **ADRs**: Does this require an ADR in adrs/ for major decisions?
  - No new ADR needed. Reuses existing patterns: CEL (ADR 009), JWKS adapter (ADR 008), hexagonal architecture. `lestrrat-go/jwx/v3` is already a project dependency for the same purpose (token exchange JWKS).
- [x] **Library-First Security**: Are we using vetted libraries for crypto/security (no custom implementations)?
  - `lestrrat-go/jwx/v3` (already in go.mod at v3.0.13) for JWT parsing, signature verification, JWKS cache. `google/cel-go` (already at v0.27.0) for CEL evaluation. No custom crypto.
- [x] **Zalando Guidelines**: Will APIs follow Zalando RESTful API and Event Guidelines?
  - Additive field on existing `UserInfo` response. Follows existing `{"data": ...}` envelope pattern.
- [x] **End-User Docs**: Will API documentation be rendered in `docs/api/` with examples?
  - Will update `docs/configuration.md` with JWT pre-auth section. API change is minimal (one field addition).
- [x] **Migration Testing**: Will migrations be tested (apply/rollback) in PostgreSQL integration tests?
  - N/A — no migrations.
- [x] **Hexagonal Architecture**: Does domain logic use ports (interfaces) with clear adapter separation?
  - JWT authentication will follow port/adapter pattern: `JWTAuthenticator` port interface, `jwx`-based adapter, CEL evaluator in domain layer.
- [x] **Persistence Patterns**: If adding persistence, will it follow specs/004-persistence-layer/quickstart.md?
  - N/A — no persistence changes.

*All BLOCKING preconditions pass. No violations. Proceeding to Phase 0.*

## Project Structure

### Documentation (this feature)

```text
specs/016-jwt-preauth/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── openapi-diff.yaml  # OpenAPI schema diff (email field addition)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── ports/
│   └── config.go                          # Add JWTConfig, JWTClaimExtractionConfig types
├── config/
│   └── schema.go                          # Wire JWT config into Config struct with validation
├── domain/
│   ├── consent/
│   │   └── user_info.go                   # Add Email field to UserInfo value object
│   └── jwtauth/                           # NEW — JWT pre-auth domain
│       ├── authenticator.go               # JWTAuthenticator port interface
│       ├── cel_evaluator.go               # CEL-based claim extraction (reuses ADR 009 pattern)
│       └── errors.go                      # Domain errors (validation, extraction failures)
├── principal/
│   └── profile.go                         # PrincipalProfile value object (enriches existing principal package)
├── adapters/
│   ├── jwtauth/                           # NEW — JWT authentication adapter
│   │   └── jwx_authenticator.go           # lestrrat-go/jwx v3 implementation
│   └── http/
│       ├── middleware/
│       │   └── principal_middleware.go     # MODIFY — add JWT authentication path
│       ├── handlers/
│       │   └── consent/
│       │       └── user_info_handler.go   # MODIFY — read enriched profile from context
│       └── routing/
│           └── enduser.go                 # MODIFY — pass JWT config to middleware
└── app/
    └── builder.go                         # MODIFY — wire JWTAuthenticator if configured

# Frontend (TypeScript/React)
web/src/
├── types/
│   └── consent.ts                         # MODIFY — add email to UserInfo interface
└── components/
    └── layout/
        └── AppLayout.tsx                  # MODIFY — display email in header

# API Documentation
api/enduser/
└── openapi.yaml                           # MODIFY — add email field to UserInfo schema

# Configuration Examples
examples/config/
├── jwt-preauth.yaml                       # NEW — JWT pre-auth examples
└── README.md                              # MODIFY — reference jwt-preauth.yaml

# E2E Tests
tests/e2e/
├── jwt_preauth_test.go                    # NEW — E2E tests (19 backend scenarios)
├── fixtures/
│   └── jwt_config.go                      # NEW — JWT test configuration fixtures
└── helpers/
    └── jwt_helpers.go                     # NEW — JWT creation/signing helpers for tests
```

**Structure Decision**: Follows existing hexagonal architecture pattern. JWT authentication is a new domain (`internal/domain/jwtauth/`) with its own adapter (`internal/adapters/jwtauth/`). The `jwtauth` domain provides a port interface consumed by the principal middleware. No new persistence layer — profile enrichment is request-scoped. CEL evaluator follows the pattern established in `internal/domain/tokenexchange/cel_evaluator.go` (ADR 009).

## Testing Strategy

<!--
  Per Constitution Principle XIII (End-to-End Acceptance Testing & Spec Traceability):
  All features MUST have E2E acceptance tests mapped 1:1 to spec scenarios.
-->

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/jwt_preauth_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: "JWT Pre-Authentication"
- **Nested Describe**: Per User Story ("Signed JWT Authentication", "Unsigned JWT Authentication", etc.)
- **Context blocks**: Preconditions ("when JWT is valid", "when JWT has expired", etc.)
- **It blocks**: Individual acceptance scenarios (one per scenario from spec.md)

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1 Scenario 1 | `jwt_preauth_test.go` | `It("should extract principal from valid signed JWT via CEL expression")` |
| US1 Scenario 2 | `jwt_preauth_test.go` | `It("should reject JWT with invalid signature with 401")` |
| US1 Scenario 3 | `jwt_preauth_test.go` | `It("should reject expired JWT with 401")` |
| US1 Scenario 4 | `jwt_preauth_test.go` | `It("should reject request without JWT header on protected route with 401")` |
| US1 Scenario 5 | `jwt_preauth_test.go` | `It("should reject JWT with wrong audience with 401")` |
| US1 Scenario 6 | `jwt_preauth_test.go` | `It("should reject JWT with wrong issuer with 401")` |
| US2 Scenario 1 | `jwt_preauth_test.go` | `It("should accept unsigned JWT when verification is none")` |
| US2 Scenario 2 | `jwt_preauth_test.go` | `It("should reject unsigned JWT when verification is jwks")` |
| US2 Scenario 3 | `jwt_preauth_test.go` | `It("should accept signed JWT without checking signature when verification is none")` |
| US2 Scenario 4 | `jwt_preauth_test.go` | `It("should reject expired JWT even when verification is none")` |
| US2 Scenario 5 | `jwt_preauth_test.go` | `It("should fail startup when verification none and jwks_uri both present")` |
| US3 Scenario 1 | `jwt_preauth_test.go` | `It("should return display name, email, and picture URL from JWT claims in /api/me")` |
| US3 Scenario 2 | `jwt_preauth_test.go` | `It("should omit missing profile attributes from /api/me response")` |
| US3 Scenario 3 | `jwt_preauth_test.go` | `It("should derive display name from principal when no display name expression configured")` |
| US3 Scenario 4 | `jwt_preauth_test.go` | `It("should return principal-only profile for plain header preauth")` |
| US4 Scenario 1 | `jwt_preauth_test.go` | Go Playwright E2E: display profile picture, display name, and email in consent UI header |
| US4 Scenario 2 | `jwt_preauth_test.go` | Go Playwright E2E: show principal as fallback when no profile attributes available |
| US4 Scenario 3 | `jwt_preauth_test.go` | Go Playwright E2E: show initials avatar when picture URL unavailable |
| US5 Scenario 1 | `jwt_preauth_test.go` | `It("should work with plain header preauth only config (no JWT block)")` |
| US5 Scenario 2 | `jwt_preauth_test.go` | `It("should prefer JWT over plain header when both configured and JWT present")` |
| US5 Scenario 3 | `jwt_preauth_test.go` | `It("should fall back to plain header when JWT header absent but plain header present")` |
| Edge: invalid JWT | `jwt_preauth_test.go` | `It("should reject request when JWT present but invalid, not falling back to plain header")` |

*Note: US4 Scenarios 1-3 (consent UI) will be covered by Go Playwright E2E tests in `tests/e2e/jwt_preauth_test.go` using page objects from `tests/e2e/pages/` following existing patterns.*

**Test Data Strategy**:
- Use fixtures from `tests/e2e/fixtures/` for stable test data
- Required fixtures: JWT signing keys (RSA, EC), test JWT builder, JWT config variants
- New fixture creation: `tests/e2e/fixtures/jwt_config.go` (test JWT configurations)
- New helper: `tests/e2e/helpers/jwt_helpers.go` (JWT creation, signing, JWKS server mock)

**Test Execution Flow**:
1. **Phase 2f (Design)**: Write E2E tests for all 22 spec scenarios (US4 frontend scenarios use Go Playwright page objects)
2. **Verify Red Phase**: `ginkgo -v ./tests/e2e/ --focus="JWT Pre-Authentication"` — all tests FAIL
3. **Implementation**: Implement feature incrementally (config → domain → adapter → middleware → handler → frontend)
4. **Verify Green Phase**: E2E tests turn GREEN as implementation satisfies acceptance criteria
5. **Minimal Changes**: Only fixture data adjustments during implementation, not test logic

**Bootstrap Strategy**:
- Tests use production bootstrap via `tests/e2e/bootstrap/` (`app.Builder`, HTTP server, routing)
- Fresh server and storage for each test (`BeforeEach`/`AfterEach` isolation)
- Mock JWKS server via `httptest.NewServer` serving a test JWK set (reuse pattern from `helpers/mock_upstream.go`)
- JWT config variations per test context (signed, unsigned, with/without profile extraction)

**Helper Utilities**:
- Custom matchers needed: none — existing HTTP response matchers sufficient
- HTTP helpers: extend existing `helpers/http_helpers.go` with JWT header injection
- Mock services: `helpers/mock_jwks_server.go` — serves test JWKS endpoint; reuses pattern from `helpers/mock_upstream.go`
- JWT builders: `helpers/jwt_helpers.go` — create signed/unsigned test JWTs with configurable claims

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/jwtauth/cel_evaluator_test.go` — CEL expression compilation, evaluation, error handling
- `internal/domain/jwtauth/principal_profile_test.go` — PrincipalProfile construction and defaults
- `internal/adapters/jwtauth/jwx_authenticator_test.go` — JWT parsing, signature verification, claim extraction
- `internal/adapters/http/middleware/principal_middleware_test.go` — JWT path integration with existing middleware
- `internal/config/schema_test.go` — JWT config validation (mutual exclusivity, required fields)
- Strategy: TDD — write tests FIRST, verify they FAIL, then implement

**Integration Tests**:
- N/A — no database changes. JWKS fetching tested via mock HTTP server in E2E tests.
- Frontend tests: Go Playwright E2E tests for consent UI header using page objects from `tests/e2e/pages/` (with/without profile attributes)

**Test Coverage Goals**:
- Unit test coverage: All domain logic (CEL evaluator, profile construction, config validation)
- E2E test coverage: 100% of acceptance scenarios (22 scenarios in Go E2E tests: 19 API + 3 Playwright frontend, mandatory per Principle XIII)
- Frontend test coverage: Header component rendering with all profile attribute combinations (Go Playwright E2E)

## Complexity Tracking

> No Constitution Check violations. All preconditions pass without exceptions.
> 
> **Post-Design Re-evaluation (Phase 1 complete)**:
> - Principle I (Security-First): ✅ JWT verification defaults to `jwks`, unsigned requires explicit opt-in, fail-closed on all failures
> - Principle III (Library-First): ✅ `lestrrat-go/jwx/v3` + `google/cel-go` — no custom crypto
> - Principle IV/X (API-First): ✅ Single additive field (`email`) on `UserInfo` — non-breaking, documented in contracts/
> - Principle V (Domain Model): ✅ `PrincipalProfile`, `AuthResult`, `JWTConfig` documented in data-model.md
> - Principle VI (Hexagonal): ✅ `JWTAuthenticator` port interface with jwx adapter
> - Principle VII (Configuration): ✅ Full `authentication.jwt` block with validation, YAML examples, mutual exclusivity
> - Principle IX (Persistence): ✅ N/A — no database changes
> - Principle XI (Design System): ✅ Uses existing Avatar primitive, semantic tokens
> - Principle XII (Builder/DI): ✅ JWT authenticator created in `Build()`, passed via routing config
> - Principle XIII (E2E Tests): ✅ 22 scenarios mapped 1:1 to spec acceptance criteria (all in Go E2E: 19 API + 3 Playwright frontend)
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
