# Tasks: AuthZEN Delegation Policy Information Point

**Input**: `specs/054-authzen-delegation-pip/{spec.md,plan.md,research.md,data-model.md,quickstart.md,contracts/}`

**Prerequisites**: Revised `plan.md`, `research.md`, `data-model.md`, and `contracts/`, including the 2026-10-28 decisions recorded in [remediation.md](./remediation.md). The preceding planning phase records maintainer design direction for the Proposed AuthZEN contract and R16 ADR decision set, not formal sign-off, and resolves I1/U1/U2 and the R15 input shape. This task list records those decisions; it makes no new stakeholder decision or ADR acceptance. T001 approval recording, API-003 final-standard verification and the separately integrated Phase 0 refactor remain implementation gates. All 101 tasks remain unchecked; no implementation or passing runtime validation is claimed.

**Tests**: Mandatory under Constitution VIII and XIII. Write tests first, compile them with minimal nonfunctional scaffolding where needed, observe semantic failure, implement, then refactor with tests green. Do not use pending/skipped tests, placeholder failures, or test-only production APIs. Preserve existing exchange tests unchanged.

**Organization**: Shared setup and mandatory design gates precede four story phases, ordered P1, P1, P2, P2. Paths are repository-relative. `[P]` means different files and no dependency on another unfinished task in the same execution wave; it never overrides an approval or red-test gate.

## 🔒 Feature Scope [MANDATORY]

Every task cites a requirement, scenario, research decision implementing that requirement, or binding principle. No new broker configuration, broker Helm values, persistence, UI, actor-token support, batch evaluation, resource search, or exchange-field removal is included. The three deferred findings in `research.md` are not implementation tasks. Do not file issues without affirmative maintainer approval.

**Execution gates**:

- Apply the recorded freshness decisions exactly: unusable `max-age` fails closed; compute `Deadline = requestStart + min(max-age, max_staleness)` once before sending the evaluation POST; preserve it through cache hits, singleflight delivery and request state; reject expiry before every policy evaluation with 503, without re-resolution. No current-request-only permit, `Age` accounting, separate grant-expiry cap, or stale fallback is authorized (I1/U1/U2; research R13–R14).
- T001 records the maintainer's 👍 on the pipeline's approval comment for the plan's Approval Items rather than accepting an ADR on the maintainer's behalf. T013 verifies the final standard against the signed-off contract, including denial form (a). Any conflict with that contract, the clarified direct OPA input, or binding ADRs requires a stakeholder handoff; do not silently select a replacement representation. Later exchange-field removal still requires a separately approved `2.0.0` contract and is not a task here.
- Constitution II requires Phase 0 refactoring in a **separate PR**. This workflow must not create that PR; T002 requires a maintainer-arranged handoff. If unavailable, leave the extraction and its dependents unchecked and report the blocker; continue independent approved work rather than combining refactoring and feature code here.
- The numbered Phase 0 appears first for review visibility, not permission to bypass Phase 2 design gates. Execution order is specified under Dependencies below.
- This task-generation session edits only feature documents. Tasks naming application code, ADRs, architecture/agent documentation, or deployment artifacts are work for a later appropriately authorized session, not changes made now.

---

## Approval prerequisite

- [ ] T001 As the first implementation task, verify that `context.approval` covers every applicable API, ADR and Decision in `plan.md`'s Approval Items (API-1–API-6, ADR-1, D-1–D-4); record the supplied URL, approving maintainers and approval times in `specs/054-authzen-delegation-pip/quickstart.md`. For each covered feature-local ADR draft, record the maintainer's approval from the pipeline approval comment (`context.approval`: URL, approving login, date) in the Status line of `specs/054-authzen-delegation-pip/adr-045-authzen-delegation-pip.md` as `Accepted (approved by <login> on <date>: <url>)`, then move it to `adrs/045-authzen-delegation-pip.md` after rechecking that 045 is the next unused number. Never record an approval the comment does not supply. Confirm the draft covers the information boundary, unchanged CEL grant type, fixed broker cap, I1/U1/U2 deadline semantics, fourth ExtProc port, ADR 028 enabled-mode ordering/input supersession with the clarified R15 typed/direct members and permanent permit-derived compatibility fields, ADR 033 audit-seam extension/early fallback (I2; R9), opt-in rationale, ADR 036 boundary and staged deprecation. Leave uncovered work unchecked with its reason and continue independent approved tasks if an item is missing or the draft deviates; do not amend binding ADRs (API-004, API-005; Principle II).

## Phase 0: Pre-implementation Refactoring

**Purpose**: FR-006's required structural extraction, isolated for review with no behavior change. Execute only after the design approvals and separate-PR handoff, before feature scaffolding. New behavior such as anchor checking belongs to US1, not this extraction.

- [ ] T002 Record the maintainer-arranged separate refactoring PR and its integration/review handoff in `specs/054-authzen-delegation-pip/quickstart.md`; if unavailable, defer this task and dependent extraction/feature work, continue independent approved tasks and report the blocker without creating another PR (Constitution II, Feature Scope; FR-006).
- [ ] T003 Run the existing tests in `internal/domain/tokenexchange/service_test.go`, `service_diagnostic_test.go`, `service_classification_test.go`, `service_security_test.go`, and `service_expiry_regression_test.go` unchanged, plus the exchange E2E regression files listed in `plan.md`; record a green baseline in `specs/054-authzen-delegation-pip/quickstart.md` before extraction (FR-006; R1).
- [ ] T004 In the separately authorized refactoring PR, extract exchange steps 2–9a into `internal/domain/tokenexchange/delegation.go` and call that resolver from `internal/domain/tokenexchange/service.go`; move observation/stage/service/agent/grant bookkeeping and `WithObservation` unchanged, retain session retrieval/refresh/decryption, session-scope checks and response construction downstream, and adjust `internal/app/builder.go` for the shared dependency without new behavior (FR-006, FR-013; R1).
- [ ] T005 Run `just check`, `just test`, and the existing exchange E2E regression journeys against the separate extraction; record the integrated refactor revision and unchanged diagnostic/session behavior in `specs/054-authzen-delegation-pip/quickstart.md` before starting feature implementation (FR-006; SR-004; Constitution II, VIII).

**Checkpoint**: Separate refactoring reviewed and integrated; existing exchange tests remain green without changed assertions. No AuthZEN behavior is implemented in Phase 0.

---

## Phase 1: Setup (Shared Infrastructure)

- [ ] T006 Confirm the Go 1.27.3 and existing toolchain commands from `justfile` (`just install-tools`, `just check`, `just test`, `just extproc-test`, E2E commands, `just docs-build`, `just verify`); record validation prerequisites in `specs/054-authzen-delegation-pip/quickstart.md`: US2.19 needs real PostgreSQL and `Label("docker")`, belongs to `just test-e2e-docker`, and is excluded from `just test-e2e-backend`; `just test-e2e` includes both. Docker-in-Docker is unavailable in this sandbox: preserve the required infrastructure gate for a capable runner and report a blocker if it cannot run, never bypass it (Constitution VIII, XIII; R18).
- [ ] T007 Record the FR/SR/API/SC-to-task and 44-scenario-to-test mapping in `specs/054-authzen-delegation-pip/tasks.md` and the implementation evidence locations in `specs/054-authzen-delegation-pip/quickstart.md`; retain exclusions and the three deferred findings without expanding scope (Feature Scope; SC-001–SC-006).

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

**🔒 CONSTITUTION REQUIREMENT**: This phase maps directly to the constitution PRECONDITIONS checklist. All sub-phases (2a-2f) MUST be included in every tasks.md, though specific task details should be adapted to the feature.

Only compile-enabling nonfunctional scaffolding is allowed before the Phase 2f semantic-red checkpoint, as expressly allowed by VIII/XIII. Complete design approvals before that scaffolding. Phase 0's behavior-preserving extraction follows its separate review gate, not permission to implement new behavior.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [ ] T008 Verify the existing aggregates, typed IDs, resolver/request/outcome/context value objects, active-grant invariants, projection rules and permit-cache transitions in `specs/054-authzen-delegation-pip/data-model.md`; retain no new entity, repository, domain event, or bounded context (FR-006–FR-009, FR-013, FR-015; Principles V, VI, IX).
- [ ] T009 Add Delegation Decision, Delegation Context and PDP identifier to `ARCHITECTURE.md`'s glossary, the fourth delegation policy-gate row, routes and independent-cache/order descriptions in §§3.1.4 and 3.2.1–3.2.3, explicitly reconciling §3.1.4.2 with the approved evaluation deferred-finalization seam and unchanged ordinary/token-route behavior; index the correctly numbered ADR in `ARCHITECTURE.md` and `AGENTS.md` with the approval provenance recorded by T001 (FR-001, FR-006, FR-014–FR-017; Principles II, V).

**Checkpoint**: Domain model and invariants documented; architectural decision explicitly accepted by the stakeholder before code work.

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [ ] T010 [P] Document all four `delegation_context` keys, file/env/flag precedence, enabled-only validation, origin default and path-prefix override in `docs/configuration.md` using `specs/054-authzen-delegation-pip/contracts/extproc-delegation-context.md` §1 (FR-014, FR-018; R17).
- [ ] T011 [P] Add `examples/config/extproc-delegation-context.yaml` with enabled usage and documented defaults (`false`, origin-derived PDP, `5s`, `60s`), existing authorization dependency and synthetic credentials; index it in `examples/config/README.md` (FR-018; Principle VII).
- [ ] T012 [P] Document ADR 011's standalone `EXTPROC_` ownership in `docs/docker-compose-setup.md` and add a commented opt-in block to `configs/config.extproc.docker.yaml`; explicitly record no new broker keys and no change to `charts/agentic-identity-broker/` because it does not deploy ExtProc (FR-001, FR-018; Principle VII).

**Checkpoint**: Configuration requirements designed with YAML examples and the applicable deployment contract documented.

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

- [ ] T013 Verify discovery members, omitted unsupported endpoints, path insertion, request/response semantics, request-ID rules and reason envelopes against final AuthZEN 1.0; record authoritative sources, the 2026-10-28 design direction and the actual T001 sign-off in `specs/054-authzen-delegation-pip/contracts/authzen-evaluation-api.md`. Preserve accepted strictness, plain-UUID anchor, principal/expiry projection, denial form (a), error mapping and `1.1.0` additive deprecation; later `2.0.0` removal is separately approved follow-up only. Check Journeys 1–3 in `specs/054-authzen-delegation-pip/spec.md` against the accepted representation without changing requirements. If the verified standard conflicts with the approved contract or requirements, leave affected tasks unchecked, continue independent approved work and report exactly what renewed sign-off is needed; do not alter `spec.md` or `plan.md`, freeze an unverified OpenAPI or retry TLS insecurely (API-001–API-006; R6, R8, R10–R12).
- [ ] T014 After T013 final-standard verification, apply the exact confirmed two-route delta, schemas, security requirements, all statuses/headers/examples, `ClientAssertion` description and API version to `api/enduser/openapi.yaml`; retain the admin API unchanged and stage only the approved additive deprecation, never removal (API-001, API-002, API-006; FR-019; Principles IV, X).
- [ ] T015 Write `docs/api/authzen-evaluation.md`, link it from `docs/api/enduser-api.md`, and run `just docs-build`; include all confirmed shapes, caller-CEL reuse, scope-contribution versus effective-ceiling distinction, freshness, non-consent versus outage, path-prefix setup and Journeys 1–3 (FR-002–FR-013; API-004; Principles IV, X, XIV).

**Checkpoint**: APIs designed and confirmed by user/stakeholder; no unresolved API-003 Verify items remain. Changing a confirmed representation later requires renewed approval.

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [ ] T016 Confirm in `specs/054-authzen-delegation-pip/data-model.md` that existing repositories suffice, all evaluation is read-only, no schema/migration/new adapter or integration-migration suite is required, and the existing PostgreSQL outage journey remains mandatory (FR-013; US2.19; Principle IX).

**Checkpoint**: No database change; do not generate migrations or CRUD scaffolding.

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

- [ ] T017 Confirm the no-UI scope in `specs/054-authzen-delegation-pip/plan.md`: existing consent/revocation flows are reused, so design-system components, Wireloom sequences, Playwright changes and screenshot additions are not applicable (FR-013, FR-019; Principles XI, XIII, XIV).

**Checkpoint**: No new frontend components or UI states.

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII (End-to-End Acceptance Testing & Spec Traceability), XIV (Specification by Example & Reviewable Journeys)

All tests below use production bootstrap, Ginkgo/Gomega `Describe → Context → It`, fresh fixtures/storage/servers per test, and nearby `// Scenario USx.y from specs/054-authzen-delegation-pip/spec.md` references. Use the exact descriptions in `plan.md`'s scenario table. Each scenario gets one `It()`, including individual denial rows; variants within a single scenario stay inside its test. Add no skipped/pending tests or red-phase annotations. Write these before business behavior; T027 executes them after the Phase 2.5 compile-only scaffolding.

- [ ] T018 [P] Add request/discovery helpers in `tests/e2e/helpers/authzen.go` using existing JWT/assertion helpers and fixture factories; capture response headers/bodies and buffered JSON audit output without introducing direct DI in scenarios (US1–US2; SR-003; R18).
- [ ] T019 [P] Add counting/failing/mismatched `MockAuthZENBroker` in `tests/e2e/extproc/bootstrap/authzen_broker.go` and extend `tests/e2e/bootstrap/extproc.go` with the delegation-context option using production constructors; retain real broker composition for consumer journeys, metadata-only inputs, and clean shutdown (US3–US4; R18; Principles XII, XIII).
- [ ] T020 [P] Add `tests/e2e/fixtures/policies/delegation_canonical.rego` matching `input.context.agent.canonical_id == "data-assistant"`, `input.context.resource.canonical_id == "repo-api"`, and `input.context.permission_sets[].canonical_id == "repo-reader"` without UUIDs. Give distinct fixed denial reasons for a defined `delegation`, `delegation_available`, or `subject` member, or availability other than `true`. Add two isolated deployment fixture sets through `tests/e2e/fixtures/agents.go`, `grants.go`, and `services.go` for both request paths (US3.1–US3.2, US3.8; FR-017–FR-018; SC-005; R18).
- [ ] T021 [P] Write eight `It()` blocks for US1.1–US1.8 in `tests/e2e/authzen_delegation_test.go` with label `authzen-delegation`: discovery, matching/omitted anchors, no provider session versus exchange, scope ceiling, normalized URI parity, expiry-bound freshness, configured-only routes and canonical omission at every nesting level (FR-001–FR-009, FR-013; SC-001).
- [ ] T022 [P] Write ten separate `It()` blocks for US2.1–US2.10 in `tests/e2e/authzen_delegation_denial_test.go` with label `authzen-delegation`, exercising grant establishment/revocation or one changed condition per journey and asserting each exact diagnostic, confirmed consent-recovery envelope, 200 false decision, no-store, request-ID echo and no credential/permit data (FR-010, FR-012; SR-003).
- [ ] T023 [P] Write ten `It()` blocks for US2.11–US2.20 in `tests/e2e/authzen_delegation_errors_test.go` with label `authzen-delegation`; assert 400/401/403/500, challenges, no-store, ID echo, no credentials, no grant read on mismatch, same CEL rejection as exchange, and a real PostgreSQL grant-table outage with cleanup; place `Label("docker")` on US2.19 only, retaining the shared `authzen-delegation` label and no local skip fallback (FR-003–FR-005, FR-011–FR-012; SR-001–SR-004).
- [ ] T024 [P] Write ten `It()` blocks for US3.1–US3.10 plus one for US4.5 in `tests/e2e/delegation_context_journey_test.go` with label `delegation-context`, using a real broker and ExtProc: both policy paths, consent/revocation/re-consent, still-cached provider tokens, broker outage, pair isolation, canonical-ID parity, policy denial and provider-session failure; assert gRPC responses/mutations and call counts on both paths with the R15 direct members and permanent permit-derived compatibility inputs. Use T020's canonical policy across both UUID sets and short local freshness with `Eventually`, not sleeps or test-only production clocks; exact typed/unknown-member and pre-policy expiry checks belong to T049–T052 unit tests (FR-014–FR-018; SC-002–SC-005).
- [ ] T025 [P] Write four startup `It()` blocks for US4.1–US4.4 in `tests/e2e/extproc/delegation_context_startup_test.go` with label `delegation-context-startup`, observing production startup/serving behavior, authorization dependency, unavailable/invalid discovery and PDP mismatch with zero evaluation credential transmission (FR-014, FR-018; SR-005; SC-006).
- [ ] T026 [P] Write the US4.6 `It()` in `tests/e2e/authzen_delegation_compat_test.go` with label `authzen-delegation`, asserting a full exchange still returns unchanged `principal`, `agent_id` and populated deprecated `granted_permission_sets`, plus contract deprecation evidence (FR-019; SC-006).
- [ ] T027 After T029–T031 compile-only scaffolding, compile and run all 44 new scenarios, recording actual semantic failures and scenario-to-file:line mapping in `specs/054-authzen-delegation-pip/quickstart.md`; assertions must target missing feature behavior (not compilation/infrastructure failure), existing disabled-mode expectations remain valid, and no placeholder, `XIt`/`PIt`/`Skip()` or red-phase test comments are allowed (Principles VIII, XIII).
- [ ] T028 Review all four linked journeys in `specs/054-authzen-delegation-pip/spec.md` against the confirmed OpenAPI, R15 typed/direct input, I1/U1/U2 freshness and configuration; record verification in `specs/054-authzen-delegation-pip/quickstart.md`. Check explicit synthetic JWT claims, status/header/body pairs, canonical/expiry omission, scope semantics and permanently retained permit-derived OPA fields; record any conflict as deferred work and continue independent approved tasks rather than changing signed-off requirements. No new UI or output-JWT wireframes/claims are required because evaluation emits no token (Principle XIV; API-002–API-003).

**Checkpoint**: Concrete journeys reviewed, all 44 acceptance tests written before business logic, compile successfully and demonstrate semantic red evidence where new behavior is absent. Existing compatibility expectations must not be made artificially red.

---

## Phase 2.5: Foundational Infrastructure (Compile-only Scaffolding)

**Purpose**: Enable Phase 2f tests to compile after design approval and separately integrated extraction. These tasks may run while acceptance tests are being authored; they must complete before T027. They are the limited scaffolding exception, not business implementation. Phase 2.7 is omitted: no new persisted entities or CRUD repositories.

- [ ] T029 Add compile-only request/context/outcome types and evaluation signatures in `internal/domain/tokenexchange/evaluation.go` plus the optional anchor field and mismatch sentinel in `internal/domain/tokenexchange/delegation.go` and `errors.go`; no new resolution or evaluation behavior yet (FR-003–FR-008; data-model §§2.1–2.5; Principles VIII, XIII).
- [ ] T030 Add stub handlers returning 501 in `internal/adapters/http/enduser/authzen_configuration.go` and `authzen_evaluation.go`, their fields in `internal/app/handlers.go`, and minimal production builder/routing seams in `internal/app/builder.go` and `internal/adapters/http/routing/enduser.go` so E2E assertions compile and reach nonfunctional handlers (FR-001; Principles VIII, XII, XIII).
- [ ] T031 Add compile-only `DelegationContextConfig` fields in `internal/extproc/config/config.go`, `DelegationResolver` port/server option in `internal/extproc/server/server.go`, the five direct context members and policy-input types (`AgentContext`, `ResourceContext`, `GrantContext`, `PermissionSetContext`, `ServiceContext`) in `internal/extproc/authorization/input.go` and `input_builder.go`, typed `delegation.Context`/`Result` signatures with `Deadline`, and nonfunctional constructors in `internal/extproc/delegation/{discovery.go,client.go,resolver.go}`; update production composition signatures in `cmd/extproc-token-exchange/root.go` without implementing discovery/cache/order behavior (FR-014–FR-018; Principles VIII, XIII).

**Checkpoint**: Compile-only seams exist. T027 must complete before any story behavior below. No new test-only production API, custom crypto, or broker imports in ExtProc.

---

## Phase 3: User Story 1 - A gateway obtains current delegation information (Priority: P1) 🎯 MVP

**Goal**: Discover and return credential-free permits using shared authoritative resolution, without provider-session access.

**Independent Test**: US1.1–US1.8 via real broker end-user server and existing fixtures, with no ExtProc/provider session required. US2's failure protections are required before the broker increment is releasable.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

> Tests MUST be written FIRST and compile and FAIL semantically before implementation begins. Acceptance tests are already authored in T021.

- [ ] T032 [P] [US1] Write resolver parity/anchor tests in `internal/domain/tokenexchange/delegation_test.go`: unchanged stages/details/observations and call ordering, `ExpectedAgentID` optional comparison after agent load and before grant read (spy), verified subject/assertion, unchanged CEL resource/grant type/absent scope, scope ceilings, included-service/zero-scope semantics and no session dependency; run to semantic red for new anchor behavior (FR-004–FR-006, FR-013; SR-001, SR-004).
- [ ] T033 [P] [US1] Write permit/projection/freshness tests in `internal/domain/tokenexchange/evaluation_test.go` with one batched canonical-ID lookup, lookup failure as 500, omission at every level, normalized URI, sorted scopes, grant/service ordering, included-only contributions and zero-scope arrays; cover no expiry, >60s, 15s, 0.9s and expired boundaries without exported test-only clocks (FR-007–FR-009; SR-003; R4–R5).
- [ ] T034 [P] [US1] Write discovery and permit HTTP contract tests in `internal/adapters/http/enduser/authzen_configuration_test.go` and `authzen_evaluation_test.go`: confirmed JSON, public-base derivation, Content-Type, private max-age and request-ID behavior (US1.1–US1.2, US1.6; FR-002, FR-009, FR-012).
- [ ] T035 [P] [US1] Write nil-handler/configured-only/shared-resolver builder tests in `internal/app/builder_test.go` and namespace/SPA-isolation tests in `internal/adapters/http/routing/enduser_test.go`; verify `/access` is always reserved and disabled routes yield 404 instead of SPA HTML (US1.7; FR-001; Principles VI, XII).
- [ ] T036 [US1] Run T032–T035 tests with the existing package/`just` test commands and record semantic-red assertions in `specs/054-authzen-delegation-pip/quickstart.md` before T037–T040; do not alter existing regression assertions (Principles VIII, XIII).

### Implementation for User Story 1

- [ ] T037 [US1] Implement the optional anchor in `internal/domain/tokenexchange/delegation.go` and `errors.go` with `ExpectedAgentID` compared before grant access; retain `DelegationRequest` constraints: `SubjectToken` and `ClientAssertion` are “Required”, `Resource` is “Required. Must be an absolute URI”, `GrantType` is “Always `urn:ietf:params:oauth:grant-type:token-exchange`”, and evaluation `Scope` is `""`; exchange passes no anchor (FR-004–FR-006; data-model §2.1).
- [ ] T038 [US1] Implement `AccessEvaluationService` permit projection in `internal/domain/tokenexchange/evaluation.go`: `DelegationContext` optional members are “omitted when unset”, “never `null`, never synthesized, and never replaced by the UUID”; only “grant-included” services appear; permission sets follow grant order, services follow `IncludedServiceIDs` order and scopes are “sorted lexicographically”; raw contributions remain distinct from final `resource.scopes`, missing non-requested definitions give `[]`, absent `valid_until` is omitted; reuse typed IDs and batch canonical reads (FR-007–FR-008, FR-013; data-model §2.3; R4).
- [ ] T039 [US1] Implement freshness in `internal/domain/tokenexchange/evaluation.go`: `MaxAge: int (0..60)`, `clamp(floor(valid_until − now), 0, 60)` or `60` without expiry; recheck expiry before permitting and never round subsecond time up; use package-local/test-file clock control only (FR-009; data-model §2.4; R5; US1.6).
- [ ] T040 [US1] Implement discovery/permit transport in `internal/adapters/http/enduser/authzen_configuration.go` and `authzen_evaluation.go`, wire the shared resolver and handlers only under `tokenExchangeEnabled` in `internal/app/builder.go` and `handlers.go`, and register both end-user-only routes with reserved `/access` and machine-facing audit middleware in `internal/adapters/http/routing/enduser.go`; no principal-header authentication or service instantiation in routing (FR-001–FR-002, FR-012–FR-013; R7–R9; Principle XII).
- [ ] T041 [US1] Run `just check`, matching unit suites and US1.1–US1.8 in `tests/e2e/authzen_delegation_test.go`; confirm no-session exchange comparison, canonical omission and zero credential leakage in bodies/logs/spans, then record green results in `specs/054-authzen-delegation-pip/quickstart.md` (SC-001, SC-004; Principles VIII, XIII).

**Checkpoint**: Independently useful permit/discovery MVP is testable; do not ship until US2 fail-closed paths also pass.

---

## Phase 4: User Story 2 - A gateway distinguishes delegation denial from request failure (Priority: P1)

**Goal**: Preserve exact diagnostics and consent recovery while separating caller/request/dependency failure from user non-consent.

**Independent Test**: Each of US2.1–US2.20 changes one condition from a known valid delegation and verifies status, headers, bounded reason and absence of permit/credential disclosure through the real broker.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

> Tests MUST be written FIRST and compile and FAIL semantically before implementation begins. Acceptance tests are already authored in T022–T023.

- [ ] T042 [P] [US2] Extend `internal/domain/tokenexchange/evaluation_test.go` with the complete R6 failure table, nine allowlisted grant details covering ten denial scenarios, `StageGrantAuthorization`/`AccessDenied` requirement, and all non-grant failures including repository/JWKS/CEL/configuration/cancellation failures; prove neither arbitrary `grant_*` text nor missing agents/resource ambiguity becomes consent denial (FR-010–FR-011; SR-002–SR-004).
- [ ] T043 [P] [US2] Extend `internal/adapters/http/enduser/authzen_evaluation_test.go` with the required `Content-Type: application/json`, 64 KiB overflow, malformed/trailing JSON, missing/null/wrong-type fields, unsupported subject/action/resource and confirmed unknown-member rejection, absent/object context and UUID anchor, missing/non-Bearer/invalid assertion, all outcome/status/body/header mappings and no resolution on decoding failure (FR-003–FR-005, FR-010–FR-012; R6, R10; confirmed contract).
- [ ] T044 [US2] Add request-ID echo on discovery/evaluation success and every error path, no echo of an internally generated audit ID, `WWW-Authenticate` cases and credential-free audit/span assertions in `internal/adapters/http/enduser/authzen_evaluation_test.go` and `authzen_configuration_test.go`. Add test-first shared-perimeter coverage in `internal/adapters/http/middleware/security_context_test.go` and the composed HTTP harness: evaluation receives a deferred holder; permits and caller-CEL denials after validation finalize exactly once with verified subject Actor, distinct assertion CallingPeer and one trace ID shared by context/audit/access log/`traceresponse`; equal identities omit CallingPeer. Test decode/missing/invalid credential fallback before in-handler audit, no invented verified identity, and unchanged ordinary HTTP/non-exchange token/delegated exchange semantics. Run T042–T044 to semantic red and record evidence in `specs/054-authzen-delegation-pip/quickstart.md` (FR-012; SR-003–SR-004; R7/R9; HIGH I2).

### Implementation for User Story 2

- [ ] T045 [US2] Implement request invariants and outcome classification in `internal/domain/tokenexchange/evaluation.go`: `subject.type` “Exactly `urn:ietf:params:oauth:token-type:jwt`”, `subject.id` “Non-empty string”, `action.name` “Exactly `urn:ietf:params:oauth:grant-type:token-exchange`”, `resource.type` “Exactly `protected_resource`”, `resource.id` “Absolute URI”, `context` “Object or absent” and `context.agent` “Agent UUID string”; map the R6 table to Permit/Deny/RequestFailure, only allowlisted grant-authorization errors deny, and build consent URLs from public base/resolved agent (FR-003, FR-005, FR-010–FR-011; data-model §§2.4–2.5).
- [ ] T046 [US2] Complete bounded JSON decoding, required Content-Type handling, bearer extraction and confirmed denial/error serialization in `internal/adapters/http/enduser/authzen_evaluation.go`: body “limited to 64 KiB”, unknown members “Rejected with 400” only as confirmed at T013, fixed ErrorResponse messages and 400/401/403/500, Bearer challenge and invalid-token challenge, no-store on every non-permit, supplied ID echo on all paths; preserve shared resolver subject-before-assertion ordering (FR-003–FR-005, FR-010–FR-012; R6, R10–R11).
- [ ] T047 [US2] After T001 records the maintainer approval and T044 supplies semantic-red evidence, extend the shared deferred-route matcher in `internal/adapters/http/middleware/security_context.go` to `POST /access/v1/evaluation`, without route-local capture or changes to ordinary/token behavior. Let the shared resolver finalize verified Actor/CallingPeer before caller authorization; use the existing idempotent request-principal/anonymous helper on early failures before in-handler audit, retaining the perimeter fallback. Add credential-free evaluation audit/span outcomes in `internal/adapters/http/enduser/authzen_evaluation.go` with `operation = access_evaluation`; prove T044's identity/trace assertions rather than only absence of credentials. Do not log raw claims, credentials or dependency errors (SR-003–SR-004; R7/R9; HIGH I2; Principle I).
- [ ] T048 [US2] Run `just check`, `just test`, and US2.1–US2.20 in `tests/e2e/authzen_delegation_denial_test.go` and `authzen_delegation_errors_test.go`, including actual PostgreSQL outage/cleanup; rerun US1 permits and unchanged exchange diagnostics, and record green results in `specs/054-authzen-delegation-pip/quickstart.md` (SC-001, SC-004, SC-006; Principles VIII, XIII).

**Checkpoint**: Broker increment is complete only with both P1 stories green and required approvals recorded.

---

## Phase 5: User Story 3 - ExtProc supplies fresh context before every policy decision (Priority: P2)

**Goal**: Give both policy paths a bounded-freshness permit independently of token-cache lifetime, with no stale or failure fallback.

**Independent Test**: US3.1–US3.10 use real broker/ExtProc composition, canonical-ID policy, controlled short freshness, consent/revocation and cached provider credentials, asserting no forwarding unless every gate succeeds.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

> Tests MUST be written FIRST and compile and FAIL semantically before implementation begins. Acceptance tests are already authored in T024.

- [ ] T049 [P] [US3] Write AuthZEN client tests in `internal/extproc/delegation/client_test.go` for the exact body, omitted request context, existing assertion provider, timeout/TLS transport, boolean decision, allowlisted denial code/absolute http(s) consent URL and non-200/malformed replies. Test typed context validation: every ID, `subject.id` and `resource_uri` is a “non-empty string”; scope/service members “are arrays. Empty arrays are allowed”; `permission_sets` “is a non-empty array”; timestamps “parse as RFC 3339” and retain received strings; optional `canonical_id`/`valid_until` is “absent or a non-empty string”, never explicit null/empty. Drop unknown fields at every nesting level, preserve absent optional members, and reject each missing/wrong-type/invalid value with uncached `invalid_response`. Missing/zero/malformed/duplicate/non-integer `max-age` must yield 503 with no use or cache, including the current request; positive values including >60 use `min(max-age, max_staleness)`. With controlled time and a delayed response, prove `Deadline = requestStart + min(max-age, max_staleness)` uses the clock read just before POST, once, without `Age` accounting or a separate grant-expiry cap (FR-014–FR-017; data-model §§3.3–3.3.1; I1/U1; SR-003, SR-005).
- [ ] T050 [P] [US3] Write cache tests in `internal/extproc/delegation/resolver_test.go` for exact pair isolation/separator safety, original-deadline preservation on insertion/hit/singleflight delivery, a hit only while `now < Deadline`, no hit at/after expiry, no denial/error caching, immediate re-consent, no stale-on-error even with a usable exchange cache, concurrent singleflight, eviction and shutdown. A delayed response must not acquire a fresh TTL at insertion or delivery; expired results cannot reach policy. Run with race coverage and keep clock helpers in `_test.go` (FR-015; SR-002; data-model §3.4; U1/U2).
- [ ] T051 [P] [US3] Write policy-input tests in `internal/extproc/authorization/input_builder_test.go` for body/batch/header-only serialization of `principal`, `agent`, `resource`, `grant` and `permission_sets` directly in `input.context`, with optional `canonical_id`/`valid_until` “omitted, never null” and empty scope/service arrays `[]`. Assert no `delegation`, `delegation_available`, or `subject` key, permanent permit-derived permission-set inputs with availability `true`, and unchanged `agent_session_id`. Pin disabled body/header-only JSON byte-for-byte, omitting all five new members while retaining existing availability semantics; preserve existing MCP `json.Number` behavior, not a verbatim permit map (FR-017–FR-018; data-model §3.5; R15).
- [ ] T052 [P] [US3] Write server tests in `internal/extproc/server/delegation_test.go` using port stubs and synchronization barriers: wait for both concurrent calls without sibling cancellation, deny > resolver error > exchange error > success in either completion/timeout/cancellation order, no policy/forwarding on failure, and header-only resolve→policy→exchange with no exchange on policy deny. Test expiry while waiting for exchange, buffering a body, between batch decisions, and before header-only policy: `now >= Deadline` returns 503 `service_unavailable` / `delegation context unavailable`, no policy call for the expired decision, no forwarding/retry/re-resolution. Use an unexported clock controlled only by `_test.go`. Test `contextInputFromDelegation` maps `subject.id` to `principal`, deep-copies every slice, sets all five direct members and availability `true`, derives compatibility fields from permits despite conflicting exchange extensions, and preserves `agent_session_id` and exchange-derived approval identities. Cover unknown-field exclusion, disabled zero resolver calls and unchanged metadata/protocol/body-limit/approval gates (FR-015–FR-018; SR-002, SR-004, SR-006; R14–R15; U2).
- [ ] T053 [US3] Run T049–T052 to semantic red using matching package tests and `just extproc-test`; record concrete expected failures in `specs/054-authzen-delegation-pip/quickstart.md` before implementation (Principles VIII, XIII).

### Implementation for User Story 3

- [ ] T054 [US3] Implement `internal/extproc/delegation/client.go` with the verified advertised endpoint, existing assertion provider, traced/TLS HTTP client, timeout and accepted request/denial shapes. Decode into an unexported wire struct, ignore unknown members at every level, validate every quoted T049/data-model §3.3.1 constraint, then return immutable `Result{Context Context, Deadline time.Time}` with typed `Context.Principal` mapped from `subject.id`; no verbatim map or `UseNumber` context decoding. Require exactly one positive integer `max-age`; all unusable freshness and malformed contexts are `invalid_response`, never usable or cached. Compute `Deadline = requestStart + min(max-age, max_staleness)` once from the pre-POST clock read; never recalculate from receipt or parse `Age`/apply a separate grant-expiry cap. Preserve timestamp strings and optional omissions, allowlist `DeniedError`, and bound timeout/unavailable/rejected errors without logging credentials/context or permitting non-200 responses (FR-014–FR-017; data-model §§3.3–3.3.1; R13/R15; I1/U1; SR-003, SR-005).
- [ ] T055 [US3] Implement `internal/extproc/delegation/resolver.go`: key `cacheKey{subjectToken string; resourceURI string}`, entry `{result Result}` expiring at the original `result.Deadline`; lookup only while `now < Deadline`. Insertion, hits and singleflight followers never extend or recompute that deadline; share immutable results. Never cache “Denials, errors, and non-200 responses”; unusable freshness produces no `Result` or entry. Use `sync.RWMutex`, separator-safe singleflight and sweep “every `max(max_staleness/2, 1s)`” stopped by `Shutdown`; never serve expired entries or borrow exchange fallback (FR-015; data-model §3.4; U1; ADR 012 boundary).
- [ ] T056 [US3] Extend `internal/extproc/authorization/input.go` with typed `AgentContext`, `ResourceContext`, `GrantContext`, `PermissionSetContext` and `ServiceContext`; add direct `principal`, `agent`, `resource`, `grant` and `permission_sets` members with `omitempty`, nested optional `canonical_id`/`valid_until` omitted, and scope/service arrays always arrays (`[]` when empty). Update `input_builder.go` so `BuildOPAInputHeadersOnly` accepts a supplied `ContextInput`. Implement pure `contextInputFromDelegation(result, agentSessionID)` in `internal/extproc/server/server.go`, mapping typed permit principal and all documented values, deep-copying every slice, and setting permanently retained `{ps.ID: [ps.Services[].ID]}` plus `granted_permission_sets_available: true` from the permit, never exchange extensions. Preserve `agent_session_id`; emit no `delegation`, `delegation_available`, `subject` or unknown member. Disabled mode never calls the mapping and retains byte-for-byte input, omitting all five additions (FR-017–FR-018; data-model §3.5; R15; T001 decision record).
- [ ] T057 [US3] Implement enabled body-path concurrency and state in `internal/extproc/server/server.go` after existing metadata/protocol/transport checks: start exchange and resolve together, wait for both without sibling cancellation, apply R14 precedence, and store typed context plus its unchanged `Deadline` in `requestState`. Immediately before body policy and every batch decision require `now < Deadline`; otherwise return 503 `service_unavailable` / `delegation context unavailable`, with no policy call for that decision, forwarding or re-resolution, including expiry during exchange or body buffering. Build every input through T056's mapping, mutate Authorization only on successful header processing, and preserve exchange-error/approval behavior and metadata provenance (FR-015–FR-017; U2; SR-002, SR-004, SR-006).
- [ ] T058 [US3] Implement enabled header-only resolve→deadline check→typed context mapping→policy→exchange ordering and bounded 403 consent-denial/503 resolver-error responses in `internal/extproc/server/server.go`. Require `now < Deadline` immediately before policy; on expiry return the same 503 with no policy, exchange, forwarding, retry or re-resolution. Keep no exchange on policy denial, no availability-false continuation, permit-derived compatibility fields with all five direct members before policy, and the unchanged disabled path (FR-015–FR-018; U2; SR-002, SR-006; ExtProc contract §§3–4).
- [ ] T059 [US3] Add credential-free operation/outcome/cache-hit attributes in `internal/extproc/delegation/client.go`, `resolver.go` and `internal/extproc/server/server.go` using existing logging/tracing vocabulary; test captured output in their `_test.go` files contains no tokens/assertions/context (SR-003–SR-004; ExtProc contract §5).
- [ ] T060 [US3] After US4 startup wiring (T066–T069), run `just check`, `just extproc-test` with race coverage and US3.1–US3.10 in `tests/e2e/delegation_context_journey_test.go`; verify the canonical policy across two UUID sets and revocation/outage despite cached exchanged tokens, then record green evidence in `specs/054-authzen-delegation-pip/quickstart.md` (SC-002–SC-005).

**Checkpoint**: Both paths enforce independent permit freshness and existing independent gates. US3 can be unit-tested before US4, but real startup journeys require its config/discovery wiring.

---

## Phase 6: User Story 4 - Operators roll out discovery and migrate safely (Priority: P2)

**Goal**: Safe opt-in startup and discovery with disabled-mode compatibility and documented staged deprecation.

**Independent Test**: US4.1–US4.4 exercise production startup/default traffic with a counting metadata server; US4.5 proves origin discovery plus advertised endpoint through real broker composition; US4.6 proves unchanged exchange identity/extensions.

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

> Tests MUST be written FIRST and compile and FAIL semantically before implementation begins. Acceptance tests are already authored in T024–T026.

- [ ] T061 [P] [US4] Write defaults/precedence/validation tests in `internal/extproc/config/loader_test.go` and `validate_test.go` for all four keys, exact error substrings, missing authorization, invalid URL/userinfo/query/fragment/scheme, permitted local http only under existing allow_http, nonpositive durations, origin with port and explicit path override, and disabled validation bypass without discovery calls (FR-018; data-model §3.1; R17).
- [ ] T062 [P] [US4] Write discovery tests in `internal/extproc/delegation/discovery_test.go` for unavailable/non-200/invalid/missing-member metadata, exact PDP equality including path preservation, standard-verified discovery path, absolute endpoint/host/userinfo/fragment/TLS checks, credentials absent on discovery and mismatch, and use of the advertised endpoint (FR-002, FR-014; SR-005; confirmed API-003).
- [ ] T063 [P] [US4] Write production composition/startup tests in `cmd/extproc-token-exchange/root_test.go` for disabled zero calls, enabled discovery before listening, actionable credential-free failure and resolver Shutdown lifecycle; use test-file helpers rather than test-only production APIs (US4.1–US4.5; FR-014, FR-018; Principles VIII, XII).
- [ ] T064 [US4] Extend `internal/extproc/config/loader_test.go`'s explicit example table to load/validate `examples/config/extproc-delegation-context.yaml` through the real loader, and add compatibility assertions for the approved deprecation schema in `internal/adapters/http/enduser/authzen_compat_test.go` without changing token response behavior (US4.6; FR-018–FR-019; Principles VII, VIII).
- [ ] T065 [US4] Run T061–T064 to semantic red for new config/discovery/deprecation expectations, preserving already-green disabled behavior, and record evidence in `specs/054-authzen-delegation-pip/quickstart.md` (Principles VIII, XIII).

### Implementation for User Story 4

- [ ] T066 [US4] Implement fields/defaults/env/flags/binding in `internal/extproc/config/config.go` and `loader.go`: enabled `false`, PDP `""` → origin, request timeout `5s`, max staleness `60s`, and all `EXTPROC_DELEGATION_CONTEXT_*`/dotted flags with flags > env > file > defaults (FR-018; ExtProc contract §1; ADR 011).
- [ ] T067 [US4] Implement enabled-only rules in `internal/extproc/config/validate.go`: enabled “Requires `authorization.enabled`”, PDP “Absolute `https` URL (or `http` with `allow_http`), with a host, without userinfo, query, or fragment”, durations “`> 0`”; derive scheme/host/port origin without erasing an explicit identifier path and emit credential-free key-naming messages (FR-014, FR-018; data-model §3.1).
- [ ] T068 [US4] Implement `internal/extproc/delegation/discovery.go` against final verified API-003 path rules using `oauth2.tls` and request timeout; metadata is immutable after startup, compare expected/advertised PDP exactly, validate advertised endpoint before credentials and reject failures rather than guessing or silently disabling (FR-002, FR-014; SR-005; ExtProc contract §§1–2).
- [ ] T069 [US4] Wire config, discovery, client/resolver, fourth server port and shutdown in `cmd/extproc-token-exchange/root.go` after the existing exchanger is built and before listening; make disabled mode create/call no resolver, preserve assertion provider lifecycle and existing telemetry composition (FR-014, FR-018; ADR 011; Principle XII).
- [ ] T070 [US4] Verify the T014 OpenAPI deprecation marker/version and document the confirmed resolver-shipping deprecation in `docs/reference/token-exchange.md` and `docs/api/authzen-evaluation.md`: retain `granted_permission_sets`, `principal`, `agent_id`; schedule removal one release later only via separately approved `2.0.0`, never remove it here (FR-019; API-006; R12).
- [ ] T071 [US4] Run `just check`, `just extproc-test`, `just test-e2e-extproc`, US4.5–US4.6 in `tests/e2e/delegation_context_journey_test.go` and `authzen_delegation_compat_test.go`, and `just docs-build`; confirm zero disabled calls and zero evaluation credentials on mismatch, and record green results in `specs/054-authzen-delegation-pip/quickstart.md` (SC-004, SC-006; Principles VIII, XIII).

**Checkpoint**: Opt-in startup is safe, disabled behavior remains compatible, all four stories pass. Deprecation is documentation/additive schema work, not permission for removal.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T072 Update `internal/AGENTS.md`'s tokenexchange description and `internal/extproc/AGENTS.md`'s fourth port, enabled/disabled paths, config, independent cache and test commands after implementation, retaining ADR 011 isolation and ADR 036 metadata-only invariants; reconcile `ARCHITECTURE.md`'s feature-affected sections without bundling independent cleanup (FR-006, FR-014–FR-018; Principles II, VI).
- [ ] T073 Reconcile verified commands, all 44 test locations and observed results in `specs/054-authzen-delegation-pip/quickstart.md`; keep `spec.md` and `plan.md` unchanged throughout implementation; verify the API/config/deployment docs match shipped behavior and no deferred issue or future exchange-field removal became feature code (SC-001–SC-006; Feature Scope; Principles IV, VII, XIV).

---

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

**These tasks MUST be included in every generated tasks.md file. They verify that all constitution principles have been followed.**

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist - verify Phase 2 tasks were completed correctly

- [ ] T074 Verify every task traces to approved scope in `specs/054-authzen-delegation-pip/spec.md` or an applicable binding constraint, and T002's separate-refactor handoff is documented in `specs/054-authzen-delegation-pip/quickstart.md` (Feature Scope; Principle II).
- [ ] T075 Verify `ARCHITECTURE.md` glossary/model entries agree with `specs/054-authzen-delegation-pip/data-model.md` and preserve existing aggregate/typed-ID semantics (Principle V).
- [ ] T076 Verify `examples/config/extproc-delegation-context.yaml` and its `examples/config/README.md` entry exist, match all four defaults and owning config schema, and use synthetic credentials only (Principle VII).
- [ ] T077 Verify `api/enduser/openapi.yaml` contains the confirmed designs, approval references and final-standard verification are recorded in `specs/054-authzen-delegation-pip/contracts/authzen-evaluation-api.md`, and no admin contract was changed (Principles IV, X).
- [ ] T078 Verify `specs/054-authzen-delegation-pip/data-model.md` confirms no new persistence/schema/migration, and `specs/054-authzen-delegation-pip/plan.md` confirms no UI/design-system/universal-component work; record frontend design review as not applicable rather than inventing UI (Principles IX, XI).
- [ ] T079 Verify `specs/054-authzen-delegation-pip/spec.md` has linked concrete journeys for every flow, consistent synthetic config/HTTP/JWT examples and confirmed omission/error semantics; Wireloom, output JWTs and new UI screenshots remain not applicable (Principle XIV).
- [ ] T080 Verify the 44 E2E scenarios in `tests/e2e/authzen_delegation_test.go`, `authzen_delegation_denial_test.go`, `authzen_delegation_errors_test.go`, `authzen_delegation_compat_test.go`, `delegation_context_journey_test.go` and `tests/e2e/extproc/delegation_context_startup_test.go` have recorded compile/semantic-red evidence in `specs/054-authzen-delegation-pip/quickstart.md`, with no placeholder failures, pending markers or red-phase comments (Principles VIII, XIII).
- [ ] T081 Verify the no-frontend scope in `specs/054-authzen-delegation-pip/plan.md` still holds; mark new Playwright tests and screenshots not applicable, without skipping any existing suites required by `just verify` (Principles XI, XIII).

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist - verify all principles followed during implementation

**Feature Scope** (Development Requirements):

- [ ] T082 Verify implementation changes against `specs/054-authzen-delegation-pip/spec.md` exclude unrelated remediation, feature/ADR collision cleanup and premature field removal; user-approved issue URLs may appear only under Future Work, not as implementation tasks (Feature Scope & Deferred Findings).

**API & Documentation** (Principles IV, X):

- [ ] T083 Verify `api/enduser/openapi.yaml` is updated only to the confirmed designs, matches implemented schemas/statuses/headers/authentication exactly and retains the deprecated exchange field and identity fields (Principles IV, X; FR-019).
- [ ] T084 Verify `docs/api/authzen-evaluation.md`, `docs/api/enduser-api.md` and `docs/reference/token-exchange.md` cover discovery, evaluation, errors, freshness, migration and the approved staged version plan with working links and rendered examples (Principles IV, X, XIV).

**Architecture & Documentation** (Principle II):

- [ ] T085 Verify `ARCHITECTURE.md`'s routes, glossary and four-gate model match implementation, and `adrs/045-authzen-delegation-pip.md` (or its recorded noncolliding replacement) has actual stakeholder acceptance and explicitly supersedes only ADR 028's enabled-mode input/order; do not silently accept/revise binding ADRs (Principle II).

**Configuration** (Principle VII):

- [ ] T086 Verify `internal/extproc/config/{config.go,loader.go,validate.go}` own all four new keys through the accepted ADR 011 boundary, file/env/flag precedence and startup validation, with no ad-hoc configuration loads or new broker keys (Principle VII).
- [ ] T087 Verify `docs/configuration.md`, `docs/docker-compose-setup.md` and `configs/config.extproc.docker.yaml` document the standalone deployment contract and `charts/agentic-identity-broker/` has no ExtProc-only settings; no broker chart change is required because no chart-managed workload settings changed (Principle VII).

**Database & Persistence** (Principle IX):

- [ ] T088 Verify no new migrations, persistence entities or repository APIs in `migrations/`, `internal/ports/storage.go` and `internal/adapters/storage/`; new migration numbering/apply/rollback/data-integrity tests are not applicable, existing PostgreSQL repository coverage remains intact and US2.19 passed with real PostgreSQL (Principle IX).

**Security** (Principles I, III):

- [ ] T089 Verify `internal/domain/tokenexchange/evaluation.go`, HTTP handlers and `internal/extproc/delegation/` enforce authentication, caller authorization, enabled-mode fail-closed behavior, no stale fallback and grant-before-session ordering; broker routes follow exchange configuration, ExtProc opt-in rationale is covered by the accepted ADR and no runtime validation bypass exists (Principle I; SR-001–SR-006).
- [ ] T090 Verify `internal/extproc/delegation/{discovery.go,client.go}` and shared resolver use existing JWT/TLS/HTTP security libraries and no custom crypto or disabled transport verification (Principle III; SR-005).
- [ ] T091 Verify structured events/spans in `internal/adapters/http/enduser/authzen_evaluation.go` and `internal/extproc/delegation/` have bounded categories, actor/calling-peer and trace correlation as applicable, and captured response/log/span tests contain zero subject/assertion/provider credentials, secrets, raw claims or dependency error text (Principle I; SR-003–SR-004; SC-004).

**Architecture Patterns** (Principle VI):

- [ ] T092 Verify `internal/domain/tokenexchange/` owns invariants, handlers only parse/serialize and delegate, dependencies flow through existing ports, no broker imports enter `internal/extproc/delegation/`, and no new anemic bounded context or cross-adapter dependency exists (Principle VI).
- [ ] T093 Verify all broker construction is in `internal/app/builder.go` and only route registration is in `internal/adapters/http/routing/enduser.go`; standalone construction stays in `cmd/extproc-token-exchange/root.go` and uses the fourth port rather than concrete server coupling (Principle XII; ADR 011).

**Testing** (Principle VIII - Unit & Integration Tests):

- [ ] T094 Verify test-first evidence for unit/HTTP/config/cache/concurrency changes in `specs/054-authzen-delegation-pip/quickstart.md`, tests drove behavior rather than being rewritten to pass, and the separate Phase 0 unchanged-suite baseline/regression record is present (Principle VIII).
- [ ] T095 Verify `internal/domain/tokenexchange/evaluation_test.go`, `internal/extproc/delegation/client_test.go`, `resolver_test.go`, `discovery_test.go` and `internal/extproc/server/delegation_test.go` cover all outcome, freshness, cache-state and request-order branches, the ExtProc example loads through the real loader, race tests pass, test changes are minimal and no Bash correctness validator or test-only production API was added; no additional infrastructure integration suite is needed beyond the existing real-PostgreSQL outage journey (Principles VIII, IX).

**E2E Acceptance Testing** (Principle XIII):

- [ ] T096 Verify all 44 spec scenarios map 1:1 to `It()` blocks in the E2E files listed in the coverage table, including ten separate denial rows and exact scenario comments, using Ginkgo/Gomega hierarchy, production bootstrap, fixtures, isolated state and real request/response cycles (Principle XIII).
- [ ] T097 Verify acceptance tests were written before behavior, compiled and failed semantically initially where new behavior was absent, changed minimally (fixtures only) and turned green; record evidence in `specs/054-authzen-delegation-pip/quickstart.md` without manufacturing failure for unchanged disabled-mode behavior (Principles VIII, XIII).
- [ ] T098 Run the three focused Ginkgo label commands in `specs/054-authzen-delegation-pip/quickstart.md` and require exactly 29 broker, 11 consumer and 4 startup scenarios passing; run `just test-e2e-docker` explicitly for US2.19 and verify it is correctly excluded from `just test-e2e-backend`. The focused broker label run includes all 29 only with real PostgreSQL infrastructure available; never remove `docker` or skip the scenario to satisfy counts. Include both canonical-ID UUID fixtures, and record actual results or the infrastructure blocker (Principle XIII; SC-001–SC-006).
- [ ] T099 Run unchanged regression journeys in `tests/e2e/token_exchange_test.go`, `grant_diagnostics_test.go`, `local_token_exchange_e2e_test.go`, `extproc_approval_journey_test.go`, `tests/e2e/extproc/opa_authorization_test.go` and `metadata_input_test.go`; record compatibility/approval/metadata evidence in `specs/054-authzen-delegation-pip/quickstart.md` (Principle XIII; SC-006; SR-006).
- [ ] T100 Run `just check`, `just test`, `just extproc-test`, `just test-e2e`, `just docs-build` and `just verify` with E2E as the final guard; record actual passed commands/results in `specs/054-authzen-delegation-pip/quickstart.md`, resolving feature-caused failures and reporting unavailable infrastructure as a blocker, never a waiver (Principles VIII, XIII).

**Frontend** (Principle XI - if applicable):

- [ ] T101 Verify the diff under `web/`, `tests/e2e/frontend/` and `tests/e2e/screenshots/` confirms no new UI work; record design-system primitives/tokens, universal Storybook components, accessibility contrast checks and new Playwright/screenshots as not applicable for this feature, retaining full-suite regression requirements (Principles XI, XIII).

---

## Coverage & Traceability

Each acceptance scenario maps to one eventual `It()`; the grouped ranges below are not permission to merge tests. Phase 2f writes every E2E test before story implementation.

| Scenarios | Count | E2E file | Authoring | Behavior / green gate |
|---|---:|---|---|---|
| US1.1–US1.8 | 8 | `tests/e2e/authzen_delegation_test.go` | T021 | T037–T041 |
| US2.1–US2.10 | 10 | `tests/e2e/authzen_delegation_denial_test.go` | T022 | T045–T048 |
| US2.11–US2.20 | 10 | `tests/e2e/authzen_delegation_errors_test.go` | T023 | T045–T048 |
| US3.1–US3.10 | 10 | `tests/e2e/delegation_context_journey_test.go` | T024 | T054–T060, T066–T069 |
| US4.1–US4.4 | 4 | `tests/e2e/extproc/delegation_context_startup_test.go` | T025 | T066–T071 |
| US4.5 | 1 | `tests/e2e/delegation_context_journey_test.go` | T024 | T068–T071 |
| US4.6 | 1 | `tests/e2e/authzen_delegation_compat_test.go` | T026 | T014, T070–T071 |
| **Total** | **44** | 29 broker + 11 consumer + 4 startup | T027 red evidence | T098–T100 final green |

| Requirement group | Primary tasks |
|---|---|
| FR-001–FR-002 availability/discovery | T013–T015, T021, T035, T040, T062, T068–T069 |
| FR-003–FR-005 request, trust/CEL, anchor | T013–T014, T023, T032, T037, T042–T046 |
| FR-006 authoritative resolution | T002–T005, T032, T037, T040, T099 |
| FR-007–FR-009 context/metadata/freshness | T008, T021, T033, T038–T039 |
| FR-010–FR-012 denials, errors, request ID | T022–T023, T034, T042–T048 |
| FR-013 read-only/session independence | T016, T021, T032, T038, T041 |
| FR-014–FR-015 independent resolver/cache | T024–T025, T049–T055, T062, T068–T069 |
| FR-016–FR-017 ordering and policy input | T024, T051–T052, T056–T059 |
| FR-018 rollout/config/canonical policy | T010–T012, T020, T025, T061–T069 |
| FR-019 / API-006 staged deprecation | T013–T015, T026, T064, T070 |
| API-001–API-005 design/standard/ADR gates | T001, T009, T013–T015, T077, T083–T085 |
| SR-001–SR-002 validation/fail closed | T032, T042–T046, T049–T058, T089 |
| SR-003–SR-004 no disclosure/ordering/audit | T022–T024, T033, T044, T047, T052, T059, T091 |
| SR-005–SR-006 discovery/independent gates | T025, T052, T057–T058, T062–T063, T068–T069, T090, T099 |
| SC-001–SC-006 | All 44 mapped scenarios, T098–T100; canonical UUID parity T020/T024/T060; compatibility T025/T026/T099 |

## Task Generation Summary

| Group | Task count | Acceptance scenarios |
|---|---:|---:|
| Shared setup, design, scaffolding, polish and compliance | 61 | E2E authoring T021–T026 covers all stories |
| US1 (T032–T041, P1) | 10 | 8 |
| US2 (T042–T048, P1) | 7 | 20 |
| US3 (T049–T060, P2) | 12 | 10 |
| US4 (T061–T071, P2) | 11 | 6 |
| **Total** | **101** | **44** |

Every task has an unchecked checkbox, unique sequential ID, repository-relative file path and requirement/scenario/binding-constraint reference. Only story-phase tasks carry `[USn]`; `[P]` tasks run only within the prerequisite waves below. All work remains for the implementation phase.

## Dependencies & Execution Order

### Phase Dependencies

1. T001 is the first implementation task: verify the supplied sign-off and record only the approvals it covers. Keep `spec.md` and `plan.md` immutable; put all handoff, red/green and validation evidence in `quickstart.md`. Missing approval or external prerequisites defer only dependent tasks, never independent approved work. Phase 1 records tooling/scope. Phase 2a–2e design work can proceed next, with T001's record of the maintainer approval and T013's final-standard verification blocking implementation. T014 depends on T013; T015 on T014; T009 uses the correct ADR identity. T010–T012 are independent documentation files and can run together.
2. T002 is a maintainer handoff gate, not authority to create a PR. After T001's approval record, T013's standard verification and domain/API/config design, T003→T004→T005 run in the separately arranged refactor PR; its integrated revision is required before feature code continues. This dependency overrides simple numeric/phase order and prevents refactoring before required approvals.
3. T018–T020 build test support after approved interfaces exist. T021–T026 may be authored in parallel after helpers/fixtures, with no scenario-file conflicts. T028 checks journeys against T013/T014. T029–T031 are compile-only scaffolding after the separate refactor and design approvals; they enable T027. T027 depends on **T018–T031** (except itself) and blocks every story implementation. No cycle: test authoring does not depend on working behavior.
4. US1 tests T032–T036 precede T037–T040 and green gate T041. US2 tests T042–T044 precede T045–T047 and green gate T048. These stories share evaluation/handler files; implement in order to avoid conflicts and ship them together as the safe broker increment.
5. US3 and US4 test authoring can overlap after foundation: T049–T053 and T061–T065 use separate files except `loader_test.go` within US4, so serialize T061/T064 if they would overlap. Config/discovery work T066–T069 can proceed alongside US3 client/cache work T054–T055 after both relevant red gates.
6. T056 requires the client Result shape; T057–T058 require T054–T056. T069 requires T066–T068, T054–T055 and server port support. T060/T071 require integrated US3 and US4 runtime wiring; US4's later numbered phase is therefore not a barrier to doing its prerequisites before US3's E2E green run.
7. Phase 7 and Phase N follow all four stories, with T100 as the full validation gate. Do not claim completion until every applicable task and acceptance journey is green.

### User Story Dependencies

```text
Design approvals + separate Phase 0 integration
  → compile-only foundation + all 44 acceptance tests RED
  → US1 permit/discovery → US2 complete broker failure handling
  → US3 client/cache/input/order + US4 config/discovery/wiring (interleaved prerequisites)
  → all US3/US4 journeys GREEN → documentation/compliance/full verification
```

US1 is independently testable without ExtProc. US2 uses US1's valid setup. US3 needs the complete broker and US4's startup configuration/discovery but can develop its port-driven components independently. US4 startup tests use the mock broker; US4.5 and compatibility testing use the real broker. Do not pretend these integration dependencies are independent deployable stories.

### Within Each User Story

Tests first → compile → semantic red evidence → minimal implementation → integration/green → refactor. Do not rewrite acceptance logic to match implementation. Only complete tasks after observable validation. If a genuine attempt fails or sign-off is missing, leave the task and its dependents unchecked, record why in `quickstart.md`, continue independent approved work and publish permitted progress as blocked when work remains. Never waive validation or mark an unfinished checklist item complete.

### Parallel Opportunities and Examples

- **Design**: T010 config reference, T011 example/index and T012 deployment docs touch different files. All wait for approvals before runtime implementation.
- **US1**: After foundation, T032 resolver tests, T033 projection tests, T034 handler tests and T035 wiring/routing tests can be authored concurrently; T036 joins before implementation. Do not parallelize US2 changes into the same handler/evaluation files.
- **US2**: T042 domain failure tests and T043 HTTP decode/response tests are independent; join before T044's additional handler assertions and red run.
- **US3**: T049 client tests, T050 cache tests, T051 input tests and T052 server-order tests are separate files. Join at T053; T054 client and T055 resolver must agree on Result and can then be completed in dependency order. Server body/header work shares one file and is serial.
- **US4**: T062 discovery tests and T063 composition tests can run alongside T061 config tests. T064 shares `loader_test.go` with T061 and must not edit it concurrently. After red evidence, T068 discovery implementation and T066/T067 config work touch separate files and can progress independently before T069 integration.

## Implementation Strategy

### MVP First (User Story 1 Only)

Complete design/approval gates, separately integrated Phase 0, compile-only foundation and all acceptance-test authoring/red evidence. Implement US1 and demonstrate its eight scenarios without a provider session or ExtProc. This is the reviewable MVP, **not** release permission: complete US2's denial/error controls before deploying the broker increment.

### Incremental Delivery

1. Additive broker increment: US1 + US2 with shared resolution and all broker acceptance/regression tests.
2. Opt-in ExtProc increment: US3 + US4 config/startup wiring with canonical-ID policy and bounded freshness independent of exchange cache.
3. Deprecation documentation in the confirmed resolver-shipping release; field removal is expressly outside this feature and requires a separately approved later major contract.
4. Complete all compliance and full verification gates. Keep the PR draft; this list authorizes neither merge nor readiness changes.

### Future Work (Non-task Notes)

The three deferred findings remain in `research.md` pending the maintainer's decision whether to file issues. No issue creation is authorized by this task list. Removing `granted_permission_sets` is deferred to a separately approved `2.0.0` change one release after resolver shipment. Neither note is a current implementation task.
