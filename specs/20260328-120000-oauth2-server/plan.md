# Implementation Plan: Broker OAuth2 Server Mode

**Branch**: `20260328-120000-oauth2-server` | **Date**: 2026-03-28 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20260328-120000-oauth2-server/spec.md`

## Summary

Add local token minting mode (`issue_token`) to the identity broker, enabling it to operate as a standalone OAuth2 authorization server. The broker issues its own JWT access tokens signed with managed ES256 or RS256 keys, supports both `client_credentials` and `authorization_code` (with PKCE) grant types, and exposes RFC 8414 discovery and JWKS endpoints. Admin API extensions allow operators to provision client credentials per agent and manage signing keys. Token claims are customizable via a global CEL expression (`token_claims_expression`) evaluated at issuance time with access to agent, principal, and request context.

**Core technology decision**: Use `ory/fosite` as a **headless domain library** — only the handler layer (`AuthorizeExplicitGrantHandler`, `ClientCredentialsGrantHandler`, `pkce.Handler`) which operates on clean interfaces (`fosite.AuthorizeRequester`, `fosite.AccessRequester`) with no `*http.Request` dependency. Strategy interfaces (`AccessTokenStrategy`, `AuthorizeCodeStrategy`) are implemented with `lestrrat-go/jwx/v3` and stdlib `crypto/rand`. Fosite types are contained within `internal/domain/oauth2server/` and never leak into ports or HTTP handlers. See [research.md](research.md) Decision 1 for full rationale, source-verified handler analysis, and effort comparison.

**Rejected alternative**: Custom protocol implementation without fosite. While viable (and serves as a fallback if fosite v0.x instability becomes a problem), it requires implementing ~510 LOC of battle-tested protocol logic (PKCE with 8+ edge cases, auth code replay detection, scope validation) that fosite already provides. See research.md Decision 1 for trade-off analysis.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: `ory/fosite` (headless OAuth2 protocol handlers), `lestrrat-go/jwx/v3` (JWT signing, JWKS), `golang.org/x/crypto/argon2` (client secret hashing), `google/cel-go` (token claims expression — already a project dependency via token exchange), `chi/v5` (HTTP routing), `sqlx` (PostgreSQL)
**Storage**: PostgreSQL (production) + in-memory (tests) — both adapter implementations per ADR 20251215-120300
**Testing**: `testing` + `testify` (unit), `testcontainers` (integration), `ginkgo/gomega` (E2E BDD)
**Target Platform**: Linux server (Docker), macOS (dev)
**Project Type**: Go monorepo with hexagonal architecture (`internal/domain → ports → adapters → app`)
**Performance Goals**: Token minting < 50ms p95 (dominated by Argon2id + ECDSA sign)
**Constraints**: Signing key private material encrypted at rest via `EncryptionPort`; fail-closed on decrypt failure
**Scale/Scope**: 3 new domain entities, 4 migrations, 3 repository interfaces, 6 admin + 3 enduser endpoints, 1 ADR, 1 CEL evaluator for token claims

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Have entities, aggregates, value objects been identified and documented?
  → Yes. `BrokerClientCredential`, `SigningKey`, `AuthorizationCode` entities + `ClientID` (credential), `KeyID` value objects documented in [data-model.md](data-model.md).
- [x] **Domain Concepts**: Will new domain terms be added to ARCHITECTURE.md Glossary?
  → Yes. `BrokerClientCredential`, `SigningKey`, `AuthorizationCode`, `ClientID` (credential), `KeyID`, `OAuth2ServerProvider` to be added.
- [x] **Entity IDs**: For each new domain entity with a UUID primary key, will a typed ID (`type XxxID uuid.UUID`) be added to `internal/domain/id/` via `gen_ids.go` and documented in `internal/domain/id/AGENTS.md`? (ADR 20260313-120000)
  → Yes. `CredentialID`, `SigningKeyID`, `AuthorizationCodeID` (UUID) + `ClientID`, `KeyID` (string) documented in data-model.md.
- [x] **Configuration Design**: Have all config requirements been identified with YAML examples?
  → Yes. `mode`, `issuer_uri`, `token_ttl` fields documented with YAML examples in [quickstart.md](quickstart.md). `token_claims_expression` (CEL) documented in spec FR-013.
- [x] **Config Examples**: Will example YAML snippets be added to examples/config/?
  → Yes. An `oauth2-server-mode.yaml` example file will be added.
- [x] **Helm Chart**: If configuration parameters are added/changed/removed, will `charts/agentic-identity-broker/` be updated (values.yaml, templates, README)?
  → Yes. `mode`, `issuer_uri`, `token_ttl`, `token_claims_expression` parameters to be added to Helm chart values.
- [x] **API Design First**: Will APIs be designed (OpenAPI spec) and confirmed BEFORE implementation?
  → Yes. API contracts documented in [contracts/api-contracts.md](contracts/api-contracts.md). OpenAPI specs to be created before implementation.
- [x] **API Documentation**: Will OpenAPI specs be created in `/api/enduser/` or `/api/admin/` as applicable?
  → Yes. Both admin (credentials, signing keys) and enduser (token, authorize, jwks, discovery) endpoints to be documented.
- [x] **API Changes**: Are all API changes confirmed by user/stakeholder (document in PR)?
  → Yes. API design confirmed via spec clarification session. Agent entity extension (`redirect_uris`) confirmed.
- [x] **Database Design**: Will all schema changes use go-migrate naming in `/migrations/`?
  → Yes. Migrations 009–012 documented in [data-model.md](data-model.md) using go-migrate naming.
- [x] **E2E Acceptance Tests**: Will E2E tests be written for ALL spec scenarios BEFORE implementation?
  → Yes. 29 acceptance scenarios across 6 user stories → 29 It() blocks in E2E tests. Written in red phase before implementation.
- [x] **E2E Test Mapping**: Will each acceptance scenario map 1:1 to one It() block in tests/e2e/?
  → Yes. See Testing Strategy section below for mapping plan.
- [x] **E2E Red Phase**: Will E2E tests FAIL initially with detailed, realistic expectations (not placeholder assertions)?
  → Yes. Assertions will target actual HTTP responses, JWT claims, and JWKS structures.
- [x] **Frontend Playwright E2E**: If this feature changes the React UI, will Playwright E2E tests be added/amended in `tests/e2e/frontend/`?
  → N/A. This feature is purely backend. The consent UI interaction is existing behavior (no changes).
- [x] **Frontend Screenshots**: If this feature changes the React UI, will screenshots be captured to `tests/e2e/screenshots/` for each UI state under test?
  → N/A. No UI changes.

**Implementation Considerations**:

- [x] **Security-First**: Are security features enabled by default? No bypasses or optional security?
  → Yes. PKCE always enforced (S256 only), Argon2id secret hashing, signing key material encrypted at rest, fail-closed on decrypt failure.
- [x] **Architecture Docs**: Will ARCHITECTURE.md be updated if this touches architecture?
  → Yes. New glossary terms and `issue_token` mode architecture to be documented.
- [x] **ADRs**: Does this require an ADR in adrs/ for major decisions?
  → Yes. ADR `20260328-120100-oauth2-server-mode.md` documenting: fosite headless integration pattern, strategy implementations, type containment rules, and fallback plan.
- [x] **Library-First Security**: Are we using vetted libraries for crypto/security (no custom implementations)?
  → Yes. `lestrrat-go/jwx/v3` for JWT signing, `golang.org/x/crypto/argon2` for secret hashing, `crypto/rand` for code generation. Fosite handlers provide battle-tested PKCE and auth code lifecycle logic.
- [x] **Zalando Guidelines**: Will APIs follow Zalando RESTful API and Event Guidelines?
  → Yes. Admin endpoints use standard REST verbs, `items` wrapper for lists, proper status codes (201/200/204/404/409).
- [x] **End-User Docs**: Will API documentation be rendered in `docs/api/` with examples?
  → Yes. Token, authorize, JWKS, and discovery endpoints to be documented.
- [x] **Migration Testing**: Will migrations be tested (apply/rollback) in PostgreSQL integration tests?
  → Yes. All 4 migrations tested via testcontainers with apply+rollback verification.
- [x] **Hexagonal Architecture**: Does domain logic use ports (interfaces) with clear adapter separation?
  → Yes. Three new repository ports in `internal/ports/storage.go`. Fosite contained within domain service. Chi handlers in adapters.
- [x] **Persistence Patterns**: If adding persistence, will it follow specs/20251215-120100-persistence-layer/quickstart.md?
  → Yes. ISP repositories, sqlx for PostgreSQL adapters, `StorageError` wrapping, in-memory + postgres duality.

*All BLOCKING checks pass. No violations require justification.*

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── domain/
│   ├── id/
│   │   ├── gen_ids.go                    # Add CredentialID, SigningKeyID, AuthorizationCodeID
│   │   └── string_ids.go                 # Add ClientID, KeyID
│   ├── storage/
│   │   ├── broker_client_credential.go   # BrokerClientCredential entity
│   │   ├── signing_key.go                # SigningKey entity
│   │   └── authorization_code.go         # AuthorizationCode entity
│   └── oauth2server/                     # NEW: fosite headless integration (types contained here)
│       ├── provider.go                   # Fosite handler wiring, Provider struct
│       ├── strategies.go                 # JWXAccessTokenStrategy, RandomCodeStrategy
│       ├── fosite_storage.go             # Storage adapters wrapping project repositories
│       ├── fosite_client.go              # brokerClient wrapper (fosite.Client implementation)
│       ├── signing_key_service.go        # Key lifecycle, auto-generation, JWKS building
│       ├── client_auth.go               # Client authentication (Argon2id, outside fosite)
│       └── token_claims_cel.go          # CEL evaluator for token_claims_expression (compile at startup, eval at issuance)
├── ports/
│   └── storage.go                        # Add BrokerClientCredentialRepository, SigningKeyRepository, AuthorizationCodeRepository
├── adapters/
│   ├── storage/
│   │   ├── memory/
│   │   │   ├── broker_client_credential_store.go
│   │   │   ├── signing_key_store.go
│   │   │   └── authorization_code_store.go
│   │   └── postgres/
│   │       ├── broker_client_credential_repo.go
│   │       ├── signing_key_repo.go
│   │       └── authorization_code_repo.go
│   └── http/
│       ├── handlers/
│       │   ├── admin/
│       │   │   ├── client_credentials_handler.go
│       │   │   └── signing_keys_handler.go
│       │   └── enduser/
│       │       ├── oauth2_token_handler.go       # Extended: client_credentials + authorization_code grants
│       │       ├── oauth2_authorize_handler.go    # NEW: authorization endpoint
│       │       ├── jwks_handler.go                # NEW: JWKS endpoint
│       │       └── discovery_handler.go           # NEW: RFC 8414 discovery
│       └── routing/
│           ├── admin.go                  # Add credential + signing key routes
│           └── enduser.go                # Add authorize, jwks, discovery routes (issue_token mode only)
├── app/
│   └── builder.go                        # Wire new repositories, domain service, handlers
└── config/                               # Mode-conditional validation in Validate()

migrations/
├── 009_add_agent_redirect_uris.{up,down}.sql
├── 010_create_broker_client_credentials.{up,down}.sql
├── 011_create_signing_keys.{up,down}.sql
└── 012_create_authorization_codes.{up,down}.sql

adrs/
└── 20260328-120100-oauth2-server-mode.md             # Fosite headless integration pattern ADR

tests/e2e/
├── oauth2_client_credentials_test.go     # P1: credential provisioning scenarios
├── oauth2_server_mode_test.go            # P2: mode configuration scenarios
├── oauth2_token_test.go                  # P3: client_credentials grant scenarios
├── oauth2_authorize_test.go              # P4: authorization code + PKCE scenarios
├── oauth2_signing_keys_test.go           # P5: signing key management scenarios
└── oauth2_discovery_test.go              # P6: discovery + JWKS scenarios
```

**Structure Decision**: Follows existing hexagonal architecture. The key addition is `internal/domain/oauth2server/` which contains all fosite integration code. Fosite types (`AuthorizeRequester`, `AccessRequester`, strategy interfaces) are used ONLY within this package — they never appear in `ports/`, `adapters/http/`, or `app/`. The domain service exposes project-native interfaces that handlers call.

## Implementation Phase Overview

*Detailed task breakdown is in `tasks.md` (generated by `/speckit.tasks`). The table below reflects
the standard phase structure — include or omit optional phases based on this feature's needs.*

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring — isolate structural changes from feature work | Optional |
| **Phase 1** | Setup — project init, dependencies | Customizable |
| **Phase 2** | Design Preconditions (domain model, config, API, DB, E2E tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate — empty CRUD handlers & repositories, isolated from business logic | If new entities |
| **Phase 2.5** | Foundational Infrastructure — feature-specific foundation code | Customizable |
| **Phase 3+** | User Stories — business logic per priority | Customizable |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

**Review ergonomics rationale**: Phases 0 and 2.7 exist to keep PRs focused and reviewable:
- **Phase 0 PR** (if needed): contains only refactoring with no behavior change — reviewers approve
  structural changes first without needing to understand new feature intent
- **Phase 2.7 PR** (if new entities): contains only empty scaffolding (501 handlers, interface stubs)
  — reviewers approve skeleton quickly, then review business logic in sharp focus

*Document which phases apply to this feature and why any optional phases are included or skipped:*

- [x] Phase 0 (refactoring): **Include** — Refactor `Validate()` in config to support mode-conditional validation without breaking existing proxy mode. This is a structural change with no behavior change, isolating it keeps the PR reviewable.
- [x] Phase 2.7 (entity boilerplate): **Include** — Three new entities (`BrokerClientCredential`, `SigningKey`, `AuthorizationCode`) require repository interfaces, empty memory/postgres adapters, 501 handler stubs, and routes. Isolating boilerplate from business logic keeps PRs sharp.

**Feature-specific phases**:

| Phase | Content | PR Scope |
|-------|---------|----------|
| **Phase 0** | Refactor `Validate()` for mode-conditional config | Config refactor only |
| **Phase 2** | Design preconditions: domain model, config, OpenAPI, migrations, E2E test design (red phase) | Design artifacts + red E2E tests |
| **Phase 2.7** | Entity boilerplate: empty CRUD handlers (501), repository stubs, routes, builder wiring | Scaffolding only |
| **Phase 2.5** | Signing key infrastructure: key generation, `EncryptionPort` integration, JWKS building, fosite provider wiring, CEL token claims evaluator (compile + startup validation) | Foundation for all token operations |
| **Phase 3** | P1: Client credential provisioning (admin endpoints: generate, get, revoke, rotate) | Business logic: credentials |
| **Phase 4** | P2+P3: Mode switch + client_credentials grant (token endpoint, fosite `ClientCredentialsGrantHandler`) | Business logic: token minting |
| **Phase 5** | P4: Authorization code flow with PKCE (authorize endpoint, fosite `AuthorizeExplicitGrantHandler` + `pkce.Handler`, consent integration) | Business logic: auth code flow |
| **Phase 6** | P5+P6: Signing key management (admin CRUD) + discovery + JWKS endpoints | Business logic: key mgmt + metadata |
| **Phase N** | Constitution compliance: ARCHITECTURE.md update, ADR 20260328-120100, Helm chart, docs, example config | Documentation + compliance |

## Testing Strategy

<!--
  Per Constitution Principle XIII (End-to-End Acceptance Testing & Spec Traceability):
  All features MUST have E2E acceptance tests mapped 1:1 to spec scenarios.
  Frontend UI changes additionally require Playwright E2E tests and screenshots.

  This section documents HOW E2E tests will be structured and implemented for this feature.
-->

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/[feature]_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: Feature name (e.g., "OAuth2 Authorization Endpoint")
- **Nested Describe/Context**: Preconditions and scenarios (e.g., "when a valid request arrives" → "and no grant exists")
- **It blocks**: Individual acceptance scenarios (one It() per scenario from spec.md)

**Scenario Mapping**:

| Spec Scenario | E2E Test File | Test Description |
|---------------|---------------|------------------|
| US1-S1: Generate credentials | `oauth2_client_credentials_test.go` | `It("generates broker client credentials for an agent")` |
| US1-S2: Rotate credentials | `oauth2_client_credentials_test.go` | `It("rotates credentials invalidating previous")` |
| US1-S3: Agent not found | `oauth2_client_credentials_test.go` | `It("returns 404 for non-existent agent")` |
| US1-S4: Get credential metadata | `oauth2_client_credentials_test.go` | `It("returns metadata without plaintext secret")` |
| US2-S1: Start in issue_token mode | `oauth2_server_mode_test.go` | `It("starts successfully in issue_token mode")` |
| US2-S2: No upstream URI needed | `oauth2_server_mode_test.go` | `It("starts without upstream URI in issue_token mode")` |
| US2-S3: Default proxy mode unchanged | `oauth2_server_mode_test.go` | `It("preserves proxy mode as default")` |
| US2-S4: Auto-generate signing key | `oauth2_server_mode_test.go` | `It("auto-generates signing key when none exists")` |
| US3-S1: Client credentials grant | `oauth2_token_test.go` | `It("issues signed access token for valid credentials")` |
| US3-S2: Invalid credentials | `oauth2_token_test.go` | `It("returns invalid_client for bad credentials")` |
| US3-S3: Token validates via JWKS | `oauth2_token_test.go` | `It("token is verifiable via JWKS endpoint")` |
| US3-S4: Scope restriction | `oauth2_token_test.go` | `It("returns invalid_scope for excessive scopes")` |
| US4-S1: Authorization code flow | `oauth2_authorize_test.go` | `It("issues authorization code and redirects")` |
| US4-S2: Invalid redirect URI | `oauth2_authorize_test.go` | `It("rejects unregistered redirect URI")` |
| US4-S3: Missing code_challenge | `oauth2_authorize_test.go` | `It("rejects request without PKCE challenge")` |
| US4-S4: Consent redirect | `oauth2_authorize_test.go` | `It("redirects to consent UI when no grant exists")` |
| US4-S5: Code exchange with PKCE | `oauth2_authorize_test.go` | `It("exchanges code for token with valid verifier")` |
| US4-S6: PKCE mismatch | `oauth2_authorize_test.go` | `It("rejects code exchange with wrong verifier")` |
| US4-S7: Code replay | `oauth2_authorize_test.go` | `It("rejects second use of authorization code")` |
| US4-S8: Code expiry | `oauth2_authorize_test.go` | `It("rejects expired authorization code")` |
| US5-S1: Add signing key | `oauth2_signing_keys_test.go` | `It("adds signing key and marks it current")` |
| US5-S2: List signing keys | `oauth2_signing_keys_test.go` | `It("lists all active keys without private material")` |
| US5-S3: Promote signing key | `oauth2_signing_keys_test.go` | `It("promotes key to current for new tokens")` |
| US5-S4: Remove non-current key | `oauth2_signing_keys_test.go` | `It("removes key from JWKS immediately")` |
| US5-S5: Cannot remove last key | `oauth2_signing_keys_test.go` | `It("returns 409 when removing last key")` |
| US5-S6: Key in JWKS via discovery | `oauth2_signing_keys_test.go` | `It("active keys appear in JWKS via discovery")` |
| US6-S1: Discovery endpoint | `oauth2_discovery_test.go` | `It("returns RFC 8414 metadata in issue_token mode")` |
| US6-S2: JWKS from discovery | `oauth2_discovery_test.go` | `It("jwks_uri returns valid JWKS with active keys")` |
| US6-S3: Discovery 404 in proxy mode | `oauth2_discovery_test.go` | `It("returns 404 for discovery in proxy mode")` |

**Red Phase Requirements**:
- E2E tests MUST compile and contain detailed, realistic expectations
- Assertions MUST target actual system output (e.g., `Expect(resp.StatusCode).To(Equal(200))`,
  `Expect(body).To(ContainSubstring("expected_field"))`)
- Placeholder always-fail assertions (e.g., `Expect(true).To(BeFalse())`) do NOT satisfy red phase —
  the failure must come from realistic assertions against the feature's expected behavior
- `XIt`, `PIt`, `XDescribe`, `PDescribe`, `XContext`, `PContext`, and `Skip()` are FORBIDDEN —
  all tests MUST run and fail
- Tests MUST NOT contain comments marking them as "in the red phase" — tests turn green naturally
  as implementation progresses and MUST NOT require annotation cleanup later

**Test Data Strategy**:
- Use fixtures from `tests/e2e/fixtures/` for stable, reusable test data
- Required fixtures: agents (with `redirect_uris`), principals (X-Remote-User), `issue_token` mode config
- New fixture creation: agent with redirect URIs, broker client credentials (generated via admin API in test setup), signing key (auto-generated or added via admin API)

**Test Execution Flow**:
1. **Phase 2f (Design)**: Write E2E tests for all spec scenarios with detailed expectations
2. **Verify Red Phase**: Run `ginkgo -v ./tests/e2e/[feature]_test.go` — all tests must FAIL semantically
3. **Implementation**: Implement feature incrementally
4. **Verify Green Phase**: E2E tests turn GREEN as implementation satisfies acceptance criteria
5. **Minimal Changes**: Only fixture adjustments during implementation, not test logic

**Bootstrap Strategy**:
- Tests use production bootstrap code via `tests/e2e/bootstrap/` (app.Builder, HTTP server, routing)
- Fresh server and storage for each test (BeforeEach/AfterEach isolation)
- Feature-specific: tests require `mode: issue_token` config with `issuer_uri` set; `EncryptionPort` must be configured (in-memory encryption adapter sufficient for E2E)

**Helper Utilities**:
- Custom matchers needed: `HaveValidJWT(issuer, kid string)` matcher for JWT validation, `HaveJWKSWithKID(kid string)` for JWKS response
- HTTP helpers: reuse existing helpers in `tests/e2e/helpers/`; add `GenerateCodeChallenge(verifier string)` and `PKCEVerifier()` helpers for PKCE test flows
- Mock services: no upstream mock needed — `issue_token` mode is fully standalone

### Frontend Playwright E2E Tests

N/A — this feature is purely backend with no React UI changes. The consent UI interaction during authorization code flow uses existing behavior.

### Unit & Integration Tests

**Unit Tests**:
- Location: `internal/domain/oauth2server/*_test.go` and `internal/domain/storage/*_test.go`
- Coverage: fosite strategy implementations (JWT signing, code generation), client authentication (Argon2id), signing key service (auto-generation, JWKS building), entity validation, CEL token claims evaluator (compile, evaluate, fail-closed on error, base claim protection)
- Strategy: TDD — write tests FIRST, verify they FAIL, then implement

**Integration Tests**:
- Location: `internal/adapters/storage/postgres/*_test.go`
- Coverage: All three PostgreSQL adapters (`BrokerClientCredentialRepo`, `SigningKeyRepo`, `AuthorizationCodeRepo`), migration apply/rollback
- Strategy: Real PostgreSQL via testcontainers, verify CRUD + edge cases (unique constraints, cascade deletes, partial index queries)

**Test Coverage Goals**:
- Unit test coverage: critical paths — all fosite strategy methods, client auth, signing key lifecycle, CEL token claims evaluator (valid expression, invalid expression startup fail, runtime error fail-closed, base claim override prevention)
- Integration test coverage: all three postgres adapter repositories + all 4 migrations
- E2E test coverage: 100% of 24+ acceptance scenarios from spec.md (mandatory per Principle XIII)
- Frontend E2E coverage: N/A (no UI changes)

## Complexity Tracking

No Constitution Check violations to justify. All preconditions pass.
