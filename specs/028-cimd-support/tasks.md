# Tasks: Client ID Metadata Document (CIMD) Support

**Input**: Design documents from `/specs/028-cimd-support/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/, quickstart.md

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY for all features. Test tasks are included in each user story below and MUST be written before or alongside implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and configuration scaffolding

- [X] T001 Add `CIMDConfig` struct to `OAuth2AuthServerConfig` in `internal/ports/config.go` with defaults (enabled: false, fetch_timeout: 1s, max_response_bytes: 5120, cache.max_ttl: 1h, cache.min_ttl: 60s)
- [ ] T001a Add startup validation in config or builder: reject `cimd.enabled: true` when `oauth2_authorization_server.mode` is `proxy` with a clear error message. CIMD requires `issue_token` mode. Write a unit test verifying the startup error.
- [X] T002 [P] Register Viper defaults for all `oauth2_authorization_server.cimd.*` keys in config initialization
- [X] T003 [P] Create example YAML config at `examples/config/cimd.yaml` with annotated CIMD configuration
- [X] T004 [P] Update Helm chart `charts/agentic-identity-broker/values.yaml` with `cimd` block under `oauth2AuthorizationServer`

**Checkpoint**: Configuration scaffolding in place, project compiles

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Domain model, configuration, API, and database design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary

- [X] T005 Add CIMD domain terms to ARCHITECTURE.md Glossary: ClientIDMetadataDocument, CIMDCacheEntry, ClientIDMetadataDocumentURL, SSRFBlocklist, BrandPinMismatchDetected, CIMDSecurityFieldChanged
- [X] T006 [P] Document ClientResolver strategy pattern and CIMDFetcher port in ARCHITECTURE.md

**Checkpoint**: Domain model documented

### Phase 2b: Configuration Design

- [X] T007 Verify `CIMDConfig` YAML examples committed to `examples/config/cimd.yaml` (from T003)
- [X] T008 [P] Update `examples/config/README.md` to reference CIMD configuration section

**Checkpoint**: Configuration designed with YAML examples

### Phase 2c: API Design

- [X] T009 Update `/api/admin/openapi.yaml`: extend AgentRequest/AgentResponse schemas with `client_uris`, `auth_method`, `jwks_uri`
- [X] T010 [P] Update `/api/enduser/openapi.yaml`: add `client_id_metadata_document_supported` to metadata response, add `cimd_metadata` to consent response, document new error responses for `/oauth2/authorize`
- [X] T011 Get user/stakeholder confirmation for API design changes (API-001 through API-004)

**Checkpoint**: APIs designed and confirmed

### Phase 2d: Database Design

- [X] T012 Create migration `migrations/015_add_cimd_support.up.sql`: create normalized `agent_client_uris` child table (`agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE`, `client_uri TEXT NOT NULL`, `UNIQUE(client_uri)`) — no `authorization_sessions` table required; session state is carried in the stateless JWE `session_token`
- [X] T013 [P] Create migration `migrations/015_add_cimd_support.down.sql`: drop `agent_client_uris` table

**Checkpoint**: Database migrations created

### Phase 2e: Frontend/Design System Review

> **Historical visual scope**: The visual choices and design checks in this task record retain their original wording and completion states, not renewed aesthetic requirements. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

- [X] T014 Review `web/src/design-system/docs/INDEX.md` for component selection for CIMD consent components (CS-001–CS-004)
- [X] T015 [P] Identify semantic tokens for domain badge (trust-deep), localhost warning (warning/danger), advanced details section

**Checkpoint**: Design system usage planned

### Phase 2f: E2E Acceptance Test Design

- [X] T016 Create CIMD test fixtures in `tests/e2e/fixtures/cimd.go`: `CIMDAgent()`, `ValidCIMDDocument()`, `CIMDConfig()`, mock CIMD HTTPS server helper
- [X] T017 Write E2E tests in `tests/e2e/cimd_authorization_test.go` for US1 scenarios (5 It blocks: resolve agent from CIMD, client_id mismatch, absent document, redirect_uri mismatch, disabled gate)
- [X] T018 [P] Write E2E tests in `tests/e2e/cimd_ssrf_test.go` for US2 scenarios (7 It blocks: private IP, loopback, link-local, HTTP scheme, dot segments, oversized, timeout)
- [X] T019 [P] Write E2E tests in `tests/e2e/cimd_caching_test.go` for US3 scenarios (4 It blocks: cache hit, expired refetch, no error cache, operator TTL override)
- [X] T020 [P] Write E2E tests in `tests/e2e/cimd_metadata_test.go` for US4 scenarios (2 It blocks: field present when enabled, absent when disabled)
- [X] T021 [P] Write E2E tests in `tests/e2e/cimd_consent_test.go` for US5 scenarios (4 It blocks: summary+badge+details, localhost warning, expand details, brand mismatch)
- [X] T022 [P] Write Playwright E2E tests in `tests/e2e/frontend/cimd_consent_test.go` for CS-001–CS-004 with screenshot captures
- [X] T023 Verify all E2E tests FAIL semantically (red phase): detailed expectations present and failing, no placeholders, no XIt/PIt/Skip markers

**Checkpoint**: E2E tests written and verified to fail before implementation

---

## Phase 2.7: Agent Entity Extension (Boilerplate)

**Purpose**: Extend the existing Agent entity with CIMD fields and add new storage method, isolated from business logic

- [X] T024 Add `ClientURIs []string`, `AuthMethod *string`, `JwksURI *string` fields to Agent struct in `internal/domain/storage/agent.go`; update `Validate()`, `ValidateForCreate()`, and `Copy()` — validate each `client_uris` entry as a well-formed HTTPS URL on both create and update paths
- [X] T025 Add `GetByClientURI(ctx context.Context, uri string) (*Agent, error)` to `AgentRepository` interface in `internal/ports/storage.go`
- [X] T026 [P] Implement `GetByClientURI` on in-memory adapter in `internal/adapters/storage/memory/` with secondary index `map[string]id.AgentID`; enforce global uniqueness on create/update (reject duplicate URIs across agents)
- [X] T027 [P] Implement `GetByClientURI` on postgres adapter in `internal/adapters/storage/postgres/` using `SELECT agent_id FROM agent_client_uris WHERE client_uri = $1`; all Create and Update operations must manage the parent agents row and child agent_client_uris rows atomically within a single transaction — a UNIQUE(client_uri) violation must roll back the entire operation leaving no partial state; hydrate `Agent.ClientURIs` from agent_client_uris on every read path (Get, List, GetByClientID, GetByClientURI); map constraint violation to 409-mappable error
- [X] T027a [P] Write postgres storage tests that: (1) round-trip ClientURIs through create/get/list/update and assert ClientURIs are fully populated on each read; (2) assert GetByClientID returns Agent.ClientURIs fully populated; (3) assert GetByClientURI returns Agent.ClientURIs fully populated; (4) prove a duplicate-URI conflict rolls back the entire create (no agent row remains) and the entire update (agent row retains its pre-update state); also write admin API tests for invalid and duplicate `client_uris` on both `POST /api/agents` (create) and `PUT /api/agents/{agent-id}` (update) — verify 400 for malformed URLs and 409 for duplicates across agents
- [X] T028 Extend `AgentRequest`/`AgentResponse` DTOs with `ClientURIs`, `AuthMethod`, `JwksURI` in `internal/adapters/http/handlers/admin/agents_handler.go`
- [X] T029 Verify project compiles with all agent entity extensions (`just build`)

**Checkpoint**: Agent entity extended, project compiles, no business logic yet

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: CIMD domain types, port interface, and fetcher adapter that ALL user stories depend on

- [X] T030 Define `ClientResolver` strategy interface, `CIMDFetcher` port interface, `ClientResolution` DTO, and `CIMDFetchResult` DTO in `internal/ports/cimd.go`
- [X] T031 [P] Implement `ClientIDMetadataDocumentURL` value object with parse-time validation (scheme, path, fragment, credentials, port, dot-segments) in `internal/domain/cimd/url.go`
- [X] T032 [P] Implement `SSRFBlocklist` value object with RFC 6890 default ranges + operator extras in `internal/domain/cimd/blocklist.go`
- [X] T033 [P] Implement `ClientIDMetadataDocument` value object with JSON parsing and validation (client_id match, auth method, redirect URI same-origin, keyword blocklist) in `internal/domain/cimd/document.go`
- [ ] T033a [P] [US2] Implement SSRF validation for embedded URL fields (`logo_uri`, `jwks_uri`, `policy_uri`, `tos_uri`) during CIMD document parsing in `internal/domain/cimd/document.go`: resolve each URL's hostname, validate against SSRFBlocklist, log blocked fields as structured audit events, silently omit blocked fields from the returned document (SR-010). Write unit tests in `internal/domain/cimd/document_test.go`.
- [X] T034 Implement `CIMDCache` (sync.RWMutex-protected map with TTL clamping from HTTP headers) in `internal/domain/cimd/cache.go`
- [X] T035 Implement SSRF-hardened HTTP fetcher adapter (custom Dialer.Control, no redirects, LimitReader, context timeout) in `internal/adapters/cimd/fetcher.go`
- [X] T036 [P] Write unit tests for `ClientIDMetadataDocumentURL` in `internal/domain/cimd/url_test.go`
- [X] T037 [P] Write unit tests for `SSRFBlocklist` in `internal/domain/cimd/blocklist_test.go`
- [X] T038 [P] Write unit tests for `ClientIDMetadataDocument` in `internal/domain/cimd/document_test.go`
- [X] T039 [P] Write unit tests for `CIMDCache` in `internal/domain/cimd/cache_test.go`
- [X] T040 Write integration tests for SSRF-hardened fetcher with injected resolver/dialer and dial-attempt spy in `internal/adapters/cimd/fetcher_test.go`

**Checkpoint**: Foundation ready — all CIMD domain types, port, and adapter in place; user story implementation can begin

---

## Phase 3: User Story 1 — Agent Authenticates with URL-Based Client ID (Priority: P1) 🎯 MVP

**Goal**: An AI agent uses its HTTPS URL as `client_id`; the broker fetches, validates, and presents CIMD metadata on the consent screen

**Independent Test**: Send a valid HTTPS URL `client_id` in an authorization request to a broker backed by a mock CIMD endpoint; verify the consent screen renders CIMD metadata

### Tests for User Story 1

- [X] T041 [P] [US1] Write unit tests for `CIMDService` orchestration (fetch → validate → cache → audit → return) in `internal/domain/cimd/service_test.go`, including: first-fetch baseline population (no audit event), subsequent fetch with changed snapshot field (audit event emitted + Agent updated via repository), unchanged field (no audit event)
- [X] T042 [P] [US1] Write unit tests for `OpaqueClientResolver` (rejects https:// prefix, resolves UUID) in `internal/domain/oauth2/client_resolver_test.go`
- [X] T043 [P] [US1] Write unit tests for `CIMDClientResolver` (URL detection → CIMD resolution → fallback to UUID) in `internal/domain/cimd/client_resolver_test.go`

### Implementation for User Story 1

- [X] T044 [US1] Implement `CIMDService` in `internal/domain/cimd/service.go`: URL validation → cache check → fetch via port → document validation → brand pin check → security field change detection → persist updated snapshot fields (redirect_uris, auth_method, jwks_uri) back to Agent via `AgentRepository.Update` → cache store (first fetch populates baseline without audit event; subsequent fetches compare then update)
- [X] T045 [US1] Implement `OpaqueClientResolver` in `internal/domain/oauth2/client_resolver.go`: reject `https://`-prefixed client_id with `invalid_client`, parse UUID for opaque IDs
- [X] T046 [US1] Implement `CIMDClientResolver` in `internal/domain/cimd/client_resolver.go`: detect URL → validate → lookup agent by client URI → fetch/validate CIMD → return resolution with metadata; fall back to UUID for non-URL
- [X] T047 [US1] Modify `OAuth2AuthorizationService` in `internal/domain/oauth2/service.go` to delegate client resolution to injected `ClientResolver` strategy instead of direct `id.ParseAgentID`
- [X] T048 [US1] Wire `ClientResolver` strategy in `internal/app/builder.go` based on `cimd.enabled`: `CIMDClientResolver` when true, `OpaqueClientResolver` when false; instantiate CIMDService/fetcher/cache only when enabled
- [X] T049 [US1] Verify US1 E2E tests in `tests/e2e/cimd_authorization_test.go` turn green

**Checkpoint**: User Story 1 fully functional — URL-based client_id resolves agent and presents CIMD metadata

---

## Phase 3.5: Authorization Session — Stateless JWE Consent Context Binding (SR-013/SR-014)

**Purpose**: Secure CIMD consent flows by sealing authorization request context into a JWE token, eliminating URL parameter tampering and server-side storage. CIMD-only; opaque client_id flows unchanged.

### Shared JWE Package

- [X] T108 Extract shared `internal/domain/jwe/` package: `TokenService` struct with `Encrypt(v any) (string, error)` and `Decrypt(token string, target any) error`; algorithm `A256GCMKW` + `A256GCM`; key source: existing `IDENTITY_BROKER_JWE_SIGNING_KEY`. Write unit tests in `token_service_test.go` for encrypt/decrypt round-trip and expiry rejection.
- [X] T109 [P] Refactor `OAuth2SessionService` in `internal/domain/oauth2session/service.go` to use `jwe.TokenService` for state token encrypt/decrypt instead of inline calls.

### Authorization Session Token

- [X] T110 Implement `AuthorizationSessionClaims` struct in `internal/domain/oauth2/authorization_session_token.go` with all authorization context fields plus `IssuedAt`/`ExpiresAt` (10-min TTL). Add `createAuthorizationSessionToken` and `validateAuthorizationSessionToken` helpers on `OAuth2AuthorizationService`.

### Integration into Authorization Flow

- [X] T115 Modify `OAuth2AuthorizationService.HandleAuthorization` in `internal/domain/oauth2/service.go`: when `ClientResolution` contains CIMD metadata, mint JWE `session_token` with full authorization context and redirect to consent with `?session_token=<jwe>` only (replacing the `OriginalURL` embedding pattern for CIMD flows).
- [X] T116 Add decode endpoint `GET /api/consent/session?token=<jwe>` in consent handler (`internal/adapters/http/handlers/consent/`): decrypt token, validate `exp`, return structured JSON with display fields and `cimd_metadata` (FR-028).
- [X] T117 Modify consent submission handler: accept `session_token` in request body, decrypt, validate `exp` and `principal` binding, use token's trusted `redirect_uri`/`state`/`code_challenge` for the authorization code redirect (FR-029).
- [X] T118 Wire shared `*jwe.TokenService` in `internal/app/builder.go`: inject into both `OAuth2SessionService` and `OAuth2AuthorizationService`.

### E2E Tests

- [X] T119 [P] Write E2E tests in `tests/e2e/cimd_consent_test.go` for token edge cases: expired `session_token` → 400, malformed token → 400, wrong principal → 400.
- [X] T120 Verify JWE-based CIMD authorization flow E2E: authorization request → `session_token` minted → consent page decodes token → consent submitted with token → authorization code redirect uses trusted redirect_uri.

**Checkpoint**: CIMD consent flows are tamper-proof — all trust metadata is sealed in the JWE `session_token`, not URL params

---

## Phase 4: User Story 2 — SSRF-Hardened Fetcher Blocks Malicious URLs (Priority: P1)

**Goal**: The CIMD fetcher proactively rejects URLs targeting private networks, loopback, link-local, and enforces timeout/size limits

**Independent Test**: Send authorization requests with client_id pointing to blocked IP ranges; verify rejection before TCP connection

### Tests for User Story 2

- [X] T050 [P] [US2] Write additional fetcher adapter tests for each SSRF category (private, loopback, link-local, HTTP scheme, dot-segments, oversized, timeout) with dial-attempt spy in `internal/adapters/cimd/fetcher_test.go`

### Implementation for User Story 2

- [X] T051 [US2] Verify SSRF enforcement is complete in fetcher adapter (all 14 categories from SC-002); add any missing checks in `internal/adapters/cimd/fetcher.go` and `internal/domain/cimd/url.go`
- [X] T052 [US2] Add structured audit logging for SSRF blocks (CIMDFetchBlocked events) in `internal/domain/cimd/service.go`
- [X] T053 [US2] Verify US2 E2E tests in `tests/e2e/cimd_ssrf_test.go` turn green

**Checkpoint**: All SSRF attack categories individually rejected, adapter tests prove no TCP dial for blocked addresses

---

## Phase 5: User Story 5 — CIMD-Enhanced Consent Screen (Priority: P1)

**Goal**: Consent screen displays CIMD-sourced metadata: summary, domain badge, localhost warning, advanced details

**Independent Test**: Render consent screen for a CIMD-based authorization request; assert each UI element (CS-001–CS-004) is present

### Tests for User Story 5

- [X] T054 [P] [US5] Write unit/component tests for `CIMDConsentSummary` in `web/src/components/consent/CIMDConsentSummary.test.tsx`
- [X] T055 [P] [US5] Write unit/component tests for `CIMDDomainBadge` in `web/src/components/consent/CIMDDomainBadge.test.tsx`
- [X] T056 [P] [US5] Write unit/component tests for `CIMDLocalhostWarning` in `web/src/components/consent/CIMDLocalhostWarning.test.tsx`
- [X] T057 [P] [US5] Write unit/component tests for `CIMDAdvancedDetails` in `web/src/components/consent/CIMDAdvancedDetails.test.tsx`

### Implementation for User Story 5

- [X] T058 [US5] Modify consent detail handler to build `cimd_metadata` response object from decrypted `session_token` when present (calls Phase 3.5 T116 decode endpoint); for non-session flows, existing behavior unchanged
- [X] T059 [P] [US5] Add CIMD-related TypeScript types to `web/src/types/consent.ts`
- [X] T060 [P] [US5] Implement `CIMDConsentSummary` component (CS-001) in `web/src/components/consent/CIMDConsentSummary.tsx`
- [X] T061 [P] [US5] Implement `CIMDDomainBadge` component (CS-002) in `web/src/components/consent/CIMDDomainBadge.tsx`
- [X] T062 [P] [US5] Implement `CIMDLocalhostWarning` component (CS-003) in `web/src/components/consent/CIMDLocalhostWarning.tsx`
- [X] T063 [P] [US5] Implement `CIMDAdvancedDetails` component (CS-004) in `web/src/components/consent/CIMDAdvancedDetails.tsx`
- [X] T064 [US5] Integrate CIMD consent components into `web/src/pages/AgentGrantDetailPage.tsx` (conditional rendering when `cimd_metadata` present)
- [X] T064a [US5] Update `web/src/pages/AgentGrantDetailPage.tsx`: when URL contains `session_token` param, call `GET /api/consent/session?token=<jwe>` to retrieve CIMD display data instead of reconstructing params from query string
- [X] T064b [P] [US5] Update `web/src/hooks/useAgentGrants.ts` and `web/src/services/api/consent.ts`: add `session_token` parameter; when present, sends token to decode endpoint instead of CIMD params
- [X] T064c [US5] Update consent submission in frontend: when `session_token` present, include it in request body instead of `redirect_uri`
- [X] T065 [US5] Verify US5 E2E tests in `tests/e2e/cimd_consent_test.go` turn green
- [X] T066 [US5] Verify Playwright tests in `tests/e2e/frontend/cimd_consent_test.go` pass with screenshots captured

**Checkpoint**: Consent screen renders all four CIMD UX elements (CS-001–CS-004)

---

## Phase 6: User Story 3 — CIMD Response Caching Reduces Latency (Priority: P2)

**Goal**: Repeated authorization requests for the same URL-based client_id use cached documents; cache respects HTTP semantics and operator TTL bounds

**Independent Test**: Issue two sequential authorization requests for the same URL-based client_id; verify mock server receives only one request

### Tests for User Story 3

- [X] T067 [P] [US3] Write unit tests for cache TTL computation (HTTP header parsing, min/max clamping, no-store/no-cache ignored) in `internal/domain/cimd/cache_test.go`

### Implementation for User Story 3

- [X] T068 [US3] Implement HTTP cache header parsing (Cache-Control max-age, Expires, ETag) and TTL computation with operator min/max clamping in `internal/domain/cimd/cache.go`
- [X] T069 [US3] Integrate cache TTL computation into `CIMDService` fetch flow in `internal/domain/cimd/service.go`
- [X] T070 [US3] Verify US3 E2E tests in `tests/e2e/cimd_caching_test.go` turn green

**Checkpoint**: Caching functional — second request within TTL serves from cache

---

## Phase 7: User Story 4 — Authorization Server Advertises CIMD Support (Priority: P3)

**Goal**: The `/.well-known/oauth-authorization-server` metadata endpoint includes `client_id_metadata_document_supported: true` when CIMD is enabled

**Independent Test**: Fetch metadata endpoint and assert field presence when enabled, absence when disabled

### Implementation for User Story 4

- [X] T071 [US4] Modify OAuth2 metadata handler in `internal/adapters/http/handlers/enduser/oauth2_metadata_handler.go` to include `client_id_metadata_document_supported` field based on `cimd.enabled` config
- [X] T072 [US4] Verify US4 E2E tests in `tests/e2e/cimd_metadata_test.go` turn green

**Checkpoint**: Metadata advertisement functional

---

## 🔒 Phase N: Constitution Compliance & Polish

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification

#### Design Phase Verification

- [X] T073 Verify domain model design documented in ARCHITECTURE.md Glossary (Principle V)
- [X] T074 Verify configuration design YAML examples exist in `examples/config/cimd.yaml` (Principle VII)
- [X] T075 [P] Verify `examples/config/README.md` references CIMD configuration (Principle VII)
- [X] T076 Verify API designs documented in `/api/admin/openapi.yaml` and `/api/enduser/openapi.yaml` (Principles IV, X)
- [X] T077 Verify user/stakeholder confirmed API designs (Principle X)
- [X] T078 Verify database migration 015 documented and tested (Principle IX)
- [X] T079 Verify design system review completed for CIMD consent components (Principle XI)
- [X] T080 Verify E2E acceptance tests in `tests/e2e/` cover all 24 spec scenarios (22 original + 2 session edge cases) (Principle XIII)
- [X] T081 Verify E2E tests were verified to FAIL before implementation (red phase) (Principle XIII)
- [X] T082 Verify Playwright E2E tests in `tests/e2e/frontend/` pass with screenshots (Principle XIII)

#### Implementation Phase Verification

**API & Documentation** (Principles IV, X):
- [X] T083 [P] Verify API implementation matches confirmed OpenAPI specification exactly
- [X] T084 [P] Update `docs/api/` with CIMD-specific API documentation

**Architecture & Documentation** (Principle II):
- [X] T085 Update ARCHITECTURE.md with CIMD architectural changes (new port, new domain package, flow description)
- [X] T086 [P] Create ADR 015 in `adrs/` for CIMD fetcher architecture (SSRF-hardened HTTP client, in-process caching, hexagonal port)

**Configuration** (Principle VII):
- [X] T087 [P] Verify configuration uses unified config port (no custom loading)
- [X] T088 Verify Helm chart updated with CIMD config block

**Database & Persistence** (Principle IX):
- [X] T089 [P] Verify migration 015 follows sequential numbering
- [X] T090 [P] Write integration tests for migration 015 apply/rollback in `tests/integration/migrations/migrations_test.go`
- [X] T091 [P] Verify postgres adapter tested with new fields and `GetByClientURI`
- [X] T091a [P] Verify shared `jwe.TokenService` is tested with encrypt/decrypt round-trips and expiry rejection in `internal/domain/jwe/token_service_test.go`

**Security** (Principles I, III):
- [X] T092 Verify SSRF protection enabled by default and cannot be fully disabled
- [X] T093 [P] Verify no custom cryptography used (Go stdlib only)
- [X] T094 [P] Verify structured audit logging for all security-critical operations (SSRF blocks, brand mismatch, security field changes)
- [X] T094a Verify CIMD consent flows use JWE `session_token` for all trust metadata (SR-013/SR-014); no CIMD query parameter relay in consent URL; `exp` and `principal` validated on every submission

**Architecture Patterns** (Principle VI):
- [X] T095 Verify domain logic depends on ports only (no adapter imports in `domain/cimd/`)

**Testing** (Principle VIII):
- [X] T096 Verify unit tests written first and failed before implementation (red-green TDD)
- [X] T097 Verify automated tests included (unit, integration, E2E)

**E2E Acceptance Testing** (Principle XIII):
- [X] T098 Verify each It() block maps to exactly one acceptance scenario from spec.md
- [X] T099 Verify E2E tests turned GREEN as implementation satisfied acceptance criteria
- [X] T100 Run full E2E test suite: `ginkgo -v ./tests/e2e/` (all tests must pass)
- [X] T101 Run frontend E2E suite: `ginkgo -v ./tests/e2e/frontend/` (all tests must pass)

**Frontend** (Principle XI):
- [X] T102 Verify CIMD consent components use design system primitives and semantic tokens
- [X] T103 [P] Verify WCAG 2.1 AA accessibility compliance for CIMD consent components

### Additional Polish

- [X] T104 Run `just check` (fmt → vet → lint) and `just verify` — all must pass
- [X] T105 Verify zero regression in existing authorization flows (SC-004) — full existing E2E suite green

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Design Preconditions (Phase 2)**: Can proceed in parallel with Phase 1; BLOCKS all implementation
  - Phase 2a–2f can proceed in parallel, but all must complete before Phase 2.7
- **Entity Boilerplate (Phase 2.7)**: Depends on Phase 2 completion; BLOCKS Phase 2.5
- **Foundational Infrastructure (Phase 2.5)**: Depends on Phase 2 + Phase 2.7; BLOCKS all user stories
- **User Stories (Phase 3–7)**: All depend on Phase 2.5 completion
  - US1 (Phase 3): No dependencies on other stories
  - **Authorization Session (Phase 3.5)**: Depends on US1 (Phase 3) — needs ClientResolution with CIMD metadata flowing through HandleAuthorization
  - US2 (Phase 4): No dependencies on other stories (SSRF infra built in Phase 2.5)
  - US5 (Phase 5): Depends on US1 AND Phase 3.5 (needs session-based CIMD metadata flowing to consent API)
  - US3 (Phase 6): No dependencies on other stories (cache infra built in Phase 2.5)
  - US4 (Phase 7): No dependencies on other stories
- **Polish (Phase N)**: Depends on all user stories complete

### User Story Dependencies

- **US1 (P1)**: Independent — can start after Phase 2.5
- **US2 (P1)**: Independent — can start after Phase 2.5 (parallel with US1)
- **US5 (P1)**: Depends on US1 AND Phase 3.5 (needs session-based CIMD metadata in consent)
- **US3 (P2)**: Independent — can start after Phase 2.5 (parallel with US1/US2)
- **US4 (P3)**: Independent — can start after Phase 2.5 (parallel with all)

### Parallel Opportunities

- Phase 1 setup tasks T001–T004 (different files)
- Phase 2a–2f design tasks (different documents)
- Phase 2.7 memory/postgres adapters T026/T027 (different files)
- Phase 2.5 domain types T031/T032/T033 (different files) and their tests T036/T037/T038/T039
- US1/US2/US3/US4 can proceed in parallel after Phase 2.5 (US5 waits for US1)
- Frontend components T060–T063 (different files)

---

## Parallel Example: Phase 2.5 Foundation

```bash
# Launch all domain types in parallel (different files):
Task: "Implement ClientIDMetadataDocumentURL in internal/domain/cimd/url.go"
Task: "Implement SSRFBlocklist in internal/domain/cimd/blocklist.go"
Task: "Implement ClientIDMetadataDocument in internal/domain/cimd/document.go"

# Launch all unit tests in parallel (different test files):
Task: "Write unit tests in internal/domain/cimd/url_test.go"
Task: "Write unit tests in internal/domain/cimd/blocklist_test.go"
Task: "Write unit tests in internal/domain/cimd/document_test.go"
Task: "Write unit tests in internal/domain/cimd/cache_test.go"
```

## Parallel Example: User Story 5 Components

```bash
# Launch all frontend components in parallel (different files):
Task: "Implement CIMDConsentSummary in web/src/components/consent/CIMDConsentSummary.tsx"
Task: "Implement CIMDDomainBadge in web/src/components/consent/CIMDDomainBadge.tsx"
Task: "Implement CIMDLocalhostWarning in web/src/components/consent/CIMDLocalhostWarning.tsx"
Task: "Implement CIMDAdvancedDetails in web/src/components/consent/CIMDAdvancedDetails.tsx"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (config scaffolding)
2. Complete Phase 2: Design Preconditions (all 6 sub-phases in parallel)
3. Complete Phase 2.7: Agent entity extension (boilerplate)
4. Complete Phase 2.5: Foundational Infrastructure (domain types, port, adapter)
5. Complete Phase 3: User Story 1 (core CIMD authorization flow)
6. **STOP and VALIDATE**: Test US1 independently — URL-based client_id resolves and presents metadata
7. Deploy/demo if ready

### Incremental Delivery

1. Setup → Design → Entity Extension → Foundation ready
2. Add US1 (core flow) → Test → Deploy/Demo (MVP!)
3. Add Phase 3.5 (authorization session) → US5 (consent screen) secured by session
4. Add US2 (SSRF hardening) → Test → Deploy/Demo
5. Add US3 (caching) → Test → Deploy/Demo
6. Add US4 (metadata advertisement) → Test → Deploy/Demo
7. Each story adds value without breaking previous stories
