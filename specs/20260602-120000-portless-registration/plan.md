# Implementation Plan: Portless Redirect URI Registration for Native App Clients

**Branch**: `feat/ephemeral-runtime-port` | **Date**: 2026-06-02 | **Spec**: [spec.md](spec.md)

## Summary

This feature amends redirect URI validation to implement RFC 8252 §7.3 / OAuth 2.1 §2.3.1: when a redirect URI's host is `localhost` or `127.0.0.1`, the port component is ignored during runtime matching. The change is **purely logical** — no schema migrations, no new entities, no API surface changes. Two call sites in the broker perform redirect URI list-membership checks; both must be updated to use a new port-agnostic comparator for loopback hosts.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: `net/url` (stdlib)
**Storage**: No changes — no new entities, no migrations
**Testing**: Go stdlib `testing`, Ginkgo/Gomega (E2E)
**Target Platform**: Linux server
**Performance Goals**: No impact — comparison is O(n) over registered URIs, unchanged complexity
**Constraints**: Change must not alter validation for non-loopback URIs in any code path
**Scale/Scope**: Two function call sites + new helper + E2E tests

## Constitution Check

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Value object `RedirectURIMatch` identified (maps to a helper function — no new struct/entity)
- [x] **Domain Concepts**: No new glossary terms required; existing `RedirectURIValidator` concept documented in spec
- [x] **Entity IDs**: No new entities with UUID primary keys
- [x] **Configuration Design**: No new configuration parameters
- [x] **Config Examples**: N/A
- [x] **Helm Chart**: No config parameters added/changed — no Helm chart update required
- [x] **API Design First**: No API surface changes — validation is internal
- [x] **API Documentation**: N/A — no endpoint changes
- [x] **API Changes**: N/A
- [x] **Database Design**: No schema changes — no migrations required
- [x] **E2E Acceptance Tests**: All spec scenarios mapped 1:1 to E2E tests (see Testing Strategy)
- [x] **E2E Test Mapping**: Each acceptance scenario maps to one `It()` block in `tests/e2e/`
- [x] **E2E Red Phase**: Assertions will be realistic expectations against actual HTTP status + redirect behavior
- [x] **Frontend Playwright E2E**: Consent screen warning (SC-005) requires Playwright test update in `tests/e2e/frontend/`
- [x] **Frontend Screenshots**: Screenshot for loopback-with-explicit-port warning state required

**Implementation Considerations**:

- [x] **Security-First**: Non-loopback validation unchanged; port-ignore strictly scoped to loopback
- [x] **Architecture Docs**: ARCHITECTURE.md update required — redirect URI validation rules change
- [x] **ADRs**: No new ADR required — the spec + clarifications session fully document the RFC 8252 §7.3 decision and its scope constraints
- [x] **Library-First Security**: Only `net/url` stdlib used — no custom crypto
- [x] **Zalando Guidelines**: N/A — no API changes
- [x] **End-User Docs**: N/A — no user-facing API changes
- [x] **Migration Testing**: N/A — no migrations
- [x] **Hexagonal Architecture**: Change lives in domain layer (`internal/domain/`); no adapter changes needed
- [x] **Persistence Patterns**: N/A — no persistence changes

## Project Structure

### Documentation (this feature)

```text
specs/20260602-120000-portless-registration/
├── plan.md              ← this file
├── research.md          ← Phase 0 output
├── data-model.md        ← Phase 1 output
├── quickstart.md        ← Phase 1 output
└── tasks.md             ← Phase 2 output (/speckit.tasks)
```

### Source Code (files touched)

```text
internal/domain/oauth2/
└── service.go                         # redirect URI list-membership check — primary fix site

internal/domain/oauth2server/
└── provider.go                        # containsRedirectURI() helper — second fix site (fosite-backed redirect path)

internal/domain/urivalidation/
├── redirect.go                        # IsValidRedirectURI + MatchesRedirectURI (moved from storage/)
└── redirect_test.go                   # table-driven unit tests for both functions (primary TDD target)

tests/e2e/
├── cimd_redirect_uri_test.go          # E2E tests for US1, US2, US3, SC-001–005
└── portless_opaque_test.go            # E2E test for SC-006 (opaque Agent flow)

tests/e2e/frontend/
└── cimd_flow_test.go                  # amended: loopback warning for explicit-port registered URI

tests/e2e/screenshots/
└── cimd_loopback_warning_explicit_port.png
```

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring | Skip — no structural changes needed before feature work |
| **Phase 1** | Setup | Minimal — no new dependencies |
| **Phase 2** | Design Preconditions (domain model, E2E tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate | Skip — no new persistent entities |
| **Phase 3** | User Story 1: portless loopback registration & matching | Required |
| **Phase 4** | User Story 2: explicit-port backward compatibility | Required |
| **Phase 5** | User Story 3: non-loopback exact-match regression guard | Required |
| **Phase 6** | SC-006: opaque Agent loopback port-ignore | Required |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- Phase 0 skipped: the existing `==` comparison is a single expression; no refactoring is needed to make the feature PR coherent.
- Phase 2.7 skipped: no new ports, storage adapters, or HTTP handlers.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/cimd_redirect_uri_test.go` + `tests/e2e/portless_opaque_test.go`

**Framework**: Ginkgo/Gomega

**Scenario Mapping**:

| Spec Scenario | E2E Test File | Test Description |
|---|---|---|
| US1 Scenario 1 | `cimd_redirect_uri_test.go` | portless loopback registered, ephemeral port in request → succeeds |
| US1 Scenario 2 | `cimd_redirect_uri_test.go` | portless loopback registered, different ephemeral port → succeeds |
| US1 Scenario 3 | `cimd_redirect_uri_test.go` | portless loopback registered, portless request → succeeds |
| US1 Scenario 4 | `cimd_redirect_uri_test.go` | portless loopback registered, path mismatch → fails |
| US2 Scenario 1 | `cimd_redirect_uri_test.go` | explicit port registered, different port in request → succeeds |
| US2 Scenario 2 | `cimd_redirect_uri_test.go` | explicit port registered, portless in request → succeeds |
| US2 Scenario 3 | `cimd_redirect_uri_test.go` | 127.0.0.1 explicit port, different ephemeral port → succeeds |
| US3 Scenario 1 | `cimd_redirect_uri_test.go` | non-loopback, different port → fails |
| US3 Scenario 2 | `cimd_redirect_uri_test.go` | non-loopback explicit port, different port → fails |
| US3 Scenario 3 | `cimd_redirect_uri_test.go` | non-loopback exact match → succeeds |
| SC-005 | `tests/e2e/frontend/cimd_flow_test.go` | loopback warning shown for explicit-port registered URI |
| SC-006 | `portless_opaque_test.go` | opaque UUID Agent, explicit-port registered, ephemeral port request → succeeds |

**Red Phase Requirements**: Each `It()` block must contain `Expect(resp.StatusCode).To(Equal(...))` or redirect location assertions. No `Expect(true).To(BeFalse())` placeholders.

**Test Data Strategy**:
- CIMD tests: mock CIMD server serving a document with loopback `redirect_uris`; agent fixture with pre-registered `client_uris`
- Opaque test: agent fixture with `redirect_uris: ["http://localhost:3000/callback"]`
- Existing `tests/e2e/fixtures/` patterns apply; new fixture helpers for CIMD docs with portless/explicit-port URIs

**Bootstrap Strategy**:
- Tests use `tests/e2e/bootstrap/` production bootstrap
- Mock CIMD server is already established pattern in existing `cimd_*_test.go` files

### Frontend Playwright E2E Tests

**Test Location**: `tests/e2e/frontend/cimd_flow_test.go` (amended)

**Scenario**: Loopback warning (CS-003 from 028) displayed when the registered URI has an explicit port and the runtime URI uses a different port — complements the existing portless warning coverage

| UI Scenario | Screenshot Filename |
|---|---|
| Loopback warning shown for explicit-port registered loopback redirect | `cimd_loopback_warning_explicit_port.png` |

### Unit Tests

**Location**: `internal/domain/urivalidation/redirect_test.go`

**Coverage**: Table-driven tests for `MatchesRedirectURI`:
- Loopback: same port → match
- Loopback: different port → match
- Loopback: registered portless, request has port → match
- Loopback: registered has port, request portless → match
- Loopback: path differs → no match
- Loopback: scheme differs → no match
- Loopback: host differs (localhost vs 127.0.0.1) → no match
- Non-loopback: same → match
- Non-loopback: port differs → no match
- Non-loopback: portless registered, port in request → no match

**Test Coverage Goals**:
- `MatchesRedirectURI`: 100% branch coverage (small pure function)
- E2E coverage: 100% of acceptance scenarios (12 scenarios)

## Complexity Tracking

No constitution violations. All changes are within domain logic, no architectural deviations.
