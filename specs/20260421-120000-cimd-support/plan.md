# Implementation Plan: Client ID Metadata Document (CIMD) Support

**Branch**: `20260421-120000-cimd-support` | **Date**: 2026-04-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20260421-120000-cimd-support/spec.md`

## Summary

Add support for the [draft-ietf-oauth-client-id-metadata-document-01](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-01) specification, enabling AI agents to identify themselves using HTTPS URLs as `client_id` values. The broker fetches, validates, and caches the metadata document at the URL, enforces SSRF protection, and presents CIMD-sourced metadata (client name, domain verification, redirect warnings) on the consent screen. This extends the existing Agent entity, OAuth2 authorization flow, consent UI, and admin API without introducing new dependencies.

**Mode Constraint**: CIMD support requires `issue_token` mode (`oauth2_authorization_server.mode: issue_token`). It is incompatible with `proxy` mode because CIMD requires the broker to control authorization code issuance, JWE session token minting, and consent flow orchestration — all of which are delegated to the upstream server in proxy mode. Startup validation rejects `cimd.enabled: true` when `mode: proxy`. A future hybrid token issuing mode is expected to support CIMD.

## Technical Context

**Language/Version**: Go 1.25.6 (backend), React 19 + TypeScript + Vite 7 (frontend)
**Primary Dependencies**: chi v5 (router), sqlx (database), Ginkgo/Gomega (E2E), Viper/Cobra (config), Tailwind CSS v4 + CVA (UI) — no new dependencies required
**Storage**: PostgreSQL (production) + in-memory (dev/test) — Agent entity extension requires migration 015
**Testing**: `go test` + Ginkgo/Gomega (E2E) + Playwright (frontend E2E) + testcontainers (integration)
**Target Platform**: Linux server (Docker/Kubernetes)
**Project Type**: Web application (Go backend + React SPA)
**Performance Goals**: CIMD-based authorization flow < 2s total when CIMD host responds < 500ms (SC-001)
**Constraints**: CIMD fetch timeout 1s, max response 5120 bytes, in-process cache only, requires `issue_token` mode (incompatible with `proxy` mode)
**Scale/Scope**: Extension to existing OAuth2 authorization flow; ~7 new domain types, 1 migration, 1 new hexagonal port, 1 new shared JWE package, 4 consent UI components

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: ClientIDMetadataDocument (value object), CIMDCacheEntry (in-process), ClientIDMetadataDocumentURL (value object), SSRFBlocklist (value object) — spec provides full entity diagram
- [x] **Domain Concepts**: ClientIDMetadataDocument, CIMDCacheEntry, ClientIDMetadataDocumentURL, SSRFBlocklist, BrandPinMismatchDetected — to be added to ARCHITECTURE.md glossary
- [x] **Entity IDs**: No new UUID-based entity IDs required — CIMD uses URL strings as identifiers, not UUIDs. Agent entity (existing ID) is extended with new fields
- [x] **Configuration Design**: `CIMDConfig` struct under `OAuth2AuthServerConfig` with enabled, fetch_timeout, max_response_bytes, cache (max_ttl, min_ttl), ssrf (extra_blocked_cidrs), client_name_blocklist — YAML examples in spec
- [x] **Config Examples**: Will add `examples/config/cimd.yaml` with annotated configuration
- [x] **Helm Chart**: Will update `charts/agentic-identity-broker/values.yaml` with `cimd` block under `oauth2AuthorizationServer`
- [x] **API Design First**: Admin API: existing `POST /api/agents` and `PUT /api/agents/{agent-id}` extended with `client_uris` field; Enduser API: `/.well-known/oauth-authorization-server` extended with `client_id_metadata_document_supported`; `/oauth2/authorize` transparently accepts URL-based client_id
- [x] **API Documentation**: Will update `/api/admin/openapi.yaml` (Agent schema with `client_uris`) and `/api/enduser/openapi.yaml` (metadata field)
- [x] **API Changes**: API-001 through API-004 defined in spec — requires user confirmation before implementation
- [x] **Database Design**: Migration 015 (`015_add_cimd_support`): create normalized `agent_client_uris` child table with `UNIQUE(client_uri)` constraint — no `authorization_sessions` table required; session state is carried in the stateless JWE `session_token` (SR-013/SR-014)
- [x] **E2E Acceptance Tests**: 20+ acceptance scenarios across 5 user stories — all mapped to E2E tests
- [x] **E2E Test Mapping**: 1:1 mapping from spec scenarios to It() blocks
- [x] **E2E Red Phase**: Detailed expectations targeting HTTP status codes, response bodies, consent screen elements
- [x] **Frontend Playwright E2E**: CS-001 through CS-004 consent screen requirements need Playwright tests
- [x] **Frontend Screenshots**: Screenshots for: CIMD consent with domain badge, localhost redirect warning, advanced details expanded, brand mismatch display

**Implementation Considerations**:

- [x] **Security-First**: CIMD disabled by default (`cimd.enabled: false`); SSRF protection always on, cannot be disabled; TLS enforced; fail-closed on all validation; CIMD requires `issue_token` mode — startup rejects `cimd.enabled: true` in `proxy` mode
- [x] **Architecture Docs**: ARCHITECTURE.md updated with CIMD domain concepts, new port, and flow description
- [x] **ADRs**: ADR 20260425-120000 for CIMD fetcher architecture (SSRF-hardened HTTP client, in-process caching, hexagonal port design)
- [x] **Library-First Security**: Uses Go `net`, `net/http`, `crypto/tls` stdlib — no custom crypto. SSRF protection via DNS resolution + IP range validation using `net.IP` stdlib
- [x] **Zalando Guidelines**: PUT for full replacement (existing pattern), structured error responses with `error`/`error_description`
- [x] **End-User Docs**: Will add CIMD-specific API documentation in `docs/api/`
- [x] **Migration Testing**: Migration 015 tested with apply/rollback in PostgreSQL integration tests
- [x] **Hexagonal Architecture**: New `CIMDPort` interface in `internal/ports/` with fetcher adapter; domain logic depends on port only
- [x] **Persistence Patterns**: Agent entity extension follows existing patterns in `internal/adapters/storage/`

## Project Structure

### Documentation (this feature)

```text
specs/20260421-120000-cimd-support/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (OpenAPI diffs)
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── domain/
│   ├── cimd/                    # NEW: CIMD domain types and validation
│   │   ├── document.go          # ClientIDMetadataDocument value object
│   │   ├── url.go               # ClientIDMetadataDocumentURL value object + validation
│   │   ├── blocklist.go         # SSRFBlocklist value object (RFC 6890 ranges)
│   │   ├── cache.go             # CIMDCacheEntry + in-process cache logic
│   │   ├── service.go           # CIMDService orchestrating fetch → validate → cache → audit
│   │   └── client_resolver.go   # CIMDClientResolver: ClientResolver strategy for CIMD-enabled mode
│   ├── jwe/
│   │   └── token_service.go     # NEW: shared JWE encrypt/decrypt (A256GCMKW + A256GCM); used by OAuth2SessionService and OAuth2AuthorizationService
│   ├── oauth2/
│   │   ├── service.go           # MODIFIED: delegates client resolution to injected ClientResolver strategy; mints JWE session_token for CIMD flows before consent redirect
│   │   ├── client_resolver.go   # NEW: OpaqueClientResolver (existing UUID lookup, rejects URL-format client_id)
│   │   └── authorization_session_token.go  # NEW: AuthorizationSessionClaims struct + mint/validate helpers
│   └── storage/
│       └── agent.go             # MODIFIED: add ClientURIs field
├── ports/
│   ├── cimd.go                  # NEW: CIMDFetcher port interface + ClientResolver strategy interface + ClientResolution DTO
│   ├── config.go                # MODIFIED: add CIMDConfig to OAuth2AuthServerConfig
│   └── storage.go               # MODIFIED: add GetByClientURI to AgentRepository
├── adapters/
│   ├── cimd/                    # NEW: SSRF-hardened HTTP fetcher adapter
│   │   └── fetcher.go           # Implements CIMDFetcher port
│   ├── http/
│   │   ├── handlers/
│   │   │   ├── admin/
│   │   │   │   └── agents_handler.go  # MODIFIED: extend AgentRequest/Response with client_uris
│   │   │   └── enduser/
│   │   │       ├── oauth2_metadata_handler.go  # MODIFIED: add client_id_metadata_document_supported
│   │   │       └── consent/
│   │   │           └── agent_detail_handler.go  # MODIFIED: add decode endpoint GET /api/consent/session; accept session_token in submission, decrypt JWE, build cimd_metadata from trusted claims (FR-028/FR-029)
│   │   └── routing/
│   │       └── admin.go         # UNCHANGED (no new endpoints)
│   └── storage/
│       ├── memory/              # MODIFIED: Agent adapter with new fields + GetByClientURI
│       └── postgres/            # MODIFIED: Agent adapter with new fields + GetByClientURI
└── app/
    └── builder.go               # MODIFIED: wire CIMDService + CIMDFetcher + shared *jwe.TokenService injected into OAuth2SessionService and OAuth2AuthorizationService

web/src/
├── components/
│   └── consent/
│       ├── CIMDDomainBadge.tsx       # NEW: verified domain badge (CS-002)
│       ├── CIMDLocalhostWarning.tsx   # NEW: localhost redirect warning (CS-003)
│       ├── CIMDAdvancedDetails.tsx    # NEW: collapsible advanced details (CS-004)
│       └── CIMDConsentSummary.tsx     # NEW: summary statement (CS-001)
├── pages/
│   └── ConsentOverviewPage.tsx       # MODIFIED: integrate CIMD consent components
│   └── AgentGrantDetailPage.tsx      # MODIFIED: read session_token from URL, call decode endpoint, pass token in consent submission body
└── types/
    └── consent.ts                    # MODIFIED: add CIMD-related API types

migrations/
└── 015_add_cimd_support.{up,down}.sql  # NEW: agent_client_uris child table (UNIQUE(client_uri))

api/
├── admin/openapi.yaml           # MODIFIED: Agent schema extension (client_uris)
└── enduser/openapi.yaml         # MODIFIED: metadata endpoint field

charts/agentic-identity-broker/
└── values.yaml                  # MODIFIED: cimd config block

examples/config/
└── cimd.yaml                    # NEW: CIMD configuration example

tests/
├── e2e/
│   ├── cimd_authorization_test.go   # NEW: User Story 1 scenarios
│   ├── cimd_ssrf_test.go            # NEW: User Story 2 scenarios
│   ├── cimd_caching_test.go         # NEW: User Story 3 scenarios
│   ├── cimd_metadata_test.go        # NEW: User Story 4 scenarios
│   └── fixtures/
│       └── cimd.go                  # NEW: CIMD-specific test fixtures
└── e2e/frontend/
    └── cimd_consent_test.go         # NEW: Playwright tests for CS-001–CS-004
```

**Structure Decision**: Extends the existing hexagonal architecture following established package conventions:
- **`domain/cimd/`**: Separate bounded context package — consistent with how `oauth2session/` is distinct from `oauth2/`. CIMD document parsing, URL validation, SSRF blocklist, and caching are a cohesive sub-concern that would roughly double `domain/oauth2/`'s surface area if merged. The `domain/oauth2/` service orchestrates the authorization flow and delegates to `domain/cimd/` for CIMD-specific logic.
- **`ports/cimd.go`**: Dedicated port file — follows the one-file-per-concern convention (`ports/jwks.go`, `ports/cel.go`). The CIMDFetcher interface is analogous to JWKSPort: an external HTTP fetch with caching behind a hexagonal boundary.
- **`adapters/cimd/`**: SSRF-hardened HTTP fetcher — mirrors the `adapters/jwks/` pattern (HTTP client from `upstream/`, caching, port implementation). Reuses the existing `upstream.NewSecureUpstreamClient()` factory for TLS-hardened HTTP transport.
- **`domain/oauth2/authorization_session_token.go`**: `AuthorizationSessionClaims` struct and mint/validate helpers live in `domain/oauth2/` alongside the service that creates and validates the token. The claims are sealed into a JWE using the shared `internal/domain/jwe/TokenService`, eliminating the need for a database-backed repository. This secures CIMD consent flows by binding authorization context client-side in an encrypted, tamper-proof token (SR-013/SR-014).

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring — isolate structural changes from feature work | Optional |
| **Phase 1** | Setup — project init, dependencies | Customizable |
| **Phase 2** | Design Preconditions (domain model, config, API, DB, E2E tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate — empty CRUD handlers & repositories, isolated from business logic | If new entities |
| **Phase 2.5** | Foundational Infrastructure — SSRF blocklist, URL validation, fetcher port | Customizable |
| **Phase 3** | User Story 1 — CIMD fetch, validate, authorize (P1) | Customizable |
| **Phase 3.5** | Authorization Session — server-side consent context binding (SR-013/SR-014) | Customizable |
| **Phase 4** | User Story 2 — SSRF hardening (P1) | Customizable |
| **Phase 5** | User Story 5 — Consent screen enhancements (P1) | Customizable |
| **Phase 6** | User Story 3 — Response caching (P2) | Customizable |
| **Phase 7** | User Story 4 — Metadata advertisement (P3) | Customizable |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **Skip** — existing Agent entity extension is additive; no structural refactoring needed before feature work
- [x] Phase 2.7 (entity boilerplate): **Include** — Agent entity gains new fields, new `GetByClientURI` repository method, and new CIMDFetcher port. Scaffolding these as empty stubs in a separate PR enables focused review

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/cimd_*_test.go` (split by user story for manageability)

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: "CIMD Authorization Flow" / "CIMD SSRF Protection" / "CIMD Caching" / "CIMD Metadata"
- **Nested Context**: Preconditions (valid CIMD document, blocked IP, cache hit, etc.)
- **It blocks**: Individual acceptance scenarios (one per spec scenario)

**Scenario Mapping**:

| Spec Scenario | E2E Test File | Test Description |
|---------------|---------------|------------------|
| US1 Scenario 1 | `cimd_authorization_test.go` | `It("resolves agent and presents client_name from CIMD document on consent screen")` |
| US1 Scenario 2 | `cimd_authorization_test.go` | `It("rejects when CIMD client_id field does not match request URL")` |
| US1 Scenario 3 | `cimd_authorization_test.go` | `It("rejects when CIMD document is absent or returns non-200")` |
| US1 Scenario 4 | `cimd_authorization_test.go` | `It("rejects when redirect_uri is not in CIMD document")` |
| Gate: disabled | `cimd_authorization_test.go` | `It("rejects URL-format client_id with invalid_client when CIMD is disabled")` |
| Gate: proxy mode | `cimd_authorization_test.go` | `It("refuses to start when cimd.enabled is true and mode is proxy")` |
| US2 Scenario 1 | `cimd_ssrf_test.go` | `It("blocks private IP ranges with invalid_client error")` |
| US2 Scenario 2 | `cimd_ssrf_test.go` | `It("blocks loopback addresses with invalid_client error")` |
| US2 Scenario 3 | `cimd_ssrf_test.go` | `It("blocks link-local and cloud metadata addresses with invalid_client error")` |
| US2 Scenario 4 | `cimd_ssrf_test.go` | `It("rejects HTTP scheme immediately")` |
| US2 Scenario 5 | `cimd_ssrf_test.go` | `It("rejects URLs with dot segments")` |
| US2 Scenario 6 | `cimd_ssrf_test.go` | `It("aborts oversized responses")` |
| US2 Scenario 7 | `cimd_ssrf_test.go` | `It("terminates on timeout")` |
| US3 Scenario 1 | `cimd_caching_test.go` | `It("serves cached document within TTL without network call")` |
| US3 Scenario 2 | `cimd_caching_test.go` | `It("re-fetches expired document")` |
| US3 Scenario 3 | `cimd_caching_test.go` | `It("never caches error responses")` |
| US3 Scenario 4 | `cimd_caching_test.go` | `It("operator max TTL overrides document max-age")` |
| US4 Scenario 1 | `cimd_metadata_test.go` | `It("includes client_id_metadata_document_supported when enabled")` |
| US4 Scenario 2 | `cimd_metadata_test.go` | `It("omits field when CIMD disabled")` |
| US5 Scenario 1 | `cimd_consent_test.go` | `It("displays summary, domain badge, and advanced details for non-localhost redirect")` |
| US5 Scenario 2 | `cimd_consent_test.go` | `It("displays localhost redirect warning")` |
| US5 Scenario 3 | `cimd_consent_test.go` | `It("reveals advanced details on expand")` |
| US5 Scenario 4 | `cimd_consent_test.go` | `It("displays CIMD client_name on brand mismatch and logs audit event")` |
| Session: expired | `cimd_authorization_test.go` | `It("rejects consent load with expired session_token")` |
| Session: replayed | `cimd_authorization_test.go` | `It("rejects consent submission with expired session_token")` |

**Test Data Strategy**:
- Use fixtures from `tests/e2e/fixtures/` for stable, reusable test data
- Required fixtures: agents with `ClientURIs` populated, CIMD-enabled config, fake public hostname mapping, mock CIMD HTTPS server
- New fixture creation: `CIMDAgent()` (agent with pre-registered CIMD URLs), `ValidCIMDDocument()` (well-formed JSON), `CIMDConfig()` (enabled CIMD config), `CIMDHostMapping()` (maps `agent.example.test` style hosts to the in-process CIMD server)

**Bootstrap Strategy**:
- Tests use production bootstrap code via `tests/e2e/bootstrap/`
- Fresh server and storage for each test (BeforeEach/AfterEach isolation)
- Mock CIMD HTTPS server started per test suite to serve controlled JSON documents and simulate error conditions (non-200, timeout, oversized)
- E2E happy-path CIMD tests MUST use fake public hostnames (for example `agent.example.test`) in `client_id`, never localhost hostnames, so the exercised flow matches the production-facing URL model
- The CIMD fetcher adapter MUST expose an injectable resolver/dialer seam so test bootstrap can map fake public hostnames to the in-process HTTPS server while preserving the original request URL and port-443 validation semantics
- E2E tests verify observable behavior only: authorization rejection/success, consent rendering, and mock server request counts. They do NOT prove "no TCP connection attempted" for blocked hosts; that guarantee is verified in adapter tests through dial-attempt spies

**Helper Utilities**:
- Custom matchers needed: `HaveCIMDField(name, value)` for JSON document assertions
- HTTP helpers: existing `tests/e2e/helpers/` + new CIMD mock server helper
- Mock services: `httptest.NewTLSServer` serving configurable CIMD JSON responses with controllable delays, sizes, and status codes
- New helper utilities: fake hostname resolver/dialer mapping, dial-attempt spy for adapter tests, and URL builder that produces `https://agent.example.test/client` style client IDs

### Frontend Playwright E2E Tests

**Test Location**: `tests/e2e/frontend/cimd_consent_test.go`

**UI Scenario Mapping**:

| UI Scenario | Playwright Test Location | Screenshot Filename |
|-------------|--------------------------|---------------------|
| CS-001: Summary statement | `cimd_consent_test.go` | `cimd_consent_summary.png` |
| CS-002: Domain verification badge | `cimd_consent_test.go` | `cimd_consent_domain_badge.png` |
| CS-003: Localhost redirect warning | `cimd_consent_test.go` | `cimd_consent_localhost_warning.png` |
| CS-004: Advanced details expanded | `cimd_consent_test.go` | `cimd_consent_advanced_details.png` |

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/cimd/document_test.go` — document parsing, validation, field extraction
- `internal/domain/cimd/url_test.go` — URL validation (scheme, path, fragment, credentials, port, dot-segments)
- `internal/domain/cimd/blocklist_test.go` — SSRF blocklist IP range matching
- `internal/domain/cimd/cache_test.go` — cache TTL computation, eviction, HTTP header parsing
- `internal/domain/cimd/service_test.go` — orchestration logic, brand pin detection, security field change detection

**Integration Tests**:
- `internal/adapters/cimd/fetcher_test.go` — SSRF-hardened HTTP client with injected resolver/dialer, verifying blocked IP categories fail before any dial attempt and allowed fake public hostnames reach the test HTTPS server
- `internal/adapters/storage/postgres/agent_integration_test.go` — extended with GetByClientURI, new fields
- Migration 015 apply/rollback tests

**Test Coverage Goals**:
- Unit test coverage: all CIMD domain types and validation logic (critical paths)
- Integration test coverage: CIMD fetcher adapter, Agent PostgreSQL adapter (new fields + methods)
- E2E test coverage: 100% of acceptance scenarios from spec.md (20+ scenarios)
- Frontend E2E coverage: CS-001 through CS-004 consent screen requirements

**Test Layer Responsibilities**:
- **Unit tests**: URL parsing, blocklist classification, cache TTL computation, document validation
- **Adapter tests**: prove SSRF dial blocking (`no dial attempted`), fake hostname resolution, timeout/size-limit enforcement, redirect rejection
- **E2E tests**: prove end-user behavior with production bootstrap using fake public hostnames and in-process HTTPS CIMD server mapping

## Complexity Tracking

No constitution violations to justify — all preconditions are satisfiable within the existing architecture.
