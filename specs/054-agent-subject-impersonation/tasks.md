# Tasks: Agent-Subject Impersonation

**Input**: Design documents from `/specs/054-agent-subject-impersonation/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`, and `adr-045-draft.md`.

**Tests**: Mandatory under Constitution VIII and XIII. Write tests first, compile them with only minimal structural scaffolding if necessary, demonstrate semantic failures, implement, then refactor with minimal test changes. Never use pending/skip markers, always-fail assertions, or red-phase comments in submitted tests.

**Organization**: Shared design and foundation precede three story increments: US1 (P1), US2 (P1), US3 (P2). All 72 tasks are unchecked; generating this list completes no implementation work. The acceptance design covers 12 scenarios (3 US1, 5 US2, 4 US3), and the requirement tables below trace every buildable requirement to tests and delivery work.

## 🔒 Feature Scope [MANDATORY]

- Every task below cites a feature requirement, acceptance scenario, or applicable constitutional constraint. Only work required for feature 054 belongs in this list.
- All nine rows A1–A9 in `plan.md` Approval Items remain **Proposed**. Earlier maintainer endorsements are rationale, not this pipeline's implementation approval. Only the implementation phase may use `context.approval` (the pipeline approval-comment URL, maintainers and approval times) to record sign-off. The re-plan resolved the ADR explanatory wording; no wording blocker remains. T001 must verify coverage of every approved API, ADR and consequential decision before code changes; absent or incomplete approval blocks implementation, not task generation.
- Do not amend binding ADRs, broaden unsigned credentials, add request inputs, or relax human delegation. No frontend, new persistence, third-party machine identity, unrelated refactoring, or new dependency is in scope.
- Unrelated findings require user approval before issue creation; approved links belong only in a non-checkbox Future Work note. Scope expansion requires explicit approval and updates to `spec.md` and this file.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish feature-local execution evidence using the existing repository tooling; no project initialization or dependency changes are needed.

- [ ] T001 Before any other implementation-phase task, verify `context.approval` covers every row A1–A9 of `specs/054-agent-subject-impersonation/plan.md` Approval Items, including both ADR 032 exceptions, configuration/OpenAPI/audit contracts, C4 replacement, no unsigned startup guard, and both backward-incompatible collision outcomes. Record the supplied approval in each approved ADR draft's Status line as `Accepted (approved by <login> on <date>: <url>)`, using only the supplied approving maintainers, dates and pipeline comment URL; move `specs/054-agent-subject-impersonation/adr-045-draft.md` to `adrs/045-agent-subject-impersonation.md` (or the next free number, without overwriting an existing ADR). Record API/decision approval coverage in `specs/054-agent-subject-impersonation/quickstart.md`. Preserve `spec.md` and `plan.md` unchanged throughout implementation; repair relocated-ADR links only in other feature artifacts. If any intended item is not covered, preserve progress and report the exact missing item; never treat earlier endorsements or task completion as approval (Constitution II, IV, X; FR-005, FR-011, API-002, SR-003, SR-005).
- [ ] T002 Confirm the selected directory is `specs/054-agent-subject-impersonation`, review `plan.md`, `research.md`, `data-model.md`, `contracts/`, and `quickstart.md`, and record execution evidence in `specs/054-agent-subject-impersonation/quickstart.md`; do not commit the local `.specify/feature.json` selection (Constitution II, Feature Scope).
- [ ] T003 Check the tool prerequisites from `justfile` for Go, Ginkgo, Helm, frontend assets, and the full verification gate; install existing tools with `just install-tools` as needed within sandbox permissions (no Docker-in-Docker or boundary changes), and record baseline `just check` / `just test` outcomes and unavailable external dependencies in `specs/054-agent-subject-impersonation/quickstart.md` without waiving failures (Constitution VIII; plan Testing Strategy).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation.

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete. Minimal scaffolding solely to compile tests is permitted by Constitution VIII; it must not implement behavior or bypass T001.

### Governance gate (enforced by the first Setup task)

**Decision status (planning re-run 2026-10-29)**: The ADR 045 draft is Proposed and its Context/Consequences now match C4. All items A1–A9 await the pipeline sign-off; C1 remains withdrawn. The clarification command `6480240156` aligned FR-011/SR-005/FR-012/SC-005/CFG-005 and US3.4 with replacement semantics. US3.4 covers default/compliant matches and detection failures with zero human-state/provider access even with same-text human state; the non-matching `false` override proves continuation through existing checks, not isolation or automatic success. No approval is recorded by this task-generation session.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management).

- [ ] T004 Following T001, verify the approved ADR now resides in `adrs/045-agent-subject-impersonation.md` (or its recorded free number), narrowly supersedes ADR 032 decisions 1 and 2 for agents only, preserves decisions 1–6 for eligible humans, and leaves ADRs 031/034 unchanged; add the actual accepted ADR reference to the `AGENTS.md` index and record its final path in `specs/054-agent-subject-impersonation/quickstart.md`. Do not modify binding ADRs or implementation-frozen `spec.md`/`plan.md`; their draft links can be resolved through this relocation record (Constitution II; research R1).
- [ ] T005 Document the reused Agent/UserGrant/UserSession entities, `SubjectCategory`, resolved subject on `ruleResult`, `sub == agent_id` agent-token identification (no category claim), the token-exchange `agent_subject_expression` detection step, and audit event invariants from `data-model.md` in `ARCHITECTURE.md` §3.1.y and the glossary; add Subject Category, Allowed Subject Categories, and Agent-Subject Token, explain byte-for-byte target equality and the deliberate human-only collision rejection, and align the `AGENTS.md` glossary; document T026–T027's two credential-free grant/session boundary spans and their request-context propagation under the existing ADR 011 provider pattern (FR-003–FR-007, FR-011; Constitution II, V).

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design).

- [ ] T006 Verify `specs/054-agent-subject-impersonation/contracts/impersonation-config.yaml` matches the T001-approved contract without changing its semantics, keeping CFG-001's confirmed key, case-sensitive enum, distinct 1–2 entries, omission-only default, and every shape/value outcome; preserve YAML-only rule lists and existing broker precedence rather than introducing ad-hoc loading; verify `specs/054-agent-subject-impersonation/contracts/token-exchange-agent-subject-config.yaml` against CFG-005 and T001's recorded C4 decision, retaining the exact guarded default, full replacement, startup/runtime outcomes, fail-closed collisions and failures, and explicit warning that a non-matching override removes categorical rejection and same-text human-credential protection (CFG-001–CFG-005, FR-011, FR-012; research R5/R8).
- [ ] T007 Create the complete `examples/config/impersonation-agent-subject.yaml` local-mode example with signed agent-only, human-only, and dual-category alternatives, explicit CEL subject/target binding and synthetic identities; update `examples/config/impersonation.yaml` with `[human]` and the omission note, and reference both from `examples/config/README.md`; document the optional `token_exchange.claim_extraction.agent_subject_expression`, its exact guarded default, full replacement semantics, startup/runtime failures, collision behavior, and the requirement to keep matching broker agent-subject tokens in `examples/config/token-exchange.yaml` per `specs/054-agent-subject-impersonation/contracts/token-exchange-agent-subject-config.yaml`; show a compliant disjunctive override and warn that non-matching overrides remove categorical rejection and same-text human-credential protection (FR-011, FR-012, CFG-003–CFG-005; Constitution VII).
- [ ] T008 Write table-driven Helm schema/render cases FIRST in `tests/integration/helm_chart_test.go` for the valid lists including reversed dual order and omission, invalid empty/unknown/case/null/scalar/mapping/non-string/duplicate values, and verbatim rendering, plus `tokenExchange.claimExtraction.agentSubjectExpression` accepted as a string, rejecting non-string values, rendering a non-empty expression unchanged as `agent_subject_expression` and omitting empty/unset values; run `go test ./tests/integration/ -run TestHelm` with Helm installed and record semantic failures for the new cases in `specs/054-agent-subject-impersonation/quickstart.md` before T009 (CFG-001, CFG-003, CFG-005; Constitution VII, VIII).
- [ ] T009 Update `charts/agentic-identity-broker/values.schema.json` with the confirmed optional array (`minItems: 1`, `maxItems: 2`, `uniqueItems: true`, string enum `human`/`agent`); document the key/default in `charts/agentic-identity-broker/values.yaml` and `README.md`, and verify unchanged `templates/configmap.yaml` passes the subtree through; add the optional `agentSubjectExpression` string to the strict `tokenExchange.claimExtraction` schema, render it conditionally in `templates/configmap.yaml`, and document its guarded default, full replacement semantics, fail-closed collisions/failures, and operator responsibility in `values.yaml`/`README.md`, explicitly warning that a non-matching override removes categorical rejection and same-text human-credential protection, turning T008 green (CFG-003, CFG-005, FR-012; research R5/R8; Constitution VII).

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development).

- [ ] T010 Only after T001 confirmation, apply `specs/054-agent-subject-impersonation/contracts/openapi-token-agent-subject.yaml` to `api/enduser/openapi.yaml`: category selection without request fields, no category claim (C1 withdrawn), generic category-denial C2, third-party denial C3 when `agent_subject_expression` matches (categorical under the default/compliant override), continuation under a non-matching replacing override, and fail-closed detection failures (C4), exact human-only collision behavior, credential validation, and concrete examples; record the supplied approval reference in `specs/054-agent-subject-impersonation/quickstart.md`, keep `spec.md`/`plan.md` unchanged, preserve established human responses, and confirm `api/admin/openapi.yaml` requires no change (API-001–API-004; Constitution IV, X).

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management).

- [ ] T011 Reconfirm in `specs/054-agent-subject-impersonation/data-model.md` that existing Agent resolution and human grant/session structures suffice: no new persisted entity, UUID type, repository, SQL, or migration; record why migration apply/rollback and new PostgreSQL adapter tasks are inapplicable (FR-004, FR-005, FR-011; Constitution IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency).

Not applicable: API-004 excludes frontend work. No new UI, universal component, tokens, Wireloom sequence, Playwright test, or screenshot is required. Existing consent URLs and UI remain unchanged. Reopen design review if scope is explicitly changed; do not add frontend work implicitly.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII (End-to-End Acceptance Testing & Spec Traceability), XIV (Specification by Example & Reviewable Journeys).

- [ ] T012 Review all three concrete journeys in `specs/054-agent-subject-impersonation/spec.md` against confirmed config/OpenAPI and decoded JWT behavior; record consistency evidence in `specs/054-agent-subject-impersonation/quickstart.md` for `sub == agent_id` identification (no category claim), the confirmed denial mappings, explicit signed credentials, target UUIDs, consent outcomes, and the same-text human collision fixture; if a requirement change is necessary, STOP for `/speckit-clarify` rather than modifying requirements during implementation (CFG-004, API-002; Constitution XIV).
- [ ] T013 Add reusable signed human/agent/dual rule builders and the second registered agent fixture in `tests/e2e/fixtures/impersonation_rules.go`, reusing `fixtures.LocalConfig`, `fixtures.ActiveGrant`, session fixtures, `helpers.MockJWKSServer`, and `helpers.ProvisionSigningKey`; use production `bootstrap.NewServerFactory(...).BuildApp(storage)` (US3.4 uses the existing `BuildAppWithTracerProvider` with `bootstrap.NewInMemoryTracerProvider`, telemetry/traces enabled, and the same concrete storage adapter), separate admin/end-user servers, and fresh storage/server lifecycle per scenario in `tests/e2e/agent_subject_impersonation_test.go` (US1.1–US3.4; Constitution VIII, XIII; ADR 007).
- [ ] T014 Write US1.1–US1.3 as exactly three scenario-referenced `It()` blocks in `tests/e2e/agent_subject_impersonation_test.go`: successful complete exchange without grants/sessions or consent, target `sub == agent_id` with no category claim, distinct client/actor with verified `act`, and invalid client/actor signature denials; assert real HTTP envelopes, signed output claims, and unchanged human state (FR-001, FR-004, FR-005, FR-007; SR-002; Constitution XIII).
- [ ] T015 Write US2.1–US2.5 as exactly five scenario-referenced `It()` blocks in `tests/e2e/agent_subject_impersonation_test.go`: category matrix, dual successes, no-delegation denial including nonmatching UUID variants, omitted-setting human regression and collision with/without grants, and invalid startup values/shapes via both `BuildApp` and production `bootstrap.LoadCredentialConfiguration` temporary YAML under the permitted temporary directory (`/tmp/gh-aw/agent/` in automation); keep fixture variants inside their corresponding scenario, not extra `It()` blocks (FR-002, FR-003, FR-006, FR-009; CFG-001; Constitution XIII).
- [ ] T016 Write US3.1–US3.4 as exactly four scenario-referenced `It()` blocks in `tests/e2e/agent_subject_impersonation_test.go`: CEL denial, invalid/cross-agent/alias/alternate-spelling subjects and unresolved target, runtime CEL evaluation failure, and mint-then-third-party-exchange US3.4 variants, all within its single `It()`. Trust the broker issuer and test each detector against both absent and same-UUID-text human grant/session fixtures: unset default and Journey 3 Step 3's compliant disjunctive override return `403 access_denied` with the confirmed description, diagnostic and no `error_uri`; override `false` requires positive downstream evidence with valid extraction/authorization/resource fixtures: no human state yields the existing missing-grant `403 access_denied`, grant-missing diagnostic, consent `error_uri` and grant boundary span; same-text active resource-covering grant plus usable session yields both named grant/session boundary spans. Also assert absence of the agent-subject description/diagnostic, without requiring automatic success or categorical rejection; dynamic override `subject_token.sub` starts but returns `500 server_error` at runtime. Use the plan's Human-state observation boundary: T026–T027 production spans captured via the existing tracer-provider bootstrap, a completed request-correlated HTTP span and positive human control per fixture, plus a mock provider, to prove zero grant/session boundary entry and zero provider/refresh calls in default/compliant/error cases; do not assert isolation for `false`. Keep recording repository fakes in unit tests only; no E2E storage wrapper, private-field injection, reflection, or custom wiring. The error-detector positive control uses the plan's sequential default-detector control broker, never non-string JWT subjects. Unit tests also cover evaluation errors/timeouts separately from non-boolean output (FR-008, FR-011, CFG-005; SR-005; Constitution XIII).
- [ ] T017 Compile and run the complete 12-scenario suite using `ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/` BEFORE behavioral implementation; record each new-behavior semantic failure in `specs/054-agent-subject-impersonation/quickstart.md`, with realistic status/error/claims/state assertions, no pending markers, no vacuous failures, and no red-phase comments; retain existing protections that already pass (Constitution VIII, XIII; plan Scenario Mapping).

**Checkpoint**: T001 approvals, architecture/glossary, config examples/Helm, confirmed OpenAPI, no-DB record, reviewed journeys, and complete compiling E2E assertions with demonstrated new-behavior failures are required before foundation or story implementation. No entity-boilerplate or pre-implementation-refactor phase is needed.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Extend existing boundary DTOs, diagnostics, and startup validation without implementing category-specific issuance. Every behavioral change follows a semantic red test.

- [ ] T018 Add only the minimal DTO fields/constants in `internal/ports/config.go` needed to compile subsequent tests: `AllowedSubjectCategories []string` with `mapstructure:"allowed_subject_categories"`, human/agent constants, and `ClaimExtractionConfig.AgentSubjectExpression string` with `mapstructure:"agent_subject_expression"`; its constraint is "Optional CEL boolean over `subject_token`", and "Empty or omitted means `tokenexchange.DefaultAgentSubjectExpression`"; leave `internal/ports/oauth2.go`, `ImpersonationMintInput`, and existing issuer/verifier interfaces unchanged (CFG-001; data-model Configuration; research R5; Constitution VI, VIII).
- [ ] T019 [P] Write and run failing YAML-shape tests in `internal/config/impersonation_shape_test.go`, through the production loader path where applicable, for omission versus explicit null, scalar, mapping, `[agent, 1]`, `[null]`, and valid string sequences; errors must name `oauth2_authorization_server.impersonation.rules[i].allowed_subject_categories` without document/credential content. Add loader round-trip and existing file/environment/CLI-precedence tests for `token_exchange.claim_extraction.agent_subject_expression` in `internal/config/loader_test.go`, including empty/omitted values remaining eligible for the evaluator default; retain the YAML-only rule-list boundary (CFG-001–CFG-003, FR-011; research R5/R8; Constitution VII, VIII).
- [ ] T020 [P] Write and run failing startup value-validation tables in `internal/config/impersonation_validator_test.go`: "`nil` means omitted, which defaults to `[human]`."; "A non-nil list must have 1–2 distinct entries from `{human, agent}`, matched case-sensitively."; include empty/unknown/case/duplicates, reversed valid order, and the R8 decision that a list containing `agent` with `roles.subject.verification: none` is accepted and does not fail startup (CFG-001, CFG-002, SR-003; data-model Configuration; Constitution VIII).
- [ ] T021 [P] Write and run failing direct-construction/rule-compilation tests in `internal/domain/impersonation/rule_test.go` for the same nil/non-nil constraints from T020, acceptance of an unsigned rule listing `agent` (R8, no startup guard), correct `allowsHuman`/`allowsAgent` flags, and unknown-category denial; keep unsigned human protections intact and prove unsigned subjects on such a rule are never agent-category (CFG-001, SR-003; data-model compiledRule; Constitution I, VIII).
- [ ] T022 [P] Write and run failing diagnostic catalogue tests in `internal/domain/tokenexchange/diagnostic_test.go` for `subject_category_ineligible` at `client_authorization`/`impersonation` and `subject_category_unsupported` at `identity_resolution`/`third_party`, with no new external OAuth error code; in `internal/domain/tokenexchange/cel_evaluator_test.go`, also write failing tables for `agent_subject_expression`: empty means the default, the default is true only when `sub` and `agent_id` are both present and equal (missing claims, unequal values, and type mismatches are false), an override replaces the default, a non-boolean result and an evaluation error/timeout are terminal `server_error` with the internal-contract diagnostics, and "a compile error or a static non-boolean output type fails startup" (permit `bool`/`dyn` as specified in internal contracts §2); include undeclared variables, a dynamic expression returning a non-boolean, and a bounded timeout case in `NewCELEvaluator`/`IsAgentSubject` tests (API-003, FR-011; contracts/internal-contracts.md §2 and §6; Constitution VIII).
- [ ] T023 Implement presence-sensitive YAML validation in `internal/config/impersonation_shape.go`, invoked by `internal/config/loader.go` before Viper weak decoding: present values must be sequence nodes containing scalar `!!str` entries, preserving omission. Turn all T019 tests green using the existing broker loader/precedence mechanisms for the optional detection expression; if its new DTO field already round-trips correctly, record that evidence instead of adding duplicate bindings or an ad-hoc configuration source (CFG-001–CFG-003, FR-011; research R5/R8; Constitution VII).
- [ ] T024 Implement `AllowedSubjectCategories` startup value checks in `internal/config/validator.go` and invariant checks/compiled category flags in `internal/domain/impersonation/rule.go`, turning T020–T021 green with the exact quoted constraints and field paths, adding no unsigned-subject guard (R8); retain existing issuer/CEL/audience validations (CFG-001, CFG-002; SR-003; research R8).
- [ ] T025 Add `DefaultAgentSubjectExpression` in `internal/domain/tokenexchange/constants.go`, compile `AgentSubjectExpression` in `NewCELEvaluator` and add `IsAgentSubject` in `internal/domain/tokenexchange/cel_evaluator.go`, pass the configured value into the token-exchange `CELEvaluatorConfig` in `internal/app/builder.go`, and add the two diagnostic details in `internal/domain/tokenexchange/diagnostic.go`, turning T022 green; add category-ineligible fall-through representation in `internal/domain/impersonation/errors.go` using existing no-match precedence rather than a terminal error. Compile the detector once at startup, enforce the existing evaluation timeout, and retain dynamic-result boolean checking and fail-closed error diagnostics; the shared approval-authentication evaluator must not invoke agent detection outside third-party token exchange (FR-009, FR-011, API-003; data-model Diagnostic details; research R3–R6; internal contracts §2).

- [ ] T026 Write and run compiling semantic-red tests in `internal/domain/tokenexchange/service_observation_test.go` for the plan's Human-state observation boundary: real consent/session services with recording repository fakes require child spans `tokenexchange.verify_agent_access` and `tokenexchange.get_valid_access_token` to start before their read-capable calls, carry the child context, end on success/missing/error returns, and preserve results, errors and cancellation. Cover missing grant, resource-covering grant/session retrieval, no-op tracing, and absence of sensitive attributes/events. Verify Exchange has no other grant/session access route; keep fakes in `_test.go` only (US3.4, FR-011, SR-005, SC-005; Constitution VIII, XIII; ADR 011).
- [ ] T027 Add only the two production child spans around `VerifyAgentAccess` / `GetValidAccessToken` in `internal/domain/tokenexchange/service.go`, via `otel.Tracer("tokenexchange")`, passing child context and ending immediately after each call before error mapping; turn T026 green without a telemetry port, SDK import, constructor change, storage wrapper or new production API. Preserve the existing Builder/production storage bootstrap and document the boundaries in `ARCHITECTURE.md` through T005/T060. Validate US3.4's observation setup with sampled HTTP traces and positive human controls before relying on zero boundary spans; T017's feature-red assertions may still fail until story work. Record focused `go test ./internal/domain/tokenexchange/...` and `just check` outcomes in `specs/054-agent-subject-impersonation/quickstart.md` (US3.4, FR-011, SR-005, SC-005; Constitution VIII, XIII; ADR 011).

**Checkpoint**: Startup constraints, diagnostics and the T026–T027 production observation boundary are tested and green; category-aware issuance remains to be implemented. Run `just check` and the matching package tests before story work, recording outcomes in `quickstart.md` under T049.

---

## Phase 3: User Story 1 - Obtain a token for an agent acting for itself (Priority: P1) 🎯 MVP

**Goal**: Signed, authorized self-agent requests mint attributable local broker tokens without consulting or creating human state.

**Independent Test**: US1.1–US1.3 complete real HTTP exchanges on a signed agent-only rule with no grants/sessions, verify `sub == agent_id == target UUID`, no category claim, and `act`, and prove invalid client/actor signatures cannot issue. Classification is wired with eligibility in this increment so there is no intermediate implicit consent bypass on human-only rules.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T028 [P] [US1] Write and run failing classification tables in `internal/domain/impersonation/subject_category_test.go`: only a verified signed raw string `claims["sub"]` byte-for-byte equal to `target.Agent.ID.String()` is agent; uppercase/braced/whitespace UUIDs, aliases, another agent UUID, missing/non-string claims, and unsigned subjects are not; prove `principal_expression` mapping another claim cannot cause agent classification (FR-003, FR-004; SR-002, SR-003; research R2).
- [ ] T029 [P] [US1] Write and run failing service tests in `internal/domain/impersonation/service_test.go` and `service_security_test.go` proving zero delegation calls/grant/session creation for agent requests, target-derived mint subject even when successful `principal_expression` extraction differs from raw verified `sub`, retained verified `act` including actor==subject, mandatory credential controls for all three signed roles, no email-expression evaluation or email passed to mint/CEL for agents, and no agent error consent URI (US1.1–US1.3; FR-005, FR-007, FR-010; SR-002).
- [ ] T030 [P] [US1] Write and run mint tables in `internal/domain/oauth2server/impersonation_mint_test.go`: an agent mint input (`Subject` = target UUID, nil `Email`) yields `sub == agent_id` with no category claim, CEL cannot set or change `sub` or `agent_id`, and authorization-code/human-token compatibility and issuer/signing/TTL/scope/policy-owned audience are unchanged; these characterize the unchanged strategy, so record when they already pass rather than forcing a production change (FR-004, FR-010, FR-011; data-model Token; research R5/R7).
- [ ] T031 [P] [US1] Write and run failing handler audit tests in `internal/adapters/http/enduser/oauth2_token_audit_test.go` for `subject_category` on classified success/denial, represented subject versus target/client/actor, available identities on preclassification failure, and absence of tokens/wholesale claims or extra attributes outside the allowlist (SR-004; contracts/internal-contracts.md §5).

### Implementation for User Story 1

- [ ] T032 [US1] Implement `SubjectCategory` and `classifySubject` in `internal/domain/impersonation/subject_category.go`, then wire classification/category eligibility immediately after principal extraction and before CEL in `internal/domain/impersonation/service.go`; use raw verified `sub`, never normalize or resolve a subject alias, and skip ineligible rules using T025 while retaining ordered no-match precedence (FR-003, FR-004, FR-009; research R2/R3; turns T028 green).
- [ ] T033 [US1] Extend `ruleResult` and service issuance in `internal/domain/impersonation/service.go` so only eligible, CEL-authorized agents skip `UserDelegationVerifier`, pass target UUID and `Email=nil` even when the successfully extracted subject principal differs, and retain verified actor identity/issuer even when equal; require T001 acceptance of both ADR 032 exceptions and no added extraction-equality restriction; do not evaluate agent email expressions, create human state, or weaken signed credential controls (FR-001, FR-004–FR-008; SR-002; turns T029 green).
- [ ] T034 [US1] Confirm with T030 that `GenerateImpersonationToken` and `baseClaims` in `internal/domain/oauth2server/strategies.go` need no change: agent tokens carry `sub == agent_id` from reserved base claims, no category claim is added, and a nil email mints no `email`; make only a minimal correction if T030 reveals a gap, without changing normal issuer/signing/scope/TTL/audience semantics (FR-007, FR-010, FR-011; research R5).
- [ ] T035 [US1] Add `AuditRecord.SubjectCategory` and represented-identity propagation in `internal/domain/impersonation/service.go`, and log only the allowed `subject_category` attribute alongside existing fields in `internal/adapters/http/enduser/oauth2_token.go`; emit credential-free decisions for success and failures after classification, retaining existing available-identity behavior before classification (SR-004; research R7; turns T031 green).
- [ ] T036 [US1] Run US1.1–US1.3 in `tests/e2e/agent_subject_impersonation_test.go` to green, plus the T028–T031 package tests and `just check`; record no-human-state and attribution evidence in `specs/054-agent-subject-impersonation/quickstart.md`, refactoring only feature-local duplication without changing acceptance assertions (SC-001, SC-002, SC-005; Constitution VIII, XIII).

**Checkpoint**: US1 is independently testable. It is an MVP demonstration, not permission to release partial work: categorical third-party protection and the remaining stories/compliance must complete before feature delivery.

---

## Phase 4: User Story 2 - Control subject eligibility without weakening human consent (Priority: P1)

**Goal**: Enforce the full human/agent matrix and safe defaults, including rejection of signed exact equality on human-only rules even with matching human grants.

**Independent Test**: US2.1–US2.5 exercise all six category/rule combinations, dual success, omission behavior, missing/expired human delegation with consent URI, terminal human denial, and invalid startup configurations. No human fallback may recover an ineligible agent request.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T037 [US2] Before any additional US2 correction, extend `internal/domain/impersonation/service_test.go` and `service_security_test.go` with matrix and rule-order tables: explicit/omitted human-only collisions with and without human grants, dual/nonmatching human paths, agent-only mismatch, later independently eligible rule success, no-match error precedence, terminal missing/expired/verifier-error human denials without retry, and unsigned human exact-text subjects retaining ADR 031 controls; verify missing behaviors fail semantically and record protections already green (US2.1–US2.4; FR-002, FR-003, FR-006, FR-009; SR-001, SR-003).
- [ ] T038 [US2] Run the US2.5 cases in `tests/e2e/agent_subject_impersonation_test.go` through production YAML loading and direct `BuildApp`, checking omission, both valid orders, every invalid CFG-001 shape/value, field paths, and startup acceptance of an unsigned rule listing `agent` (R8); retain the T017 red evidence and add a failing regression test before correcting any uncovered defect (CFG-001, CFG-002; SR-003; Constitution VIII, XIII).

### Implementation for User Story 2

- [ ] T039 [US2] Complete any behavior gaps exposed by T037 in `internal/domain/impersonation/service.go` and `errors.go`: classify signed exact equality as agent on every rule selection, fall through only under independent category/credential/policy eligibility, retain terminal human delegation denial and consent URI, and never retry an agent as human or a denied human as agent (FR-003, FR-006, FR-009; turns T037 green).
- [ ] T040 [US2] Complete any CFG-001/direct-config gaps exposed by T038 in `internal/config/impersonation_shape.go`, `validator.go`, and `internal/domain/impersonation/rule.go`, retaining the omission-only human default and no unsigned-subject guard (R8) without broadening existing config/ADR 031 behavior; if T024 already satisfies all cases, verify and record that evidence rather than add duplicate logic (US2.5; CFG-001, CFG-002; SR-003).
- [ ] T041 [US2] Turn all US2.1–US2.5 E2E cases green in `tests/e2e/agent_subject_impersonation_test.go`, then run `ginkgo -v --focus "OAuth2 User Impersonation|Local Mode Token Exchange" ./tests/e2e/` against unchanged `tests/e2e/impersonation_test.go` and `local_token_exchange_e2e_test.go`; record the six-combination outcomes and missing/expired human response compatibility in `specs/054-agent-subject-impersonation/quickstart.md` (SC-003, SC-004; FR-006; Constitution VIII, XIII).

**Checkpoint**: Existing nonmatching human behavior and intentional signed-UUID collision rejection are demonstrated independently; config fails before traffic for invalid values.

---

## Phase 5: User Story 3 - Authorize machine identity and keep third-party exchange out of scope (Priority: P2)

**Goal**: Fail closed on identity/policy/detection failures and protect human third-party state under the default detector or a compliant override; prove the operator-owned effect of replacing the detector with a non-matching expression, without implementing third-party agent identity.

**Independent Test**: US3.1–US3.4 cover denied CEL, unresolved/noncanonical/cross-agent identities, evaluation failure, and complete issuance followed by third-party exchange under four detector variants in both human-state conditions. Default/compliant matches yield `403 access_denied`; dynamic non-boolean detection yields `500 server_error`; all three consult zero human grants/sessions/providers. Override `false` instead continues through existing checks with no agent-subject rejection; no categorical isolation or automatic success is asserted.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T042 [P] [US3] Extend and run policy/identity tests in `internal/domain/impersonation/service_security_test.go` and `cel_test.go` for explicit `subject_token.sub` and `request.agent_id` CEL binding, false policy fall-through, terminal runtime errors/timeouts, failed extraction including non-string/empty identities, unresolved target/repository failure, cross-agent and alias rejection on agent-only rules, retained scope checks, and proxy/hybrid startup rejection; assert zero mint calls, adding a semantic red test before any needed correction (US3.1–US3.3; FR-008, FR-010; SR-001, SR-002).
- [ ] T043 [P] [US3] Write and run failing rejection tables in `internal/domain/tokenexchange/service_security_test.go`: a validated subject token matching `agent_subject_expression` (default: `sub == agent_id`) rejects before agent extraction, CEL authorization, grant/session/provider lookups; prove zero calls with recording fakes, finalized actor/calling-peer capture and diagnostic detail, no consent URI, and identical rejection for trusted broker/external issuers; a detection evaluation error is terminal and consults no human state; non-matching tokens (missing `agent_id` or `sub != agent_id`) keep existing exchange behavior, a compliant replacing override that keeps the default as a disjunct rejects both broker agent tokens and the external machine-token form while preserving ordinary human tokens, and a non-matching replacing override (for example `false`) demonstrates the accepted C4 replacement semantics as specified by FR-011/CFG-005 and clarification command `6480240156`, with the agent-subject description/diagnostic absent and existing checks reached in both human-state conditions; include evaluation-error, timeout and dynamic non-boolean failures with zero human-state access, plus fail-closed human-token collisions (legacy, mapped-principal, and ADR 031 unsigned-subject outputs with equal `sub`/`agent_id`). Never treat the non-matching case as evidence that the default guarantee holds (US3.4; FR-011, CFG-005; SR-005; research R5/R6; internal contracts §4).

### Implementation for User Story 3

- [ ] T044 [US3] Add the `IsAgentSubject` rejection step to `internal/domain/tokenexchange/service.go` after validated principal extraction and `security.FinalizeCaptureHolder`, BEFORE `ExtractAgentID`, CEL authorization, resource, grants, or sessions; propagate detection errors unchanged, continue the existing flow on `false` with no second built-in/default check, and on `true` return the confirmed C3 `403 access_denied` with `identity_resolution/subject_category_unsupported` and no `error_uri`, independent of issuer or human state (FR-011; SR-005; turns T043 green).
- [ ] T045 [US3] Make only minimal corrections, if T042 reveals a feature-path gap, in `internal/domain/impersonation/service.go`, `cel.go`, or `audience.go`, preserving existing denial/error distinctions and no-match precedence; use existing CEL context and target resolution rather than new policy variables or alias normalization, and record unchanged protections when tests already pass (FR-004, FR-008, FR-010; SR-001; turns T042 green).
- [ ] T046 [US3] Turn US3.1–US3.4 green in `tests/e2e/agent_subject_impersonation_test.go`; verify all four US3.4 detector variants from T016, each with absent and same-text human state. Record default/compliant `403` rejections and detection-error `500`, no consent URI, unchanged matching state and zero human-state/provider/refresh access; for `false`, require T016's positive missing-grant envelope/consent URI/grant span with no human state and both grant/session spans with matching usable human state, in addition to no agent-subject description/diagnostic. Use T027's sampled request-correlated traces and positive controls; unchanged storage or zero upstream calls alone do not prove zero reads. Record outcomes in `specs/054-agent-subject-impersonation/quickstart.md` (SC-005; FR-011, CFG-005; SR-005; Constitution XIII).

**Checkpoint**: All 12 acceptance scenarios pass, including all US3.4 variants. Default/compliant detection and detection failures prevent human-credential access; a non-matching replacement deliberately removes that guarantee without promising exchange success.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Finish operator documentation and validation evidence required by this feature, not unrelated improvements.

- [ ] T047 [P] Update `docs/configuration.md` with `allowed_subject_categories` enum/default/validation and complete linked examples, YAML-only rule-list boundary, signed identity and actor requirements, human-only collision rejection, and `token_exchange.claim_extraction.agent_subject_expression` with its default, startup/runtime failure semantics, fail-closed collision cases, the C4 replacement semantics, and the requirement that overrides keep matching broker agent-subject tokens (for example by keeping the default as a disjunct); explicitly warn that non-matching overrides such as `false` remove categorical rejection and protection against same-text human credential access; explain the no-effect `agent` setting on unsigned-subject rules (R8); preserve normal human consent guidance (FR-011, FR-012, CFG-003–CFG-005; research R5/R8/R11).
- [ ] T048 [P] Extend the existing token-endpoint documentation in `docs/api/enduser-api.md` with concrete agent-subject requests, decoded claims, the three category outcomes including human-only signed collision rejection even with delegation, and category/third-party denial examples aligned with the confirmed OpenAPI. Retain updates to `docs/concepts/token-exchange.md` and `docs/reference/token-exchange.md` for third-party restriction under the default/compliant detector, including colliding human and trusted external issuer tokens, confirmed errors/no consent recovery, both diagnostic details/stages/kinds, full replacement and fail-closed detection failures; explicitly warn that a non-matching override removes categorical rejection and same-text human-credential protection; cross-link the API, concept/reference guides, and configuration examples without adding a new endpoint (Constitution IV; FR-012, API-002, API-004; research R6/R11).
- [ ] T049 Update `specs/054-agent-subject-impersonation/quickstart.md` with actual commands, red/green evidence by task, 12-scenario coverage, Helm execution (not a skipped check), and approval references; keep remaining failures/dependencies explicit and never mark unexecuted or failed checks complete (Constitution VIII, XIII; plan Testing Strategy).

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

**These tasks MUST be included in every generated tasks.md file. They verify that all constitution principles have been followed.**

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist - verify Phase 2 tasks were completed correctly.

- [ ] T050 Verify every task and changed file traces to a feature requirement/scenario or applicable binding constraint, and unrelated findings/independent remediation are absent from `specs/054-agent-subject-impersonation/tasks.md` and the implementation diff; approved issue links may appear only as Future Work (Feature Scope & Deferred Findings).
- [ ] T051 Verify `ARCHITECTURE.md` documents all `data-model.md` entities/value objects/invariants and glossary terms, and the accepted superseding ADR plus its actual approval reference are present and indexed; ADRs 031/032/034 are not silently amended (Constitution II, V; FR-003–FR-007).
- [ ] T052 Verify configuration examples exist in `examples/config/impersonation-agent-subject.yaml` and `impersonation.yaml`, are referenced from `examples/config/README.md`, and match CFG-001 and `spec.md` Journeys 1–2; verify `examples/config/token-exchange.yaml` matches Journey 3 and CFG-005's guarded default, replacement semantics, collision/failure behavior and non-matching-override warning (Constitution VII, XIV; CFG-003–CFG-005).
- [ ] T053 Verify `api/enduser/openapi.yaml` matches confirmed C2–C4 with no category claim (C1 withdrawn), exact category identity semantics, existing human responses and API-004 scope; verify the supplied approval reference and A1–A9 coverage recorded in `specs/054-agent-subject-impersonation/quickstart.md`, keep `spec.md`/`plan.md` unchanged, and never treat task completion as confirmation (Constitution IV, X).
- [ ] T054 Verify the no-database-change decision in `specs/054-agent-subject-impersonation/data-model.md` still holds and no new entity/repository/migration entered scope; migration numbering, apply/rollback/data-integrity and new PostgreSQL adapter tasks are N/A only while that remains true (Constitution IX).
- [ ] T055 Verify all three journeys in `specs/054-agent-subject-impersonation/spec.md` remain linked to their scenarios with consistent config/HTTP/decoded claims and synthetic credential markers; reaffirm API-004 excludes UI/Wireloom/Playwright/screenshots and design-system changes (Constitution XI, XIII, XIV).
- [ ] T056 Verify `tests/e2e/agent_subject_impersonation_test.go` contains exactly one scenario-referenced `It()` for each of the 12 scenarios, uses hierarchical Ginkgo/Gomega and production bootstrap/fixtures, and the recorded semantic-red evidence in `quickstart.md` predates implementation; no pending markers, vacuous assertions, or red-phase annotations (Constitution VIII, XIII; ADR 007).

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist - verify all principles followed during implementation.

**Feature Scope** (Development Requirements):

- [ ] T057 Review `specs/054-agent-subject-impersonation/tasks.md` against the final implementation diff for feature-only corrections, no unauthorized issue creation, no third-party agent identity work, and no optional refactor/telemetry/chart additions (Feature Scope & Deferred Findings; FR-010, FR-011, API-004).

**API & Documentation** (Principles IV, X):

- [ ] T058 Verify actual token endpoint responses/claims against confirmed `api/enduser/openapi.yaml`, preserving all existing human error envelopes/consent URLs and no new request category field; verify `docs/concepts/token-exchange.md`, `docs/reference/token-exchange.md`, and configuration examples render the same contract (Constitution IV, X; API-001–API-004).
- [ ] T059 Verify `docs/api/enduser-api.md` renders the confirmed agent-subject token-endpoint contract with request examples, decoded claims, eligibility and human-only signed collision rejection, and category/third-party denial examples; verify consistency with OpenAPI and discoverability through cross-links with `docs/concepts/token-exchange.md`, `docs/reference/token-exchange.md`, and configuration examples, and record evidence in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution IV; research R11; FR-012).

**Architecture & Documentation** (Principle II):

- [ ] T060 Verify `ARCHITECTURE.md` §3.1.y, token-exchange flow, and glossary describe the implemented classification/eligibility/CEL/delegation sequence, `sub == agent_id` identification and `agent_subject_expression` detection, audit, scope and performance characteristics, narrow accepted ADR supersession, and T026–T027's two grant/session observation boundaries without SDK imports or test-only production APIs; synchronize `AGENTS.md` index/glossary without altering binding decisions (Constitution II, V; FR-003–FR-011).

**Configuration** (Principle VII):

- [ ] T061 Verify the feature uses `internal/ports/config.go` and the existing broker loader/validator, not an ad-hoc config source, preserves file/env/CLI precedence and the established YAML-only rule list, and applies both loader and direct-domain startup validation, including the recorded R8 decision that adds no unsigned-subject guard (Constitution VII; CFG-001–CFG-003; SR-003).
- [ ] T062 Verify `charts/agentic-identity-broker/{values.yaml,values.schema.json,README.md}` match the confirmed settings/defaults, that `templates/configmap.yaml` renders the impersonation subtree unchanged, and that it renders `agent_subject_expression` only when set; execute `go test ./tests/integration/ -run TestHelm` with Helm available and require the new cases to run and pass, not skip (Constitution VII, VIII; CFG-003, CFG-005, FR-012).

**Database & Persistence** (Principle IX):

- [ ] T063 Verify `internal/domain/impersonation/service.go` never creates grants/sessions for agents and `internal/domain/tokenexchange/service.go` rejects tokens matching `agent_subject_expression` or detection failures before any human-state lookup, while a non-matching replacement continues existing checks with no additional default guard; confirm the diff has no persistence/schema change requiring both adapters or migration testing and document N/A in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution IX; FR-005, FR-011; SR-005).

**Security** (Principles I, III):

- [ ] T064 Verify `internal/domain/impersonation/{service,rule,subject_category}.go` fail closed by default, require explicit agent eligibility and all signed-role controls, keep unsigned subjects human-only under ADR 031 and ADR 034 audience guards, and never use identifier normalization, delegation failure, or fallback as a consent bypass (Constitution I; SR-001–SR-003).
- [ ] T065 Verify `internal/domain/oauth2server/strategies.go` and the unchanged signed credential validation paths retain vetted `jwx` cryptography, target-derived identity and protected `sub`/`agent_id`/actor claims, without custom crypto, plaintext fallbacks, weaker signing, or a new category claim (Constitution I, III; FR-004, FR-007, FR-010; SR-006).
- [ ] T066 Verify credential-free `impersonation_decision` success/failure audit fields in `internal/domain/impersonation/service.go` and `internal/adapters/http/enduser/oauth2_token.go`, and post-validation security-context finalization before third-party rejection in `internal/domain/tokenexchange/service.go`, using captured-log/security tests (Constitution I; SR-004; internal contracts §4–§5).

**Architecture Patterns** (Principle VI):

- [ ] T067 Verify classification, category checks and delegation branching live in `internal/domain/impersonation`, DTOs stay minimal in `internal/ports/`, and HTTP only parses/maps/logs; inspect `internal/app/builder.go` and `internal/adapters/http/routing/{admin,enduser}.go` to confirm existing Builder-only wiring/routes remain intact, with no new handler-to-port bypass, adapter cross-import, or fosite type leak (Constitution VI, XII; ADR 014).

**Testing** (Principle VIII - Unit & Integration Tests):

- [ ] T068 Verify unit/integration test history and recorded results in `specs/054-agent-subject-impersonation/quickstart.md` demonstrate compiling semantic-red tests before each behavioral change, green/refactor with minimal assertion changes, meaningful new-branch coverage, and no test-only production APIs or Bash-based correctness tests (Constitution VIII; plan Testing Strategy).
- [ ] T069 Run `just check` and `just test` after all Go/test/chart changes; run `go test ./internal/domain/impersonation/... ./internal/domain/oauth2server/... ./internal/domain/tokenexchange/... ./internal/config/... ./internal/adapters/http/enduser/... -cover` for focused coverage and `just test-integration` for the chart/config boundary, requiring passing non-skipped Helm cases; record outcomes in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution VIII; repository local-validation rules).

**E2E Acceptance Testing** (Principle XIII):

- [ ] T070 Run `ginkgo -v --focus "Agent-Subject Impersonation" ./tests/e2e/` and confirm all 12 mapped scenarios in `tests/e2e/agent_subject_impersonation_test.go` are green with independent fresh state and real HTTP/production initialization; preserve original acceptance assertions and record proof of no human-state creation for issuance and zero grants/sessions/provider calls under default/compliant detection and detection failures, using T026–T027 boundary spans with completed sampled HTTP traces and positive controls (not E2E storage spies), plus T016's positive missing-grant envelope/consent URI/grant span without human state and both grant/session spans with matching usable human state for `false`, in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution XIII; SC-001–SC-005, CFG-005).
- [ ] T071 Run `ginkgo -v --focus "OAuth2 User Impersonation|Local Mode Token Exchange" ./tests/e2e/` and `just test-e2e-backend`; require unchanged existing impersonation/local-exchange scenarios and all applicable backend E2E to pass, recording outcomes in `specs/054-agent-subject-impersonation/quickstart.md` (Constitution VIII, XIII; FR-006, FR-010).
- [ ] T072 Run the plan's full `just verify` gate with all required tools/dependencies and record the actual outcome in `specs/054-agent-subject-impersonation/quickstart.md`; if required infrastructure is unavailable, preserve progress and report a blocker rather than skipping a suite or declaring delivery complete (Constitution VIII, XIII; plan Phase N).

**Frontend** (Principle XI - if applicable):

N/A under API-004. No frontend component, Storybook story, semantic-token/WCAG change, new Playwright journey, or screenshot is introduced. T055 verifies this scope; existing frontend checks still run when required by `just verify`.

---

## Acceptance Coverage

All locations below refer to `tests/e2e/agent_subject_impersonation_test.go`. The initial design work is T013–T017; scenario variants remain inside the single corresponding `It()`, with nearby `// USx.y from specs/054-agent-subject-impersonation/spec.md` references.

| Scenario | First test task | Behavioral/completion tasks | Observable assertion |
| --- | --- | --- | --- |
| US1.1 | T014 | T032–T036 | 200, verified signed `sub == agent_id == target UUID`, no category claim; no consent, grants, sessions created |
| US1.2 | T014 | T033–T036 | `act.iss` verified actor issuer, `act.sub` extracted actor, not client identity |
| US1.3 | T014 | T029, T033, T036 | Invalid client or actor signatures issue no token |
| US2.1 | T015 | T032, T037, T039, T041 | Agent-only mismatch and human-only signed collision deny, even with human grant |
| US2.2 | T015 | T033, T037, T039, T041 | Dual rule succeeds for agent without consent and human with delegation |
| US2.3 | T015 | T037, T039, T041 | Human missing delegation returns consent URI; UUID shape causes no retry |
| US2.4 | T015 | T037, T039, T041 | Omission keeps human behavior; signed equality denied with/without human grant |
| US2.5 | T015 | T019–T024, T038, T040, T041 | All invalid list shapes/values fail startup through actual production paths |
| US3.1 | T016 | T042, T045, T046 | CEL false means no token, irrespective of upstream authorization |
| US3.2 | T016 | T028, T042, T045, T046 | Missing/invalid/cross-agent/alias/noncanonical subject or unresolved target means no token |
| US3.3 | T016 | T042, T045, T046 | Runtime CEL failure yields server_error, never fallback or consent bypass |
| US3.4 | T016 | T022, T025, T043, T044, T046, T026–T027 | Default/compliant: 403; error: 500; no grant/session boundary spans or provider calls, with live HTTP traces and positive controls; `false`: missing-grant envelope/consent URI/grant span without human state, both grant/session spans with matching state; no agent rejection |

## Requirement Coverage

| Requirement | Tasks |
| --- | --- |
| FR-001 | T014, T029, T032–T036 |
| FR-002 | T006, T018–T024, T037–T041 |
| FR-003 | T005, T010, T015, T028, T032, T037, T039 |
| FR-004 | T005, T028–T030, T032–T034, T042, T045 |
| FR-005 | T001, T004–T005, T014, T029, T033, T036, T063 |
| FR-006 | T015, T037, T039, T041, T058, T071 |
| FR-007 | T014, T029–T030, T033–T036, T065 |
| FR-008 | T016, T029, T033, T042, T045–T046 |
| FR-009 | T025, T032, T037, T039 |
| FR-010 | T030, T034, T042, T045, T065, T071 |
| FR-011 | T016, T019, T022–T025, T030, T034, T043–T046, T047–T048, T062–T063, T026–T027 |
| FR-012 | T006–T009, T047–T048, T052, T058–T059, T062 |
| CFG-001 | T006, T008–T009, T015, T018–T024, T038, T040 |
| CFG-002 | T018–T024, T037–T041, T061 |
| CFG-003 | T007–T009, T047, T052, T061–T062 |
| CFG-004 | T006–T007, T012, T047, T052 |
| CFG-005 | T006–T010, T016, T018–T019, T022–T025, T043–T044, T046–T048, T052, T061–T063, T070 |
| API-001–API-004 | T001, T010, T012, T022, T025, T030, T043–T044, T048, T053, T055, T058–T059 |
| SR-001 | T028, T032–T033, T037, T039, T042, T045, T064 |
| SR-002 | T014, T028–T029, T033, T042, T064 |
| SR-003 | T001, T004, T020–T024, T028, T037, T040, T061, T064 |
| SR-004 | T031, T035, T066 |
| SR-005 | T016, T043–T046, T063, T070, T026–T027 |
| SR-006 | T030, T034, T065 |
| SC-001–SC-006 | T007, T012, T036, T041, T046–T048, T052, T058, T070–T071, T026–T027 |

## Dependencies & Execution Order

### Phase Dependencies

1. T001 is the first task: verify all A1–A9 coverage using only `context.approval`, record the supplied approval, and relocate the approved ADR. Then Setup T002–T003 establishes selection/tooling/baseline evidence. No implementation-phase design or code work precedes the approval gate.
2. Following T001, domain T004–T005, configuration T006–T009, API T010, and no-DB T011 can proceed as separate design streams. T008 precedes T009. T012 reviews the settled contracts/journeys.
3. E2E fixture T013 precedes T014 → T015 → T016 (same file; do not edit concurrently). T017 records semantic failures before foundation work. Minimal DTO-only scaffolding from T018 may be brought forward strictly to compile tests, after T001; no behavior is brought forward.
4. Foundation: T018 → parallel T019/T020/T021/T022. T019 → T023; T020/T021 → T024; T022 → T025. After T017, run T026 → T027 before story work or using E2E negative span assertions as evidence; T026/T027 edit the same service area as T043/T044 and must not run concurrently with them. All foundation work is required before stories.
5. Story graph: `US1 (T028–T036) → US2 (T037–T041) → US3 (T042–T046)`. US2 and US3 share service/security files with US1, so sequential integration is intentional. US3's exchange-only T043 can be authored independently after foundation, but its full acceptance requires US1's minted token.
6. Polish T047/T048 may run together after contracts are settled and their story behavior is green. T049 consolidates evidence. Phase N T050–T072 verifies every gate and the complete feature, not merely the MVP.

### Within Each User Story

- Tests and observed semantic failures precede the behavior they drive. Existing already-green protections stay green; do not deliberately break them to manufacture red evidence.
- Classification/model changes precede service integration; service integration precedes full-journey completion. Claims/audit/exchange changes use their own first-written tests.
- Mark a task complete only with evidence. Stop for unresolved approvals, external dependencies, or failed checks; never reinterpret an implementation label as authority to decide or waive a gate.

### Parallel Opportunities and Examples

`[P]` means different files with no dependence on another incomplete task in that test/document batch, after phase prerequisites. It does not authorize parallel edits to shared files or starting green implementation before its tests fail.

- Foundation: T019 YAML tests, T020 config value tests, T021 domain rule tests, and T022 diagnostic tests use separate files.
- US1 example: after foundation, run T028 classification tests, T029 service tests, T030 mint tests, and T031 audit tests concurrently; then sequence T032 → T033 → T034 → T035 → T036. T032/T033/T035 share `service.go` and cannot run concurrently.
- US2 example: no same-story edit pair is independent because the matrix and startup/E2E cases integrate earlier shared files. Run T037 → T038 → T039 → T040 → T041; an independent reviewer may check the CFG-001 table in `contracts/impersonation-config.yaml` while T037 is authored, without editing service files.
- US3 example: T042 impersonation/CEL tests and T043 tokenexchange security tests can run together after US2; T044 and any T045 corrections affect different packages and can proceed after their respective red tests, then T046 integrates the journey.
- Documentation: T047 configuration documentation and T048 concept/reference documentation use separate files and can proceed together; do not parallelize T049 with tasks recording evidence in `quickstart.md`.

## Implementation Strategy

### MVP First (User Story 1 Only)

Complete T001 approval recording/ADR relocation → remaining Setup → all Phase 2 design/red E2E work → foundation → US1. Demonstrate the complete signed self-agent exchange and attribution with no human state. Do not publish this partial increment as a completed feature: the default/compliant-detector third-party denial, non-matching replacement coverage, eligibility regression proof, and full compliance gate are mandatory.

### Incremental Delivery

1. US1 establishes the secured, explicitly eligible self-agent issuance path (`sub == agent_id`) with protected actor identity.
2. US2 proves the complete matrix, startup failures and retained human consent/terminal-denial behavior.
3. US3 proves policy/identity/detection failures, blocks default/compliant-detector matches from using human provider state, and separately proves non-matching replacement continuation without categorical protection or promised success; no category claim is added.
4. Finish operator examples/docs and Phase N. Execute `just check`, `just test`, focused coverage, Helm/config integration, all 12 acceptance scenarios, unchanged regressions, `just test-e2e-backend`, and the plan's `just verify` before declaring delivery complete.

No new dependencies, entity boilerplate, schema changes, frontend work, or separate refactor PR is necessary. Required checks that need unavailable infrastructure remain blockers, not N/A or skipped successes.
