# Implementation Plan: Unified Session Token State Transport

**Branch**: `20260517-120000-unified-session-token` | **Date**: 2026-05-18 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/20260517-120000-unified-session-token/spec.md`

## Summary

Extend the existing JWE session token mechanism (currently CIMD-only, per ADR 20260428-120000) to ALL agent modes (local, proxy). This eliminates the insecure `redirect_uri` query parameter fallback in `buildConsentURL` and unifies consent handler resolution to exclusively use `session_token`. No new entities, no database changes, no new configuration — purely a behavioral change within existing domain services and HTTP handlers.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: chi v5, lestrrat-go/jwx (JWE), internal `domain/jwe` TokenService
**Storage**: N/A — session tokens are stateless (JWE-sealed, no persistence)
**Testing**: Ginkgo/Gomega (E2E), Go standard testing (unit)
**Target Platform**: Linux server (containers)
**Project Type**: Web application (Go backend + React frontend)
**Performance Goals**: No regression — JWE encrypt/decrypt already benchmarked for CIMD flows
**Constraints**: 10-minute TTL, A256GCMKW + A256GCM encryption
**Scale/Scope**: Affects 3 files primarily: `oauth2/service.go`, `consent/agent_detail_handler.go`, `consent/grants_handler.go`

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: No new entities. Existing `AuthorizationSessionClaims` extended to all agent modes (CIMDMetadata=nil for non-CIMD).
- [x] **Domain Concepts**: No new domain terms needed — reuses existing "OAuth2StateToken" and "AuthorizationSessionClaims".
- [x] **Entity IDs**: No new entities with UUID PKs.
- [x] **Configuration Design**: No new configuration parameters.
- [x] **Config Examples**: N/A — no config changes.
- [x] **Helm Chart**: N/A — no config parameter changes.
- [x] **API Design First**: No API contract changes — the `/authorize` endpoint already returns 302 redirects; only the query parameter name changes (internal transport, not a public API contract). Consent page URL is internal.
- [x] **API Documentation**: N/A — no public API changes.
- [x] **API Changes**: N/A — internal state transport only.
- [x] **Database Design**: No schema changes required.
- [x] **E2E Acceptance Tests**: Yes — will write E2E tests for all 7 spec scenarios before implementation.
- [x] **E2E Test Mapping**: Each acceptance scenario maps 1:1 to one It() block.
- [x] **E2E Red Phase**: E2E tests will fail semantically (expecting session_token in redirects, rejection of redirect_uri).
- [x] **Frontend Playwright E2E**: No — consent page UI itself is unchanged; only the URL parameter changes.
- [x] **Frontend Screenshots**: N/A — no UI changes.

**Implementation Considerations**:

- [x] **Security-First**: Yes — removes insecure fallback, session_token mandatory for all flows.
- [x] **Architecture Docs**: Minor update to ARCHITECTURE.md glossary to note session tokens apply to all agent modes.
- [ ] **ADRs**: No new ADR needed — ADR 20260428-120000 already supports this (it chose redirect_uri fallback for pragmatic reasons; extending session_token is aligned with the ADR's security rationale).
- [x] **Library-First Security**: Uses existing `lestrrat-go/jwx` JWE implementation.
- [x] **Zalando Guidelines**: N/A — no public API changes.
- [x] **End-User Docs**: N/A.
- [x] **Migration Testing**: N/A — no migrations.
- [x] **Hexagonal Architecture**: Yes — changes are within domain service and handler adapter layers.
- [x] **Persistence Patterns**: N/A — stateless tokens.

## Project Structure

### Documentation (this feature)

```text
specs/20260517-120000-unified-session-token/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (N/A — no new external contracts)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
internal/
├── domain/
│   └── oauth2/
│       ├── service.go                        # buildConsentURL — remove redirect_uri branch
│       └── authorization_session_token.go    # AuthorizationSessionClaims (unchanged)
├── adapters/
│   └── http/
│       └── handlers/
│           └── consent/
│               ├── agent_detail_handler.go   # Remove redirect_uri fallback, require session_token
│               └── grants_handler.go         # Remove redirect_uri fallback, require session_token

tests/
└── e2e/
    └── unified_session_token_test.go         # E2E acceptance tests
```

**Structure Decision**: Existing hexagonal layout. Changes touch domain service (token generation) and adapter layer (consent handlers). No new packages.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 2** | Design Preconditions (E2E tests) | **MANDATORY** |
| **Phase 3** | User Story 1 & 2 — extend buildConsentURL | Customizable |
| **Phase 4** | User Story 3 — unify consent handlers | Customizable |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **skip** — no structural refactoring needed; changes are minimal and localized.
- [x] Phase 2.7 (entity boilerplate): **skip** — no new entities introduced.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/unified_session_token_test.go`

**Framework**: Ginkgo/Gomega BDD

**Test Organization**:
- **Top-level Describe**: "Unified Session Token State Transport"
- **Context**: "Local Agent", "Proxy Agent", "Consent Handlers"
- **It blocks**: One per acceptance scenario

**Scenario Mapping**:

| Spec Scenario | Test Description |
|---------------|------------------|
| US1 Scenario 1 | `It("includes session_token and omits redirect_uri for local agent authorize")` |
| US1 Scenario 2 | `It("session token contains agent ID, principal, original URL, iat, exp with 10min TTL")` |
| US1 Scenario 3 | `It("rejects session token when principal does not match authenticated user")` |
| US2 Scenario 1 | `It("includes session_token for proxy agent authorize")` |
| US2 Scenario 2 | `It("rejects expired session token for proxy agent")` |
| US3 Scenario 1 | `It("consent handler extracts context from session token for any agent mode")` |
| US3 Scenario 2 | `It("consent handler rejects request with redirect_uri but no session_token")` |

**Red Phase Requirements**: Tests assert on HTTP redirect Location header containing `session_token=` and NOT containing `redirect_uri=`. Consent rejection tests assert 400/403 status codes.

**Test Data Strategy**:
- Required fixtures: local agent (no CIMD), proxy agent (no CIMD), CIMD agent (regression), principal
- Use existing fixture patterns from `tests/e2e/fixtures/`

**Bootstrap Strategy**: Production bootstrap via `tests/e2e/bootstrap/`. Fresh server per test.

**Helper Utilities**:
- Existing HTTP helpers for making requests and parsing redirect responses
- May need JWE decrypt helper to inspect token claims in tests

### Frontend Playwright E2E Tests

Not applicable — no UI changes. Consent page renders identically; only the URL parameter changes.

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/oauth2/service_test.go` — verify `buildConsentURL` always produces session_token
- `internal/adapters/http/handlers/consent/agent_detail_handler_test.go` — verify rejection of redirect_uri-only requests
- `internal/adapters/http/handlers/consent/grants_handler_test.go` — verify rejection of redirect_uri-only requests

**Test Coverage Goals**:
- Unit: all branches in modified functions
- E2E: 100% of acceptance scenarios (7 scenarios)
