# Implementation Plan: Agent-Subject Impersonation

**Branch**: `speckit/issue-222` | **Date**: 2026-10-27 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/054-agent-subject-impersonation/spec.md`

## Summary

Existing local-mode RFC 8693 impersonation rules gain an explicit, per-rule `allowed_subject_categories` list (`[human]` by default, `[agent]`, or `[human, agent]`). A request is **agent-category** only when three conditions hold:

- The rule's subject role is signed.
- The subject credential verifies.
- Its verified `sub` claim is byte-for-byte equal to the UUID of the target agent selected by the audience.

Every other request is human-category. An authorized agent-category request mints a broker token with:

- `sub` and `agent_id` both set to the target UUID
- the existing `act` claim
- a new reserved claim, `subject_category: "agent"`

It requires no user delegation and creates no grant or session. Human requests keep the active-delegation requirement unchanged. Signed exact-equality requests are ineligible on human-only rules. The third-party token exchange rejects any subject token that carries `subject_category: "agent"`, before it consults grants or sessions.

The technical approach:

- Classification and category gating live in `internal/domain/impersonation`, between principal extraction and CEL authorization.
- Two port DTOs change: `ImpersonationRuleConfig` and `ImpersonationMintInput`. No port interface changes.
- The reserved claim is minted in `oauth2server` and rejected in `tokenexchange`.
- Presence-sensitive YAML shape checks run in the config loader.
- A narrowly superseding ADR (044, **Proposed**) covers the deviation from ADR 032.

Decisions are in [research.md](research.md) R1–R11.

## Technical Context

**Language/Version**: Go 1.27.3

**Primary Dependencies**: existing only.
- `lestrrat-go/jwx/v4`: credential verification and token signing
- `cel-go`: ADR 009
- `ory/fosite`: local issuance
- Viper/Cobra and `go.yaml.in/yaml/v3`: config loading and the YAML shape check
- chi v5

No new module.

**Storage**: No schema change and no migration. Reads only the existing Agent repository (target resolution). The delegation lookup is skipped for agent-category requests.

**Testing**:
- stdlib `testing` + testify (unit, table-driven)
- Ginkgo/Gomega E2E in `tests/e2e/` with production bootstrap, using `helpers.MockJWKSServer` and the in-memory storage
- Helm render and schema check in `tests/integration/helm_chart_test.go`

**Target Platform**: Linux containers (broker binary, Helm chart).

**Project Type**: Single Go service in the monorepo (backend only, no frontend).

**Performance Goals**:
- Agent-category issuance removes one storage lookup (delegation) from the success path.
- Classification is one string comparison per evaluated rule.
- The third-party rejection adds one map lookup.
- Existing SC-001 impersonation latency budgets remain unchanged.

**Constraints**:
- Fail closed everywhere (Principle I).
- No widening of ADR 031.
- No new request input.
- No change to established human responses.
- Implementation cannot start before ADR 044 acceptance and confirmation of the API delta (Principles II, IV, X).

**Scale/Scope**:
- About 9 production Go files: `ports/config.go`, `ports/oauth2.go`, `domain/impersonation/{rule,service,subject_category}.go`, `domain/oauth2server/strategies.go`, `domain/tokenexchange/{service,diagnostic,constants}.go`, `config/{loader,validator}.go`, and the token handler audit.
- 1 new E2E file with 12 scenarios.
- Docs: OpenAPI, configuration, reference, concepts, examples, Helm chart (3 files), `ARCHITECTURE.md`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: [data-model.md](data-model.md). No new entity or aggregate. Value objects: `SubjectCategory` and the Resolved Impersonation Subject on `ruleResult`. The token value object gains a reserved claim. Domain event: the existing `impersonation_decision` audit event gains `subject_category`.
- [x] **Domain Concepts**: Implementation Phase 2a adds *Subject Category*, *Allowed Subject Categories*, and *Agent-Subject Token* to the `ARCHITECTURE.md` glossary (§ RFC 8693 User Impersonation, `ARCHITECTURE.md:1737`) and to §3.1.y. The AGENTS.md glossary row is added after ADR 044 is accepted.
- [x] **Entity IDs**: N/A. There is no new UUID-keyed entity. The existing `id.AgentID` is used for the target.
- [x] **Configuration Design**: [contracts/impersonation-config.yaml](contracts/impersonation-config.yaml) gives the key, enum, default, every CFG-001 outcome, and the layered validation (research R8).
- [x] **Config Examples**: The plan adds `examples/config/impersonation-agent-subject.yaml` (agent-only, human-only, and dual alternatives), updates `examples/config/impersonation.yaml` to show the explicit `[human]` setting and the omission note, and references both from `examples/config/README.md`.
- [x] **Helm Deployment Contract**: The broker is chart-managed. Updates: `values.schema.json` (the strict rule schema gains `allowed_subject_categories`), the `values.yaml` comment example, and `README.md`. `templates/configmap.yaml` already renders the impersonation subtree verbatim; the plan verifies this with a render check.
- [x] **API Design First**: The OpenAPI delta is drafted in [contracts/openapi-token-agent-subject.yaml](contracts/openapi-token-agent-subject.yaml). It adds no new path, parameter, or error code: only description text, examples, and the reserved token claim.
- [x] **API Documentation**: The delta targets `api/enduser/openapi.yaml`. `docs/api/` has no token-endpoint page, and the plan adds none (scope). `docs/concepts/token-exchange.md` and `docs/reference/token-exchange.md` are updated.
- [ ] **API Changes**: **Pending stakeholder confirmation** of C1 (the `subject_category` claim), C2 (the category-ineligible mapping), and C3 (the third-party rejection mapping). These must be confirmed on PR #293 before implementation. This is a pre-implementation gate, not a planning violation.
- [x] **Database Design**: No schema change (spec Assumptions; data-model).
- [x] **E2E Acceptance Tests**: All 12 scenarios are mapped below. They are written in Phase 2f before implementation.
- [x] **E2E Test Mapping**: Each scenario maps 1:1 to one `It()` (table below).
- [x] **E2E Red Phase**: Each `It` asserts real responses and decoded claims. Before implementation, they fail on startup acceptance of the new key, missing `subject_category`, a consent-required response instead of `200`, and so on. No `Skip`, `XIt`, or `PIt`.
- [x] **Frontend Playwright E2E**: N/A. There is no UI change (API-004).
- [x] **Frontend Screenshots**: N/A.

**Implementation Considerations**:

- [x] **Security-First**: Agent issuance is off by default (omission means `[human]`). It requires signed and verified credentials and exact UUID equality. CEL still decides. Agent tokens are categorically rejected for third-party exchange. ADR 031 is not widened, and the unsigned-subject guard is added. Audit events are credential-free.
- [x] **Architecture Docs**: `ARCHITECTURE.md` §3.1.y (the user-delegation sentence, category gating, and the reserved claim), the token-exchange flow description, and the glossary.
- [ ] **ADRs**: [ADR 044](adr-044-draft.md) is drafted with **Status: Proposed**. It narrowly supersedes ADR 032 decision 1 for agent-category requests. **Stakeholder acceptance is required before implementation** (Principle II). This plan does not accept it.
- [x] **Library-First Security**: Verification and signing stay in `jwx`. Classification is a string comparison of an already-verified claim. There is no custom cryptography.
- [x] **Zalando Guidelines**: No new resource or field naming beyond the snake_case claim and config key.
- [x] **End-User Docs**: `docs/configuration.md` (the key, defaults, examples, the human-only collision rejection, and the reserved-claim consequence) and `docs/concepts/token-exchange.md` (the third-party limitation). This satisfies FR-012.
- [x] **Migration Testing**: N/A.
- [x] **Hexagonal Architecture**:
  - Classification and gating logic live in the domain service, not the handler.
  - The handler only logs the new audit field.
  - No port interface is added, and no adapter imports another.
  - `oauth2server` → `tokenexchange` is an existing domain dependency.
- [x] **Persistence Patterns**: N/A (no persistence).

**Gate status**: PASS for planning. There is no unjustified violation. Two **pre-implementation gates** stay open by design: ADR 044 acceptance, and confirmation of C1–C3 plus the R8 unsigned-subject guard. `/speckit-implement` must stop at the first task that depends on either gate until both are recorded on PR #293.

**Post-Design Re-check**: PASS.
- The design adds no persistence, no port interface, and no request input.
- The design does not change established human responses: the existing E2E suites must pass unchanged.
- Deviations from binding decisions are limited to ADR 032 decision 1, routed through the Proposed ADR 044.
- Principle XIV journeys in the spec match the contracts. The proposed claim appears in the OpenAPI example, and the spec's Journey 1 claim excerpt explicitly allows additional local claims.

## Spec Alignment

| Requirement | Design anchor |
| --- | --- |
| FR-001, FR-005 | Research R3 step 6. ADR 044 decision 1. |
| FR-002, CFG-001–CFG-004 | Research R8. [contracts/impersonation-config.yaml](contracts/impersonation-config.yaml). |
| FR-003, FR-004, FR-009 | Research R2 and R3. Data-model classification. |
| FR-006 | Research R3 (the human path is unchanged). Regression suites. |
| FR-007 | Research R7 (`act` is unchanged). |
| FR-008 | Research R3 and R9 (CEL is mandatory; extraction failure falls through; an error is terminal). |
| FR-010 | No change to issuer, TTL, scope checks, or `aud` policy. Proxy and hybrid modes are still rejected by the validator. |
| FR-011, SR-005 | Research R5 and R6. Internal contracts §4. |
| FR-012 | Research R11. |
| SR-001–SR-003, SR-006 | Research R3, R8, and R2. Credential verification is unchanged. |
| SR-004 | Research R7. Internal contracts §5. |
| API-001–API-004 | [contracts/openapi-token-agent-subject.yaml](contracts/openapi-token-agent-subject.yaml) (proposed). |

## Project Structure

### Documentation (this feature)

```text
specs/054-agent-subject-impersonation/
├── spec.md
├── plan.md                                   # This file
├── research.md                               # Phase 0 (R1–R11)
├── data-model.md                             # Phase 1
├── quickstart.md                             # Phase 1
├── contracts/
│   ├── impersonation-config.yaml             # allowed_subject_categories contract + Helm schema
│   ├── openapi-token-agent-subject.yaml      # PROPOSED OpenAPI delta (C1–C3)
│   └── internal-contracts.md                 # ports, mint, exchange rejection, audit, diagnostics
├── checklists/requirements.md
└── tasks.md                                  # /speckit-tasks (not created here)

specs/054-agent-subject-impersonation/adr-044-draft.md  # Proposed ADR draft; becomes adrs/044-agent-subject-impersonation.md on acceptance
```

### Source Code (repository root)

```text
internal/
├── ports/config.go                                   # AllowedSubjectCategories, SubjectCategory* constants
├── ports/oauth2.go                                   # ImpersonationMintInput.SubjectCategory
├── domain/impersonation/
│   ├── subject_category.go (+ _test.go)              # new: SubjectCategory, classifySubject
│   ├── rule.go (+ rule invariants tests)             # category compile + validation, unsigned guard
│   ├── service.go (+ service_test.go, service_security_test.go)  # gating, delegation skip, audit
│   └── errors.go                                     # categoryIneligible fall-through error
├── domain/oauth2server/strategies.go (+ impersonation_mint_test.go)  # reserved claim
├── domain/tokenexchange/
│   ├── constants.go                                  # SubjectCategoryClaim / SubjectCategoryAgent
│   ├── diagnostic.go (+ _test.go)                    # two new details
│   └── service.go (+ service_security_test.go)       # agent-subject rejection step
├── config/loader.go, config/impersonation_shape.go (+ _test.go)  # YAML shape check
├── config/validator.go (+ impersonation_validator_test.go)
└── adapters/http/enduser/oauth2_token.go (+ oauth2_token_audit_test.go)  # subject_category attr

tests/e2e/agent_subject_impersonation_test.go         # new, US1.1–US3.4
tests/e2e/fixtures/impersonation_rules.go             # new, agent/human/dual rule builders
tests/integration/helm_chart_test.go                  # schema + render check for the new key

api/enduser/openapi.yaml                              # after confirmation
docs/configuration.md, docs/reference/token-exchange.md, docs/concepts/token-exchange.md
examples/config/impersonation.yaml, examples/config/impersonation-agent-subject.yaml, examples/config/README.md
charts/agentic-identity-broker/{values.yaml,values.schema.json,README.md}
ARCHITECTURE.md, AGENTS.md (glossary + ADR index once ADR 044 is accepted)
```

**Structure Decision**: The existing monorepo layout is used. Changes stay inside the existing `impersonation`, `oauth2server`, and `tokenexchange` bounded contexts and the established config, handler, and test directories. No new top-level directory or package is added.

## Implementation Phase Overview

| Phase | Purpose | Included |
|-------|---------|----------|
| **Phase 0** | Pre-implementation refactoring | Skip. `evaluateRule` accepts the new step without restructuring. |
| **Phase 1** | Setup | Skip. No new dependency or tooling. |
| **Phase 2** | Design preconditions | Required. In order: the governance gate (ADR 044 acceptance, C1–C3, the R8 guard); glossary and architecture; config contract, examples, and Helm schema; the OpenAPI edit; the DB "no change" record; the E2E file in red. |
| **Phase 2.7** | Entity boilerplate | Skip. No new entity. |
| **Phase 2.5** | Foundational | Port DTO fields, constants, diagnostic details, YAML shape check and validator, and rule compilation of categories. Unit tests first. |
| **Phase 3** | US1 (P1) | Classification, delegation skip, minting of the reserved claim, `act` retention, audit field. |
| **Phase 4** | US2 (P1) | Category gating and fall-through, the human-only collision rejection, the default, and the startup failures. |
| **Phase 5** | US3 (P2) | CEL binding and unresolved targets (already enforced, now asserted), plus the third-party rejection. |
| **Phase N** | Constitution compliance | `just check`, `just test`, the feature E2E, the regression E2E, the Helm check, `just verify`. |

- [x] Phase 0 (refactoring): skip. The rule-evaluation pipeline already has discrete steps.
- [x] Phase 2.7 (entity boilerplate): skip. There is no new entity, handler, or repository.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/agent_subject_impersonation_test.go`

**Framework**: Ginkgo/Gomega following [tests/e2e/README.md](../../tests/e2e/README.md), `tests/e2e/AGENTS.md`, and ADR 007.

**Test Organization**:
- Top-level `Describe("Agent-Subject Impersonation")`.
- `Context` blocks per rule selection: agent-only, human-only, dual, omitted setting, startup validation, and third-party exchange.
- Each `It` carries `// USx.y from specs/054-agent-subject-impersonation/spec.md`.

#### Scenario Mapping

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1.1 | `tests/e2e/agent_subject_impersonation_test.go` | `It("mints an agent token with sub and agent_id equal to the target UUID and subject_category agent, without consent or grant creation")` |
| US1.2 | same | `It("retains act.iss as the verified actor issuer and act.sub as the extracted actor, distinct from the privileged client")` |
| US1.3 | same | `It("issues no token when the client assertion or actor signature is invalid on an otherwise eligible agent request")` |
| US2.1 | same | `It("rejects a non-matching subject on an agent-only rule and an exact-UUID subject on a human-only rule even with an active human delegation")` |
| US2.2 | same | `It("issues an agent token without consent and a human token with delegation under a dual-category rule")` |
| US2.3 | same | `It("returns the consent-required denial for a non-matching human subject without delegation and never retries it as an agent, including a different agent's UUID or an alternate spelling")` |
| US2.4 | same | `It("keeps human-only behavior when the setting is omitted and rejects exact-UUID subjects with and without human delegation")` |
| US2.5 | same | `It("fails startup for empty, unknown, null, scalar, mapping, non-string, and duplicate allowed_subject_categories")`: direct-config cases via `BuildApp`; YAML cases via `bootstrap.LoadCredentialConfiguration` with a temporary file |
| US3.1 | same | `It("issues no token for an agent subject the rule's CEL policy does not authorize, even when no upstream check ran")` |
| US3.2 | same | `It("issues no token for empty, alias, alternate-spelling, or other-agent subjects, or an unresolved audience target, on an agent-only rule")` |
| US3.3 | same | `It("fails closed with server_error and no consent bypass when authorization evaluation fails")` (CEL evaluation error via a type-erroring predicate on a crafted claim) |
| US3.4 | same | `It("rejects third-party exchange of an agent token with and without a matching human grant and session, never using that session")` |

**Red Phase Requirements**: as in the template.
- Assertions target status codes, `error` and `error_uri`, decoded claims (`sub`, `agent_id`, `act`, `subject_category`), storage state (no grant created), and startup errors.
- No `XIt`, `PIt`, or `Skip`, and no red-phase comments.

**Test Data Strategy**:
- Existing fixtures: `fixtures.LocalConfig()`, `fixtures.AgentWithClientID`, `fixtures.ActiveGrant`, the session fixtures (`tests/e2e/fixtures/sessions.go`), `helpers.MockJWKSServer`, and `helpers.ProvisionSigningKey`.
- New fixture: `tests/e2e/fixtures/impersonation_rules.go`. It holds signed-rule builders that take `allowed_subject_categories`, plus a second registered agent for the cross-agent cases.

**Test Execution Flow**: Phase 2f writes all 12 tests. Running `ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/` must show semantic failures. Implementation turns them green with fixture-only adjustments.

**Bootstrap Strategy**:
- Production `bootstrap.NewServerFactory(config, logger).BuildApp(storage)`, with admin and end-user test servers, per test.
- US3.4 configures `token_exchange` in local mode with the broker issuer as subject-token trust, following `tests/e2e/local_token_exchange_e2e_test.go`.
- A mock upstream provider records calls, so the test can assert that no refresh or provider request is made.

**Helper Utilities**:
- Reuse `decodeJWTClaims` from the impersonation suite by moving it to `tests/e2e/helpers/`, only if it is shared. Otherwise, use a local helper.
- No new matcher.

### Unit & Integration Tests

**Unit Tests** (written first, table-driven):
- `internal/domain/impersonation/subject_category_test.go`: exact match, uppercase or braced UUID, canonical alias, other agent, a non-string `sub`, a missing `sub`, and an unsigned subject.
- `internal/domain/impersonation/service_test.go` and `service_security_test.go`:
  - gating and fall-through order
  - the delegation verifier is never called for an agent request (recording fake)
  - a human delegation failure stays terminal
  - email suppression for agent requests
  - audit `SubjectCategory`
- `internal/domain/impersonation` rule invariants: every CFG-001 value case and the unsigned guard.
- `internal/config/impersonation_shape_test.go`: null, scalar, mapping, `[agent, 1]`, `[null]`, and an omitted key.
- `internal/config/impersonation_validator_test.go`: field paths and messages.
- `internal/domain/oauth2server/impersonation_mint_test.go`: the claim only for `agent`, CEL cannot set or remove `subject_category`, and an unknown category fails.
- `internal/domain/tokenexchange/service_security_test.go`: rejection before grant and session lookups, ADR 033 capture still finalized, and non-agent values pass through.
- `internal/adapters/http/enduser/oauth2_token_audit_test.go`: the `subject_category` attribute and no credential leakage.

**Integration Tests**:
- `tests/integration/helm_chart_test.go`: the schema accepts the valid lists and rejects invalid ones, and the subtree renders verbatim.
- No PostgreSQL change.

**Test Coverage Goals**:
- Every new branch in classification, gating, the YAML shape check, minting, and rejection.
- 100% of the acceptance scenarios in E2E.
- The existing impersonation and local token-exchange E2E suites pass unchanged.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Deviation from Accepted ADR 032 decision 1, through the Proposed superseding ADR 044 | FR-005 requires consent-free agent issuance. ADR 032 forbids it with no opt-out. | A synthetic delegation violates FR-005. Amending ADR 032 in place is forbidden (Principle II). |
| New reserved token claim `subject_category` | FR-011 and the spec forbid inferring the category from the subject text alone. | `sub == agent_id` inference misclassifies human tokens with colliding text and tokens issued before the feature. |
| YAML shape check in addition to struct validation | CFG-001 requires rejecting `null` and scalars, while omission defaults to `[human]`. Viper's weak decoding erases that difference. | Struct-only validation would silently accept `null` as human-only and a scalar `agent` as `[agent]`. |
