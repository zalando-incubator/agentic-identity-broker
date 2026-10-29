# Implementation Plan: Wildcard Redirect URI Hosts

**Branch**: `feat/wildcard-redirect` | **Date**: 2026-10-29 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/055-wildcard-redirect-uri/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Extend Agent-configured HTTPS `redirect_uris` with host-label wildcards for preview callbacks. Validate the public-suffix boundary when patterns are registered, and match only the URI host labels while preserving the exact scheme, port, path, query, and redirect destination. Use the existing `golang.org/x/net/publicsuffix` and `golang.org/x/net/idna` packages. Keep CIMD callbacks, private-use callbacks, loopback behavior, and exact registrations unchanged.

## Technical Context

**Language/Version**: Go 1.27.3
**Primary Dependencies**: `net/url`; existing `golang.org/x/net v0.60.0` (`publicsuffix`, `idna`); Ginkgo v2/Gomega for E2E
**Storage**: Existing `Agent.RedirectURIs []string`, persisted by current memory and PostgreSQL adapters; no schema change
**Testing**: Go `testing` with testify for domain packages; Ginkgo v2/Gomega E2E; `just check`, `just test`, and `just test-e2e-backend`
**Target Platform**: Linux server
**Project Type**: Go backend service
**Performance Goals**: No network lookup during validation or authorization; match cost bounded by the registered URI count and hostname labels
**Constraints**: Wildcards are HTTPS hostname-label patterns for opaque Agent callbacks only. Scheme and non-host URI components remain exact. Public-suffix boundary checks fail closed. CIMD and private-use callbacks remain exact.
**Scale/Scope**: Existing per-Agent URI lists and authorization flow; no new endpoint, configuration, entity, persistence, or frontend surface

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Reuse `Agent.RedirectURIs`; model the wildcard as a validated URI-pattern value, not a new persisted entity.
- [x] **Domain Concepts**: Add “Redirect URI Pattern” to the `ARCHITECTURE.md` glossary.
- [x] **Entity IDs**: Not applicable; no new entity or UUID is introduced.
- [x] **Configuration Design**: No runtime configuration is added.
- [x] **Config Examples**: Not applicable; no configuration is added.
- [x] **Helm Deployment Contract**: Not applicable; no workload configuration changes.
- [x] **API Design First**: No new endpoint or field; document the changed behavior in the existing Agent and authorization contracts before implementation.
- [x] **API Documentation**: Update relevant schemas in `/api/admin/openapi.yaml` and the authorization behavior in `/api/enduser/openapi.yaml`.
- [x] **API Changes**: The requested behavior change is explicitly authorized; record confirmation with the implementation review.
- [x] **Database Design**: Not applicable; the existing URI string list and schema remain unchanged.
- [x] **E2E Acceptance Tests**: Write E2E tests for all nine acceptance scenarios before implementation.
- [x] **E2E Test Mapping**: Map each acceptance scenario to exactly one `It()` block in `tests/e2e/`.
- [x] **E2E Red Phase**: Run the focused suite before implementation and verify realistic assertions fail semantically.
- [x] **Frontend Playwright E2E**: Not applicable; the feature changes no React UI.
- [x] **Frontend Screenshots**: Not applicable; the feature changes no React UI.

**Implementation Considerations**:

- [x] **Security-First**: Validate patterns at registration and again before matching; reject malformed and out-of-bound patterns.
- [x] **Architecture Docs**: Update the architecture glossary and redirect-validation description.
- [x] **ADRs**: Add ADR 044 for host wildcard grammar and the public-suffix trust boundary; accept it with the implementation change.
- [x] **Library-First Security**: Use `golang.org/x/net/publicsuffix` and `golang.org/x/net/idna`; do not implement a public suffix list.
- [x] **API Guidelines**: No endpoint or payload shape is added; retain existing error contracts.
- [x] **End-User Docs**: Update `docs/api/enduser-api.md` with the authorization callback matching behavior.
- [x] **Migration Testing**: Not applicable; no database migration is planned.
- [x] **Hexagonal Architecture**: Keep parsing, boundary validation, and matching in `internal/domain/urivalidation` and its domain callers.
- [x] **Persistence Patterns**: Not applicable; no new repository or persisted field is introduced.

*If any BLOCKING check fails, stop and clarify requirements. Implementation cannot begin until all preconditions complete.*

**Post-design recheck**: Pass. Phase 1 adds no new entity, configuration, migration, endpoint, or UI. The contract, URI data model, source-specific matcher decision, ADR, and nine-scenario E2E plan satisfy the security, API-first, and traceability requirements.

## Project Structure

### Documentation (this feature)

```text
specs/055-wildcard-redirect-uri/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/domain/urivalidation/
├── redirect.go                         # registration validation and matching
├── redirect_test.go                    # exact, loopback, and wildcard matching tests
└── redirect_wildcard_security_test.go  # PSL, label-boundary, and malformed-host cases
internal/domain/storage/
├── agent.go                            # Agent redirect URI registration validation
└── agent_test.go                       # create/update validation cases
internal/domain/oauth2/
├── service.go                          # source-specific redirect matching
└── service_test.go                     # CIMD exact-match regression coverage
api/admin/openapi.yaml                  # Agent create/update/response contract
api/enduser/openapi.yaml                # OAuth2 authorization contract
docs/api/enduser-api.md                 # maintained end-user API documentation
ARCHITECTURE.md                         # domain glossary and architecture notes
adrs/044-wildcard-web-redirect-uris.md  # security decision
tests/e2e/wildcard_redirect_uri_test.go # nine spec-mapped HTTP scenarios
```

**Structure Decision**: Extend existing domain validation and OAuth2 authorization paths; do not add a package, endpoint, adapter, persisted field, or frontend component. Keep the public-suffix policy in the URI-validation domain package, where the existing redirect parser and matcher already live.

## Implementation Phase Overview

*Detailed task breakdown is in `tasks.md` (generated by `/speckit-tasks`). The table below reflects
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

- [x] Phase 0 (refactoring): Skip. Existing URI parsing and validation are already isolated in `internal/domain/urivalidation`.
- [x] Phase 2.7 (entity boilerplate): Skip. No new entity, repository, or endpoint is introduced.

Phase 1 setup is not required: the required `golang.org/x/net` dependency is already present. Phase 2 is required for the OpenAPI contract updates, glossary/ADR, and E2E scenario design. Phase 2.5 is limited to the URI-pattern validator and source-specific matching helper. Phase 3 implements the two user stories. Phase N verifies the security boundaries, documentation, and regression suite.

## Testing Strategy

<!--
  Per Constitution Principle XIII (End-to-End Acceptance Testing & Spec Traceability):
  All features MUST have E2E acceptance tests mapped 1:1 to spec scenarios.
  Frontend UI changes additionally require Playwright E2E tests and screenshots.

  This section documents HOW E2E tests will be structured and implemented for this feature.
-->

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/wildcard_redirect_uri_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in [tests/e2e/README.md](../../tests/e2e/README.md)

**Test Organization**:
- **Top-level Describe**: `Wildcard Redirect URI`
- **Nested Describe/Context**: Preconditions and scenarios (e.g., "when a valid request arrives" → "and no grant exists")
- **It blocks**: Individual acceptance scenarios (one It() per scenario from spec.md)

**Scenario Mapping**:

| Spec Scenario | E2E Test | Behavior |
|---------------|----------|----------|
| US1-S1 | `wildcard_redirect_uri_test.go` | Redirect to matching PR preview 123 with its state |
| US1-S2 | `wildcard_redirect_uri_test.go` | Redirect to matching PR preview 456 with its state |
| US1-S3 | `wildcard_redirect_uri_test.go` | Accept the stable test host |
| US1-S4 | `wildcard_redirect_uri_test.go` | Accept the sandbox host |
| US1-S5 | `wildcard_redirect_uri_test.go` | Accept the staging host |
| US2-S1 | `wildcard_redirect_uri_test.go` | Reject a host outside the registered domain |
| US2-S2 | `wildcard_redirect_uri_test.go` | Reject scheme and path mismatches |
| US2-S3 | `wildcard_redirect_uri_test.go` | Reject patterns that wildcard the registrable domain, suffix, or whole host through Agent registration |
| US2-S4 | `wildcard_redirect_uri_test.go` | Preserve exact callback matching |

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
- Use `fixtures.LocalAgent()`, `fixtures.LocalConfig()`, and `fixtures.DefaultPrincipal()` with isolated in-memory storage.
- Create an active grant where the scenario must reach the registered callback.
- Use `bootstrap.NewAdminTestServer()` for registration validation and `bootstrap.NewEndUserTestServer()` for authorization requests.
- No new fixtures, mock upstreams, or external network dependencies are required.

**Test Execution Flow**:
1. **Phase 2f (Design)**: Write E2E tests for all spec scenarios with detailed expectations
2. **Verify Red Phase**: Run `ginkgo -v --focus="Wildcard Redirect URI" ./tests/e2e/` — all new acceptance tests must FAIL semantically
3. **Implementation**: Implement feature incrementally
4. **Verify Green Phase**: E2E tests turn GREEN as implementation satisfies acceptance criteria
5. **Minimal Changes**: Only fixture adjustments during implementation, not test logic

**Bootstrap Strategy**:
- Use production bootstrap wrappers and fresh in-memory storage for each test.
- Start both the admin and end-user test servers for Agent-write and authorization scenarios.

**Helper Utilities**:
- Reuse existing HTTP helpers and OAuth2 error matchers; no custom matcher is planned.
- No mock services are required because callback validation completes before any upstream exchange.

### Frontend Playwright E2E Tests

Not applicable. This feature changes no React UI and requires no frontend screenshots.

### Unit & Integration Tests

**Unit Tests**:
- Location: `internal/domain/urivalidation/redirect_test.go`, `internal/domain/urivalidation/redirect_wildcard_security_test.go`, `internal/domain/storage/agent_test.go`, and `internal/domain/oauth2/service_test.go`
- Coverage: Host-label matching, component equality, PSL boundaries, malformed/unsafe patterns, and Agent create/update validation
- Strategy: Table-driven TDD; write tests first and verify semantic failures before implementation

**Integration Tests**:
- Location: `tests/e2e/wildcard_redirect_uri_test.go` for the HTTP-level Agent API and authorization contract
- Coverage: Admin registration, OAuth2 callback selection, exact matching, and fail-closed errors
- Strategy: Production app bootstrap with isolated in-memory storage; no database adapter change or migration is expected

**Test Coverage Goals**:
- Unit test coverage: All wildcard parsing, registration, matching, and security-boundary branches
- Integration test coverage: All nine acceptance scenarios through production HTTP bootstrap
- E2E test coverage: 100% of acceptance scenarios from spec.md, with one `It()` per scenario
- Frontend E2E coverage: Not applicable; no UI changes

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No constitution violations are planned. No new project, repository, persisted field, or endpoint is required.
