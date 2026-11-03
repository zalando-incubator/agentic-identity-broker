# Implementation Plan: Agent-Subject Impersonation

**Branch**: `speckit/issue-222` | **Date**: 2026-10-29 (re-planned for single-approval sign-off; ADR renumbered to 045) | **Spec**: [spec.md](spec.md)

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

It requires no user delegation and creates no grant or session. Human requests keep the active-delegation requirement unchanged. Signed exact-equality requests are ineligible on human-only rules. The third-party token exchange evaluates a new optional CEL boolean, `token_exchange.claim_extraction.agent_subject_expression`, on the validated subject token. Its default is `sub == agent_id` (guarded by `has()`). A match is rejected before the exchange consults grants, sessions, or providers. A configured expression replaces the default entirely (C4). With the default or a compliant override, every broker agent-subject token is rejected; a non-matching override such as `false` lets it continue through the existing exchange checks, under operator responsibility (clarified FR-011, SR-005, CFG-005).

The technical approach:

- Classification and category gating live in `internal/domain/impersonation`, between principal extraction and CEL authorization.
- Two configuration DTOs change: `ImpersonationRuleConfig` (`allowed_subject_categories`) and `ClaimExtractionConfig` (`agent_subject_expression`). `ImpersonationMintInput` and every port interface are unchanged.
- Minting in `oauth2server` is unchanged: `sub` and `agent_id` are already reserved base claims. Detection and rejection live in `tokenexchange` (`CELEvaluator.IsAgentSubject`).
- Presence-sensitive YAML shape checks run in the config loader.
- A narrowly superseding ADR ([045](../../adrs/045-agent-subject-impersonation.md), **Accepted**; endorsed earlier in maintainer comment `6477637352`) covers ADR 032 decisions 1 and 2 for agents only: delegation is skipped and minted `sub` is target-derived, not necessarily the successfully extracted principal. Human invariants remain intact.

Decisions are in [research.md](research.md) R1–R12. Analysis `40859455167/1` remediation proposes End-User API 2.0.0 on the existing endpoint (A10; [versioning and migration contract](contracts/enduser-api-versioning.md)), moves all production Helm behavior after the complete acceptance-red gate, and clarifies that nil agent email omits credential-derived input, not policy-owned output. These changes do not record approval.

### PR #293 review feedback applied (planning re-run)

| Review comment | Disposition |
| --- | --- |
| `research.md` R4 (category-ineligible mapping): "confirmed" | C2 recorded as endorsed in research R4 and the OpenAPI delta; registered for sign-off as Approval Items A4. |
| `research.md` R6 (third-party rejection): "confirmed" | C3 recorded as endorsed in research R6 (Approval Items A4). The detection input now comes from the R5 expression instead of a claim. |
| `research.md` R5 (reserved claim): follow the spec's "no separate category claim", and add a CEL expression in the token-exchange config that defaults to `sub == agent_id` | The claim (C1) is withdrawn. Research R5 adds `token_exchange.claim_extraction.agent_subject_expression` with the guarded default. The new [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml) defines it. The data model, internal contracts, OpenAPI delta, ADR 045 draft, quickstart, and dependent `tasks.md` entries are aligned. `spec.md` is unchanged: its requirements already state that no category claim is required and leave the token representation to design. |

### Earlier stakeholder input — maintainer `/speckit-plan` command (comment `6477637352`, 2026-10-28)

| Input | Recorded effect |
| --- | --- |
| "ADR 044 accepted" | The ADR draft became [ADR 045](../../adrs/045-agent-subject-impersonation.md) (044 is used twice on `main`). It supersedes ADR 032 decisions 1 and 2 for agent-category requests only. Its revised text was signed off through the single pipeline approval comment (Approval Items A1), and it is Accepted. |
| "C4: a configured agent_subject_expression replaces the default is accepted" | Research R5: a non-empty configured expression replaces the default, with no additional built-in check. The operator must keep the override matching broker agent-subject tokens. Registered as Approval Items A3 and A6. |
| "R8 unsigned-subject startup guard is not needed and should not fail startup" | Research R8: no startup guard. An unsigned-subject rule that lists `agent` starts, and the setting has no effect because unsigned subjects are always human (SR-003). Registered as Approval Items A7. |

The earlier input is the rationale for each recommended option. The implementation phase records the approval only from the maintainer's 👍 on the pipeline approval comment.

### FR-011/SR-005 specification alignment (clarification session 2026-10-28)

Authorized maintainer command `6480240156` asked the clarification phase to align the specification with C4. `spec.md` (Clarifications, *Session 2026-10-28*) now records:

- **FR-011 / SR-005**: with the default `agent_subject_expression`, every broker agent-subject token is categorically rejected before any human grant, session, or provider access. A configured expression replaces the default with no additional built-in check. A compliant override keeps the guarantee; a non-matching override such as `false` removes it, and the token continues through the existing exchange checks. Detection failures fail closed.
- **FR-012 / CFG-005**: documentation, examples, and the Helm contract state the default, the replacement semantics, fail-closed detection failures, and the operator's responsibility, and warn about non-matching overrides.
- **US3.4 / Journey 3 Step 3 / SC-005**: acceptance covers the default, a compliant replacing override, a non-matching replacing override, and detection errors.

The design matches these semantics (research R5 and R6).

### Planning re-run (2026-10-29): governance wording and sign-off

- **ADR wording (HIGH finding of analysis `40773362732/1`) — resolved.** ADR 045's Context and Positive Consequences promised unconditional non-exchangeability. They now say that, with the default detector or a matching override, agent-subject tokens are rejected before human-state access; that detection failures fail closed; and that a non-matching result continues the existing checks, which may consult matching human grants and sessions and return existing consent errors. Decision 6 now states the same scope. C4 is unchanged.
- **Renumbering.** `main` already has two ADR 044 files, so the draft is ADR 045. The governance task still uses the next free number if 045 is taken at merge time.
- **Sign-off.** Every API change, the ADR, and each consequential decision is listed in [Approval Items](#approval-items) and approved through comment [6495558326](https://github.com/zalando-incubator/agentic-identity-broker/pull/293#issuecomment-6495558326).
- **Human-state observation boundary.** The production-span design for US3.4 (T026–T027) from the previous remediation is retained unchanged; it is internal instrumentation under ADR 011 and needs no sign-off.

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
- No change to nonmatching human responses; A8/A9 collision denials require the approved End-User API 2.0.0 major contract bump (A10), not an unversioned patch.
- ADR 045, the C2–C4 API contracts, and the decisions in [Approval Items](#approval-items) need the maintainer's sign-off before implementation (Principles II, IV, X). FR-011/SR-005 are aligned with C4 (clarification session 2026-10-28).

**Scale/Scope**:
- About 10 production Go files: `ports/config.go`, `domain/impersonation/{rule,service,subject_category}.go`, `domain/tokenexchange/{service,cel_evaluator,diagnostic,constants}.go`, `config/{loader,validator}.go`, `app/builder.go` (pass the new expression), and the token handler audit. `domain/oauth2server/strategies.go` and `ports/oauth2.go` need no change.
- 1 new E2E file with 12 scenarios.
- Docs: OpenAPI, `docs/api/enduser-api.md`, configuration, reference, concepts, examples (including `examples/config/token-exchange.yaml`), Helm chart (4 files, including `templates/configmap.yaml` for the optional expression), `ARCHITECTURE.md`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: [data-model.md](data-model.md). No new entity or aggregate. Value objects: `SubjectCategory` and the Resolved Impersonation Subject on `ruleResult`. The token gains no claim; agent-subject tokens are identified by `sub == agent_id`. Domain event: the existing `impersonation_decision` audit event gains the `subject_category` attribute.
- [x] **Domain Concepts**: Implementation Phase 2a adds *Subject Category*, *Allowed Subject Categories*, and *Agent-Subject Token* to the `ARCHITECTURE.md` glossary (§ RFC 8693 User Impersonation, `ARCHITECTURE.md:1737`) and to §3.1.y. Phase 2 also adds the AGENTS.md glossary row and the ADR 045 index entry.
- [x] **Entity IDs**: N/A. There is no new UUID-keyed entity. The existing `id.AgentID` is used for the target.
- [x] **Configuration Design**: [contracts/impersonation-config.yaml](contracts/impersonation-config.yaml) gives the key, enum, default, every CFG-001 outcome, and the layered validation (research R8). [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml) gives `token_exchange.claim_extraction.agent_subject_expression`, its default, and its startup and runtime outcomes (research R5). Both use `internal/ports/config.go` and the existing loader (Principle VII).
- [x] **Config Examples**: The plan adds `examples/config/impersonation-agent-subject.yaml` (agent-only, human-only, and dual alternatives), updates `examples/config/impersonation.yaml` to show the explicit `[human]` setting and the omission note, and references both from `examples/config/README.md`. `examples/config/token-exchange.yaml` documents the optional `agent_subject_expression`, its default, the replacement semantics, and the override requirement.
- [x] **Helm Deployment Contract**: The broker is chart-managed. Updates: `values.schema.json` (the strict rule schema gains `allowed_subject_categories`), the `values.yaml` comment example, and `README.md`. `templates/configmap.yaml` already renders the impersonation subtree verbatim; the plan verifies this with a render check. The strict `tokenExchange.claimExtraction` schema gains the optional `agentSubjectExpression` string, and `templates/configmap.yaml` renders `agent_subject_expression` only when it is set.
- [x] **API Design First**: The OpenAPI delta is drafted in [contracts/openapi-token-agent-subject.yaml](contracts/openapi-token-agent-subject.yaml). It adds no path, parameter, token claim, or error code, but changes behavior. [contracts/enduser-api-versioning.md](contracts/enduser-api-versioning.md) treats the existing 1.0.0 contract as released and proposes `info.version: 2.0.0` on existing routes (A10). T010 applies both the version and behavior documentation plus the `ARCHITECTURE.md` versioning strategy before production behavior; T048 supplies migration guidance, and T053/T058/T059 verify agreement before delivery. No unreleased-contract exemption is assumed (Constitution IV/X; API-005).
- [x] **API Documentation**: The delta targets `api/enduser/openapi.yaml`. Extend the existing token-endpoint documentation in `docs/api/enduser-api.md` with agent-subject requests, decoded claims, eligibility and denial examples, aligned with the confirmed OpenAPI. Retain the `docs/concepts/token-exchange.md` and `docs/reference/token-exchange.md` updates and cross-link the guides (Principle IV).
- [x] **API Changes**: Registered and signed off: C2 (the category-ineligible mapping) and C3 (the third-party rejection mapping), endorsed in the PR #293 review, and C4 (a configured `agent_subject_expression` replaces the default), endorsed in maintainer comment `6477637352`, are **Approved** (Approval Items A2–A6). C1 (the `subject_category` claim) was **withdrawn** by that review. The OpenAPI delta is applied in Phase 2 after approval.
- [x] **Database Design**: No schema change (spec Assumptions; data-model).
- [x] **E2E Acceptance Tests**: All 12 scenarios are mapped below. T013–T017 write and run them in Phase 2f before any production behavior, including Helm schema/template changes. T008's Helm tests run red first; T009 is deferred to Phase 2g after T017, then makes Helm tests green and reruns acceptance tests before broker foundation work.
- [x] **E2E Test Mapping**: Each scenario maps 1:1 to one `It()` (table below).
- [x] **E2E Red Phase**: Each `It` asserts real responses and decoded claims. Before implementation, they fail on startup acceptance of the new key, a consent-required response instead of `200`, a third-party exchange that is not rejected, and so on. No `Skip`, `XIt`, or `PIt`.
- [x] **Frontend Playwright E2E**: N/A. There is no UI change (API-004).
- [x] **Frontend Screenshots**: N/A.

**Implementation Considerations**:

- [x] **Security-First**: Agent issuance is off by default (omission means `[human]`) and requires signed, verified credentials, exact UUID equality, and CEL authorization. Unsigned subjects are always human (SR-003); per the proposed R8 decision (Approval Items A7), an unsigned rule listing `agent` starts and the setting has no effect, so ADR 031 is not widened. The default `agent_subject_expression` rejects every broker agent-subject token before any human-state lookup. Detection errors, timeouts, and non-boolean results fail closed and never become `false`. Under the proposed C4 replacement semantics (Approval Items A6), an override is operator-owned; the clarified FR-011/SR-005 make the guarantee explicit for the default and compliant overrides, and FR-012/CFG-005 require documentation to warn that a non-matching override removes it. Audit events remain credential-free.
- [x] **Architecture Docs**: `ARCHITECTURE.md` §3.1.y (the user-delegation sentence, category gating, and `sub == agent_id` identification), the token-exchange flow description (the `agent_subject_expression` step), and the glossary.
- [x] **ADRs**: [ADR 045](../../adrs/045-agent-subject-impersonation.md) is **Accepted** (Approval Items A1; endorsed earlier in maintainer comment `6477637352`). It narrowly supersedes ADR 032 decisions 1 and 2 for agent-category requests: the delegation exception, and a target-derived minted subject that can differ from the extracted principal. All principal extractions remain mandatory, and eligible human requests retain decisions 1–6. ADRs 031 and 034 are unchanged. The first implementation task records the supplied `context.approval` in its Status line and moves it into `adrs/`; Phase 2 verifies and indexes it (Principle II).
- [x] **Library-First Security**: Verification and signing stay in `jwx`. Classification is a string comparison of an already-verified claim, and detection uses `cel-go` (ADR 009). There is no custom cryptography.
- [x] **Zalando Guidelines**: No new resource or field naming beyond the two snake_case config keys.
- [x] **End-User Docs**: `docs/configuration.md` (both keys, defaults, examples, the human-only collision rejection, the no-effect `agent` setting on unsigned rules, the fail-closed detection collisions and failures, the replacement semantics, the override requirement, and the warning that a non-matching override removes the categorical rejection and same-text human-credential protection) and `docs/concepts/token-exchange.md` (the third-party limitation). This satisfies FR-012.
- [x] **Migration Testing**: N/A.
- [x] **Hexagonal Architecture**:
  - Classification and gating logic live in the domain service, not the handler.
  - The handler only logs the new audit field.
  - No port interface is added, and no adapter imports another.
  - No new cross-context dependency: detection lives entirely in `tokenexchange`, and `oauth2server` is unchanged.
- [x] **Persistence Patterns**: N/A (no persistence).

**Gate status**: PASS for planning. No violation is unjustified, and no `NEEDS CLARIFICATION` remains. The ADR wording finding from analysis `40773362732/1` is resolved in the draft. Implementation remains gated on the maintainer's 👍 for every row in [Approval Items](#approval-items); the first implementation task records only the supplied `context.approval` in ADR 045 and relocates it before any code change. Keep `spec.md` and this plan unchanged during implementation; `quickstart.md` records approval coverage and the final ADR path, including any numbering collision, so the frozen draft links remain traceable.

**Post-Design Re-check**: PASS, subject to the same approvals. Structural observations:
- The design adds no persistence, no port interface, no request input, and no token claim. It does bump the End-User API contract major version under approved A10 to cover A8/A9's incompatible denials; neither approval of denials alone nor unchanged response shapes waives that gate.
- The design preserves established human delegation and responses except for the human-only signed exact-equality rejection (spec Q4; Approval Items A8); the existing E2E suites must pass unchanged.
- Deviations from binding decisions are limited to ADR 032 decisions 1 and 2 for agents only, covered by the proposed ADR 045 (Approval Items A1).
- Principle XIV journeys in the spec match the contracts. Journey 1's decoded claims (`sub == agent_id`, `act`, no category claim) match the OpenAPI example exactly. Journey 3's `subject_token.agent_id` extraction matches the default detection's input, and Step 3's compliant override is the exact example in [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml).

## Spec Alignment

| Requirement | Design anchor |
| --- | --- |
| FR-001, FR-005 | Research R3 step 6. ADR 045 decision 1. |
| FR-002, CFG-001–CFG-004 | Research R8. [contracts/impersonation-config.yaml](contracts/impersonation-config.yaml). |
| FR-003, FR-004, FR-009 | Research R2 and R3. Data-model classification. |
| FR-006 | Research R3 (the human path is unchanged). Regression suites. |
| FR-007 | Research R7 (`act` is unchanged). |
| FR-008 | Research R3 and R9 (CEL is mandatory; extraction failure falls through; an error is terminal). |
| FR-010 | No change to issuer, TTL, scope checks, or `aud` policy. Proxy and hybrid modes are still rejected by the validator. |
| FR-011, SR-005 | Research R5 and R6. Internal contracts §2 and §4. [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml). Guaranteed with the default detector and compliant overrides; a non-matching override continues through the existing flow (clarified C4 semantics). |
| FR-012, CFG-003, CFG-005 | Research R5, R8, and R11. [contracts/token-exchange-agent-subject-config.yaml](contracts/token-exchange-agent-subject-config.yaml). |
| SR-001–SR-003, SR-006 | Research R3, R8, and R2. Credential verification is unchanged. |
| SR-004 | Research R7. Internal contracts §5. |
| API-001–API-004 | [contracts/openapi-token-agent-subject.yaml](contracts/openapi-token-agent-subject.yaml) (C2–C4 proposed, C1 withdrawn; Approval Items A4). |
| API-005 | [contracts/enduser-api-versioning.md](contracts/enduser-api-versioning.md) (approved A10); T001 approval, T010 blocking version/architecture design, T048 migration, T053/T058/T059 verification. |

## Project Structure

### Documentation (this feature)

```text
specs/054-agent-subject-impersonation/
├── spec.md
├── plan.md                                   # This file
├── research.md                               # Phase 0 (R1–R12)
├── data-model.md                             # Phase 1
├── quickstart.md                             # Phase 1
├── contracts/
│   ├── impersonation-config.yaml             # allowed_subject_categories contract + Helm schema
│   ├── token-exchange-agent-subject-config.yaml  # agent_subject_expression contract + Helm schema
│   ├── openapi-token-agent-subject.yaml      # OpenAPI delta (C2–C4 proposed; C1 withdrawn)
│   ├── enduser-api-versioning.md             # approved A10: 2.0.0 major contract and migration
│   └── internal-contracts.md                 # ports, mint, exchange rejection, audit, diagnostics
├── checklists/requirements.md
└── tasks.md                                  # /speckit-tasks (dependent entries aligned with the review)

adrs/045-agent-subject-impersonation.md  # Accepted ADR (relocated from the plan draft)
```

### Source Code (repository root)

```text
internal/
├── ports/config.go                                   # AllowedSubjectCategories, SubjectCategory* constants, ClaimExtractionConfig.AgentSubjectExpression
├── domain/impersonation/
│   ├── subject_category.go (+ _test.go)              # new: SubjectCategory, classifySubject
│   ├── rule.go (+ rule invariants tests)             # category compile + validation (no unsigned guard)
│   ├── service.go (+ service_test.go, service_security_test.go)  # gating, delegation skip, audit
│   └── errors.go                                     # categoryIneligible fall-through error
├── domain/oauth2server/impersonation_mint_test.go     # agent input: sub == agent_id, no category claim (no production change)
├── domain/tokenexchange/
│   ├── constants.go                                  # DefaultAgentSubjectExpression
│   ├── cel_evaluator.go (+ cel_evaluator_test.go)    # AgentSubjectExpression compile, IsAgentSubject
│   ├── diagnostic.go (+ _test.go)                    # two new details
│   └── service.go (+ service_security_test.go, service_observation_test.go)  # rejection + production grant/session boundary spans
├── app/builder.go                                    # pass agent_subject_expression to the token-exchange CEL evaluator
├── config/loader.go, config/impersonation_shape.go (+ _test.go)  # YAML shape check
├── config/validator.go (+ impersonation_validator_test.go)
└── adapters/http/enduser/oauth2_token.go (+ oauth2_token_audit_test.go)  # subject_category attr

tests/e2e/agent_subject_impersonation_test.go         # new, US1.1–US3.4
tests/e2e/fixtures/impersonation_rules.go             # new, agent/human/dual rule builders
tests/integration/helm_chart_test.go                  # schema + render check for the new key

api/enduser/openapi.yaml                              # approved delta (C2–C4), applied after sign-off
docs/api/enduser-api.md, docs/configuration.md, docs/reference/token-exchange.md, docs/concepts/token-exchange.md
examples/config/impersonation.yaml, examples/config/impersonation-agent-subject.yaml, examples/config/token-exchange.yaml, examples/config/README.md
charts/agentic-identity-broker/{values.yaml,values.schema.json,README.md,templates/configmap.yaml}
ARCHITECTURE.md, AGENTS.md (glossary + ADR 045 index entry)
```

**Structure Decision**: The existing monorepo layout is used. Changes stay inside the existing `impersonation`, `oauth2server`, and `tokenexchange` bounded contexts and the established config, handler, and test directories. No new top-level directory or package is added.

## Implementation Phase Overview

| Phase | Purpose | Included |
|-------|---------|----------|
| **Phase 0** | Pre-implementation refactoring | Skip. `evaluateRule` accepts the new step without restructuring. |
| **Phase 1** | Setup | T001 verifies `context.approval` covers all A1–A10, records the supplied approval in ADR 045 and `quickstart.md`, and moves the ADR to `adrs/`; T002–T003 establish feature selection, existing tools and baseline evidence. No new dependency or tooling. |
| **Phase 2** | Design preconditions | Required. Verify T001's supplied A1–A10 approval record and relocated ADR (without independent acceptance); reconcile existing FR-011/SR-005 alignment; document glossary, architecture and major-version strategy; config contract/examples and Helm tests red (T008); versioned OpenAPI edit (T010); no-DB record; all 12 E2E scenarios authored and run red (T013–T017). Only then Phase 2g T009 implements Helm behavior, runs Helm green and reruns E2E before broker foundation. |
| **Phase 2.7** | Entity boilerplate | Skip. No new entity. |
| **Phase 2.5** | Foundational | Config DTO fields, constants, diagnostic details, YAML shape check and validator, rule compilation of categories, and compilation of `agent_subject_expression`. Unit tests first. |
| **Phase 3** | US1 (P1) | Classification, delegation skip, agent minting (`sub == agent_id`, no credential-derived email; custom claims remain policy-owned), `act` retention, audit field. |
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
| US3.4 | same | `It("rejects third-party exchange of an agent token before human-state access under the default and a compliant replacing agent_subject_expression, continues through existing checks under a non-matching override, and fails closed on detection errors")` |

**Red Phase Requirements**: as in the template.
- Assertions target status codes, `error` and `error_uri`, decoded claims (`sub`, `agent_id`, `act`, and the absence of any category claim), storage state (no grant created), and startup errors.
- No `XIt`, `PIt`, or `Skip`, and no red-phase comments.

**Test Data Strategy**:
- Existing fixtures: `fixtures.LocalConfig()`, `fixtures.AgentWithClientID`, `fixtures.ActiveGrant`, the session fixtures (`tests/e2e/fixtures/sessions.go`), `helpers.MockJWKSServer`, and `helpers.ProvisionSigningKey`.
- New fixture: `tests/e2e/fixtures/impersonation_rules.go`. It holds signed-rule builders that take `allowed_subject_categories`, plus a second registered agent for the cross-agent cases.

**Test Execution Flow**: Phase 2f writes all 12 tests. Running `ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/` must show semantic failures before any production behavior, including T009's chart schema/templates. T008's Helm tests run red before T009 as well. After both red gates, Phase 2g T009 implements only chart behavior and runs Helm tests green; rerun the complete acceptance suite and record remaining broker semantic failures before foundation. No behavioral change may move forward as scaffolding. Later implementation turns the acceptance scenarios green with fixture-only adjustments.

**Bootstrap Strategy**:
- Production `bootstrap.NewServerFactory(config, logger).BuildApp(storage)`, with admin and end-user test servers, per test. US3.4 uses the existing `BuildAppWithTracerProvider(storage, tp)` variant and `bootstrap.NewInMemoryTracerProvider()` (AlwaysSample); it still passes the concrete production storage adapter and uses production Builder wiring.
- US3.4 configures `token_exchange` in local mode with the broker issuer as subject-token trust, following `tests/e2e/local_token_exchange_e2e_test.go`. Inside the single `It`, it boots one broker per detector configuration and exchanges the same kind of agent token in both fixture states (no human state; same-UUID-text human grant and valid session):
  1. `agent_subject_expression` unset (default): `403 access_denied` with the agent-subject description, no `error_uri`, and no grant, session, or provider access.
  2. Journey 3 Step 3's compliant override (default kept as a disjunct): the same rejection.
  3. Non-matching override `false`: valid extraction, privileged-client authorization, registered target, resource, permission-set and scope fixtures ensure downstream checks are reachable. Without human state, assert the existing `403 access_denied` missing-grant description, `grant_authorization / grant_missing` diagnostic, and consent `error_uri` ending `/agents/<target UUID>`, plus a `tokenexchange.verify_agent_access` span. With a same-text active grant covering the resource and a valid session, require both `tokenexchange.verify_agent_access` and `tokenexchange.get_valid_access_token` spans in the exchange trace. Both fixtures additionally assert absence of the agent-subject description/diagnostic. An unrelated early 400/401/500 cannot satisfy these positive checkpoints; neither fixture requires categorical isolation or automatic exchange success (spec US3.4, SC-005).
  4. Detection error: override `subject_token.sub` (statically `dyn`, so startup accepts it) returns a string at runtime: `500 server_error` and no human-state access.
- A mock upstream provider records calls, so the test can assert that no refresh or provider request is made in variants 1, 2, and 4.

#### Human-state observation boundary (required prerequisite, T026–T027)

**Source constraint:** `bootstrap.ServerFactory.BuildApp` type-asserts `*storage.Adapter`; Builder stores that concrete type, and its repository fields are private (`tests/e2e/bootstrap/server_factory.go:91–115`, `internal/app/builder.go:168–195`, `internal/adapters/storage/factory.go:43–50`). An E2E spy wrapper cannot be injected. Do not change these signatures, expose repository setters, use reflection, or build services outside Builder.

**Minimal production change:** In `internal/domain/tokenexchange/service.go`, instrument the existing `consentService.VerifyAgentAccess` and `oauth2SessionService.GetValidAccessToken` calls with child spans named `tokenexchange.verify_agent_access` and `tokenexchange.get_valid_access_token` from `otel.Tracer("tokenexchange")`. Start each span immediately before its call, pass the child context into the service, and end it immediately after return, including errors, before mapping the result. These are the exchange's only entry boundaries into grant/session reads; verify that invariant over the complete call graph before using the negative evidence. Detection remains before both. Span presence means entry, not necessarily a successful repository read; absence proves neither read-capable boundary was entered. Existing source at Exchange steps 9/10 calls these concrete services; `consent.Service.VerifyAgentAccess` calls the grant repository and `OAuth2SessionService.GetValidAccessToken` calls the session repository. Any future read path must be covered before claiming zero reads.

The production justification is request-correlated latency and reachability of authorization and credential-retrieval boundaries, specifically needed for FR-011/SR-005/SC-005 evidence. Follow ADR 011's global OTel API and context propagation, without an SDK dependency in the domain, new constructor/port, or test-only production API. Emit no token/claim/principal/URL/error-cause attributes or events. Disabled telemetry remains a no-op and must not affect exchange results. T005/T060 document these two boundaries in `ARCHITECTURE.md`; no general telemetry cleanup is in scope.

**Test-first validation:** T026 writes compiling semantic-red tests in `internal/domain/tokenexchange/service_observation_test.go`, using real consent/session services with recording repository fakes in unit tests only. Require a started child span before each repository access, correct parenting, closure on success/missing/error returns, unchanged errors/results/context cancellation, and no sensitive attributes/events. Cover missing grant, valid grant plus session retrieval, and no-op tracing. T027 adds only the two production spans and turns these tests green before story work. Existing unit fakes continue to prove exact zero repository calls under matches/errors in T043; they supplement rather than replace E2E evidence.

**E2E capture and positive controls:** Keep one US3.4 `It`, with fresh storage/server/provider per detector variant and fixture state. Set telemetry and traces enabled; capture with the existing AlwaysSample in-memory provider through `BuildAppWithTracerProvider`. Use a unique sampled W3C `traceparent` for each exchange, require its completed HTTP server span, and inspect only spans with that trace ID after bounded `Eventually` completion/`ForceFlush`, not setup/mint/admin traces or recorder totals. In the same fixture/bootstrap, send an ordinary human token with unequal `sub`/`agent_id`: with no grant it must emit the grant span and the existing missing-grant envelope; with a resource-covering grant and usable session it must emit both spans. Run these controls for default, compliant and `false` configurations. The `subject_token.sub` error configuration cannot admit an ordinary string-subject control token: run its control in an otherwise identical broker with the default detector, sequentially using the same tracer provider/recorder setup pattern, and separately require the failing request's completed server span. Never alter credential validation or fabricate non-string subjects to make a control pass. Then assert zero boundary spans for agent-token default/compliant/error requests, plus zero mock-provider/refresh calls. An absent server/control span is a broken observation setup, never evidence of isolation. `false` requests require the positive checkpoints above.

Manage the global provider within one broker lifetime at a time: snapshot/restore the prior provider and propagator, drain the app and shut down the provider in cleanup; do not run goroutine-parallel broker builds inside the scenario. Reuse trace helpers in `tests/e2e/helpers/` if needed; no production or bootstrap observation API is added. Storage-state comparisons and zero provider calls alone cannot establish zero reads.

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
  - credential-email extraction suppression for agent requests (not custom-claim suppression)
  - audit `SubjectCategory`
- `internal/domain/impersonation` rule invariants: every CFG-001 value case, and an unsigned rule listing `agent` accepted at startup while its subjects stay human.
- `internal/config/impersonation_shape_test.go`: null, scalar, mapping, `[agent, 1]`, `[null]`, and an omitted key.
- `internal/config/impersonation_validator_test.go`: field paths and messages.
- `internal/domain/oauth2server/impersonation_mint_test.go`: an agent mint input (`Subject` = target UUID, nil email) yields `sub == agent_id` and no category claim, and CEL cannot override `sub` or `agent_id`. Characterize no-policy/no-`email` output, `{"email": principal.email}` yielding `email: ""`, and a literal policy-supplied email. Existing CEL exposes an empty-string `principal.email`; it receives no credential-derived email. These tests must not demand a mint-strategy change.
- `internal/domain/tokenexchange/cel_evaluator_test.go`: the default and a replacing override `agent_subject_expression` (the override, not the default, decides) (equal, unequal, missing `sub` or `agent_id`, type mismatch), a non-boolean result, an evaluation error, and startup compile failures for syntax errors and static non-boolean output.
- `internal/domain/tokenexchange/service_security_test.go`: rejection before grant and session lookups, ADR 033 capture still finalized, a detection error that is terminal and consults no human state, non-matching tokens that pass through, a compliant replacing override that still rejects broker agent tokens, and a non-matching replacing override (`false`) that reaches the existing flow.
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
| Agent-only deviation from Accepted ADR 032 decisions 1 and 2, through the superseding ADR 045 (Accepted, Approval Items A1) | FR-005 requires consent-free agent issuance; FR-004 and R2/R7 require target-derived minted `sub`, not necessarily the successfully extracted principal. ADR 032 requires delegation and extracted-principal-as-minted-`sub`. | A synthetic delegation violates FR-005. Restricting extraction to equal the raw `sub` would silently add a constraint. Amending ADR 032 in place is forbidden (Principle II); ADR 045 was endorsed in maintainer comment `6477637352` and awaits sign-off. |
| New optional token-exchange CEL expression `agent_subject_expression` | FR-011 needs a categorical rejection by default (CFG-005), the spec requires no category claim, and the PR #293 review asked for configurable detection that defaults to `sub == agent_id`. | A reserved claim was withdrawn by the review. A hard-coded comparison cannot cover trusted external issuers. The default fails closed on the rare human collisions documented in research R5. The maintainer endorsed replacement over a non-removable built-in check plus widening (C4; Approval Items A6). |
| YAML shape check in addition to struct validation | CFG-001 requires rejecting `null` and scalars, while omission defaults to `[human]`. Viper's weak decoding erases that difference. | Struct-only validation would silently accept `null` as human-only and a scalar `agent` as `[agent]`. |

## Approval Items

Every row is **Approved** through the pipeline approval comment [6495558326](https://github.com/zalando-incubator/agentic-identity-broker/pull/293#issuecomment-6495558326) (2026-10-29); ADR 045's Status line records it. Architecture-review follow-ups A11 and A12 are listed after A10. Earlier PR #293 endorsements are recorded as rationale in [research.md](research.md#resolved-unknowns).

| ID | Type | Decision | Details |
| --- | --- | --- | --- |
| A1 | ADR | ADR 045 (drafted as 044): agent-only supersession of ADR 032 decisions 1 and 2 (no delegation for eligible, CEL-authorized agent-category requests; minted `sub` is the target UUID), with C4-consistent Context and Consequences and the major-version decision 8 (A10). Alternative: synthetic delegation (violates FR-005). | `specs/054-agent-subject-impersonation/adr-045-draft.md:21-32` |
| A2 | API | New configuration key `oauth2_authorization_server.impersonation.rules[].allowed_subject_categories` (`human`/`agent` list, default `[human]`, fail-closed shape and value validation) and the matching strict Helm rule schema. Alternative: a boolean `allow_agent` flag (cannot express agent-only). | `specs/054-agent-subject-impersonation/contracts/impersonation-config.yaml:14-96` |
| A3 | API | New configuration key `token_exchange.claim_extraction.agent_subject_expression` (CEL boolean, default `has(sub) && has(agent_id) && sub == agent_id`, startup and runtime failure semantics) and Helm value `tokenExchange.claimExtraction.agentSubjectExpression`. Alternative: hard-coded comparison (cannot cover external issuers). | `specs/054-agent-subject-impersonation/contracts/token-exchange-agent-subject-config.yaml:1-68` |
| A4 | API | Within the approved End-User API 2.0.0 (A10), `POST /oauth2/token` OpenAPI delta: agent-subject issuance semantics; C2 category-ineligible requests use the existing generic `403 access_denied` without `error_uri`; C3 third-party exchange of a detected agent-subject token returns `403 access_denied` with a fixed description and no `error_uri`; detection failures return `500 server_error`. No new path, parameter, claim, or error code. Alternative: distinct error descriptions per cause (would leak rule contents). | `specs/054-agent-subject-impersonation/contracts/openapi-token-agent-subject.yaml:1-126` |
| A5 | API | New documented diagnostic details `subject_category_ineligible` and `subject_category_unsupported`, and the `subject_category` attribute on the `impersonation_decision` audit event. Alternative: reuse existing details (operators could not tell category denials from policy denials). | `specs/054-agent-subject-impersonation/data-model.md:98-115` |
| A6 | Decision | Security trade-off (C4): a configured `agent_subject_expression` replaces the default with no additional built-in check; a non-matching override such as `false` removes categorical rejection and same-text human-state isolation, under documented operator responsibility. Alternative: a non-removable built-in check that overrides can only widen. | `specs/054-agent-subject-impersonation/research.md:80` |
| A7 | Decision | No unsigned-subject startup guard: an unsigned rule listing `agent` starts, and the setting has no effect because unsigned subjects are always human. Alternative: fail startup for that combination. | `specs/054-agent-subject-impersonation/research.md:124` |
| A8 | Decision | Subject to approved A10's major-version gate, backward-incompatible: on a human-only rule (including the omitted setting), a signed subject whose `sub` exactly equals the target UUID is rejected as ineligible even with active human delegation (spec Q4). Alternative: keep legacy human handling for that collision. | `specs/054-agent-subject-impersonation/spec.md:445` |
| A9 | Decision | Subject to approved A10's major-version gate, backward-incompatible, fail closed: with the default detector, any subject token with equal `sub` and `agent_id` (including pre-upgrade human tokens and trusted external issuer tokens) is rejected for third-party exchange. Alternative: restrict detection to the broker's own issuer (no security benefit; expressible in an override). | `specs/054-agent-subject-impersonation/research.md:86-89` |
| A10 | API | Major End-User API contract bump from 1.0.0 to 2.0.0 on existing routes, treating the baseline as released; includes A8/A9 compatibility impact, architecture versioning strategy, migration guidance and blocking delivery checks. No runtime version selector or legacy compatibility route. Alternative: a new versioned endpoint with parallel old semantics. | `specs/054-agent-subject-impersonation/contracts/enduser-api-versioning.md:5-32` |
