# Tasks: Broker OAuth2 Server Mode

**Input**: Design documents from `/specs/20260328-120000-oauth2-server/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/api-contracts.md, quickstart.md

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing) and Principle XIII (End-to-End Acceptance Testing), automated tests are MANDATORY. E2E tests are written in red phase before implementation. Unit and integration tests follow TDD within each user story.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing. Six user stories from spec.md (P1–P6) map to Phases 3–8.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Exact file paths included in descriptions

---

## Phase 0: Pre-implementation Refactoring

**Purpose**: Refactor `Validate()` in config to support mode-conditional validation without breaking existing proxy mode. This is a structural change with no behavior change, isolating it keeps the PR reviewable.

- [x] T001 Refactor `Validate()` in `internal/config/` to support mode-conditional validation: extract proxy-mode validation into a named function, add `issue_token` mode validation branch (issuer_uri required, token_ttl defaults to 1h, upstream fields not required) in `internal/config/`
- [x] T002 [P] Add `IssuerURI`, `TokenTTL`, and `TokenClaimsExpression` fields to `OAuth2AuthServerConfig` in `internal/ports/config.go` (note: `Mode` field already exists)
- [x] T003 [P] Add unit tests for mode-conditional validation: proxy mode unchanged, issue_token mode requires issuer_uri, issue_token mode defaults token_ttl to 1h, warning when upstream URI set in issue_token mode — in `internal/config/`
- [x] T004 Verify all existing tests pass after refactoring (zero behavior changes): `just test`

**Checkpoint**: Config refactoring complete, all existing tests pass, proxy mode behavior unchanged

---

## Phase 1: Setup (Dependencies & Project Structure)

**Purpose**: Add new dependencies and create package structure for the feature

- [x] T005 Add `ory/fosite` dependency to `go.mod`: `go get github.com/ory/fosite`
- [x] T006 [P] Add `golang.org/x/crypto` dependency (for argon2id) if not already present: verify in `go.mod`
- [x] T007 [P] Create package directory `internal/domain/oauth2server/` for fosite headless integration
- [x] T008 Run `go mod tidy` and verify project compiles: `just build`

**Checkpoint**: Dependencies added, project compiles, new package directory exists

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Domain model, configuration, API, database design, and E2E test red phase MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary

**Constitution Reference**: Principles II, V

- [x] T009 Add new domain terms to ARCHITECTURE.md Glossary: `BrokerClientCredential`, `SigningKey`, `AuthorizationCode`, `ClientID` (credential), `KeyID`, `OAuth2ServerProvider`, `TokenClaimsExpression`
- [x] T010 [P] Document domain model invariants: one credential per agent (UNIQUE agent_id), exactly one is_current signing key, authorization codes single-use with 60s TTL — in ARCHITECTURE.md or spec reference

**Checkpoint**: Domain model documented in ARCHITECTURE.md

### Phase 2b: Configuration Design

**Constitution Reference**: Principle VII

- [x] T011 Create example YAML configuration file at `examples/config/oauth2-server-mode.yaml` showing `mode: issue_token`, `issuer_uri`, `token_ttl`, and `token_claims_expression` with defaults and comments
- [x] T012 [P] Update `examples/config/README.md` to reference the new oauth2-server-mode configuration section
- [x] T013 [P] Update Helm chart `charts/agentic-identity-broker/values.yaml` with `mode`, `issuer_uri`, `token_ttl`, `token_claims_expression` parameters; update ConfigMap templates and chart README

**Checkpoint**: Configuration designed with YAML examples, Helm chart updated

### Phase 2c: API Design

**Constitution Reference**: Principles IV, X

- [x] T014 Add admin API endpoints to `/api/admin/openapi.yaml`: POST/GET/DELETE `/agents/{agent_id}/client-credentials`, POST/GET/DELETE/PUT `/oauth2-server/signing-keys` per contracts/api-contracts.md
- [x] T015 [P] Add enduser API endpoints to `/api/enduser/openapi.yaml`: POST `/oauth2/token` (client_credentials + authorization_code grants), GET `/oauth2/authorize`, GET `/oauth2/jwks.json`, GET `/.well-known/oauth-authorization-server` per contracts/api-contracts.md
- [x] T016 [P] Add `redirect_uris` field to Agent create/update endpoints in `/api/admin/openapi.yaml`
- [x] T017 Get user/stakeholder confirmation for API designs (document in PR description)

**Checkpoint**: APIs designed in OpenAPI specs and confirmed

### Phase 2d: Database Design

**Constitution Reference**: Principle IX

- [x] T018 Create migration `migrations/009_add_agent_redirect_uris.up.sql` and `migrations/009_add_agent_redirect_uris.down.sql` per data-model.md (includes both `redirect_uris` and `allowed_scopes` columns)
- [x] T019 [P] Create migration `migrations/010_create_broker_client_credentials.up.sql` and `migrations/010_create_broker_client_credentials.down.sql` per data-model.md
- [x] T020 [P] Create migration `migrations/011_create_signing_keys.up.sql` and `migrations/011_create_signing_keys.down.sql` per data-model.md
- [x] T021 [P] Create migration `migrations/012_create_authorization_codes.up.sql` and `migrations/012_create_authorization_codes.down.sql` per data-model.md

**Checkpoint**: All 4 migration pairs created with up/down SQL

### Phase 2e: ADR

**Constitution Reference**: Principle II

- [x] T022 Create ADR `adrs/20260328-120100-oauth2-server-mode.md` documenting: fosite headless integration pattern, strategy implementations (JWX for access tokens, stdlib crypto for auth codes), type containment rules (fosite types only in `internal/domain/oauth2server/`), and custom-implementation fallback plan

**Checkpoint**: ADR 20260328-120100 written and committed

### Phase 2f: E2E Acceptance Test Design (Red Phase)

**Constitution Reference**: Principle XIII

- [x] T023 Write E2E tests for US1 scenarios (4 It blocks) in `tests/e2e/oauth2_client_credentials_test.go`: generate credentials, rotate credentials, 404 for missing agent, get metadata without secret
- [x] T024 [P] Write E2E tests for US2 scenarios (4 It blocks) in `tests/e2e/oauth2_server_mode_test.go`: start in issue_token mode, no upstream URI needed, default proxy mode unchanged, auto-generate signing key
- [x] T025 [P] Write E2E tests for US3 scenarios (4 It blocks) in `tests/e2e/oauth2_token_test.go`: client_credentials grant issues signed token, invalid credentials returns 401, token validates via JWKS, invalid_scope for excessive scopes
- [x] T026 [P] Write E2E tests for US4 scenarios (8 It blocks) in `tests/e2e/oauth2_authorize_test.go`: auth code flow, invalid redirect URI, missing code_challenge, consent redirect, code exchange with PKCE, PKCE mismatch, code replay, code expiry
- [x] T027 [P] Write E2E tests for US5 scenarios (6 It blocks) in `tests/e2e/oauth2_signing_keys_test.go`: add signing key, list keys, promote key, remove non-current key, cannot remove last key, key in JWKS via discovery
- [x] T028 [P] Write E2E tests for US6 scenarios (3 It blocks) in `tests/e2e/oauth2_discovery_test.go`: discovery endpoint in issue_token mode, JWKS from discovery, discovery 404 in proxy mode
- [x] T029 [P] Create E2E test helpers: `GenerateCodeChallenge(verifier string)` and `PKCEVerifier()` in `tests/e2e/helpers/`; `HaveValidJWT(issuer, kid)` and `HaveJWKSWithKID(kid)` custom Gomega matchers in `tests/e2e/matchers/`
- [x] T030 [P] Create E2E test fixtures: agent with redirect_uris, issue_token mode config with issuer_uri, in `tests/e2e/fixtures/`
- [x] T031 Verify all 29 E2E tests compile and FAIL semantically (red phase): `ginkgo -v ./tests/e2e/oauth2_*` — detailed expectations present and failing, no placeholders, no XIt/PIt/Skip

**Checkpoint**: 29 E2E tests written, compile, and fail semantically (red phase verified)

---

## Phase 2.7: Entity Boilerplate (New Entities)

**Purpose**: Create empty-but-compiling CRUD scaffolding for all three new entities plus Agent modification. Isolates structural additions from business logic for clean code review.

### Boilerplate: Typed IDs

- [x] T032 Add `CredentialID`, `SigningKeyID`, `AuthorizationCodeID` UUID types to `internal/domain/id/gen_ids.go` and regenerate `internal/domain/id/uuid_ids_gen.go`
- [x] T033 [P] Add `ClientID`, `KeyID` string types to `internal/domain/id/string_ids.go`
- [x] T034 [P] Update `internal/domain/id/AGENTS.md` with new typed ID documentation

### Boilerplate: BrokerClientCredential

- [x] T035 Define `BrokerClientCredential` domain struct in `internal/domain/storage/broker_client_credential.go`
- [x] T036 Define `BrokerClientCredentialRepository` interface in `internal/ports/storage.go` (Create, GetByAgentID, GetByClientID, Delete)
- [x] T037 [P] Implement empty in-memory `BrokerClientCredentialStore` in `internal/adapters/storage/memory/broker_client_credential_store.go` (methods compile, return zero values / not-found)
- [x] T038 [P] Implement empty postgres `BrokerClientCredentialRepo` in `internal/adapters/storage/postgres/broker_client_credential_repo.go` (methods compile, return not-implemented)
- [x] T039 Add `BrokerClientCredentials()` accessor to both memory and postgres adapter structs
- [x] T040 [P] Create empty `ClientCredentialsHandler` in `internal/adapters/http/handlers/admin/client_credentials_handler.go` (Generate/Get/Revoke methods return 501)
- [x] T041 Register credential routes in `internal/adapters/http/routing/admin.go`: POST/GET/DELETE `/api/agents/{agent-id}/client-credentials`

### Boilerplate: SigningKey

- [x] T042 Define `SigningKey` domain struct in `internal/domain/storage/signing_key.go`
- [x] T043 Define `SigningKeyRepository` interface in `internal/ports/storage.go` (Create, GetByKID, GetCurrent, ListActive, SetCurrent, Delete, CountActive)
- [x] T044 [P] Implement empty in-memory `SigningKeyStore` in `internal/adapters/storage/memory/signing_key_store.go`
- [x] T045 [P] Implement empty postgres `SigningKeyRepo` in `internal/adapters/storage/postgres/signing_key_repo.go`
- [x] T046 Add `SigningKeys()` accessor to both memory and postgres adapter structs
- [x] T047 [P] Create empty `SigningKeysHandler` in `internal/adapters/http/handlers/admin/signing_keys_handler.go` (Add/List/SetCurrent/Remove methods return 501)
- [x] T048 Register signing key routes in `internal/adapters/http/routing/admin.go`: POST/GET `/api/oauth2-server/signing-keys`, PUT `/{kid}/current`, DELETE `/{kid}`

### Boilerplate: AuthorizationCode

- [x] T049 Define `AuthorizationCode` domain struct in `internal/domain/storage/authorization_code.go`
- [x] T050 Define `AuthorizationCodeRepository` interface in `internal/ports/storage.go` (Create, FindByCodeHash, MarkUsed, DeleteExpired)
- [x] T051 [P] Implement empty in-memory `AuthorizationCodeStore` in `internal/adapters/storage/memory/authorization_code_store.go`
- [x] T052 [P] Implement empty postgres `AuthorizationCodeRepo` in `internal/adapters/storage/postgres/authorization_code_repo.go`
- [x] T053 Add `AuthorizationCodes()` accessor to both memory and postgres adapter structs

### Boilerplate: Agent Modification & Enduser Handlers

- [x] T054 Add `RedirectURIs []string` and `AllowedScopes []string` fields to `Agent` struct in `internal/domain/storage/agent.go`
- [x] T055 [P] Create empty `JWKSHandler` in `internal/adapters/http/handlers/enduser/jwks_handler.go` (ServeJWKS returns 501)
- [x] T056 [P] Create empty `DiscoveryHandler` in `internal/adapters/http/handlers/enduser/discovery_handler.go` (ServeDiscovery returns 501)
- [x] T057 [P] Create empty `OAuth2AuthorizeHandler` in `internal/adapters/http/handlers/enduser/oauth2_authorize_handler.go` (Authorize returns 501)
- [x] T058 Register enduser routes conditionally (issue_token mode) in `internal/adapters/http/routing/enduser.go`: GET `/oauth2/authorize`, GET `/oauth2/jwks.json`, GET `/.well-known/oauth-authorization-server`

### Boilerplate: Builder Wiring

- [x] T059 Wire all new repository accessors, handler structs, and route registrations in `internal/app/builder.go`
- [x] T060 Verify project compiles with all scaffolding: `just build`

**Checkpoint**: All entity scaffolding compiles, empty handlers return 501, no business logic — ready for user story implementation

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Signing key infrastructure, fosite provider wiring, Argon2id hasher, and CEL token claims evaluator — foundations that ALL token-related user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Signing Key Service

- [x] T061 Implement `SigningKeyService` in `internal/domain/oauth2server/signing_key_service.go`: key pair generation (ES256 via `crypto/ecdsa` + P-256 and RS256 via `crypto/rsa` 2048-bit), PEM PKCS#8 serialization, encryption via `EncryptionPort`, JWKS building from active keys via `lestrrat-go/jwx/v3`
- [x] T062 [P] Write unit tests for `SigningKeyService` in `internal/domain/oauth2server/signing_key_service_test.go`: generate ES256 key pair, generate RS256 key pair, encrypt/decrypt round-trip, JWKS building with multiple keys of different algorithms, fail-closed on decrypt failure

### Argon2id Hasher

- [x] T063 Implement `Argon2Hasher` in `internal/domain/oauth2server/client_auth.go`: Hash (generate salt + argon2id + PHC string encoding) and Compare (decode PHC + constant-time comparison) per research.md Decision 4 parameters (64 MiB, 3 iterations, 4 parallelism, 16-byte salt, 32-byte key)
- [x] T064 [P] Write unit tests for `Argon2Hasher` in `internal/domain/oauth2server/client_auth_test.go`: hash-then-compare succeeds, wrong secret fails, PHC string format correct

### Client Authentication Service

- [x] T065 Implement `ClientAuthService` in `internal/domain/oauth2server/client_auth.go`: authenticate client_id + secret against `BrokerClientCredentialRepository`, look up agent, verify hash, return domain error on mismatch
- [x] T066 [P] Write unit tests for `ClientAuthService` in `internal/domain/oauth2server/client_auth_test.go`: valid credentials succeed, unknown client_id fails, wrong secret fails, deleted agent fails

### Fosite Headless Provider Wiring

- [x] T067 Implement `Provider` struct in `internal/domain/oauth2server/provider.go`: construct fosite `AuthorizeExplicitGrantHandler`, `ClientCredentialsGrantHandler`, `pkce.Handler` with custom strategies; expose domain-native methods (HandleClientCredentials, HandleAuthorize, HandleTokenExchange) that translate between project types and fosite types
- [x] T068 [P] Implement `JWXAccessTokenStrategy` in `internal/domain/oauth2server/strategies.go`: `GenerateAccessToken` (get current key, decrypt, sign JWT with ES256 or RS256 via jwx depending on key algorithm), `AccessTokenSignature` (SHA-256), `ValidateAccessToken` (verify via JWKS)
- [x] T069 [P] Implement `RandomCodeStrategy` in `internal/domain/oauth2server/strategies.go`: `GenerateAuthorizeCode` (32 bytes crypto/rand, base64url), `AuthorizeCodeSignature` (SHA-256), `ValidateAuthorizeCode` (no-op, validation in storage)
- [x] T070 [P] Implement `FositeStorage` adapters in `internal/domain/oauth2server/fosite_storage.go`: `CreateAuthorizeCodeSession`, `GetAuthorizeCodeSession`, `InvalidateAuthorizeCodeSession` wrapping `AuthorizationCodeRepository`; `CreateAccessTokenSession` (no-op for stateless JWT)
- [x] T071 [P] Implement `brokerClient` wrapper in `internal/domain/oauth2server/fosite_client.go`: wrap Agent + BrokerClientCredential as `fosite.Client` interface

### CEL Token Claims Evaluator

- [x] T072 Implement CEL token claims evaluator in `internal/domain/oauth2server/token_claims_cel.go`: compile expression at startup via `CELCompilerPort` (or use cel-go directly with typed environment), evaluate at issuance time with `agent`, `principal`, `request` variables, validate return type is `map[string]dyn`, strip base claim keys (`iss`, `sub`, `iat`, `exp`, `jti`, `kid`, `agent_id`, `scope`)
- [x] T073 [P] Write unit tests for CEL token claims evaluator in `internal/domain/oauth2server/token_claims_cel_test.go`: valid expression compiles, invalid expression fails startup, runtime error rejects token issuance (fail-closed), base claim override prevention (silently dropped), empty expression produces no claims

### Fosite Provider Tests

- [x] T074 Write unit tests for strategies in `internal/domain/oauth2server/strategies_test.go`: JWT round-trip (sign + verify) for both ES256 and RS256, code generation randomness, signature determinism
- [x] T075 [P] Write unit tests for fosite storage adapter in `internal/domain/oauth2server/fosite_storage_test.go`: create/get/invalidate code sessions, no-op access token storage

### Signing Key Auto-Generation at Startup

- [x] T076 Implement auto-generation logic in `internal/app/builder.go` or `internal/domain/oauth2server/provider.go`: check `CountActive`, if zero generate ES256 key, encrypt and store, mark as current, log info-level event
- [x] T077 [P] Write unit test for auto-generation: no keys → generates one; keys exist → no-op — in `internal/domain/oauth2server/signing_key_service_test.go`

**Checkpoint**: Signing key service, Argon2id hasher, client auth service, fosite provider, CEL evaluator, and auto-generation all implemented and tested. Ready for user story business logic.

---

## Phase 3: User Story 1 — Operator Generates Client Credentials for an Agent (Priority: P1) 🎯 MVP

**Goal**: Admin API enables operators to generate, view, and revoke broker-issued OAuth2 client credentials for agents.

**Independent Test**: Call admin API to generate credentials for an existing agent, verify unique client_id and secret returned. Rotate and verify old secret invalidated.

### Tests for User Story 1 ⚠️

- [x] T078 [P] [US1] Write unit tests for credential generation logic in `internal/domain/oauth2server/client_auth_test.go`: generate client_id format (`broker_` + 22 chars), secret entropy (32 bytes base64url), hash stored not plaintext
- [x] T079 [P] [US1] Write unit tests for credential rotation in `internal/domain/oauth2server/client_auth_test.go`: rotation replaces secret_hash + updates rotated_at, old hash no longer validates
- [x] T080 [P] [US1] Write integration tests for `BrokerClientCredentialRepo` postgres adapter in `internal/adapters/storage/postgres/broker_client_credential_repo_test.go`: CRUD operations, unique constraint on agent_id, cascade delete with agent, unique constraint on client_id

### Implementation for User Story 1

- [x] T081 [P] [US1] Implement in-memory `BrokerClientCredentialStore` CRUD in `internal/adapters/storage/memory/broker_client_credential_store.go` (replace 501 stubs with working logic)
- [x] T082 [P] [US1] Implement postgres `BrokerClientCredentialRepo` CRUD in `internal/adapters/storage/postgres/broker_client_credential_repo.go` (replace stubs with sqlx queries)
- [x] T083 [US1] Implement `ClientCredentialsHandler.Generate` in `internal/adapters/http/handlers/admin/client_credentials_handler.go`: validate agent exists, generate client_id + secret, hash with Argon2id, store credential, return 201 (first) or 200 (rotation) with plaintext secret
- [x] T084 [P] [US1] Implement `ClientCredentialsHandler.Get` in `internal/adapters/http/handlers/admin/client_credentials_handler.go`: return metadata (client_id, created_at, rotated_at) without secret, 404 if not found
- [x] T085 [P] [US1] Implement `ClientCredentialsHandler.Revoke` in `internal/adapters/http/handlers/admin/client_credentials_handler.go`: delete credentials, return 204, 404 if not found
- [x] T086 [US1] Add structured audit logging for credential generation, rotation, and revocation events

**Checkpoint**: US1 complete — credential provisioning works end-to-end via admin API. E2E tests for US1 scenarios (T023) should turn green.

---

## Phase 4: User Story 2 + User Story 3 — Mode Switch + Client Credentials Grant (Priority: P2+P3)

**Goal**: Broker starts in `issue_token` mode with signing key auto-generation; token endpoint accepts `client_credentials` grant and returns locally-signed JWT access tokens.

**Independent Test**: Start broker with `mode: issue_token`, call token endpoint with valid credentials, verify signed JWT returned and verifiable via JWKS.

### Tests for User Stories 2+3 ⚠️

- [x] T087 [P] [US2] Write unit tests for mode validation in `internal/config/`: issue_token mode accepts issuer_uri + token_ttl only, proxy mode requires upstream fields, default is proxy
- [x] T088 [P] [US3] Write unit tests for `Provider.HandleClientCredentials` in `internal/domain/oauth2server/provider_test.go`: valid credentials → signed JWT with correct claims (iss, sub, iat, exp, jti, kid, agent_id, scope), invalid credentials → error, invalid scope → error (when agent has allowed_scopes configured)
- [x] T089 [P] [US3] Write unit tests for JWT claims including CEL custom claims in `internal/domain/oauth2server/provider_test.go`: base claims always present, custom claims merged when expression configured, base claim keys not overridden

### Implementation for User Stories 2+3

- [x] T090 [US2] Implement mode-conditional startup in `internal/app/builder.go`: when `mode=issue_token`, initialize `Provider`, skip upstream proxy setup, wire token endpoint to local minting, log if upstream URI configured (warning)
- [x] T091 [US3] Implement or extend token handler in `internal/adapters/http/handlers/enduser/oauth2_token_handler.go`: dispatch `client_credentials` grant type to `Provider.HandleClientCredentials`, parse form body (client_id, client_secret, scope), return OAuth2 token response (access_token, token_type, expires_in)
- [x] T092 [P] [US3] Implement `Provider.HandleClientCredentials` in `internal/domain/oauth2server/provider.go`: authenticate client via `ClientAuthService`, validate requested scopes against agent's `AllowedScopes` (if non-empty), construct fosite `AccessRequest`, call `ClientCredentialsGrantHandler.HandleTokenEndpointRequest` + `PopulateTokenEndpointResponse`, extract token from response
- [x] T093 [US3] Add structured audit logging for token issuance events (agent_id, grant_type, scope, token jti)
- [x] T094 [US2] Wire enduser JWKS and discovery handler routes conditionally only in `issue_token` mode in `internal/adapters/http/routing/enduser.go`

**Checkpoint**: US2+US3 complete — broker starts in issue_token mode, client_credentials grant returns signed JWT, token verifiable via JWKS. E2E tests for US2 (T024) and US3 (T025) should turn green.

---

## Phase 5: User Story 4 — Authorization Code Flow with PKCE (Priority: P4)

**Goal**: Agents obtain tokens on behalf of authenticated users via authorization code flow with PKCE, fully standalone (no upstream OAuth2 redirect).

**Independent Test**: Initiate full authorization code + PKCE flow with authenticated user (X-Remote-User), verify broker-signed access token returned after code exchange.

### Tests for User Story 4 ⚠️

- [x] T095 [P] [US4] Write unit tests for authorization endpoint in `internal/domain/oauth2server/provider_test.go`: valid request with PKCE returns redirect with code, missing code_challenge rejects, unregistered redirect_uri rejects (no redirect), invalid client_id rejects
- [x] T096 [P] [US4] Write unit tests for code exchange in `internal/domain/oauth2server/provider_test.go`: valid code + verifier returns token, wrong verifier rejects, replay rejects, expired code rejects
- [x] T097 [P] [US4] Write unit tests for redirect_uri validation in `internal/domain/oauth2server/provider_test.go`: exact match required, empty redirect_uris list rejects
- [x] T098 [P] [US4] Write integration tests for `AuthorizationCodeRepo` postgres adapter in `internal/adapters/storage/postgres/authorization_code_repo_test.go`: create, find by hash, mark used, unique constraint on code_hash, delete expired

### Implementation for User Story 4

- [x] T099 [P] [US4] Implement in-memory `AuthorizationCodeStore` CRUD in `internal/adapters/storage/memory/authorization_code_store.go` (replace stubs with working logic including expiry and single-use checks)
- [x] T100 [P] [US4] Implement postgres `AuthorizationCodeRepo` CRUD in `internal/adapters/storage/postgres/authorization_code_repo.go` (atomic mark-used via `UPDATE ... SET used_at = NOW() WHERE used_at IS NULL RETURNING`)
- [x] T101 [US4] Implement `OAuth2AuthorizeHandler.Authorize` in `internal/adapters/http/handlers/enduser/oauth2_authorize_handler.go`: parse query params (response_type, client_id, redirect_uri, state, code_challenge, code_challenge_method, scope), validate client_id via `BrokerClientCredentialRepository`, validate redirect_uri against agent's registered URIs, enforce PKCE (S256 only), check consent (redirect to consent UI if no grant), call `Provider.HandleAuthorize`, redirect with code + state
- [x] T102 [US4] Implement `Provider.HandleAuthorize` in `internal/domain/oauth2server/provider.go`: construct fosite `AuthorizeRequest`, set session with principal from preauth, call `AuthorizeExplicitGrantHandler.HandleAuthorizeEndpointRequest` + `pkce.Handler.HandleAuthorizeEndpointRequest`, extract code from response
- [x] T103 [US4] Extend token handler to dispatch `authorization_code` grant type in `internal/adapters/http/handlers/enduser/oauth2_token_handler.go`: parse code, redirect_uri, client_id, code_verifier; call `Provider.HandleAuthorizationCodeExchange`
- [x] T104 [US4] Implement `Provider.HandleAuthorizationCodeExchange` in `internal/domain/oauth2server/provider.go`: authenticate client, construct fosite `AccessRequest`, call `AuthorizeExplicitGrantHandler.HandleTokenEndpointRequest` + `pkce.Handler.HandleTokenEndpointRequest` + `PopulateTokenEndpointResponse`, extract token
- [x] T105 [P] [US4] Update Agent admin handler to accept `redirect_uris` and `allowed_scopes` fields on create/update in `internal/adapters/http/handlers/admin/agents_handler.go`
- [x] T106 [P] [US4] Update in-memory Agent store to persist `redirect_uris` and `allowed_scopes` in `internal/adapters/storage/memory/agent_repository.go`
- [x] T107 [P] [US4] Update postgres Agent repo to persist `redirect_uris` and `allowed_scopes` in `internal/adapters/storage/postgres/agent_repository.go`

**Checkpoint**: US4 complete — full authorization code + PKCE flow works end-to-end. E2E tests for US4 (T026) should turn green.

---

## Phase 6: User Story 5 — Operator Manages Signing Keys (Priority: P5)

**Goal**: Admin API allows operators to add, list, promote, and remove signing keys for key rotation without disrupting active tokens.

**Independent Test**: Add a key (becomes current), issue token (check kid), add second key (becomes current), verify first key still validates old token, new key signs new tokens.

### Tests for User Story 5 ⚠️

- [x] T108 [P] [US5] Write unit tests for signing key admin operations in `internal/domain/oauth2server/signing_key_service_test.go`: add key becomes current (previous demoted), list returns metadata only, promote key changes current, remove non-current succeeds, remove last key returns conflict
- [x] T109 [P] [US5] Write integration tests for `SigningKeyRepo` postgres adapter in `internal/adapters/storage/postgres/signing_key_repo_test.go`: CRUD operations, partial index on active keys, is_current invariant, soft delete via removed_at

### Implementation for User Story 5

- [x] T110 [P] [US5] Implement in-memory `SigningKeyStore` CRUD in `internal/adapters/storage/memory/signing_key_store.go` (replace stubs with working logic including is_current invariant)
- [x] T111 [P] [US5] Implement postgres `SigningKeyRepo` CRUD in `internal/adapters/storage/postgres/signing_key_repo.go` (replace stubs with sqlx queries, transactional SetCurrent)
- [x] T112 [US5] Implement `SigningKeysHandler.Add` in `internal/adapters/http/handlers/admin/signing_keys_handler.go`: generate key pair server-side based on algorithm (default ES256), encrypt private material, store, mark as current (demote previous), return 201 with kid + metadata
- [x] T113 [P] [US5] Implement `SigningKeysHandler.List` in `internal/adapters/http/handlers/admin/signing_keys_handler.go`: list active keys with kid, algorithm, is_current, created_at — never return private key material, use `items` wrapper per Zalando guidelines
- [x] T114 [P] [US5] Implement `SigningKeysHandler.SetCurrent` in `internal/adapters/http/handlers/admin/signing_keys_handler.go`: promote key to current, return 200 with key metadata, 404 if not found
- [x] T115 [US5] Implement `SigningKeysHandler.Remove` in `internal/adapters/http/handlers/admin/signing_keys_handler.go`: soft-delete (set removed_at), return 204, 409 if last key, 404 if not found

**Checkpoint**: US5 complete — signing key lifecycle (add, list, promote, remove) works end-to-end. E2E tests for US5 (T027) should turn green.

---

## Phase 7: User Story 6 — Client Discovers Broker OAuth2 Metadata (Priority: P6)

**Goal**: Standard RFC 8414 discovery and JWKS endpoints enable zero-config client integration with the broker as authorization server.

**Independent Test**: Fetch `/.well-known/oauth-authorization-server`, verify all RFC 8414 fields present, follow jwks_uri, verify JWKS contains active public keys.

### Tests for User Story 6 ⚠️

- [x] T116 [P] [US6] Write unit tests for discovery handler in `internal/adapters/http/handlers/enduser/discovery_handler_test.go`: returns correct issuer, endpoints, grant types, response types, code_challenge_methods; 404 in proxy mode
- [x] T117 [P] [US6] Write unit tests for JWKS handler in `internal/adapters/http/handlers/enduser/jwks_handler_test.go`: returns JWK Set with active public keys, correct Cache-Control header, no private key material

### Implementation for User Story 6

- [x] T118 [US6] Implement `DiscoveryHandler.ServeDiscovery` in `internal/adapters/http/handlers/enduser/discovery_handler.go`: return RFC 8414 metadata JSON with issuer, authorization_endpoint, token_endpoint, jwks_uri, response_types_supported, grant_types_supported, token_endpoint_auth_methods_supported, code_challenge_methods_supported; set Cache-Control: public, max-age=3600
- [x] T119 [US6] Implement `JWKSHandler.ServeJWKS` in `internal/adapters/http/handlers/enduser/jwks_handler.go`: call `SigningKeyService.BuildJWKS()` to get active public keys, return JWK Set JSON; set Cache-Control: public, max-age=300
- [x] T120 [US6] Ensure discovery and JWKS endpoints return 404 in proxy mode (verify conditional route registration from T058/T094)

**Checkpoint**: US6 complete — discovery and JWKS endpoints work, clients can auto-discover broker metadata. E2E tests for US6 (T028) should turn green.

---

## Phase 8: Integration Testing (PostgreSQL Adapters + Migrations)

**Purpose**: Verify all PostgreSQL adapters and migration apply/rollback with real database

- [x] T121 Write integration tests for migration 009 (redirect_uris) apply/rollback in `internal/adapters/storage/postgres/` using testcontainers
- [x] T122 [P] Write integration tests for migration 010 (broker_client_credentials) apply/rollback in `internal/adapters/storage/postgres/` using testcontainers
- [x] T123 [P] Write integration tests for migration 011 (signing_keys) apply/rollback in `internal/adapters/storage/postgres/` using testcontainers
- [x] T124 [P] Write integration tests for migration 012 (authorization_codes) apply/rollback in `internal/adapters/storage/postgres/` using testcontainers

**Checkpoint**: All migrations tested, all postgres adapters verified with real database

---

## 🔒 Phase N: Constitution Compliance & Polish

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification

#### Design Phase Verification

- [x] T125 Verify domain model (BrokerClientCredential, SigningKey, AuthorizationCode) documented in ARCHITECTURE.md Glossary (Principle V)
- [x] T126 Verify configuration YAML examples exist in `examples/config/oauth2-server-mode.yaml` (Principle VII)
- [x] T127 Verify configuration examples referenced in `examples/config/README.md` (Principle VII)
- [x] T128 Verify API designs documented in `/api/enduser/openapi.yaml` and `/api/admin/openapi.yaml` (Principles IV, X)
- [x] T129 Verify user/stakeholder confirmed API designs (document reference in PR) (Principle X)
- [x] T130 Verify database schema migrations 009–012 in `/migrations/` follow sequential numbering (Principle IX)
- [x] T131 Verify E2E acceptance tests written in `tests/e2e/` for all 29 acceptance scenarios from spec.md (Principle XIII)
- [x] T132 Verify E2E tests verified to FAIL before implementation (red phase completed at T031) (Principle XIII)

#### Implementation Phase Verification

**API & Documentation** (Principles IV, X):
- [x] T133 [P] Verify API implementation matches confirmed OpenAPI specification exactly
- [x] T134 [P] Update `docs/api/` with OAuth2 server mode API documentation and integration examples

**Architecture & Documentation** (Principle II):
- [x] T135 Update ARCHITECTURE.md with `issue_token` mode architecture, import rules for `internal/domain/oauth2server/`
- [x] T136 [P] Verify ADR 20260328-120100 in `adrs/20260328-120100-oauth2-server-mode.md` complete and accurate

**Configuration** (Principle VII):
- [x] T137 [P] Verify configuration uses `internal/ports/config.go` config port (no ad-hoc loading)
- [x] T138 Verify Helm chart updated: `charts/agentic-identity-broker/values.yaml` reflects mode, issuer_uri, token_ttl, token_claims_expression parameters

**Database & Persistence** (Principle IX):
- [x] T139 [P] Verify migrations 009–012 tested (apply/rollback) in PostgreSQL integration tests
- [x] T140 [P] Verify all three PostgreSQL-backed repositories tested in integration tests

**Security** (Principles I, III):
- [x] T141 Verify security features enabled by default: PKCE enforced (S256 only), Argon2id hashing, signing key material encrypted at rest, fail-closed on decrypt failure
- [x] T142 [P] Verify no custom cryptography: `lestrrat-go/jwx/v3` for JWT, `golang.org/x/crypto/argon2` for hashing, `crypto/rand` for code generation, `crypto/ecdsa` for key generation
- [x] T143 [P] Verify structured audit logging for credential generation/rotation, token issuance, key management events

**Architecture Patterns** (Principle VI):
- [x] T144 Verify fosite types contained within `internal/domain/oauth2server/` — never leak into `ports/`, `adapters/http/`, or `app/`

**Testing** (Principles VIII, XIII):
- [x] T145 Verify all E2E tests pass: `ginkgo -v ./tests/e2e/oauth2_*`
- [x] T146 Verify all unit tests pass: `just test`
- [x] T147 Run static checks with `just check` (fmt → vet → lint), then run `just verify`

### Additional Polish

- [x] T148 [P] Create end-user documentation for OAuth2 server mode in `docs/features/oauth2-server-mode.md`
- [x] T148b [P] Update `docs/configuration.md` with `mode`, `issuer_uri`, `token_ttl`, `token_claims_expression` parameters (Constitution Principle VII)
- [x] T149 Code cleanup: remove any TODO markers, verify all 501 stubs replaced with business logic
- [x] T150 Run quickstart.md validation: verify the YAML config example from quickstart.md works end-to-end

---

## Dependencies & Execution Order

### Phase Dependencies

```
Phase 0 (Config refactor)
  │
  ▼
Phase 1 (Setup/Dependencies) ──── can run parallel with Phase 0
  │
  ▼
Phase 2 (Design Preconditions) ── BLOCKS all implementation
  │ ├── 2a (Domain Model)     ──┐
  │ ├── 2b (Config Design)     │ All sub-phases can
  │ ├── 2c (API Design)        │ proceed in parallel
  │ ├── 2d (Database Design)   │
  │ ├── 2e (ADR)               │
  │ └── 2f (E2E Red Phase)    ──┘
  │
  ▼
Phase 2.7 (Entity Boilerplate) ── depends on all Phase 2 completion
  │
  ▼
Phase 2.5 (Foundational Infra) ── depends on Phase 2.7 completion
  │
  ▼
Phase 3 (US1: Credentials) ──────────────── P1 MVP
  │
  ▼
Phase 4 (US2+US3: Mode + Token) ─────────── P2+P3
  │
  ▼
Phase 5 (US4: Auth Code + PKCE) ─────────── P4
  │
  ▼
Phase 6 (US5: Signing Key Admin) ────────── P5 (can parallel with Phase 5)
  │
  ▼
Phase 7 (US6: Discovery + JWKS) ─────────── P6 (can parallel with Phase 5/6)
  │
  ▼
Phase 8 (Integration Testing) ───────────── depends on all user stories
  │
  ▼
Phase N (Constitution Compliance) ────────── FINAL
```

### User Story Dependencies

- **US1 (P1 — Credentials)**: Depends on Phase 2.5 (Argon2id hasher, client auth). No other US dependency.
- **US2+US3 (P2+P3 — Mode + Token)**: Depends on US1 (needs credentials to authenticate) + Phase 2.5 (fosite provider, signing keys, CEL evaluator).
- **US4 (P4 — Auth Code)**: Depends on US2+US3 (needs token endpoint + mode switch). Also needs Phase 2.5 (code strategy, PKCE handler).
- **US5 (P5 — Signing Key Admin)**: Depends on Phase 2.5 (signing key service). Can parallel with US4.
- **US6 (P6 — Discovery + JWKS)**: Depends on Phase 2.5 (JWKS building). Can parallel with US4/US5.

### Parallel Opportunities Per Story

**Within Phase 2**: All sub-phases (2a–2f) can run in parallel.
**Within Phase 2.7**: BrokerClientCredential, SigningKey, AuthorizationCode boilerplate blocks are independent.
**Within Phase 2.5**: Signing key service, Argon2id hasher, and CEL evaluator can be built in parallel. Fosite provider wiring depends on all three.
**US5 ∥ US4**: Signing key admin is independent of authorization code flow.
**US6 ∥ US4 ∥ US5**: Discovery/JWKS endpoints only need signing key service from Phase 2.5.
**Phase 8**: All 4 migration integration tests can run in parallel.

---

## Parallel Example: Phase 2.5 (Foundational)

```bash
# Three independent foundational components:
T061 + T062: SigningKeyService (key generation, encryption, JWKS building)
T063 + T064: Argon2Hasher (hash + compare)
T072 + T073: CEL token claims evaluator (compile + eval)

# Then sequential (depends on above):
T067: Provider struct wiring (needs strategies, storage, CEL)
T068-T071: Strategy + storage adapter implementations
T076-T077: Auto-generation at startup
```

## Parallel Example: User Story 1

```bash
# Tests first (parallel):
T078: Unit tests for credential generation
T079: Unit tests for credential rotation
T080: Integration tests for postgres adapter

# Implementation (some parallel):
T081 ∥ T082: Memory + Postgres adapter implementations
T083: Generate handler (sequential — needs adapters)
T084 ∥ T085: Get + Revoke handlers (parallel after T083 pattern established)
T086: Audit logging (sequential — needs handlers)
```

---

## Implementation Strategy

### MVP Scope
**Phase 0 + 1 + 2 + 2.7 + 2.5 + Phase 3 (US1)**: Operators can generate client credentials for agents. This is the minimum viable increment that delivers demonstrable value without token minting.

### Incremental Delivery
1. **MVP**: Credential provisioning (US1) — proves admin workflow
2. **Core**: Mode switch + client_credentials grant (US2+US3) — proves token minting end-to-end
3. **Full Flow**: Authorization code + PKCE (US4) — proves user-delegated token issuance
4. **Operations**: Signing key management (US5) + discovery (US6) — operational completeness
5. **Hardening**: Integration tests + constitution compliance — production readiness

### Risk Mitigation
- **Fosite v0.x instability**: ADR 20260328-120100 documents fallback to custom implementation. Strategy interfaces isolate fosite from project code — replacing fosite handlers with custom logic requires changes only in `internal/domain/oauth2server/`.
- **Encryption failure**: Fail-closed by design. SR-006 ensures corrupt signing keys halt startup; SR-004 ensures private material always encrypted.
- **CEL expression errors**: FR-013b validates at startup; FR-013e rejects tokens at runtime. No partial-claims tokens ever issued.
