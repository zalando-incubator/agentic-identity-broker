# Implementation Plan: Hybrid OAuth Server Modes

**Branch**: `20260511-120000-hybrid-oauth-modes` | **Date**: 2026-05-12 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20260511-120000-hybrid-oauth-modes/spec.md`

## Summary

Overhaul OAuth server mode management: rename `issue_token` → `local`, add `hybrid` mode allowing proxy and local clients to coexist, introduce universal agent resolution and property-based classification (`ProxyClient`, `CIMDClient`, `LocalClient`), and enforce mode strategies as accept/reject filters. No new Agent entity fields — classification derives from existing `Agent.ClientID` and `client_uris` properties.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: chi v5 (router), ory/fosite (local token issuance per ADR 20260328-120100), lestrrat-go/jwx (JWT/JWKS), Viper/Cobra (config)
**Storage**: PostgreSQL (sqlx) + in-memory — no schema changes needed (FR-017)
**Testing**: Go testing + Ginkgo/Gomega (E2E), testify (assertions)
**Target Platform**: Linux server (Docker), macOS dev
**Project Type**: Go monorepo with React frontend
**Performance Goals**: Same startup time as current single-mode operation (SC-001)
**Constraints**: Mode wiring at startup only — no runtime `if mode ==` checks in handlers (FR-014)
**Scale/Scope**: Config + domain refactoring, no new entities, no migrations

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: No new entities. Existing Agent entity unchanged (FR-017). New value object: `OAuthServerMode` enum (`proxy`, `local`, `hybrid`). New domain concept: agent classification (`ProxyClient`, `CIMDClient`, `LocalClient`). New interface: `ModeStrategy`.
- [x] **Domain Concepts**: Yes — `OAuthServerMode`, `ClientMode`, `ModeStrategy` will be added to ARCHITECTURE.md Glossary.
- [x] **Entity IDs**: N/A — no new entities with UUID PKs.
- [x] **Configuration Design**: Yes — mode config restructured into `proxy`, `local`, `cimd` subsections. `issue_token` rejected with deprecation message. `hybrid` requires both proxy and local sections.
- [x] **Config Examples**: Yes — `examples/config/oauth2-server-mode.yaml` and `examples/config/oauth2-authorization-server.yaml` will be updated; new hybrid example added.
- [x] **Helm Chart**: Yes — `charts/agentic-identity-broker/values.yaml` mode value and structure updated.
- [x] **API Design First**: Behavioral change only (mode-based routing). No new API endpoints. OpenAPI descriptions updated to reflect three modes. User confirmation required before implementation.
- [x] **API Documentation**: OpenAPI specs updated to document mode-specific behavior (JWKS availability, metadata differences).
- [x] **API Changes**: Mode naming is config-only; API behavior changes (hybrid mode accepting all client modes) require user confirmation.
- [x] **Database Design**: N/A — no schema changes.
- [x] **E2E Acceptance Tests**: Yes — 20+ scenarios across 4 user stories will have E2E tests in `tests/e2e/hybrid_oauth_modes_e2e_test.go`.
- [x] **E2E Test Mapping**: Yes — 1:1 mapping from spec acceptance scenarios to It() blocks.
- [x] **E2E Red Phase**: Yes — detailed realistic assertions before implementation.
- [x] **Frontend Playwright E2E**: N/A — no React UI changes in this feature.
- [x] **Frontend Screenshots**: N/A.

**Implementation Considerations**:

- [x] **Security-First**: Mode enforcement is strict — proxy agents cannot reach local issuance, local/CIMD agents cannot reach upstream proxy (SR-002). `issue_token` fails closed with error (SR-003). CIMD rejected in proxy mode (SR-001).
- [x] **Architecture Docs**: Yes — ARCHITECTURE.md updated with mode strategy pattern, client mode classification, glossary terms.
- [x] **ADRs**: No new ADR needed — the spec, plan, and research documents capture all architectural decisions. ADR 20260328-120100 (fosite for local token issuance) remains the binding ADR; this feature extends its patterns without changing the foundation.
- [x] **Library-First Security**: No new crypto — fosite/jwx remain unchanged. Proxy tokens pass through as-is (SR-004).
- [x] **Zalando Guidelines**: No new endpoints; existing endpoints follow guidelines.
- [x] **End-User Docs**: `docs/configuration.md` updated with new mode names and hybrid mode docs.
- [x] **Migration Testing**: N/A — no migrations.
- [x] **Hexagonal Architecture**: Mode acceptance logic (`AcceptsClientMode`) is domain code in `internal/domain/oauth2/`. Dispatching between proxy/local proceed and grant strategies is adapter code in `internal/adapters/http/enduser/`, wired by the builder. Domain never references HTTP adapters.
- [x] **Persistence Patterns**: N/A — no persistence changes.

## Project Structure

### Documentation (this feature)

```text
specs/20260511-120000-hybrid-oauth-modes/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
internal/
├── ports/
│   └── config.go                          # Mode enum, restructured OAuth2AuthServerConfig
├── domain/
│   └── oauth2/
│       ├── mode_strategy.go               # ModeStrategy interface + ClientMode
│       ├── client_resolver.go             # Updated: CIMD agent UUID rejection (FR-005)
│       └── cimd/
│           └── client_resolver.go         # Unchanged
├── adapters/
│   └── http/
│       └── enduser/
│           ├── proceed_strategy.go        # Renamed: issueToken → local
│           └── token_grant_strategy.go    # Renamed: issueToken → local
├── app/
│   └── builder.go                         # Hybrid mode wiring: both proxy + local strategies
examples/config/
├── oauth2-server-mode.yaml                # Updated: mode: local (was issue_token)
├── oauth2-authorization-server.yaml       # Updated: mode: proxy (unchanged value)
└── oauth2-hybrid-mode.yaml                # New: hybrid mode example
charts/agentic-identity-broker/
└── values.yaml                            # Updated mode config structure
tests/e2e/
├── hybrid_oauth_modes_e2e_test.go         # New: all 20+ spec scenarios
└── mode_configuration_e2e_test.go         # Updated: rename issue_token references
```

**Structure Decision**: Existing hexagonal architecture. Changes span config (ports), domain (mode strategy + classification), adapters (strategy renaming), app (builder wiring), and config examples. No new packages — `ModeStrategy` lives in `internal/domain/oauth2/`, `ClientMode` lives in `internal/domain/storage/`.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Rename `issue_token` → `local` across codebase (config, strategies, tests, docs) | **Include** |
| **Phase 1** | Setup — no new deps needed | Skip |
| **Phase 2** | Design Preconditions (domain model, config, API, E2E tests) | **MANDATORY** |
| **Phase 2.5** | Mode strategy interface + ClientMode + universal resolver | Customizable |
| **Phase 3** | User Story 1 — Symmetric mode naming and config restructuring | P1 |
| **Phase 4** | User Story 2 — Universal resolution, classification, mode enforcement | P1 |
| **Phase 5** | User Story 3 — Mode-specific feature gating (CIMD in hybrid) | P2 |
| **Phase 6** | User Story 4 — Builder wiring as first-class strategy | P2 |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **Include** — renaming `issue_token` → `local` is a pure refactor with no behavior change, keeping it isolated from feature work enables focused review.
- [x] Phase 2.7 (entity boilerplate): **Skip** — no new entities introduced.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/hybrid_oauth_modes_e2e_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: "Hybrid OAuth Server Modes"
- **Context: User Story 1** — Mode naming and configuration (6 scenarios)
- **Context: User Story 2** — Universal resolution, classification, mode enforcement (11 scenarios)
- **Context: User Story 3** — Mode-specific feature gating (4 scenarios)
- **Context: User Story 4** — Mode as architectural strategy (2 scenarios, verified structurally)

**Scenario Mapping**:

| Spec Scenario | Test Description |
|---------------|------------------|
| US1-S1 | `It("accepts proxy mode configuration and operates in proxy mode")` |
| US1-S2 | `It("accepts local mode configuration and operates in local mode")` |
| US1-S3 | `It("accepts hybrid mode configuration and operates in hybrid mode")` |
| US1-S4 | `It("rejects issue_token mode with deprecation message suggesting local")` |
| US1-S5 | `It("rejects proxy mode with local-only fields like token_ttl")` |
| US1-S6 | `It("rejects local mode with proxy-only fields like upstream_issuer_uri")` |
| US2-S1 | `It("hybrid mode: proxy agent resolved by UUID, forwarded to upstream")` |
| US2-S2 | `It("hybrid mode: plain local agent resolved by UUID, tokens issued locally")` |
| US2-S3 | `It("hybrid mode with CIMD: CIMD agent resolved by URL, tokens issued locally")` |
| US2-S4 | `It("rejects UUID access to CIMD agent that has client_uris")` |
| US2-S5 | `It("rejects client_id that is neither UUID nor URL")` |
| US2-S6 | `It("rejects UUID that does not match any agent")` |
| US2-S7 | `It("proxy mode rejects local/CIMD agent")` |
| US2-S8 | `It("local mode rejects proxy agent")` |
| US2-S9 | `It("hybrid mode: local plain agent uses client credentials grant")` |
| US2-S10 | `It("hybrid mode serves JWKS endpoint")` |
| US2-S11 | `It("hybrid mode metadata reflects hybrid capabilities")` |
| US3-S1 | `It("rejects CIMD in proxy mode")` |
| US3-S2 | `It("accepts CIMD in local mode")` |
| US3-S3 | `It("accepts CIMD in hybrid mode")` |
| US3-S4 | `It("hybrid mode: proxy agent unaffected by CIMD")` |

**Test Data Strategy**:
- Use fixtures from `tests/e2e/fixtures/` for stable, reusable test data
- Required fixtures: proxy agent (has ClientID), local agent (no ClientID, no client_uris), CIMD agent (has client_uris, no ClientID), config fixtures for each mode
- New fixture creation: hybrid mode config fixture, proxy agent fixture (if not existing)

**Bootstrap Strategy**:
- Tests use production bootstrap code via `tests/e2e/bootstrap/`
- Fresh server and storage for each test (BeforeEach/AfterEach isolation)
- Hybrid mode tests need mock upstream OAuth2 server for proxy agent forwarding

**Helper Utilities**:
- Mock upstream OAuth2 server (may already exist for proxy mode tests)
- Existing HTTP helpers in `tests/e2e/helpers/`

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/oauth2/mode_strategy_test.go` — client mode classification logic, mode acceptance/rejection
- `internal/ports/config_test.go` — mode validation, cross-mode field rejection, deprecation error
- `internal/domain/oauth2/client_resolver_test.go` — CIMD agent UUID rejection (FR-005)

**Integration Tests**: N/A — no persistence changes.

**Test Coverage Goals**:
- Unit test coverage: all classification paths, all mode validation paths
- E2E test coverage: 100% of 21 acceptance scenarios from spec.md
