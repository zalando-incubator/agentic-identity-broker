# Implementation Plan: Broker-Hosted Aggregated JWKS

**Branch**: `20260604-120000-aggregated-jwks` | **Date**: 2026-06-04 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/20260604-120000-aggregated-jwks/spec.md`

## Summary

Transform the broker's `/oauth2/jwks.json` endpoint from serving local signing keys only (in local/hybrid modes) into a mode-dependent aggregated verification surface served in all three modes. In `local` mode: local keys only. In `proxy` mode: upstream keys republished. In `hybrid` mode: union of local + upstream keys with duplicate `kid` detection. The discovery endpoint advertises `jwks_uri` in all modes. Fail-closed semantics ensure incomplete verification surfaces are never served.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: chi v5 (HTTP router), lestrrat-go/jwx/v3 (JWK caching + key sets), Viper/Cobra (config)
**Storage**: N/A — operates on in-memory key sets (local signing keys from repo, upstream from cache)
**Testing**: Ginkgo/Gomega (E2E), Go testing + testify (unit/integration)
**Target Platform**: Linux server (Kubernetes deployment)
**Project Type**: Go monorepo (hexagonal architecture)
**Performance Goals**: JWKS endpoint responds within 10ms p99 (serving cached material)
**Computation Model**: Aggregated key set is pre-computed eagerly on each upstream cache refresh event (not per-request). JWKS requests serve the current immutable snapshot via atomic read. Conflict detection and error state transitions occur at refresh time.
**Constraints**: Must not increase startup latency beyond upstream JWKS fetch time; Cache-Control: public, max-age=300
**Scale/Scope**: Single new domain service, modified handler, modified metadata generation; no DB changes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Value objects identified: `AggregatedKeySet` (immutable snapshot), `KeySource` (abstraction). No new persisted entities.
- [x] **Domain Concepts**: `AggregatedKeySet`, `KeySource`, `JWKSPublisher` will be added to ARCHITECTURE.md Glossary.
- [x] **Entity IDs**: No new UUID-keyed entities. No new typed IDs needed.
- [x] **Configuration Design**: No new configuration parameters. Behavior derives from existing `mode` + `upstream_issuer_uri`.
- [x] **Config Examples**: No new config parameters → no example changes needed.
- [x] **Helm Chart**: No config parameters added/changed/removed → no Helm chart change needed.
- [x] **API Design First**: OpenAPI spec for `/oauth2/jwks.json` will be updated (semantics change) and `/.well-known/oauth-authorization-server` will add `jwks_uri` in proxy mode. Confirmed via spec acceptance.
- [x] **API Documentation**: Updates to `/api/enduser/openapi.yaml` for both endpoints.
- [x] **API Changes**: Spec explicitly supersedes prior specs (030 FR-012, 025 US6 Scenario 3). Confirmed by stakeholder in spec creation.
- [x] **Database Design**: No database changes required.
- [x] **E2E Acceptance Tests**: 21 acceptance scenarios across 4 user stories → 21 `It()` blocks in `tests/e2e/aggregated_jwks_test.go`.
- [x] **E2E Test Mapping**: Each scenario from spec.md maps 1:1 to one `It()` block.
- [x] **E2E Red Phase**: Tests will contain detailed realistic assertions (HTTP status, JSON keys, `kid` values, Cache-Control headers).
- [x] **Frontend Playwright E2E**: No UI changes → not applicable.
- [x] **Frontend Screenshots**: No UI changes → not applicable.

**Implementation Considerations**:

- [x] **Security-First**: Fail-closed on upstream unavailability (503). Duplicate `kid` = error. Never expose private keys. HTTPS-only upstream.
- [x] **Architecture Docs**: ARCHITECTURE.md glossary updated with new domain concepts.
- [x] **ADRs**: No new ADR required — follows existing patterns (ADR 20260119-120000 JWKS adapter, hexagonal architecture).
- [x] **Library-First Security**: Uses `lestrrat-go/jwx/v3` for JWK operations. No custom crypto.
- [x] **Zalando Guidelines**: Existing endpoint paths unchanged. Response format follows RFC 7517.
- [x] **End-User Docs**: `docs/api/` updated with aggregated JWKS semantics.
- [x] **Migration Testing**: No migrations → not applicable.
- [x] **Hexagonal Architecture**: New `JWKSPublisher` port interface → domain service → adapter composition in builder.
- [x] **Persistence Patterns**: No persistence → not applicable.

*All BLOCKING preconditions satisfied. Proceeding to Phase 0.*

## Project Structure

### Documentation (this feature)

```text
specs/20260604-120000-aggregated-jwks/
├── plan.md              # This file
├── research.md          # Phase 0 output — design decisions & alternatives
├── data-model.md        # Phase 1 output — domain model & value objects
├── quickstart.md        # Phase 1 output — implementation guide
├── contracts/           # Phase 1 output — OpenAPI updates
│   └── jwks-endpoint.yaml
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
internal/
├── domain/
│   └── jwkspublisher/           # NEW: Domain service for aggregated JWKS
│       ├── service.go           # JWKSPublisherService — aggregation logic, kid conflict detection
│       └── service_test.go      # TDD unit tests
├── ports/
│   ├── jwks_publisher.go        # NEW: JWKSPublisherPort interface
│   └── oauth2.go                # MODIFIED: MetadataResponse.JWKSURI in all modes
├── adapters/
│   ├── jwks/
│   │   └── adapter.go           # EXISTING: Reused for upstream key fetching
│   └── http/
│       ├── handlers/enduser/
│       │   └── jwks_handler.go  # MODIFIED: Delegates to JWKSPublisherPort
│       └── routing/
│           └── enduser.go       # MODIFIED: JWKS route registered unconditionally
├── app/
│   └── builder.go               # MODIFIED: Wire JWKSPublisher per mode

tests/e2e/
└── aggregated_jwks_test.go      # NEW: 21 E2E scenarios
```

**Structure Decision**: This feature adds a single domain service package (`jwkspublisher/`) that composes existing ports (`JWKSPort` for upstream, `SigningKeyManager` for local). The handler changes are minimal — it delegates to the new publisher port. No new adapters needed; the existing JWKS adapter is reused for upstream key fetching.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | N/A — no pre-implementation refactoring needed | Skip |
| **Phase 1** | N/A — no new dependencies or project init needed | Skip |
| **Phase 2** | Design Preconditions: domain model, API contract, E2E tests (red phase) | **MANDATORY** |
| **Phase 2.5** | New port interface + domain service skeleton | Yes |
| **Phase 3** | User Story 1 + 2: Aggregated JWKS in all modes + discovery | **MANDATORY** |
| **Phase 4** | User Story 3: Fail-closed upstream availability | **MANDATORY** |
| **Phase 5** | User Story 4: Duplicate kid detection | **MANDATORY** |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **Skip** — existing handler structure supports modification in-place.
- [x] Phase 2.7 (entity boilerplate): **Skip** — no new persisted entities or CRUD handlers.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/aggregated_jwks_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: "Aggregated JWKS Endpoint"
- **Nested Describe/Context**: Per user story and mode configuration
- **It blocks**: One per acceptance scenario (21 total)

**Scenario Mapping**:

| Spec Scenario | Test Description |
|---------------|------------------|
| US1 Scenario 1 | `It("returns only local keys in local mode")` |
| US1 Scenario 2 | `It("returns only upstream keys in proxy mode")` |
| US1 Scenario 3 | `It("returns both local and upstream keys in hybrid mode")` |
| US1 Scenario 4 | `It("validates locally-minted token using broker JWKS in hybrid mode")` |
| US1 Scenario 5 | `It("validates upstream-issued token using broker JWKS in hybrid mode")` |
| US1 Scenario 6 | `It("validates upstream-issued token using broker JWKS in proxy mode")` |
| US1 Scenario 7 | `It("never exposes private key material")` |
| US1 Scenario 8 | `It("includes Cache-Control: public, max-age=300")` |
| US2 Scenario 1 | `It("discovery includes jwks_uri in local mode")` |
| US2 Scenario 2 | `It("discovery includes jwks_uri in proxy mode pointing to broker endpoint")` |
| US2 Scenario 3 | `It("discovery includes jwks_uri in hybrid mode")` |
| US2 Scenario 4 | `It("jwks_uri from proxy discovery resolves to upstream keys")` |
| US3 Scenario 1 | `It("fails startup when upstream unreachable in proxy mode")` |
| US3 Scenario 2 | `It("fails startup when upstream unreachable in hybrid mode")` |
| US3 Scenario 3 | `It("returns 503 when cached upstream expired in proxy mode")` |
| US3 Scenario 4 | `It("returns 503 when cached upstream expired in hybrid mode")` |
| US3 Scenario 5 | `It("starts without upstream in local mode")` |
| US4 Scenario 1 | `It("fails closed on kid conflict in hybrid mode")` |
| US4 Scenario 2 | `It("publishes all keys without modification when no kid conflict")` |
| US4 Scenario 3 | `It("fails startup on initial kid conflict")` |
| US4 Scenario 4 | `It("returns 503 and logs conflict when runtime refresh introduces kid conflict")` |

**Red Phase Requirements**:
- Assertions target HTTP status codes, JSON key counts, specific `kid` values, `Cache-Control` headers
- Token validation assertions use actual JWT signing/verification
- No placeholder always-fail assertions

**Test Data Strategy**:
- Use `fixtures.LocalConfig()`, `fixtures.OAuth2ConfigWithUpstream()` for mode configs
- Use `helpers.ProvisionSigningKey()` for local key setup
- New fixture: `helpers.NewMockUpstreamJWKS()` — mock HTTP server serving a deterministic JWKS
- New helper: `helpers.MintTestJWT()` — creates a JWT signed with a test key

**Bootstrap Strategy**:
- Tests use production bootstrap via `tests/e2e/bootstrap/`
- Mock upstream JWKS via `httptest.NewServer` per test
- Fresh server per test (BeforeEach/AfterEach isolation)
- Proxy/hybrid mode tests pass mock upstream URL via config

### Frontend Playwright E2E Tests

Not applicable — this feature has no UI changes.

### Unit & Integration Tests

**Unit Tests**:
- Location: `internal/domain/jwkspublisher/service_test.go`
- Coverage: Aggregation logic, kid conflict detection, mode-specific behavior, fail-closed on unavailable upstream
- Strategy: TDD with hand-rolled mocks for `JWKSPort` and `SigningKeyManager`

**Integration Tests**:
- Location: `tests/integration/aggregated_jwks_test.go`
- Coverage: Full server startup with mock upstream, JWKS endpoint end-to-end via httptest

**Test Coverage Goals**:
- Unit test coverage: All aggregation code paths (local-only, proxy-only, hybrid, conflicts, errors)
- E2E test coverage: 100% of 21 acceptance scenarios from spec.md (mandatory per Principle XIII)

## Complexity Tracking

No constitution violations. No complexity justification needed.
