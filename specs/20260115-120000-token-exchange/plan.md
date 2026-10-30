# Implementation Plan: RFC 8693 OAuth 2.0 Token Exchange

**Branch**: `20260115-120000-token-exchange` | **Date**: 2026-01-16 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20260115-120000-token-exchange/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implement RFC 8693 OAuth 2.0 Token Exchange via the existing `/oauth2/token` endpoint, enabling privileged clients (e.g., API gateways, reverse proxies) to exchange tokens issued by the Upstream OAuth2 Server for third-party OAuth2 tokens stored in the token vault. The implementation uses:
- **lestrrat-go/jwx/v3** for JWT validation and JWKS handling (already in codebase)
- **google/cel-go** for CEL-based authorization and claim extraction expressions
- **Dedicated JWKS adapter** for abstracted JWKS fetching with built-in caching support
- **Resource-based service discovery** via new `protected_resources` field on ThirdpartyOAuth2Service

## Technical Context

**Language/Version**: Go 1.24.0 (per go.mod)
**Primary Dependencies**: 
- lestrrat-go/jwx/v3 (existing) - JWT/JWE/JWK handling with JWKS caching via jwk.Cache
- google/cel-go (NEW) - CEL expression evaluation for claim extraction and authorization
- go-chi/chi/v5 (existing) - HTTP routing
- sqlx + pgx/v5 (existing) - PostgreSQL adapter

**Storage**: PostgreSQL 12+ (production), In-memory (development/testing)
**Testing**: 
- Ginkgo/Gomega for E2E tests (existing framework)
- testify for unit tests
- testcontainers for PostgreSQL integration tests

**Target Platform**: Linux server (Docker/Kubernetes)
**Project Type**: Backend Go service (hexagonal architecture)

**Performance Goals**:
- Token exchange (no refresh): <500ms p95
- Token exchange (with refresh): <2000ms p95
- CEL evaluation: <100ms per expression
- 100 concurrent requests without degradation

**Constraints**:
- JWT validation MUST use library (no custom crypto)
- JWKS fetching MUST be in adapter layer (HTTP calls abstracted)
- CEL expressions validated at startup (fail-fast)
- All security operations audit-logged

**Scale/Scope**:
- Single new domain service (TokenExchangeService)
- 1 migration file (add protected_resources)
- 29 E2E test scenarios from spec

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Note**: This checklist is forward-looking, answering "Will these be done?" not "Are these done now?". Checkboxes [x] indicate planned compliance, not current completion status. Actual implementation occurs in Phase 2 (Design Preconditions) and later phases.

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Have entities, aggregates, value objects been identified and documented?
  - TokenExchangeRequest, TokenExchangeResponse, ClientAssertion, SubjectToken, ResourceURI (see spec.md Domain Model section)
  - ClientAssertion represents privileged client identity (e.g., API gateway, reverse proxy)
- [x] **Domain Concepts**: Will new domain terms be added to ARCHITECTURE.md Glossary?
  - TokenExchangeRequest, TokenExchangeResponse, ClientAssertion, SubjectToken, ResourceURI, Privileged Client, CEL Authorization
- [x] **Configuration Design**: Have all config requirements been identified with YAML examples?
  - token_exchange.claim_extraction.*, token_exchange.authorization.cel.expression, token_exchange.refresh.enabled
  - Reuses existing upstream_oauth2 config for issuer/jwks_uri
- [x] **Config Examples**: Will example YAML snippets be added to examples/config/?
  - examples/config/token-exchange.yaml with full example
- [x] **API Design First**: Will APIs be designed (OpenAPI spec) and confirmed BEFORE implementation?
  - Token exchange endpoint in /api/enduser/openapi.yaml
  - protected_resources in /api/admin/openapi.yaml
- [x] **API Documentation**: Will OpenAPI specs be created in `/api/enduser/` or `/api/admin/` as applicable?
  - Yes, both APIs documented
- [x] **API Changes**: Are all API changes confirmed by user/stakeholder (document in PR)?
  - User input in spec.md clarifications section serves as confirmation
- [x] **Database Design**: Will all schema changes use go-migrate naming in `/migrations/`?
  - 005_add_service_protected_resources.up.sql / down.sql
- [x] **E2E Acceptance Tests**: Will E2E tests be written for ALL spec scenarios BEFORE implementation?
  - 29 scenarios from spec.md → tests/e2e/token_exchange_test.go
- [x] **E2E Test Mapping**: Will each acceptance scenario map 1:1 to one It() block in tests/e2e/?
  - Mapping table below in Testing Strategy section
- [x] **E2E Red Phase**: Will E2E tests FAIL initially, proving they test actual functionality?
  - Tests written against non-existent endpoint, must fail initially

**Implementation Considerations**:

- [x] **Security-First**: Are security features enabled by default? No bypasses or optional security?
  - JWT validation mandatory, CEL authorization enabled by default (expression defaults to "true")
  - UserGrant verification cannot be bypassed
  - Privileged client authentication via client_assertion JWT required
- [x] **Architecture Docs**: Will ARCHITECTURE.md be updated if this touches architecture?
  - Add JWKS Adapter subsection, TokenExchangeService, CEL authorization docs
- [x] **ADRs**: Does this require an ADR in adrs/ for major decisions?
  - ADR 20260119-120000: Token Exchange JWKS Adapter Pattern (JWKS caching abstraction)
  - ADR 20260116-120100: CEL for Authorization Policies (if not existing)
- [x] **Library-First Security**: Are we using vetted libraries for crypto/security (no custom implementations)?
  - lestrrat-go/jwx/v3 for JWT validation (existing)
  - google/cel-go for policy evaluation (well-maintained, Google-backed)
- [x] **Zalando Guidelines**: Will APIs follow Zalando RESTful API and Event Guidelines?
  - RFC 8693 response format (access_token, token_type, issued_token_type, expires_in)
  - Error responses follow RFC 8693 Section 5.2
- [x] **End-User Docs**: Will API documentation be rendered in `docs/api/` with examples?
  - docs/api/token-exchange.md with curl examples
- [x] **Migration Testing**: Will migrations be tested (apply/rollback) in PostgreSQL integration tests?
  - Integration tests verify protected_resources column and GIN index
- [x] **Hexagonal Architecture**: Does domain logic use ports (interfaces) with clear adapter separation?
  - JWKSPort interface for JWKS fetching (adapter does HTTP)
  - JWT validation in domain (uses library, no HTTP)
  - CEL evaluation in domain (pure computation)
- [x] **Persistence Patterns**: If adding persistence, will it follow specs/20251215-120100-persistence-layer/quickstart.md?
  - Extends ThirdpartyOAuth2ServiceRepository with FindByProtectedResource method

*All BLOCKING checks pass. Proceeding to Phase 0 research.*

## Project Structure

### Documentation (this feature)

```text
specs/20260115-120000-token-exchange/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output - JWKS caching, CEL, RFC 8693 research
├── data-model.md        # Phase 1 output - Entity definitions
├── quickstart.md        # Phase 1 output - Implementation guide
├── contracts/           # Phase 1 output - OpenAPI specs
│   ├── token-exchange-endpoint.yaml  # Token exchange request/response
│   └── admin-protected-resources.yaml # Service protected_resources field
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
internal/
├── adapters/
│   ├── http/
│   │   ├── enduser/
│   │   │   └── oauth2_token_exchange.go    # Token exchange HTTP handler
│   │   └── routing/
│   │       └── enduser.go                   # Updated routing (grant_type detection)
│   ├── jwks/                                # NEW: JWKS adapter package
│   │   ├── adapter.go                       # JWKSAdapter implementation
│   │   └── adapter_test.go                  # Adapter tests
│   └── storage/
│       ├── memory/
│       │   └── thirdparty_service.go        # Add FindByProtectedResource
│       └── postgres/
│           └── thirdparty_service.go        # Add FindByProtectedResource
├── domain/
│   ├── tokenexchange/                       # NEW: Token exchange domain
│   │   ├── service.go                       # TokenExchangeService
│   │   ├── service_test.go                  # Unit tests
│   │   ├── request.go                       # TokenExchangeRequest value object
│   │   ├── response.go                      # TokenExchangeResponse value object
│   │   ├── jwt_validator.go                 # JWT validation (uses lestrrat library)
│   │   ├── cel_evaluator.go                 # CEL expression evaluation
│   │   ├── cel_evaluator_test.go            # CEL tests
│   │   └── errors.go                        # Domain errors
│   └── storage/
│       └── thirdparty_service.go            # Add ProtectedResources field
└── ports/
    ├── jwks.go                              # NEW: JWKSPort interface
    ├── config.go                            # Add TokenExchangeConfig
    └── storage.go                           # Add FindByProtectedResource method

tests/
├── e2e/
│   ├── token_exchange_test.go              # E2E acceptance tests (29 scenarios)
│   ├── fixtures/
│   │   └── token_exchange.go               # Test fixtures for token exchange
│   └── helpers/
│       └── mock_upstream_jwks.go           # Mock JWKS endpoint helper
├── integration/
│   └── tokenexchange/
│       └── service_test.go                 # Integration tests
└── unit/
    └── tokenexchange/
        └── cel_test.go                     # CEL expression unit tests

migrations/
├── 005_add_service_protected_resources.up.sql
└── 005_add_service_protected_resources.down.sql

examples/config/
└── token-exchange.yaml                     # Configuration example
```

**Structure Decision**: Following existing hexagonal architecture patterns:
- Domain logic in `internal/domain/tokenexchange/` depends on ports only
- JWKS HTTP fetching abstracted into `internal/adapters/jwks/` adapter (per user requirement)
- JWT validation uses library in domain (acceptable as it's pure computation, no HTTP)
- CEL evaluation in domain (pure computation, sandboxed)

## Testing Strategy

<!--
  Per Constitution Principle XIII (End-to-End Acceptance Testing & Spec Traceability):
  All features MUST have E2E acceptance tests mapped 1:1 to spec scenarios.
-->

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/token_exchange_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: "Token Exchange E2E (RFC 8693)"
- **Nested Context**: Per User Story (e.g., "User Story 1: Gateway Exchanges Token")
- **It blocks**: Individual acceptance scenarios (one It() per scenario from spec.md)

**Scenario Mapping**:

| Spec Scenario | User Story | E2E Test Description |
|---------------|------------|----------------------|
| US1-S1 | Privileged Client Token Exchange | `It("should detect token exchange via grant_type parameter")` |
| US1-S2 | Privileged Client Token Exchange | `It("should look up service by resource URI in protected_resources")` |
| US1-S3 | Privileged Client Token Exchange | `It("should return RFC 8693 response with access_token and token_type")` |
| US1-S4 | Privileged Client Token Exchange | `It("should auto-refresh expired token if refresh_token valid")` |
| US1-S5 | Privileged Client Token Exchange | `It("should return 401 invalid_client without valid client_assertion")` |
| US1-S6 | Privileged Client Token Exchange | `It("should return 400 invalid_request with invalid subject_token")` |
| US2-S1 | Resource Discovery | `It("should store protected_resources via admin API")` |
| US2-S2 | Resource Discovery | `It("should normalize resource URI removing trailing slashes")` |
| US2-S3 | Resource Discovery | `It("should return tokens for matching service only")` |
| US2-S4 | Resource Discovery | `It("should return 400 invalid_target when no service matches")` |
| US2-S5 | Resource Discovery | `It("should return 400 invalid_target for ambiguous resource")` |
| US3-S1 | Grant Verification | `It("should proceed when user has active grant for agent+service")` |
| US3-S2 | Grant Verification | `It("should return 403 access_denied without user grant")` |
| US3-S3 | Grant Verification | `It("should return 403 access_denied when grant revoked")` |
| US3-S4 | Grant Verification | `It("should return 403 access_denied when grant expired")` |
| US4-S1 | CEL Authorization | `It("should evaluate CEL expression against client_assertion")` |
| US4-S2 | CEL Authorization | `It("should proceed when CEL evaluates to true")` |
| US4-S3 | CEL Authorization | `It("should return 403 access_denied when CEL evaluates to false")` |
| US4-S4 | CEL Authorization | `It("should fail startup with invalid CEL syntax")` |
| US4-S5 | CEL Authorization | `It("should provide client_assertion claims in CEL context")` |
| US4-S6 | CEL Authorization | `It("should provide request context in CEL context")` |
| US5-S1 | No Session Error | `It("should return 400 invalid_grant when no session exists")` |
| US5-S2 | No Session Error | `It("should return 400 invalid_grant when tokens fully expired")` |
| US5-S3 | No Session Error | `It("should include sufficient info for re-auth flow")` |
| US6-S1 | Admin API | `It("should accept protected_resources in PUT /api/services/{id}")` |
| US6-S2 | Admin API | `It("should return 400 for invalid URI in protected_resources")` |
| US6-S3 | Admin API | `It("should return 409 for duplicate resource URI")` |
| US6-S4 | Admin API | `It("should include protected_resources in GET response")` |
| US6-S5 | Admin API | `It("should accept protected_resources in POST /api/services")` |

**Test Data Strategy**:
- Use existing fixtures from `tests/e2e/fixtures/` (agents.go, grants.go, principals.go)
- **New fixtures required**:
  - `fixtures/token_exchange.go`: Mock JWTs (subject_token, client_assertion), services with protected_resources
  - `fixtures/config.go`: Add TokenExchangeConfig defaults

**Test Execution Flow**:
1. **Phase 2e (Design)**: Write all 29 E2E tests with proper Ginkgo structure
2. **Verify Red Phase**: `ginkgo -v ./tests/e2e/token_exchange_test.go` - all tests FAIL (endpoint doesn't exist)
3. **Implementation**: Implement token exchange incrementally
4. **Verify Green Phase**: E2E tests turn GREEN as implementation satisfies scenarios
5. **Minimal Changes**: Only fixture adjustments during implementation, not test logic

**Bootstrap Strategy**:
- Tests use production `app.Builder` via `tests/e2e/bootstrap/`
- Fresh server and storage for each test (BeforeEach/AfterEach isolation)
- **Feature-specific requirements**:
  - Mock JWKS endpoint serving test public keys
  - Pre-seeded services with protected_resources
  - Pre-seeded user sessions with tokens
  - Pre-seeded user grants for agent+service
  - Registered privileged clients (agents) that can perform token exchange

**Helper Utilities**:
- **New matchers**: `matchers/rfc8693_matchers.go` for RFC 8693 response validation
- **New helper**: `helpers/mock_upstream_jwks.go` for JWKS endpoint mock
- **Existing helpers**: `helpers/http_helpers.go` for form-urlencoded POST

### Unit & Integration Tests

**Unit Tests**:
- Location: `tests/unit/tokenexchange/` and `internal/domain/tokenexchange/*_test.go`
- Coverage:
  - CEL expression compilation and evaluation
  - JWT claim extraction
  - Resource URI normalization
  - TokenExchangeRequest/Response validation
- Strategy: TDD - write tests FIRST, verify they FAIL, then implement

**Integration Tests**:
- Location: `tests/integration/tokenexchange/`
- Coverage:
  - PostgreSQL FindByProtectedResource query
  - Migration apply/rollback
  - JWKS adapter HTTP integration
- Strategy: Real PostgreSQL via testcontainers

**Test Coverage Goals**:
- Unit test coverage: Critical paths (CEL, JWT validation, URI normalization)
- Integration test coverage: PostgreSQL repository methods, migrations
- E2E test coverage: 100% of acceptance scenarios (mandatory per Principle XIII)

## Complexity Tracking

> No Constitution Check violations requiring justification. All checks pass.

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| JWKS in adapter | HTTP calls abstracted to JWKSAdapter | User requirement + enables future caching without touching domain |
| JWT validation in domain | Acceptable | Uses library (lestrrat), no HTTP calls, pure computation |
| CEL in domain | Acceptable | Pure computation, sandboxed, no external dependencies |
| New domain package | `tokenexchange` separate from `oauth2session` | Different concerns: exchange vs session lifecycle |
| Privileged client terminology | Replaces "gateway" in implementation | More accurate: includes API gateways, reverse proxies, and similar entities |
