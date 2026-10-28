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
- no additional category claim (the spec requires none)

It requires no user delegation and creates no grant or session. Human requests keep the active-delegation requirement unchanged. Signed exact-equality requests are ineligible on human-only rules. The third-party token exchange evaluates a new optional CEL boolean, `token_exchange.claim_extraction.agent_subject_expression`, on the validated subject token. Its default is `sub == agent_id` (guarded by `has()`). A match is rejected before the exchange consults grants or sessions.

The technical approach:

- Classification and category gating live in `internal/domain/impersonation`, between principal extraction and CEL authorization.
- Two configuration DTOs change: `ImpersonationRuleConfig` (`allowed_subject_categories`) and `ClaimExtractionConfig` (`agent_subject_expression`). `ImpersonationMintInput` and every port interface are unchanged.
- Minting in `oauth2server` is unchanged: `sub` and `agent_id` are already reserved base claims. Detection and rejection live in `tokenexchange` (`CELEvaluator.IsAgentSubject`).
- Presence-sensitive YAML shape checks run in the config loader.
- A narrowly superseding ADR (044, **Proposed**) covers ADR 032 decisions 1 and 2 for agents only: delegation is skipped and minted `sub` is target-derived, not necessarily the successfully extracted principal. Human invariants remain intact.

Decisions are in [research.md](research.md) R1–R11.

### PR #293 review feedback applied (planning re-run)

| Review comment | Disposition |
| --- | --- |
| `research.md` R4 (category-ineligible mapping): "confirmed" | C2 recorded as confirmed in research R4 and the OpenAPI delta. |
| `research.md` R6 (third-party rejection): "confirmed" | C3 recorded as confirmed in research R6. The detection input now comes from the R5 expression instead of a claim. |
| `research.md` R5 (reserved claim): follow the spec's "no separate category claim", and add a CEL expression in the token-exchange config that defaults to `sub == agent_id` | The claim (C1) is withdrawn. Research R5 adds `token_exchange.claim_extraction.agent_subject_expression` with the guarded default. The new [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml) defines it. The data model, internal contracts, OpenAPI delta, ADR 044 draft, quickstart, and dependent `tasks.md` entries are aligned. `spec.md` is unchanged: its requirements already state that no category claim is required and leave the token representation to design. |

The override semantics of the new expression (C4) are **blocked pending a stakeholder decision**. Replacement is not implementation-ready: a valid expression such as `false` removes broker-agent detection, contradicting unconditional FR-011 and SR-005. Requiring operators to retain the default as a disjunct does not enforce those requirements. The recommended option is a non-removable built-in rejection check plus configurable widening, but this remediation does not approve that option or change requirements.

### Remediation approval record — analysis run `40707318996/2`

| Finding / gate | Evidence available in this session | Required resolution |
| --- | --- | --- |
| I1 / C4: categorical rejection under overrides | No authorized C4 decision supplied. Existing replacement semantics cannot guarantee FR-011/SR-005. | Obtain a stakeholder decision preserving categorical broker-agent rejection; reconcile research R5/R6, contracts, draft ADR, data model, quickstart, and dependent tasks. Require regression coverage with a detector configured as `false`, an external-only detector, and a compliant widening detector, with and without same-text human grants/sessions; broker agent tokens must never reach human-state or provider lookups. If requirements must change, use `/speckit-clarify` first. |
| G1 / ADR 044 | Proposed draft only; no authorized acceptance reference supplied. | Record explicit acceptance of the agent-only supersession of ADR 032 decisions 1 and 2. Until then ADR 032 remains binding. |
| G1 / R8 unsigned-subject guard | Proposal only; no authorized confirmation reference supplied. | Obtain and record the stakeholder decision on rejecting agent-capable unsigned-subject rules at startup. |

C2/C3 confirmations recorded above do not settle C4, accept ADR 044, or confirm R8. T003 remains unchecked and blocks Phase 2a–2f and all implementation. No approval is inferred from this remediation run. Dependent artifacts retain unapproved proposals pending these decisions; the findings are not fully resolved.

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
- The third-party rejection adds one compiled CEL evaluation per exchange, within the existing evaluation timeout.
- Existing SC-001 impersonation latency budgets remain unchanged.

**Constraints**:
- Fail closed everywhere (Principle I).
- No widening of ADR 031.
- No new request input.
- No change to established human responses.
- Implementation cannot start before ADR 044 acceptance and confirmation of the API delta (Principles II, IV, X).

**Scale/Scope**:
- About 10 production Go files: `ports/config.go`, `domain/impersonation/{rule,service,subject_category}.go`, `domain/tokenexchange/{service,cel_evaluator,diagnostic,constants}.go`, `config/{loader,validator}.go`, `app/builder.go` (pass the new expression), and the token handler audit. `domain/oauth2server/strategies.go` and `ports/oauth2.go` need no change.
- 1 new E2E file with 12 scenarios.
- Docs: OpenAPI, `docs/api/enduser-api.md`, configuration, reference, concepts, examples (including `examples/config/token-exchange.yaml`), Helm chart (4 files, including `templates/configmap.yaml` for the optional expression), `ARCHITECTURE.md`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: [data-model.md](data-model.md). No new entity or aggregate. Value objects: `SubjectCategory` and the Resolved Impersonation Subject on `ruleResult`. The token gains no claim; agent-subject tokens are identified by `sub == agent_id`. Domain event: the existing `impersonation_decision` audit event gains the `subject_category` attribute.
- [x] **Domain Concepts**: Implementation Phase 2a adds *Subject Category*, *Allowed Subject Categories*, and *Agent-Subject Token* to the `ARCHITECTURE.md` glossary (§ RFC 8693 User Impersonation, `ARCHITECTURE.md:1737`) and to §3.1.y. The AGENTS.md glossary row is added after ADR 044 is accepted.
- [x] **Entity IDs**: N/A. There is no new UUID-keyed entity. The existing `id.AgentID` is used for the target.
- [x] **Configuration Design**: [contracts/impersonation-config.yaml](contracts/impersonation-config.yaml) gives the key, enum, default, every CFG-001 outcome, and the layered validation (research R8). [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml) gives `token_exchange.claim_extraction.agent_subject_expression`, its default, and its startup and runtime outcomes (research R5). Both use `internal/ports/config.go` and the existing loader (Principle VII).
- [x] **Config Examples**: The plan adds `examples/config/impersonation-agent-subject.yaml` (agent-only, human-only, and dual alternatives), updates `examples/config/impersonation.yaml` to show the explicit `[human]` setting and the omission note, and references both from `examples/config/README.md`. `examples/config/token-exchange.yaml` documents the optional `agent_subject_expression` with its default and override requirement.
- [x] **Helm Deployment Contract**: The broker is chart-managed. Updates: `values.schema.json` (the strict rule schema gains `allowed_subject_categories`), the `values.yaml` comment example, and `README.md`. `templates/configmap.yaml` already renders the impersonation subtree verbatim; the plan verifies this with a render check. The strict `tokenExchange.claimExtraction` schema gains the optional `agentSubjectExpression` string, and `templates/configmap.yaml` renders `agent_subject_expression` only when it is set.
- [x] **API Design First**: The OpenAPI delta is drafted in [contracts/openapi-token-agent-subject.yaml](contracts/openapi-token-agent-subject.yaml). It adds no new path, parameter, token claim, or error code: only description text and examples.
- [x] **API Documentation**: The delta targets `api/enduser/openapi.yaml`. Extend the existing token-endpoint documentation in `docs/api/enduser-api.md` with agent-subject requests, decoded claims, eligibility and denial examples, aligned with the confirmed OpenAPI. Retain the `docs/concepts/token-exchange.md` and `docs/reference/token-exchange.md` updates and cross-link the guides (Principle IV).
- [ ] **API Changes**: C2 (the category-ineligible mapping) and C3 (the third-party rejection mapping) were **confirmed** in the PR #293 review. C1 (the `subject_category` claim) was **withdrawn** by that review. C4 (a configured `agent_subject_expression` replaces the default) is **pending stakeholder confirmation** on PR #293 before implementation. This gate is unresolved; replacement alone also conflicts with FR-011/SR-005, so the current design is not ready for implementation.
- [x] **Database Design**: No schema change (spec Assumptions; data-model).
- [x] **E2E Acceptance Tests**: All 12 scenarios are mapped below. They are written in Phase 2f before implementation.
- [x] **E2E Test Mapping**: Each scenario maps 1:1 to one `It()` (table below).
- [x] **E2E Red Phase**: Each `It` asserts real responses and decoded claims. Before implementation, they fail on startup acceptance of the new key, a consent-required response instead of `200`, a third-party exchange that is not rejected, and so on. No `Skip`, `XIt`, or `PIt`.
- [x] **Frontend Playwright E2E**: N/A. There is no UI change (API-004).
- [x] **Frontend Screenshots**: N/A.

**Implementation Considerations**:

- [ ] **Security-First**: Agent issuance is off by default (omission means `[human]`) and requires signed, verified credentials, exact UUID equality, and CEL authorization. However, the proposed replacement detector does not enforce FR-011/SR-005 under every valid configuration. Documentation and compliant-override tests cannot establish categorical rejection. C4 must resolve this invariant before the design can pass; the R8 unsigned-subject guard also remains unconfirmed. ADR 031 must not be widened, and audit events must remain credential-free.
- [x] **Architecture Docs**: `ARCHITECTURE.md` §3.1.y (the user-delegation sentence, category gating, and `sub == agent_id` identification), the token-exchange flow description (the `agent_subject_expression` step), and the glossary.
- [ ] **ADRs**: [ADR 044](adr-044-draft.md) is drafted with **Status: Proposed**. It narrowly supersedes ADR 032 decisions 1 and 2 for agent-category requests: the delegation exception and target-derived minted subject despite a potentially different extracted principal. All principal extractions remain mandatory; eligible human requests retain decisions 1–6. **Stakeholder acceptance is required before implementation** (Principle II). This plan does not accept it.
- [x] **Library-First Security**: Verification and signing stay in `jwx`. Classification is a string comparison of an already-verified claim, and detection uses `cel-go` (ADR 009). There is no custom cryptography.
- [x] **Zalando Guidelines**: No new resource or field naming beyond the two snake_case config keys.
- [x] **End-User Docs**: `docs/configuration.md` (both keys, defaults, examples, the human-only collision rejection, the fail-closed detection collisions, and the override requirement) and `docs/concepts/token-exchange.md` (the third-party limitation). This satisfies FR-012.
- [x] **Migration Testing**: N/A.
- [x] **Hexagonal Architecture**:
  - Classification and gating logic live in the domain service, not the handler.
  - The handler only logs the new audit field.
  - No port interface is added, and no adapter imports another.
  - No new cross-context dependency: detection lives entirely in `tokenexchange`, and `oauth2server` is unchanged.
- [x] **Persistence Patterns**: N/A (no persistence).

**Gate status**: BLOCKED. Design artifacts are available, but design availability is not approval or constitutional compliance. I1 requires a C4 decision that preserves FR-011/SR-005; G1 requires explicit ADR 044 acceptance and R8 confirmation. C2/C3 are recorded as confirmed and C1 is withdrawn. T003 is a hard stop until all required decisions and approval references are recorded on PR #293 and dependent artifacts are reconciled.

**Post-Design Re-check**: BLOCKED on I1 and G1. The following structural observations do not override the open gates.
- The design adds no persistence, no port interface, no request input, and no token claim.
- The design preserves established human delegation and responses except for the confirmed human-only signed exact-equality rejection; the existing E2E suites must pass unchanged.
- Deviations from binding decisions are limited to ADR 032 decisions 1 and 2 for agents only, routed through the Proposed ADR 044. T003 must obtain acceptance of both precise exceptions before implementation.
- Principle XIV journeys in the spec match the contracts. Journey 1's decoded claims (`sub == agent_id`, `act`, no category claim) match the OpenAPI example exactly. Journey 3's `subject_token.agent_id` extraction matches the default detection's input.

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
| FR-011, SR-005 | Research R5 and R6. Internal contracts §2 and §4. [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml). |
| FR-012, CFG-003 | Research R5, R8, and R11. |
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
│   ├── token-exchange-agent-subject-config.yaml  # agent_subject_expression contract + Helm schema
│   ├── openapi-token-agent-subject.yaml      # OpenAPI delta (C2, C3 confirmed; C1 withdrawn; C4 pending)
│   └── internal-contracts.md                 # ports, mint, exchange rejection, audit, diagnostics
├── checklists/requirements.md
└── tasks.md                                  # /speckit-tasks (dependent entries aligned with the review)

specs/054-agent-subject-impersonation/adr-044-draft.md  # Proposed ADR draft; becomes adrs/044-agent-subject-impersonation.md on acceptance
```

### Source Code (repository root)

```text
internal/
├── ports/config.go                                   # AllowedSubjectCategories, SubjectCategory* constants, ClaimExtractionConfig.AgentSubjectExpression
├── domain/impersonation/
│   ├── subject_category.go (+ _test.go)              # new: SubjectCategory, classifySubject
│   ├── rule.go (+ rule invariants tests)             # category compile + validation, unsigned guard
│   ├── service.go (+ service_test.go, service_security_test.go)  # gating, delegation skip, audit
│   └── errors.go                                     # categoryIneligible fall-through error
├── domain/oauth2server/impersonation_mint_test.go     # agent input: sub == agent_id, no category claim (no production change)
├── domain/tokenexchange/
│   ├── constants.go                                  # DefaultAgentSubjectExpression
│   ├── cel_evaluator.go (+ cel_evaluator_test.go)    # AgentSubjectExpression compile, IsAgentSubject
│   ├── diagnostic.go (+ _test.go)                    # two new details
│   └── service.go (+ service_security_test.go)       # agent-subject rejection step
├── app/builder.go                                    # pass agent_subject_expression to the token-exchange CEL evaluator
├── config/loader.go, config/impersonation_shape.go (+ _test.go)  # YAML shape check
├── config/validator.go (+ impersonation_validator_test.go)
└── adapters/http/enduser/oauth2_token.go (+ oauth2_token_audit_test.go)  # subject_category attr

tests/e2e/agent_subject_impersonation_test.go         # new, US1.1–US3.4
tests/e2e/fixtures/impersonation_rules.go             # new, agent/human/dual rule builders
tests/integration/helm_chart_test.go                  # schema + render check for the new key

api/enduser/openapi.yaml                              # after confirmation
docs/api/enduser-api.md, docs/configuration.md, docs/reference/token-exchange.md, docs/concepts/token-exchange.md
examples/config/impersonation.yaml, examples/config/impersonation-agent-subject.yaml, examples/config/token-exchange.yaml, examples/config/README.md
charts/agentic-identity-broker/{values.yaml,values.schema.json,README.md,templates/configmap.yaml}
ARCHITECTURE.md, AGENTS.md (glossary + ADR index once ADR 044 is accepted)
```

**Structure Decision**: The existing monorepo layout is used. Changes stay inside the existing `impersonation`, `oauth2server`, and `tokenexchange` bounded contexts and the established config, handler, and test directories. No new top-level directory or package is added.

## Implementation Phase Overview

| Phase | Purpose | Included |
|-------|---------|----------|
| **Phase 0** | Pre-implementation refactoring | Skip. `evaluateRule` accepts the new step without restructuring. |
| **Phase 1** | Setup | Skip. No new dependency or tooling. |
| **Phase 2** | Design preconditions | Required. In order: the governance gate (ADR 044 acceptance, C4, the R8 guard; C2 and C3 confirmed); glossary and architecture; config contract, examples, and Helm schema; the OpenAPI edit; the DB "no change" record; the E2E file in red. |
| **Phase 2.7** | Entity boilerplate | Skip. No new entity. |
| **Phase 2.5** | Foundational | Config DTO fields, constants, diagnostic details, YAML shape check and validator, rule compilation of categories, and compilation of `agent_subject_expression`. Unit tests first. |
| **Phase 3** | US1 (P1) | Classification, delegation skip, agent minting (`sub == agent_id`, no email), `act` retention, audit field. |
| **Phase 4** | US2 (P1) | Category gating and fall-through, the human-only collision rejection, the default, and the startup failures. |
| **Phase 5** | US3 (P2) | CEL binding and unresolved targets (already enforced, now asserted), plus `IsAgentSubject` and the third-party rejection. |
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
| US1.1 | `tests/e2e/agent_subject_impersonation_test.go` | `It("mints an agent token with sub and agent_id equal to the target UUID and no category claim, without consent or grant creation")` |
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
| US3.4 | same | `It("rejects third-party exchange of an agent token under the default agent_subject_expression, with and without a matching human grant and session, never using that session")` |

**Red Phase Requirements**: as in the template.
- Assertions target status codes, `error` and `error_uri`, decoded claims (`sub`, `agent_id`, `act`, and the absence of any category claim), storage state (no grant created), and startup errors.
- No `XIt`, `PIt`, or `Skip`, and no red-phase comments.

**Test Data Strategy**:
- Existing fixtures: `fixtures.LocalConfig()`, `fixtures.AgentWithClientID`, `fixtures.ActiveGrant`, the session fixtures (`tests/e2e/fixtures/sessions.go`), `helpers.MockJWKSServer`, and `helpers.ProvisionSigningKey`.
- New fixture: `tests/e2e/fixtures/impersonation_rules.go`. It holds signed-rule builders that take `allowed_subject_categories`, plus a second registered agent for the cross-agent cases.

**Test Execution Flow**: Phase 2f writes all 12 tests. Running `ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/` must show semantic failures. Implementation turns them green with fixture-only adjustments.

**Bootstrap Strategy**:
- Production `bootstrap.NewServerFactory(config, logger).BuildApp(storage)`, with admin and end-user test servers, per test.
- US3.4 configures `token_exchange` in local mode with the broker issuer as subject-token trust, following `tests/e2e/local_token_exchange_e2e_test.go`. It leaves `agent_subject_expression` unset to prove the default.
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
- `internal/domain/oauth2server/impersonation_mint_test.go`: an agent mint input (`Subject` = target UUID, nil email) yields `sub == agent_id` and no category claim, and CEL cannot override `sub` or `agent_id`.
- `internal/domain/tokenexchange/cel_evaluator_test.go`: the default and override `agent_subject_expression` (equal, unequal, missing `sub` or `agent_id`, type mismatch), a non-boolean result, an evaluation error, and startup compile failures for syntax errors and static non-boolean output.
- `internal/domain/tokenexchange/service_security_test.go`: rejection before grant and session lookups, ADR 033 capture still finalized, a detection error that is terminal and consults no human state, and non-matching tokens that pass through.
- `internal/adapters/http/enduser/oauth2_token_audit_test.go`: the `subject_category` attribute and no credential leakage.

**Integration Tests**:
- `tests/integration/helm_chart_test.go`: the schema accepts the valid lists and rejects invalid ones, and the subtree renders verbatim. `agentSubjectExpression` renders only when set.
- No PostgreSQL change.

**Test Coverage Goals**:
- Every new branch in classification, gating, the YAML shape check, minting, detection, and rejection.
- 100% of the acceptance scenarios in E2E.
- The existing impersonation and local token-exchange E2E suites pass unchanged.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Agent-only deviation from Accepted ADR 032 decisions 1 and 2, through the Proposed superseding ADR 044 | FR-005 requires consent-free agent issuance; FR-004 and R2/R7 require target-derived minted `sub`, not necessarily the successfully extracted principal. ADR 032 requires delegation and extracted-principal-as-minted-`sub`. | A synthetic delegation violates FR-005. Restricting extraction to equal the raw `sub` would silently add a constraint. Amending ADR 032 in place is forbidden (Principle II); acceptance of both precise exceptions is required at T003. |
| New optional token-exchange CEL expression `agent_subject_expression` | FR-011 needs a categorical rejection, the spec requires no category claim, and the PR #293 review asked for configurable detection that defaults to `sub == agent_id`. | A reserved claim was withdrawn by the review. A hard-coded comparison cannot cover trusted external issuers. The default fails closed on the rare human collisions documented in research R5. |
| YAML shape check in addition to struct validation | CFG-001 requires rejecting `null` and scalars, while omission defaults to `[human]`. Viper's weak decoding erases that difference. | Struct-only validation would silently accept `null` as human-only and a scalar `agent` as `[agent]`. |
