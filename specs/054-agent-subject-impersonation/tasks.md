# Tasks: Agent-Subject Impersonation

**Input**: Design documents from `/specs/054-agent-subject-impersonation/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`, and `adr-044-draft.md`.

**Tests**: Mandatory under Constitution VIII and XIII. Write tests first, compile them with only minimal structural scaffolding if necessary, demonstrate semantic failures, implement, then refactor with minimal test changes. Never use pending/skip markers, always-fail assertions, or red-phase comments in submitted tests.

**Organization**: Shared design and foundation precede three story increments: US1 (P1), US2 (P1), US3 (P2). All 70 tasks are initially unchecked; generating this list completes no implementation work. The acceptance design covers 12 scenarios (3 US1, 5 US2, 4 US3), and the requirement tables below trace every buildable requirement to tests and delivery work.

## 🔒 Feature Scope [MANDATORY]

- Every task below cites a feature requirement, acceptance scenario, or applicable constitutional constraint. Only work required for feature 054 belongs in this list.
- Stakeholder decisions (maintainer comment `6477637352`): ADR 044 is accepted, C4 replacement semantics are accepted, and the R8 unsigned-subject startup guard is not adopted. The PR #293 review confirmed C2 and C3 and withdrew C1. FR-011/SR-005 still need alignment with the C4 decision through `/speckit-clarify` (plan.md, *Remaining blocker*). T003 is a hard stop, not an instruction to manufacture approval.
- Do not amend binding ADRs, broaden unsigned credentials, add request inputs, or relax human delegation. No frontend, new persistence, third-party machine identity, unrelated refactoring, or new dependency is in scope.
- Unrelated findings require user approval before issue creation; approved links belong only in a non-checkbox Future Work note. Scope expansion requires explicit approval and updates to `spec.md` and this file.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish feature-local execution evidence using the existing repository tooling; no project initialization or dependency changes are needed.

- [ ] T001 Confirm the selected directory is `specs/054-agent-subject-impersonation`, review `plan.md`, `research.md`, `data-model.md`, `contracts/`, and `quickstart.md`, and record execution evidence in `specs/054-agent-subject-impersonation/quickstart.md`; do not commit the local `.specify/feature.json` selection (Constitution II, Feature Scope).
- [ ] T002 Check the tool prerequisites from `justfile` for Go, Ginkgo, Helm, frontend assets, and the full verification gate; install existing tools with `just install-tools` as needed, and record baseline `just check` / `just test` outcomes and unavailable external dependencies in `specs/054-agent-subject-impersonation/quickstart.md` without waiving failures (Constitution VIII; plan Testing Strategy).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation.

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete. Minimal scaffolding solely to compile tests is permitted by Constitution VIII; it must not implement behavior or bypass T003.

### Governance gate (before Phase 2a–2f execution)

- [ ] T003 Record the authorized stakeholder references in `specs/054-agent-subject-impersonation/plan.md`: acceptance of `adr-044-draft.md` explicitly superseding ADR 032 decisions 1 and 2 for agents only (maintainer comment `6477637352`), the PR #293 review references confirming C2 and C3 and withdrawing C1, the C4 replacement-semantics acceptance (comment `6477637352`), and the decision not to adopt the research R8 unsigned-subject startup guard (comment `6477637352`). Also verify that `spec.md` FR-011/SR-005 have been aligned with the C4 decision through `/speckit-clarify`. STOP while that alignment or any reference is missing, never infer approval from a phase label or command, and reconcile affected feature artifacts only to the actual decisions before proceeding (Constitution II, IV, X; FR-005, FR-011, API-002, SR-003, SR-005).

**Decision status (planning run `40748299287/2`)**: ADR 044 acceptance, the C4 replacement semantics, and the R8 decision (no startup guard) are recorded in `plan.md` and research R1/R5/R8. The I1/G1 design gates from analysis run `40707318996/2` are resolved by these decisions, except for the FR-011/SR-005 specification alignment that requires `/speckit-clarify`. C4 regression coverage includes the default detector, a compliant replacing override, and a non-matching replacing override (for example `false`). The default and compliant cases must prove that broker agent-subject tokens never access human grants, sessions, or providers, even with same-text human state. The non-matching case must prove only the documented, operator-owned replacement effect that the aligned FR-011 wording allows.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management).

- [ ] T004 Following T003, verify an explicitly accepted superseding ADR is recorded in `adrs/044-agent-subject-impersonation.md` (or the next free number) from `specs/054-agent-subject-impersonation/adr-044-draft.md`, explicitly covering ADR 032 decisions 1 and 2 for agents only while preserving decisions 1–6 for eligible humans and ADRs 031/034 unchanged; add the accepted decision reference to `AGENTS.md` and repair feature links if the draft moves, without independently accepting or modifying a binding ADR (Constitution II; research R1).
- [ ] T005 Document the reused Agent/UserGrant/UserSession entities, `SubjectCategory`, resolved subject on `ruleResult`, `sub == agent_id` agent-token identification (no category claim), the token-exchange `agent_subject_expression` detection step, and audit event invariants from `data-model.md` in `ARCHITECTURE.md` §3.1.y and the glossary; add Subject Category, Allowed Subject Categories, and Agent-Subject Token, explain byte-for-byte target equality and the deliberate human-only collision rejection, and align the `AGENTS.md` glossary (FR-003–FR-007, FR-011; Constitution II, V).

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design).

- [ ] T006 Finalize `specs/054-agent-subject-impersonation/contracts/impersonation-config.yaml` against the T003 decision, keeping CFG-001's confirmed key, case-sensitive enum, distinct 1–2 entries, omission-only default, and every shape/value outcome; preserve YAML-only rule lists and existing broker precedence rather than introducing ad-hoc loading; finalize `contracts/token-exchange-agent-subject-config.yaml` against the T003 C4 decision, keeping the default `sub == agent_id` detection and its startup/runtime outcomes (CFG-001–CFG-004, FR-011; research R5/R8).
- [ ] T007 Create the complete `examples/config/impersonation-agent-subject.yaml` local-mode example with signed agent-only, human-only, and dual-category alternatives, explicit CEL subject/target binding and synthetic identities; update `examples/config/impersonation.yaml` with `[human]` and the omission note, and reference both from `examples/config/README.md`; document the optional `token_exchange.claim_extraction.agent_subject_expression`, its default, and the replacement semantics and the keep-matching-broker-agent-tokens override requirement in `examples/config/token-exchange.yaml` per `contracts/token-exchange-agent-subject-config.yaml` (FR-011, FR-012, CFG-003, CFG-004; Constitution VII).
- [ ] T008 Write table-driven Helm schema/render cases FIRST in `tests/integration/helm_chart_test.go` for the valid lists including reversed dual order and omission, invalid empty/unknown/case/null/scalar/mapping/non-string/duplicate values, and verbatim rendering, plus `tokenExchange.claimExtraction.agentSubjectExpression` accepted as a string and rendered as `agent_subject_expression` only when set; run `go test ./tests/integration/ -run TestHelm` with Helm installed and record semantic failures for the new cases in `specs/054-agent-subject-impersonation/quickstart.md` before T009 (CFG-001, CFG-003; Constitution VII, VIII).
- [ ] T009 Update `charts/agentic-identity-broker/values.schema.json` with the confirmed optional array (`minItems: 1`, `maxItems: 2`, `uniqueItems: true`, string enum `human`/`agent`); document the key/default in `charts/agentic-identity-broker/values.yaml` and `README.md`, and verify unchanged `templates/configmap.yaml` passes the subtree through; add the optional `agentSubjectExpression` string to the strict `tokenExchange.claimExtraction` schema, render it conditionally in `templates/configmap.yaml`, and document it in `values.yaml`/`README.md`, turning T008 green (CFG-003; research R5/R8; Constitution VII).

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development).

- [ ] T010 Only after T003 confirmation, apply `specs/054-agent-subject-impersonation/contracts/openapi-token-agent-subject.yaml` to `api/enduser/openapi.yaml`: category selection without request fields, no category claim (C1 withdrawn), generic category-denial C2, categorical third-party denial C3 with `agent_subject_expression` detection (C4), exact human-only collision behavior, credential validation, and concrete examples; record the approval reference in `plan.md`, preserve established human responses, and confirm `api/admin/openapi.yaml` requires no change (API-001–API-004; Constitution IV, X).

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management).

- [ ] T011 Reconfirm in `specs/054-agent-subject-impersonation/data-model.md` that existing Agent resolution and human grant/session structures suffice: no new persisted entity, UUID type, repository, SQL, or migration; record why migration apply/rollback and new PostgreSQL adapter tasks are inapplicable (FR-004, FR-005, FR-011; Constitution IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency).

Not applicable: API-004 excludes frontend work. No new UI, universal component, tokens, Wireloom sequence, Playwright test, or screenshot is required. Existing consent URLs and UI remain unchanged. Reopen design review if scope is explicitly changed; do not add frontend work implicitly.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII (End-to-End Acceptance Testing & Spec Traceability), XIV (Specification by Example & Reviewable Journeys).

- [ ] T012 Review all three concrete journeys in `specs/054-agent-subject-impersonation/spec.md` against confirmed config/OpenAPI and decoded JWT behavior; record consistency evidence in `specs/054-agent-subject-impersonation/quickstart.md` for `sub == agent_id` identification (no category claim), the confirmed denial mappings, explicit signed credentials, target UUIDs, consent outcomes, and the same-text human collision fixture; if a requirement change is necessary, STOP for `/speckit-clarify` rather than modifying requirements during implementation (CFG-004, API-002; Constitution XIV).
- [ ] T013 Add reusable signed human/agent/dual rule builders and the second registered agent fixture in `tests/e2e/fixtures/impersonation_rules.go`, reusing `fixtures.LocalConfig`, `fixtures.ActiveGrant`, session fixtures, `helpers.MockJWKSServer`, and `helpers.ProvisionSigningKey`; use production `bootstrap.NewServerFactory(...).BuildApp(storage)`, separate admin/end-user servers, and fresh storage/server lifecycle per scenario in `tests/e2e/agent_subject_impersonation_test.go` (US1.1–US3.4; Constitution VIII, XIII; ADR 007).
- [ ] T014 Write US1.1–US1.3 as exactly three scenario-referenced `It()` blocks in `tests/e2e/agent_subject_impersonation_test.go`: successful complete exchange without grants/sessions or consent, target `sub == agent_id` with no category claim, distinct client/actor with verified `act`, and invalid client/actor signature denials; assert real HTTP envelopes, signed output claims, and unchanged human state (FR-001, FR-004, FR-005, FR-007; SR-002; Constitution XIII).
- [ ] T015 Write US2.1–US2.5 as exactly five scenario-referenced `It()` blocks in `tests/e2e/agent_subject_impersonation_test.go`: category matrix, dual successes, no-delegation denial including nonmatching UUID variants, omitted-setting human regression and collision with/without grants, and invalid startup values/shapes via both `BuildApp` and production `bootstrap.LoadCredentialConfiguration` temporary YAML; keep fixture variants inside their corresponding scenario, not extra `It()` blocks (FR-002, FR-003, FR-006, FR-009; CFG-001; Constitution XIII).
- [ ] T016 Write US3.1–US3.4 as exactly four scenario-referenced `It()` blocks in `tests/e2e/agent_subject_impersonation_test.go`: CEL denial, invalid/cross-agent/alias/alternate-spelling subjects and unresolved target, runtime CEL evaluation failure, and mint-then-third-party-exchange rejection in both human-state conditions; trust the broker issuer for exchange, leave `agent_subject_expression` unset so the default detection is exercised, and use a recording mock provider to assert zero provider/refresh calls (FR-008, FR-011; SR-005; Constitution XIII).
- [ ] T017 Compile and run the complete 12-scenario suite using `ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/` BEFORE behavioral implementation; record each new-behavior semantic failure in `specs/054-agent-subject-impersonation/quickstart.md`, with realistic status/error/claims/state assertions, no pending markers, no vacuous failures, and no red-phase comments; retain existing protections that already pass (Constitution VIII, XIII; plan Scenario Mapping).

**Checkpoint**: T003 approvals, architecture/glossary, config examples/Helm, confirmed OpenAPI, no-DB record, reviewed journeys, and complete compiling E2E assertions with demonstrated new-behavior failures are required before foundation or story implementation. No entity-boilerplate or pre-implementation-refactor phase is needed.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Extend existing boundary DTOs, diagnostics, and startup validation without implementing category-specific issuance. Every behavioral change follows a semantic red test.

- [ ] T018 Add only the minimal DTO fields/constants in `internal/ports/config.go` needed to compile subsequent tests: `AllowedSubjectCategories []string` with `mapstructure:"allowed_subject_categories"`, human/agent constants, and `ClaimExtractionConfig.AgentSubjectExpression string` with `mapstructure:"agent_subject_expression"`; its constraint is "Optional CEL boolean over `subject_token`", and "Empty or omitted means `tokenexchange.DefaultAgentSubjectExpression`"; leave `internal/ports/oauth2.go`, `ImpersonationMintInput`, and existing issuer/verifier interfaces unchanged (CFG-001; data-model Configuration; research R5; Constitution VI, VIII).
- [ ] T019 [P] Write and run failing YAML-shape tests in `internal/config/impersonation_shape_test.go`, through the production loader path where applicable, for omission versus explicit null, scalar, mapping, `[agent, 1]`, `[null]`, and valid string sequences; errors must name `oauth2_authorization_server.impersonation.rules[i].allowed_subject_categories` without document/credential content. Add loader round-trip and existing file/environment/CLI-precedence tests for `token_exchange.claim_extraction.agent_subject_expression` in `internal/config/loader_test.go`, including empty/omitted values remaining eligible for the evaluator default; retain the YAML-only rule-list boundary (CFG-001–CFG-003, FR-011; research R5/R8; Constitution VII, VIII).
- [ ] T020 [P] Write and run failing startup value-validation tables in `internal/config/impersonation_validator_test.go`: "nil means omitted, which defaults to [human]"; "A non-nil list must have 1–2 distinct entries from {human, agent}, matched case-sensitively"; include empty/unknown/case/duplicates, reversed valid order, and the R8 decision that a list containing `agent` with `roles.subject.verification: none` is accepted and does not fail startup (CFG-001, CFG-002, SR-003; data-model Configuration; Constitution VIII).
- [ ] T021 [P] Write and run failing direct-construction/rule-compilation tests in `internal/domain/impersonation/rule_test.go` for the same nil/non-nil constraints from T020, acceptance of an unsigned rule listing `agent` (R8, no startup guard), correct `allowsHuman`/`allowsAgent` flags, and unknown-category denial; keep unsigned human protections intact and prove unsigned subjects on such a rule are never agent-category (CFG-001, SR-003; data-model compiledRule; Constitution I, VIII).
- [ ] T022 [P] Write and run failing diagnostic catalogue tests in `internal/domain/tokenexchange/diagnostic_test.go` for `subject_category_ineligible` at `client_authorization`/`impersonation` and `subject_category_unsupported` at `identity_resolution`/`third_party`, with no new external OAuth error code; in `internal/domain/tokenexchange/cel_evaluator_test.go`, also write failing tables for `agent_subject_expression`: empty means the default, the default is true only when `sub` and `agent_id` are both present and equal (missing claims, unequal values, and type mismatches are false), an override replaces the default, a non-boolean result and an evaluation error/timeout are terminal `server_error` with the internal-contract diagnostics, and "a compile error or a static non-boolean output type fails startup" (permit `bool`/`dyn` as specified in internal contracts §2); include undeclared variables, a dynamic expression returning a non-boolean, and a bounded timeout case in `NewCELEvaluator`/`IsAgentSubject` tests (API-003, FR-011; contracts/internal-contracts.md §2 and §6; Constitution VIII).
- [ ] T023 Implement presence-sensitive YAML validation in `internal/config/impersonation_shape.go`, invoked by `internal/config/loader.go` before Viper weak decoding: present values must be sequence nodes containing scalar `!!str` entries, preserving omission. Turn all T019 tests green using the existing broker loader/precedence mechanisms for the optional detection expression; if its new DTO field already round-trips correctly, record that evidence instead of adding duplicate bindings or an ad-hoc configuration source (CFG-001–CFG-003, FR-011; research R5/R8; Constitution VII).
- [ ] T024 Implement `AllowedSubjectCategories` startup value checks in `internal/config/validator.go` and invariant checks/compiled category flags in `internal/domain/impersonation/rule.go`, turning T020–T021 green with the exact quoted constraints and field paths, adding no unsigned-subject guard (R8); retain existing issuer/CEL/audience validations (CFG-001, CFG-002; SR-003; research R8).
- [ ] T025 Add `DefaultAgentSubjectExpression` in `internal/domain/tokenexchange/constants.go`, compile `AgentSubjectExpression` in `NewCELEvaluator` and add `IsAgentSubject` in `internal/domain/tokenexchange/cel_evaluator.go`, pass the configured value into the token-exchange `CELEvaluatorConfig` in `internal/app/builder.go`, and add the two diagnostic details in `internal/domain/tokenexchange/diagnostic.go`, turning T022 green; add category-ineligible fall-through representation in `internal/domain/impersonation/errors.go` using existing no-match precedence rather than a terminal error. Compile the detector once at startup, enforce the existing evaluation timeout, and retain dynamic-result boolean checking and fail-closed error diagnostics; the shared approval-authentication evaluator must not invoke agent detection outside third-party token exchange (FR-009, FR-011, API-003; data-model Diagnostic details; research R3–R6; internal contracts §2).

**Checkpoint**: Startup constraints and diagnostics are tested and green; category-aware issuance remains to be implemented. Run `just check` and the matching package tests before story work, recording outcomes in `quickstart.md` under T047.

---

## Phase 3: User Story 1 - Obtain a token for an agent acting for itself (Priority: P1) 🎯 MVP

**Goal**: Signed, authorized self-agent requests mint attributable local broker tokens without consulting or creating human state.

**Independent Test**: US1.1–US1.3 complete real HTTP exchanges on a signed agent-only rule with no grants/sessions, verify `sub == agent_id == target UUID`, no category claim, and `act`, and prove invalid client/actor signatures cannot issue. Classification is wired with eligibility in this increment so there is no intermediate implicit consent bypass on human-only rules.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T026 [P] [US1] Write and run failing classification tables in `internal/domain/impersonation/subject_category_test.go`: only a verified signed raw string `claims["sub"]` byte-for-byte equal to `target.Agent.ID.String()` is agent; uppercase/braced/whitespace UUIDs, aliases, another agent UUID, missing/non-string claims, and unsigned subjects are not; prove `principal_expression` mapping another claim cannot cause agent classification (FR-003, FR-004; SR-002, SR-003; research R2).
- [ ] T027 [P] [US1] Write and run failing service tests in `internal/domain/impersonation/service_test.go` and `service_security_test.go` proving zero delegation calls/grant/session creation for agent requests, target-derived mint subject even when successful `principal_expression` extraction differs from raw verified `sub`, retained verified `act` including actor==subject, mandatory credential controls for all three signed roles, no email-expression evaluation or email passed to mint/CEL for agents, and no agent error consent URI (US1.1–US1.3; FR-005, FR-007, FR-010; SR-002).
- [ ] T028 [P] [US1] Write and run mint tables in `internal/domain/oauth2server/impersonation_mint_test.go`: an agent mint input (`Subject` = target UUID, nil `Email`) yields `sub == agent_id` with no category claim, CEL cannot set or change `sub` or `agent_id`, and authorization-code/human-token compatibility and issuer/signing/TTL/scope/policy-owned audience are unchanged; these characterize the unchanged strategy, so record when they already pass rather than forcing a production change (FR-004, FR-010, FR-011; data-model Token; research R5/R7).
- [ ] T029 [P] [US1] Write and run failing handler audit tests in `internal/adapters/http/enduser/oauth2_token_audit_test.go` for `subject_category` on classified success/denial, represented subject versus target/client/actor, available identities on preclassification failure, and absence of tokens/wholesale claims or extra attributes outside the allowlist (SR-004; contracts/internal-contracts.md §5).

### Implementation for User Story 1

- [ ] T030 [US1] Implement `SubjectCategory` and `classifySubject` in `internal/domain/impersonation/subject_category.go`, then wire classification/category eligibility immediately after principal extraction and before CEL in `internal/domain/impersonation/service.go`; use raw verified `sub`, never normalize or resolve a subject alias, and skip ineligible rules using T025 while retaining ordered no-match precedence (FR-003, FR-004, FR-009; research R2/R3; turns T026 green).
- [ ] T031 [US1] Extend `ruleResult` and service issuance in `internal/domain/impersonation/service.go` so only eligible, CEL-authorized agents skip `UserDelegationVerifier`, pass target UUID and `Email=nil` even when the successfully extracted subject principal differs, and retain verified actor identity/issuer even when equal; require T003 acceptance of both ADR 032 exceptions and no added extraction-equality restriction; do not evaluate agent email expressions, create human state, or weaken signed credential controls (FR-001, FR-004–FR-008; SR-002; turns T027 green).
- [ ] T032 [US1] Confirm with T028 that `GenerateImpersonationToken` and `baseClaims` in `internal/domain/oauth2server/strategies.go` need no change: agent tokens carry `sub == agent_id` from reserved base claims, no category claim is added, and a nil email mints no `email`; make only a minimal correction if T028 reveals a gap, without changing normal issuer/signing/scope/TTL/audience semantics (FR-007, FR-010, FR-011; research R5).
- [ ] T033 [US1] Add `AuditRecord.SubjectCategory` and represented-identity propagation in `internal/domain/impersonation/service.go`, and log only the allowed `subject_category` attribute alongside existing fields in `internal/adapters/http/enduser/oauth2_token.go`; emit credential-free decisions for success and failures after classification, retaining existing available-identity behavior before classification (SR-004; research R7; turns T029 green).
- [ ] T034 [US1] Run US1.1–US1.3 in `tests/e2e/agent_subject_impersonation_test.go` to green, plus the T026–T029 package tests and `just check`; record no-human-state and attribution evidence in `specs/054-agent-subject-impersonation/quickstart.md`, refactoring only feature-local duplication without changing acceptance assertions (SC-001, SC-002, SC-005; Constitution VIII, XIII).

**Checkpoint**: US1 is independently testable. It is an MVP demonstration, not permission to release partial work: categorical third-party protection and the remaining stories/compliance must complete before feature delivery.

---

## Phase 4: User Story 2 - Control subject eligibility without weakening human consent (Priority: P1)

**Goal**: Enforce the full human/agent matrix and safe defaults, including rejection of signed exact equality on human-only rules even with matching human grants.

**Independent Test**: US2.1–US2.5 exercise all six category/rule combinations, dual success, omission behavior, missing/expired human delegation with consent URI, terminal human denial, and invalid startup configurations. No human fallback may recover an ineligible agent request.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T035 [US2] Before any additional US2 correction, extend `internal/domain/impersonation/service_test.go` and `service_security_test.go` with matrix and rule-order tables: explicit/omitted human-only collisions with and without human grants, dual/nonmatching human paths, agent-only mismatch, later independently eligible rule success, no-match error precedence, terminal missing/expired/verifier-error human denials without retry, and unsigned human exact-text subjects retaining ADR 031 controls; verify missing behaviors fail semantically and record protections already green (US2.1–US2.4; FR-002, FR-003, FR-006, FR-009; SR-001, SR-003).
- [ ] T036 [US2] Run the US2.5 cases in `tests/e2e/agent_subject_impersonation_test.go` through production YAML loading and direct `BuildApp`, checking omission, both valid orders, every invalid CFG-001 shape/value, field paths, and startup acceptance of an unsigned rule listing `agent` (R8); retain the T017 red evidence and add a failing regression test before correcting any uncovered defect (CFG-001, CFG-002; SR-003; Constitution VIII, XIII).

### Implementation for User Story 2

- [ ] T037 [US2] Complete any behavior gaps exposed by T035 in `internal/domain/impersonation/service.go` and `errors.go`: classify signed exact equality as agent on every rule selection, fall through only under independent category/credential/policy eligibility, retain terminal human delegation denial and consent URI, and never retry an agent as human or a denied human as agent (FR-003, FR-006, FR-009; turns T035 green).
- [ ] T038 [US2] Complete any CFG-001/direct-config gaps exposed by T036 in `internal/config/impersonation_shape.go`, `validator.go`, and `internal/domain/impersonation/rule.go`, retaining the omission-only human default and no unsigned-subject guard (R8) without broadening existing config/ADR 031 behavior; if T024 already satisfies all cases, verify and record that evidence rather than add duplicate logic (US2.5; CFG-001, CFG-002; SR-003).
- [ ] T039 [US2] Turn all US2.1–US2.5 E2E cases green in `tests/e2e/agent_subject_impersonation_test.go`, then run `ginkgo -v --focus "OAuth2 User Impersonation|Local Mode Token Exchange" ./tests/e2e/` against unchanged `tests/e2e/impersonation_test.go` and `local_token_exchange_e2e_test.go`; record the six-combination outcomes and missing/expired human response compatibility in `specs/054-agent-subject-impersonation/quickstart.md` (SC-003, SC-004; FR-006; Constitution VIII, XIII).

**Checkpoint**: Existing nonmatching human behavior and intentional signed-UUID collision rejection are demonstrated independently; config fails before traffic for invalid values.

---

## Phase 5: User Story 3 - Authorize machine identity and keep third-party exchange out of scope (Priority: P2)

**Goal**: Fail closed on identity/policy failures and categorically prevent agent-subject tokens from accessing any human's third-party state.

**Independent Test**: US3.1–US3.4 cover denied CEL, unresolved/noncanonical/cross-agent identities, evaluation failure, and a complete issuance followed by third-party denial both without human state and with a same-text human grant and service session. The provider receives zero calls.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T040 [P] [US3] Extend and run policy/identity tests in `internal/domain/impersonation/service_security_test.go` and `cel_test.go` for explicit `subject_token.sub` and `request.agent_id` CEL binding, false policy fall-through, terminal runtime errors/timeouts, failed extraction including non-string/empty identities, unresolved target/repository failure, cross-agent and alias rejection on agent-only rules, retained scope checks, and proxy/hybrid startup rejection; assert zero mint calls, adding a semantic red test before any needed correction (US3.1–US3.3; FR-008, FR-010; SR-001, SR-002).
- [ ] T041 [P] [US3] Write and run failing rejection tables in `internal/domain/tokenexchange/service_security_test.go`: a validated subject token matching `agent_subject_expression` (default: `sub == agent_id`) rejects before agent extraction, CEL authorization, grant/session/provider lookups; prove zero calls with recording fakes, finalized actor/calling-peer capture and diagnostic detail, no consent URI, and identical rejection for trusted broker/external issuers; a detection evaluation error is terminal and consults no human state; non-matching tokens (missing `agent_id` or `sub != agent_id`) keep existing exchange behavior, a compliant replacing override that keeps the default as a disjunct rejects both broker agent tokens and the external machine-token form while preserving ordinary human tokens, and a non-matching replacing override (for example `false`) demonstrates the accepted C4 replacement semantics as permitted by the T003-aligned FR-011 wording; never treat the non-matching case as evidence that the default guarantee holds (US3.4; FR-011; SR-005; research R6; internal contracts §4).

### Implementation for User Story 3

- [ ] T042 [US3] Add the `IsAgentSubject` rejection step to `internal/domain/tokenexchange/service.go` after validated principal extraction and `security.FinalizeCaptureHolder`, BEFORE `ExtractAgentID`, CEL authorization, resource, grants, or sessions; propagate detection errors unchanged and return the confirmed C3 `403 access_denied` with `identity_resolution/subject_category_unsupported` and no `error_uri`, independent of issuer or human state (FR-011; SR-005; turns T041 green).
- [ ] T043 [US3] Make only minimal corrections, if T040 reveals a feature-path gap, in `internal/domain/impersonation/service.go`, `cel.go`, or `audience.go`, preserving existing denial/error distinctions and no-match precedence; use existing CEL context and target resolution rather than new policy variables or alias normalization, and record unchanged protections when tests already pass (FR-004, FR-008, FR-010; SR-001; turns T040 green).
- [ ] T044 [US3] Turn US3.1–US3.4 green in `tests/e2e/agent_subject_impersonation_test.go`; verify the issued signed token's third-party rejection twice, then assert unchanged matching human state and zero upstream refresh/provider calls, and record all results in `specs/054-agent-subject-impersonation/quickstart.md` (SC-005; FR-011; SR-005; Constitution XIII).

**Checkpoint**: All 12 acceptance scenarios pass and no agent-subject token can obtain a human service credential.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Finish operator documentation and validation evidence required by this feature, not unrelated improvements.

- [ ] T045 [P] Update `docs/configuration.md` with `allowed_subject_categories` enum/default/validation and complete linked examples, YAML-only rule-list boundary, signed identity and actor requirements, human-only collision rejection, and `token_exchange.claim_extraction.agent_subject_expression` with its default, startup/runtime failure semantics, fail-closed collision cases, the C4 replacement semantics, and the requirement that overrides keep matching broker agent-subject tokens (for example by keeping the default as a disjunct); the no-effect `agent` setting on unsigned-subject rules (R8); preserve normal human consent guidance (FR-011, FR-012, CFG-003, CFG-004; research R5/R8/R11).
- [ ] T046 [P] Extend the existing token-endpoint documentation in `docs/api/enduser-api.md` with concrete agent-subject requests, decoded claims, the three category outcomes including human-only signed collision rejection even with delegation, and category/third-party denial examples aligned with the confirmed OpenAPI. Retain updates to `docs/concepts/token-exchange.md` and `docs/reference/token-exchange.md` for categorical third-party restriction even for a colliding human or trusted external issuer token matching `agent_subject_expression`, confirmed errors/no consent recovery, and both diagnostic details/stages/kinds; cross-link the API, concept/reference guides, and configuration examples without adding a new endpoint (Constitution IV; FR-012, API-002, API-004; research R6/R11).
- [ ] T047 Update `specs/054-agent-subject-impersonation/quickstart.md` with actual commands, red/green evidence by task, 12-scenario coverage, Helm execution (not a skipped check), and approval references; keep remaining failures/dependencies explicit and never mark unexecuted or failed checks complete (Constitution VIII, XIII; plan Testing Strategy).

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

**These tasks MUST be included in every generated tasks.md file. They verify that all constitution principles have been followed.**

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist - verify Phase 2 tasks were completed correctly.

- [ ] T048 Verify every task and changed file traces to a feature requirement/scenario or applicable binding constraint, and unrelated findings/independent remediation are absent from `specs/054-agent-subject-impersonation/tasks.md` and the implementation diff; approved issue links may appear only as Future Work (Feature Scope & Deferred Findings).
- [ ] T049 Verify `ARCHITECTURE.md` documents all `data-model.md` entities/value objects/invariants and glossary terms, and the accepted superseding ADR plus its actual approval reference are present and indexed; ADRs 031/032/034 are not silently amended (Constitution II, V; FR-003–FR-007).
- [ ] T050 Verify configuration examples exist in `examples/config/impersonation-agent-subject.yaml` and `impersonation.yaml`, are referenced from `examples/config/README.md`, and match CFG-001 and `spec.md` Journeys 1–2 (Constitution VII, XIV; CFG-003, CFG-004).
- [ ] T051 Verify `api/enduser/openapi.yaml` matches confirmed C2–C4 with no category claim (C1 withdrawn), exact category identity semantics, existing human responses and API-004 scope; retain the actual stakeholder reference in `specs/054-agent-subject-impersonation/plan.md`, never treat task completion as confirmation (Constitution IV, X).
- [ ] T052 Verify the no-database-change decision in `specs/054-agent-subject-impersonation/data-model.md` still holds and no new entity/repository/migration entered scope; migration numbering, apply/rollback/data-integrity and new PostgreSQL adapter tasks are N/A only while that remains true (Constitution IX).
- [ ] T053 Verify all three journeys in `specs/054-agent-subject-impersonation/spec.md` remain linked to their scenarios with consistent config/HTTP/decoded claims and synthetic credential markers; reaffirm API-004 excludes UI/Wireloom/Playwright/screenshots and design-system changes (Constitution XI, XIII, XIV).
- [ ] T054 Verify `tests/e2e/agent_subject_impersonation_test.go` contains exactly one scenario-referenced `It()` for each of the 12 scenarios, uses hierarchical Ginkgo/Gomega and production bootstrap/fixtures, and the recorded semantic-red evidence in `quickstart.md` predates implementation; no pending markers, vacuous assertions, or red-phase annotations (Constitution VIII, XIII; ADR 007).

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist - verify all principles followed during implementation.

**Feature Scope** (Development Requirements):

- [ ] T055 Review `specs/054-agent-subject-impersonation/tasks.md` against the final implementation diff for feature-only corrections, no unauthorized issue creation, no third-party agent identity work, and no optional refactor/telemetry/chart additions (Feature Scope & Deferred Findings; FR-010, FR-011, API-004).

**API & Documentation** (Principles IV, X):

- [ ] T056 Verify actual token endpoint responses/claims against confirmed `api/enduser/openapi.yaml`, preserving all existing human error envelopes/consent URLs and no new request category field; verify `docs/concepts/token-exchange.md`, `docs/reference/token-exchange.md`, and configuration examples render the same contract (Constitution IV, X; API-001–API-004).
- [ ] T057 Verify `docs/api/enduser-api.md` renders the confirmed agent-subject token-endpoint contract with request examples, decoded claims, eligibility and human-only signed collision rejection, and category/third-party denial examples; verify consistency with OpenAPI and discoverability through cross-links with `docs/concepts/token-exchange.md`, `docs/reference/token-exchange.md`, and configuration examples, and record evidence in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution IV; research R11; FR-012).

**Architecture & Documentation** (Principle II):

- [ ] T058 Verify `ARCHITECTURE.md` §3.1.y, token-exchange flow, and glossary describe the implemented classification/eligibility/CEL/delegation sequence, `sub == agent_id` identification and `agent_subject_expression` detection, audit, scope and performance characteristics, and narrow accepted ADR supersession; synchronize `AGENTS.md` index/glossary without altering binding decisions (Constitution II, V; FR-003–FR-011).

**Configuration** (Principle VII):

- [ ] T059 Verify the feature uses `internal/ports/config.go` and the existing broker loader/validator, not an ad-hoc config source, preserves file/env/CLI precedence and the established YAML-only rule list, and applies both loader and direct-domain startup validation, including the recorded R8 decision that adds no unsigned-subject guard (Constitution VII; CFG-001–CFG-003; SR-003).
- [ ] T060 Verify `charts/agentic-identity-broker/{values.yaml,values.schema.json,README.md}` match the confirmed settings/defaults, that `templates/configmap.yaml` renders the impersonation subtree unchanged, and that it renders `agent_subject_expression` only when set; execute `go test ./tests/integration/ -run TestHelm` with Helm available and require the new cases to run and pass, not skip (Constitution VII, VIII; CFG-003).

**Database & Persistence** (Principle IX):

- [ ] T061 Verify `internal/domain/impersonation/service.go` never creates grants/sessions for agents and `internal/domain/tokenexchange/service.go` rejects tokens matching `agent_subject_expression` before any human-state lookup; confirm the diff has no persistence/schema change requiring both adapters or migration testing and document N/A in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution IX; FR-005, FR-011; SR-005).

**Security** (Principles I, III):

- [ ] T062 Verify `internal/domain/impersonation/{service,rule,subject_category}.go` fail closed by default, require explicit agent eligibility and all signed-role controls, keep unsigned subjects human-only under ADR 031 and ADR 034 audience guards, and never use identifier normalization, delegation failure, or fallback as a consent bypass (Constitution I; SR-001–SR-003).
- [ ] T063 Verify `internal/domain/oauth2server/strategies.go` and the unchanged signed credential validation paths retain vetted `jwx` cryptography, target-derived identity and protected `sub`/`agent_id`/actor claims, without custom crypto, plaintext fallbacks, weaker signing, or a new category claim (Constitution I, III; FR-004, FR-007, FR-010; SR-006).
- [ ] T064 Verify credential-free `impersonation_decision` success/failure audit fields in `internal/domain/impersonation/service.go` and `internal/adapters/http/enduser/oauth2_token.go`, and post-validation security-context finalization before third-party rejection in `internal/domain/tokenexchange/service.go`, using captured-log/security tests (Constitution I; SR-004; internal contracts §4–§5).

**Architecture Patterns** (Principle VI):

- [ ] T065 Verify classification, category checks and delegation branching live in `internal/domain/impersonation`, DTOs stay minimal in `internal/ports/`, and HTTP only parses/maps/logs; inspect `internal/app/builder.go` and `internal/adapters/http/routing/{admin,enduser}.go` to confirm existing Builder-only wiring/routes remain intact, with no new handler-to-port bypass, adapter cross-import, or fosite type leak (Constitution VI, XII; ADR 014).

**Testing** (Principle VIII - Unit & Integration Tests):

- [ ] T066 Verify unit/integration test history and recorded results in `specs/054-agent-subject-impersonation/quickstart.md` demonstrate compiling semantic-red tests before each behavioral change, green/refactor with minimal assertion changes, meaningful new-branch coverage, and no test-only production APIs or Bash-based correctness tests (Constitution VIII; plan Testing Strategy).
- [ ] T067 Run `just check` and `just test` after all Go/test/chart changes; run `go test ./internal/domain/impersonation/... ./internal/domain/oauth2server/... ./internal/domain/tokenexchange/... ./internal/config/... ./internal/adapters/http/enduser/... -cover` for focused coverage and `just test-integration` for the chart/config boundary, requiring passing non-skipped Helm cases; record outcomes in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution VIII; repository local-validation rules).

**E2E Acceptance Testing** (Principle XIII):

- [ ] T068 Run `ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/` and confirm all 12 mapped scenarios in `tests/e2e/agent_subject_impersonation_test.go` are green with independent fresh state and real HTTP/production initialization; preserve original acceptance assertions and record proof of no grants/sessions/provider calls in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution XIII; SC-001–SC-005).
- [ ] T069 Run `ginkgo -v --focus "OAuth2 User Impersonation|Local Mode Token Exchange" ./tests/e2e/` and `just test-e2e-backend`; require unchanged existing impersonation/local-exchange scenarios and all applicable backend E2E to pass, recording outcomes in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution VIII, XIII; FR-006, FR-010).
- [ ] T070 Run the plan's full `just verify` gate with all required tools/dependencies and record the actual outcome in `specs/054-agent-subject-impersonation/quickstart.md`; if required infrastructure is unavailable, preserve progress and report a blocker rather than skipping a suite or declaring delivery complete (Constitution VIII, XIII; plan Phase N).

**Frontend** (Principle XI - if applicable):

N/A under API-004. No frontend component, Storybook story, semantic-token/WCAG change, new Playwright journey, or screenshot is introduced. T053 verifies this scope; existing frontend checks still run when required by `just verify`.

---

## Acceptance Coverage

All locations below refer to `tests/e2e/agent_subject_impersonation_test.go`. The initial design work is T013–T017; scenario variants remain inside the single corresponding `It()`, with nearby `// USx.y from specs/054-agent-subject-impersonation/spec.md` references.

| Scenario | First test task | Behavioral/completion tasks | Observable assertion |
| --- | --- | --- | --- |
| US1.1 | T014 | T030–T034 | 200, verified signed `sub == agent_id == target UUID`, no category claim; no consent, grants, sessions created |
| US1.2 | T014 | T031–T034 | `act.iss` verified actor issuer, `act.sub` extracted actor, not client identity |
| US1.3 | T014 | T027, T031, T034 | Invalid client or actor signatures issue no token |
| US2.1 | T015 | T030, T035, T037, T039 | Agent-only mismatch and human-only signed collision deny, even with human grant |
| US2.2 | T015 | T031, T035, T037, T039 | Dual rule succeeds for agent without consent and human with delegation |
| US2.3 | T015 | T035, T037, T039 | Human missing delegation returns consent URI; UUID shape causes no retry |
| US2.4 | T015 | T035, T037, T039 | Omission keeps human behavior; signed equality denied with/without human grant |
| US2.5 | T015 | T019–T024, T036, T038, T039 | All invalid list shapes/values fail startup through actual production paths |
| US3.1 | T016 | T040, T043, T044 | CEL false means no token, irrespective of upstream authorization |
| US3.2 | T016 | T026, T040, T043, T044 | Missing/invalid/cross-agent/alias/noncanonical subject or unresolved target means no token |
| US3.3 | T016 | T040, T043, T044 | Runtime CEL failure yields server_error, never fallback or consent bypass |
| US3.4 | T016 | T041, T042, T044 | Minted agent token categorically denied for service exchange; same-text human state unused |

## Requirement Coverage

| Requirement | Tasks |
| --- | --- |
| FR-001 | T014, T027, T030–T034 |
| FR-002 | T006, T018–T024, T035–T039 |
| FR-003 | T005, T010, T015, T026, T030, T035, T037 |
| FR-004 | T005, T026–T028, T030–T032, T040, T043 |
| FR-005 | T003–T005, T014, T027, T031, T034, T061 |
| FR-006 | T015, T035, T037, T039, T056, T069 |
| FR-007 | T014, T027–T028, T031–T034, T063 |
| FR-008 | T016, T027, T031, T040, T043–T044 |
| FR-009 | T025, T030, T035, T037 |
| FR-010 | T028, T032, T040, T043, T063, T069 |
| FR-011 | T016, T019, T022–T025, T028, T032, T041–T044, T045–T046, T060–T061 |
| FR-012 | T007, T045–T046, T050, T056–T057 |
| CFG-001 | T006, T008–T009, T015, T018–T024, T036, T038 |
| CFG-002 | T018–T024, T035–T039, T059 |
| CFG-003 | T007–T009, T045, T050, T059–T060 |
| CFG-004 | T006–T007, T012, T045, T050 |
| API-001–API-004 | T003, T010, T012, T022, T025, T028, T041–T042, T046, T051, T053, T056–T057 |
| SR-001 | T026, T030–T031, T035, T037, T040, T043, T062 |
| SR-002 | T014, T026–T027, T031, T040, T062 |
| SR-003 | T003–T004, T020–T024, T026, T035, T038, T059, T062 |
| SR-004 | T029, T033, T064 |
| SR-005 | T016, T041–T044, T061 |
| SR-006 | T028, T032, T063 |
| SC-001–SC-006 | T007, T012, T034, T039, T044–T046, T050, T056, T068–T069 |

## Dependencies & Execution Order

### Phase Dependencies

1. Setup T001–T002 establishes prerequisites; T003 blocks all subsequent design execution and implementation until actual approvals are supplied.
2. Following T003, domain T004–T005, configuration T006–T009, API T010, and no-DB T011 can proceed as separate design streams. T008 precedes T009. T012 reviews the settled contracts/journeys.
3. E2E fixture T013 precedes T014 → T015 → T016 (same file; do not edit concurrently). T017 records semantic failures before foundation work. Minimal DTO-only scaffolding from T018 may be brought forward strictly to compile tests, after T003; no behavior is brought forward.
4. Foundation: T018 → parallel T019/T020/T021/T022. T019 → T023; T020/T021 → T024; T022 → T025. All foundation work is required before stories.
5. Story graph: `US1 (T026–T034) → US2 (T035–T039) → US3 (T040–T044)`. US2 and US3 share service/security files with US1, so sequential integration is intentional. US3's exchange-only T041 can be authored independently after foundation, but its full acceptance requires US1's minted token.
6. Polish T045/T046 may run together after contracts are settled and their story behavior is green. T047 consolidates evidence. Phase N T048–T070 verifies every gate and the complete feature, not merely the MVP.

### Within Each User Story

- Tests and observed semantic failures precede the behavior they drive. Existing already-green protections stay green; do not deliberately break them to manufacture red evidence.
- Classification/model changes precede service integration; service integration precedes full-journey completion. Claims/audit/exchange changes use their own first-written tests.
- Mark a task complete only with evidence. Stop for unresolved approvals, external dependencies, or failed checks; never reinterpret an implementation label as authority to decide or waive a gate.

### Parallel Opportunities and Examples

`[P]` means different files with no dependence on another incomplete task in that test/document batch, after phase prerequisites. It does not authorize parallel edits to shared files or starting green implementation before its tests fail.

- Foundation: T019 YAML tests, T020 config value tests, T021 domain rule tests, and T022 diagnostic tests use separate files.
- US1 example: after foundation, run T026 classification tests, T027 service tests, T028 mint tests, and T029 audit tests concurrently; then sequence T030 → T031 → T032 → T033 → T034. T030/T031/T033 share `service.go` and cannot run concurrently.
- US2 example: no same-story edit pair is independent because the matrix and startup/E2E cases integrate earlier shared files. Run T035 → T036 → T037 → T038 → T039; an independent reviewer may check the CFG-001 table in `contracts/impersonation-config.yaml` while T035 is authored, without editing service files.
- US3 example: T040 impersonation/CEL tests and T041 tokenexchange security tests can run together after US2; T042 and any T043 corrections affect different packages and can proceed after their respective red tests, then T044 integrates the journey.
- Documentation: T045 configuration documentation and T046 concept/reference documentation use separate files and can proceed together; do not parallelize T047 with tasks recording evidence in `quickstart.md`.

## Implementation Strategy

### MVP First (User Story 1 Only)

Complete Setup → actual T003 approvals → all Phase 2 design/red E2E work → foundation → US1. Demonstrate the complete signed self-agent exchange and attribution with no human state. Do not publish this partial increment as a completed feature: the categorical third-party denial, eligibility regression proof, and full compliance gate are mandatory.

### Incremental Delivery

1. US1 establishes the secured, explicitly eligible self-agent issuance path (`sub == agent_id`) with protected actor identity.
2. US2 proves the complete matrix, startup failures and retained human consent/terminal-denial behavior.
3. US3 proves policy/identity failures and blocks agent-subject tokens detected by the confirmed CEL contract from using human provider state, without adding a category claim.
4. Finish operator examples/docs and Phase N. Execute `just check`, `just test`, focused coverage, Helm/config integration, all 12 acceptance scenarios, unchanged regressions, `just test-e2e-backend`, and the plan's `just verify` before declaring delivery complete.

No new dependencies, entity boilerplate, schema changes, frontend work, or separate refactor PR is necessary. Required checks that need unavailable infrastructure remain blockers, not N/A or skipped successes.
