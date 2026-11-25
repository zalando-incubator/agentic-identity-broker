# Tasks: OPA policies for impersonation authorization

**Input**: Design documents from `specs/20261120-153502-opa-impersonation-authorization/`.
**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `quickstart.md`, and `contracts/`.
**Tests**: Mandatory test-first red-green-refactor (Principles VIII, XIII). Keep run evidence in the session result, never in feature documents. Check a task only after its work is complete.
**Organization**: Approval and refactor first, then shared design and foundation, then independently testable user-story increments. Proposed API, ADR, and decision items need the maintainer's recorded approval before their implementation.

## Format: `[ID] [P?] [Story] Description`

- `[P]` marks disjoint files with no dependency on another incomplete task in the same group.
- `[US1]`–`[US4]` map to the user stories in `spec.md` § User Scenarios & Testing.
- Paths are relative to the repository root. Design citations without the feature prefix resolve inside this feature directory; for example `data-model.md` § OPA Engine resolves to `specs/20261120-153502-opa-impersonation-authorization/data-model.md`.
- Each implementation task cites the governing requirements and contracts; read those sections, the nearest `AGENTS.md`, and the applicable `coding-standards/` before changing code.

## 🔒 Feature Scope [MANDATORY]

- Every generated task MUST cite an approved feature requirement or acceptance scenario (ID if available, otherwise title), or a binding constraint applicable to the affected feature path. Include work necessary to make that path functional, secure, and compliant; do not add independent remediation or optional improvements.
- If a finding blocks safe or compliant delivery, include only the minimum necessary correction. For unrelated findings, ask the user before filing an issue; record only user-approved issue URLs in a non-checkbox `Future Work` note in the feature planning or implementation record, never as feature tasks.
- Expanding feature scope requires explicit user approval and corresponding updates to `spec.md` and `tasks.md` before implementation.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the approval boundary without granting approval. Prepare document changes here, but commit the Phase 0 refactor before any ADR or feature commit.

- [ ] T001 Verify recorded maintainer approval for every Proposed row of `plan.md` § Approval Items (ADR-OPA, API-CFG, API-BOOT, API-INPUT, API-DECISION, API-DIAG, DEC-BUILTINS) and that CONST-AMEND is present in `.specify/memory/constitution.md` v4.8.0; move `specs/20261120-153502-opa-impersonation-authorization/adr-20261121-061324-opa-impersonation-authorization.md` to `adrs/20261121-061324-opa-impersonation-authorization.md` with status `Accepted (approved by <login> on <date>: <url>)` from the recorded approval only, and fix its relative supersedes links and feature references. Leave work for any unapproved item unchecked; do not edit any ADR already on the default branch (Principles II, X).
- [ ] T002 Confirm `go.mod` already pins `github.com/open-policy-agent/opa` v1.21.4 with the packages listed in `plan.md` § Technical Context and that no new module is needed; read `justfile` recipes and `internal/AGENTS.md`, `internal/adapters/AGENTS.md`, and `tests/e2e/AGENTS.md` before implementing (Principle III, FR-009).

## Phase 0: Pre-implementation Refactoring

**Purpose**: The behavior-preserving restructuring required by `plan.md` § Implementation Phase Overview. No configuration change or OPA implementation belongs in this commit.

- [ ] T003 Run existing `internal/domain/impersonation/*_test.go` and `internal/adapters/jwks/*_test.go` through `just test` and existing impersonation acceptance scenarios through `just test-e2e-backend` before refactoring; keep their assertions unchanged (FR-008, SC-005, Principle VIII).
- [ ] T004 Extract the static CEL `authorization` seam (`label()`, `engine()`, `evaluate(ctx, inputs)`) into `internal/domain/impersonation/authorizer.go`, make `internal/domain/impersonation/cel.go`'s `authorizationProgram` the CEL static authorizer, and replace the three `rule.authz` reads per evaluation with one authorization read in `rule.go` and `service.go`; keep the CEL startup checks, the `Impersonation Rule.Binding requirements` of `data-model.md` § Impersonation Rule (extended) unchanged, and today's CEL error text and pipeline order (`contracts/internal-contracts.md` § 3, FR-008, FR-013).
- [ ] T005 Extract the contained-file logic of `internal/adapters/jwks/trust_directory.go` (`EvalSymlinks`, `os.OpenRoot` rejecting `/`, relative open, same-descriptor regular-file check, bounded `io.LimitReader` read, and escape classification) into a new shared package `internal/fscontain/` and make `trust_directory.go` use it with identical behavior and errors; add focused tests in `internal/fscontain/` and keep the existing jwks tests unchanged (`plan.md` § Complexity Tracking, ADR `040-trust-anchor-local-key-sets`, FR-003).
- [ ] T006 Run `just check`, `just test`, and `just test-e2e-backend` against the refactor; commit only the T004–T005 paths (`internal/domain/impersonation/`, `internal/fscontain/`, `internal/adapters/jwks/trust_directory.go`) in their own behavior-preserving commit, ahead of the prepared T001 ADR changes and any feature code (Phase 0 rule, Principles II, VIII).

**Checkpoint**: Existing tests unchanged and passing; the refactor is isolated ahead of feature commits.

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation.

**⚠️ CRITICAL**: No feature business implementation can begin until this entire phase is complete. T015 adds only compile-enabling declarations and mechanical migrations; it must not implement an implicit allow or weaken assertions.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management).

- [ ] T007 Verify the value objects, state transitions, and per-request use in `specs/20261120-153502-opa-impersonation-authorization/data-model.md`, including unchanged Agent/UserGrant boundaries and no new typed ID, entity, repository, or persisted event; reconcile only routine gaps within the approved contract (FR-008, FR-013, Principles II, V, IX).
- [ ] T008 Add Impersonation Policy Engine, Impersonation OPA Engine, Policy/Data Revision, and Policy Bootstrap to the `ARCHITECTURE.md` glossary and restate Impersonation Rule, Subject Category, and Agent-Subject Token as `data-model.md` § Glossary additions describes (OPA rules: exact-target unsigned subjects are agent category; CEL rules: only with the predicate's `sub` binding); update the same entries, the impersonation boundary text, and the ADR index row for `20261121-061324-opa-impersonation-authorization` in the root `AGENTS.md` (Principles II, V).

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design).

- [ ] T009 Verify every new key, default, selected-block rule, timeout bound, bootstrap restriction, and indexed diagnostic against `specs/20261120-153502-opa-impersonation-authorization/contracts/impersonation-opa-config.yaml`; assign loader versus domain responsibilities, including that `OPA Engine.startup_timeout`, `OPA Rule Decision.package`, `OPA Rule Decision.decision`, and `OPA Rule Decision.evaluation_timeout` (`data-model.md` § OPA Engine, § OPA Rule Decision) default at use and never as loader or Viper defaults, and the explicit ordinary-exchange `type: opa` startup rejection (`plan.md` § Summary, FR-001–FR-003, FR-009, FR-010).
- [ ] T010 Create approved examples in `examples/config/impersonation-opa.yaml` and `examples/config/opa/impersonation/` (`bootstrap.yaml` with the `aib_bearer_token_file` plugin, `local/` bundle directory, `governance.rego`, `data.json`, and `exceptional-modes.rego`) showing the shared `impersonation.opa` engine block, equivalent CEL/Rego governance predicates, and policies that constrain `context.subject_token.sub`, `context.subject_token.email`, and each audience-absent role's token map; index them in `examples/config/README.md` (`contracts/impersonation-opa-config.yaml` `example`, `example_bootstrap`, `example_local`; `contracts/policy-input-decision.md` § 4; FR-011, SR-005, Principle VII).
- [ ] T011 Apply the `impersonation.opa` engine block (`policy` oneOf `config_file`/`path`, `startup_timeout`) and mutually exclusive CEL/OPA rule branches (OPA rule: `package`, `decision`, `evaluation_timeout` only) to `charts/agentic-identity-broker/values.schema.json`, explain the unchanged snake_case passthrough in `charts/agentic-identity-broker/values.yaml`, and document bootstrap, local bundle, and token-file Secret mounts through existing volume values in `charts/agentic-identity-broker/README.md`; keep ExtProc configuration out of the broker chart (`contracts/impersonation-opa-config.yaml` `helm`, FR-011, Principle VII).

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development).

- [ ] T012 Verify the recorded API sign-off and publish the approved `POST /oauth2/token` description delta, including the Agent subjects paragraph (CEL rules need the `sub` binding; on OPA rules an unsigned exact-target subject is agent category and the policy must constrain `sub`; cross-agent rejection), the 500 example, and version `1.4.0` (from `1.3.0`) in `api/enduser/openapi.yaml` before endpoint implementation; confirm no Admin API, request/response envelope, or token-claim change (`contracts/openapi-token-opa.yaml`, FR-007–FR-009, Principles IV, X).

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management).

- [ ] T013 Confirm the no-database-change design in `specs/20261120-153502-opa-impersonation-authorization/data-model.md` (in-memory engine snapshots only, existing Agent/UserGrant access); add no migration or repository scaffolding (FR-008, Principle IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency).

- [ ] T014 Confirm `specs/20261120-153502-opa-impersonation-authorization/spec.md` § Key Entities introduces no UI, consent flow, or frontend component; record that no frontend, Wireloom, Playwright, or screenshot work applies (Principles XI, XIII, XIV).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII (End-to-End Acceptance Testing & Spec Traceability), XIV (Specification by Example & Reviewable Journeys).

- [ ] T015 Add only the compile-enabling OPA configuration declarations of `contracts/internal-contracts.md` § 1 to `internal/ports/config.go` and append the trailing `policySources PolicySourceFactory, publicURL string` parameters to `impersonation.NewService` in `internal/domain/impersonation/service.go`; migrate mechanically, in the same commit, every file listed under "Mechanical fixture migration" in `plan.md` § Source Code (rule literals to the new type; CEL-only `NewService` call sites pass `nil, ""`) with behavior unchanged; OPA stays unsupported until feature implementation (FR-001, FR-009, Principle XIII).
- [ ] T016 Create `tests/e2e/helpers/opa_bundle_server.go` (tarballs built with `bundle.Write`, controllable ETag, fetch failure and latency, optional bearer-token enforcement with a switchable expected token, and a recorder proving an `http.send` target is never contacted) and Rego/data fixtures in `tests/e2e/fixtures/opa/impersonation/`; reuse `helpers.TrustDir` from `tests/e2e/helpers/local_jwks.go` for token rotation and containment, place every file in `GinkgoT().TempDir()`, and add thin production-bootstrap wrappers in `tests/e2e/bootstrap/` (`plan.md` § Testing Strategy, research R14, Principles VIII, XIII).
- [ ] T017 Write the 17 scenario-mapped `It()` blocks of `plan.md` § Scenario Mapping in `tests/e2e/opa_impersonation_authorization_test.go` before feature logic: `Describe("OPA Impersonation Authorization")`, one `Context` per story, the spec scenario ID beside each `It`, real HTTP status, OAuth error body, decoded-claims, startup-error, and captured-log assertions, and no placeholders, `Skip`, `XIt`, or `PIt`; cover each scenario's variants inside its own `It` (Scenarios 1.1–4.3, Principle XIII).
- [ ] T018 Review Journeys 1–4 in `specs/20261120-153502-opa-impersonation-authorization/spec.md` § Concrete Example Journeys against the 17 scenarios, the configuration and HTTP contracts, and decoded JWT claims; run `just web-build`, then `ginkgo -v --label-filter="!performance && !docker" --focus "OPA Impersonation Authorization" ./tests/e2e/` and confirm semantic failures for absent OPA support, not compile failures (Principles VIII, XIII, XIV).

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

**Constitution Reference**: Principle XV (Binding Coding Standards).

- [ ] T019 Read `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`; verify `specs/20261120-153502-opa-impersonation-authorization/contracts/observability.md` lists the new diagnostics, the evaluation span and its value sets, the revisions counter (`outcome`, `reason` without binding reasons) and age gauge, lifecycle log events, audit attributes, the traced `aib_bearer_token_file` client through `telemetryhttp.NewTransport`, and the safety tests; reuse the existing `tokenexchange` scope and `impersonation` attribute namespace, so no standard amendment is needed (SR-003, SR-004, Principle XV).

**Checkpoint**: Approved designs published, test scaffolding compiles, all 17 acceptance tests written and locally seen failing semantically. No frontend or persistence scaffold applies.

## Phase 2.7: Entity Boilerplate

Skipped: no new entity, handler, or repository (`plan.md` § Implementation Phase Overview).

## Phase 2.5: Foundational Infrastructure

**Purpose**: The runtime boundary, contained bootstrap, restricted capabilities, and decision-presence admission before any OPA issuance.

- [ ] T020 Define the port of `contracts/internal-contracts.md` § 2 in `internal/ports/impersonation_policy.go`: `ImpersonationPolicyAction`, the sensitive `ImpersonationPolicyInput`, `ImpersonationPolicyDecisionRef`, `ImpersonationPolicyRevision`, `ImpersonationPolicySource`, `ImpersonationPolicyAdmission func(decisionDefined map[string]bool) error`, `ImpersonationPolicyAdmissionError`, and the two sentinel errors (FR-007, SR-002).
- [ ] T021 [P] Write semantic red tests in `internal/ports/impersonation_policy_test.go` for `LogValue`, `String`, and `GoString` through runtime interface assertions, value/pointer `%s`, `%v`, `%+v`, `%#v`, nil/empty/populated input, and `slog.Any` with JSON and text handlers using distinctive synthetic values; only `[REDACTED]` may render while evaluation still receives the full typed document (`contracts/internal-contracts.md` § 3, `contracts/policy-input-decision.md` § 2, SR-003, LOGGING § 6.4).
- [ ] T022 Implement constant-redacted `LogValue`, `String`, and `GoString` on `ImpersonationPolicyInput` in `internal/ports/impersonation_policy.go`, driven by T021; never convert the input to a raw map for logging (SR-003, Principle XV).
- [ ] T023 [P] Write bootstrap tests in `internal/adapters/policy/opa/bootstrap_test.go`: missing, unreadable, invalid, YAML/JSON, and oversized or non-regular files; every unsupported top-level key and every plugin other than `aib_bearer_token_file`; no bundles; `persist: true`; containment through `internal/fscontain` over the file's own directory (relative `..data` links accepted; escaping and absolute symlinks, relative paths, and a parent of `/` rejected); each OPA-native local-file credential, TLS, or bundle reference rejected; diagnostics name keys and reason classes, never values (`contracts/impersonation-opa-config.yaml` `file_containment`, `bootstrap_subset`, `startup_diagnostics`; FR-003, SR-003).
- [ ] T024 [P] Write capability tests in `internal/adapters/policy/opa/capabilities_test.go`: every removed nondeterministic built-in (`http.send`, `net.lookup_ip_addr`, `opa.runtime`, `rand.intn`, `uuid.rfc4122`, `io.jwt.encode_sign`, `io.jwt.encode_sign_raw`) is rejected as `prohibited_builtin` even in unreachable modules, `time.now_ns` stays allowed, print is disabled, and other compile errors classify as `compile_failed` (`contracts/policy-input-decision.md` § 5, research R4, FR-012, SC-007).
- [ ] T025 Implement startup-only bootstrap reading in `internal/adapters/policy/opa/bootstrap.go` through `internal/fscontain` with a same-descriptor 1 MiB bound, subset validation, and SDK-local-file rejection before `sdk.New`, with sanitized reason classes; fulfill `OPA Engine.policy.config_file` from `data-model.md` § OPA Engine (T023, FR-003, SR-003).
- [ ] T026 Implement restricted capabilities in `internal/adapters/policy/opa/capabilities.go` (`ast.CapabilitiesForThisVersion()` minus every nondeterministic built-in except `time.now_ns`) and one broker-owned `ast.NewCompiler().WithCapabilities(restricted)` per candidate whose compiler also prepares every rule's query through `rego.Compiler`; classify an undefined-function error naming a removed built-in as `prohibited_builtin` (`plan.md` § Summary, research R4, T024, FR-012).
- [ ] T027 [P] Write admission tests in `internal/domain/impersonation/admission_test.go`: a candidate is admitted only when every OPA rule's decision is defined, rejection names the first failing rule in rule order through `ImpersonationPolicyAdmissionError`, and CEL rules are never passed to OPA admission (`contracts/internal-contracts.md` § 3, research R6, R11, FR-001).
- [ ] T028 Implement the OPA admission function in `internal/domain/impersonation/admission.go`, driven by T027, with no reference or field-binding requirement for OPA rules (`contracts/policy-input-decision.md` § 4, SR-005).
- [ ] T029 Run `just check` and `just test` for the foundation; verify no source enters readiness without domain admission and that `internal/adapters/policy/opa/` imports neither another adapter nor `internal/extproc` (FR-009, Principles VI, VIII).

**Checkpoint**: Bootstrap, capabilities, and admission are tested. Runtime and issuance are not yet complete.

## Phase 3: User Story 1 - Enforce organizational governance before issuing a token (Priority: P1) 🎯 MVP

**Goal**: One OPA rule authorizes governed delegated humans and eligible signed self-agents while every existing issuance control stays authoritative.
**Independent Test**: Scenarios 1.1–1.4: an approved human request and a signed self-agent request mint the normal claims; governance marking the agent unregistered denies with `403 access_denied`; absent human delegation returns the existing recovery URL; the local bundle variant of 1.1 gives the same `200`.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [ ] T030 [P] [US1] Write decision and readiness tests in `internal/adapters/policy/opa/revision_test.go` and `internal/adapters/policy/opa/source_test.go`: allow, deny, exactly one object result, ignored extra members, strict built-in errors, undefined/malformed/unsupported actions, and a usable first-bundle subset followed by a rejected complete two-bundle candidate (neither admitted nor ready) (`contracts/policy-input-decision.md` § 3, Scenarios 1.1–1.2, FR-007, SR-002).
- [ ] T031 [P] [US1] Write OPA-rule service and builder tests in `internal/domain/impersonation/service_test.go` and `internal/app/builder_test.go` with domain-local port doubles: normal signed claims, governance denial, delegation after the OPA allow, signed self-agent issuance with no human state, scope and reserved claims, the input and transport metadata Journey 1 needs, and factory/shutdown wiring (Scenarios 1.1–1.4, `contracts/internal-contracts.md` § 3, § 5, FR-008, SR-001).

### Implementation for User Story 1

- [ ] T032 [US1] Implement immutable revisions and decision parsing in `internal/adapters/policy/opa/revision.go`, fulfilling `Policy/Data Revision.Prepared queries` and `Policy/Data Revision.Data snapshot` from `data-model.md` § Policy/Data Revision and the value mapping of `data-model.md` § Impersonation Policy Decision; `Evaluate` applies the rule's `OPA Rule Decision.evaluation_timeout` (research R2, T030, FR-004, FR-007).
- [ ] T033 [US1] Implement the shared engine in `internal/adapters/policy/opa/source.go`: one `sdk.New` with an in-memory store and `OnCommit` trigger, candidates built from one transaction with one prepared query per OPA rule, all-configured-bundle completeness before admission, one admission call per candidate (all-or-nothing), an `atomic.Pointer` swap, and initial readiness on that same admitted snapshot; fulfill `Policy/Data Revision.Label`, `Policy/Data Revision.Admission ID` (assigned only on successful admission), `Policy/Data Revision.Bundle completeness`, and `Policy/Data Revision.Admitted at` from `data-model.md` § Policy/Data Revision (`contracts/internal-contracts.md` § 4, T030, FR-003, FR-004).
- [ ] T034 [US1] Wire OPA rules into `internal/domain/impersonation/rule.go` and `service.go` (one `source.Current()` read per request before the first rule, CEL extraction retained with its default timeout, human delegation and target-derived agent minting unchanged), build the initial input in `internal/domain/impersonation/policy_input.go` with `type: "impersonation"` and broker-resolved `context.agent {id, canonical_id?, external_id?}` without UUID fallback, add the transport DTO in `request.go`, and pass handler metadata from `internal/adapters/http/enduser/oauth2_token.go`; fulfill `data-model.md` § Impersonation Policy Input through `contracts/policy-input-decision.md` § 1 (T031, FR-005, FR-006, FR-008, SR-001).
- [ ] T035 [US1] Construct the policy-source factory only in `internal/app/builder.go` with the injected logger, pass the configured end-user public URL as `NewService`'s trailing `publicURL`, and register `Service.Close` through `App.chainShutdown`; update thin bootstrap wrappers, not routing logic (`contracts/internal-contracts.md` § 5, T031, FR-003, Principles VI, XII).
- [ ] T036 [US1] Run `just check`, `just test`, focused Ginkgo for Scenarios 1.1–1.4 in `tests/e2e/opa_impersonation_authorization_test.go`, then `just test-e2e-backend`; confirm normal claims, consent recovery, and no human grant or session for the signed self-agent (SC-001, SC-002, Principles VIII, XIII).

**Checkpoint**: The signed governance MVP works. Input parity, exceptional modes, the update and failure matrix, and migration checks remain mandatory before completion.

## Phase 4: User Story 2 - Reuse policy input conventions without losing identity context (Priority: P1)

**Goal**: Complete Envoy-compatible transport facts and CEL-equivalent trusted context, including every exceptional credential mode on OPA rules.
**Independent Test**: Scenarios 2.1–2.5 exercise context parity, spoof-resistant context, unsigned human with email, unsigned exact-target self-agent with cross-agent rejection, and audience-absent signed-role combinations.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [ ] T037 [P] [US2] Write input and handler tests in `internal/domain/impersonation/policy_input_test.go` and `internal/adapters/http/enduser/oauth2_token_test.go`: every input member, optional claims, exact numbers/arrays/nested objects, repeated and filtered headers, public-URL host and scheme, `version`, `parsed_path`, `parsed_query`, `parsed_body` from the raw parse copy (repeated keys as arrays, `null` for other content types, and `client_assertion`, `actor_token`, `subject_token`, `client_secret`, `refresh_token`, `code`, and `code_verifier` absent even when the handler passes them), `truncated_body: false`, no raw body or token string anywhere, `type: "impersonation"`, broker-authoritative `context.agent` with unset IDs omitted, and every reserved ExtProc member absent; create the per-case vectors `tests/testdata/opa-envoy-input/<case>.json` in the schema of `plan.md` § Unit & Integration Tests and consume them in both the broker test and `internal/extproc/authorization/input_builder_test.go` without production cross-binary imports (`contracts/policy-input-decision.md` § 1, Scenarios 2.1–2.2, FR-005, FR-006, SR-003).
- [ ] T038 [P] [US2] Write exceptional-mode tests in `internal/domain/impersonation/opa_security_test.go`: on an OPA rule the unsigned human's extracted email is minted when the decision allows, an unsigned raw `sub` equal to the target UUID is agent category with no human grant or session, another registered agent's raw `sub` or extracted principal is a rejected cross-agent request without consent link or human fallback, and each audience-absent role and valid combination succeeds while a present `aud`, bad signature, wrong issuer or algorithm, or expiry fails even when OPA allows; on a CEL rule the existing reference and field-binding checks and outcomes are unchanged (`contracts/policy-input-decision.md` § 4, Scenarios 2.3–2.5, FR-013, SR-001, SR-005).

### Implementation for User Story 2

- [ ] T039 [US2] Complete type-preserving and edge-case input construction in `internal/domain/impersonation/policy_input.go` and `RequestAttributes{Method, Path, RawQuery, Protocol, ContentType, Headers, Form}` in `internal/domain/impersonation/request.go`, building `parsed_body` from the raw parse copy and deleting the seven credential-bearing OAuth parameters in the domain, without importing `net/http` into the domain (`contracts/internal-contracts.md` § 3, T037, FR-005, FR-006, SR-003).
- [ ] T040 [US2] Complete repeated-header, query, protocol, content-type, and raw-form capture (a copy taken before `dropOmittedValues`) in `internal/adapters/http/enduser/oauth2_token.go`, keeping claim derivation and policy decisions out of the handler (`contracts/internal-contracts.md` § 3, T037, FR-006, Principle VI).
- [ ] T041 [US2] Implement exceptional-mode handling for OPA rules in `internal/domain/impersonation/service.go` and `authorizer.go`: exact-target classification of unsigned subjects, minting of the extracted email when the decision allows, cross-agent rejection on raw and extracted identities, and unchanged cryptographic, audience-absence, category, and delegation guards; CEL rules keep their `Impersonation Rule.Binding requirements` from `data-model.md` § Impersonation Rule (extended) (`contracts/policy-input-decision.md` § 4, T038, FR-013, SR-001, SR-005).
- [ ] T042 [US2] Add paired CEL and OPA fixture policies for the exceptional modes in `tests/e2e/fixtures/opa/impersonation/` and configuration builders in `tests/e2e/fixtures/impersonation_rules.go`; each OPA policy constrains the caller-asserted values as `contracts/policy-input-decision.md` § 4 requires (Scenarios 2.3–2.5, SC-008).
- [ ] T043 [US2] Complete Scenarios 2.1–2.5 in `tests/e2e/opa_impersonation_authorization_test.go`, asserting captured policy inputs through a test policy's decision, token claims, and zero grant or session effects for agent variants through the production bootstrap; add no test-only production getter (Scenarios 2.1–2.5, Principles VI, XIII).
- [ ] T044 [US2] Run `just check`, `just test`, focused Ginkgo for Scenarios 2.1–2.5 in `tests/e2e/opa_impersonation_authorization_test.go`, and `just test-e2e-backend` (SC-002, SC-008, Principles VIII, XIII).

## Phase 5: User Story 3 - Apply managed policy and data changes safely (Priority: P2)

**Goal**: Live activation, last-admitted-revision retention, bounded startup and evaluation, credential-free diagnostics, local bundle mode, and a rotating, traced bundle token.
**Independent Test**: Scenarios 3.1–3.5 revoke through an ETag update and token rotation, make runtime failures terminal without fallthrough, fail unusable startup, retain `r1` after failed updates, and reject `http.send` revisions without contacting the destination.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [ ] T045 [P] [US3] Extend `internal/adapters/policy/opa/source_test.go` and `revision_test.go` with multiple bundles, same-snapshot completeness, equal or empty manifest labels producing distinct admission IDs, rejected, fetch-failed, and partial updates keeping the active revision, one shared revision for every OPA rule with all-or-nothing rejection when one rule's decision is undefined, cancellation and timeouts, in-flight snapshots across swaps, shutdown cleanup, and `-race` stress during revision swaps (research R2, R7, R11; Scenarios 3.1, 3.3–3.5; FR-003, FR-004, FR-010, FR-012).
- [ ] T046 [P] [US3] Write abort and diagnostic tests in `internal/domain/impersonation/opa_failure_test.go` and telemetry tests in `internal/adapters/policy/opa/telemetry_test.go`: distinctive synthetic token, claim, decision, and bootstrap values never appear in logs, spans, or metrics for allow, deny, invalid, error, rejection, and startup; span name, kind, `error.type`, and `canceled` outcome; revisions counter `outcome`/`reason` and age gauge with a manual reader; domain cancellation maps to terminal `server_error` without fallthrough; no admission ID or revision label in a span or metric (`contracts/observability.md`, Scenarios 3.2–3.3, SR-003, SR-004, Principle XV).
- [ ] T047 [P] [US3] Write local bundle-directory tests in `internal/adapters/policy/opa/local_test.go`: a valid directory admitted through the same capability and admission path, `policy_revision` from `.manifest`, unsupported file types, symlinks escaping the directory, a relative path, a directory resolving to `/`, more than 1,000 files or 16 MiB, no polling or later re-read, and the once-only `oauth2_server.impersonation_policy.local_mode` record without file contents (`contracts/impersonation-opa-config.yaml` `engine`, research R15, Scenario 1.1 local variant, FR-003).
- [ ] T048 [P] [US3] Write `aib_bearer_token_file` tests in `internal/adapters/policy/opa/tokenplugin_test.go`: startup validation (missing, relative, escaping or absolute symlink, empty, over 64 KiB), rotation to a link outside the directory failing that download, the token re-read before every bundle request against an `httptest` server, rotation without restart, a later read failure keeping the active revision with rate-limited `token_unavailable` and no contents, `credentials.plugin` naming another plugin rejected, and a client span from the `telemetryhttp.NewTransport`-wrapped transport without the token as an attribute (research R16, `contracts/observability.md` § Span, Scenario 3.1).

### Implementation for User Story 3

- [ ] T049 [US3] Complete bounded same-snapshot readiness (partial candidates never admitted), the single shared startup wait, engine cleanup on failure, cancellation-aware deadlines, and immutable last-admitted-revision retention in `internal/adapters/policy/opa/source.go`, `revision.go`, and `internal/domain/impersonation/service.go`; enforce `OPA Engine.startup_timeout` and `OPA Rule Decision.evaluation_timeout` from `data-model.md` § OPA Engine and § OPA Rule Decision (`contracts/internal-contracts.md` § 3–4, T045, FR-003, FR-010, FR-012).
- [ ] T050 [US3] Implement local bundle-directory loading in `internal/adapters/policy/opa/local.go` through `internal/fscontain`, building one candidate with OPA's bundle reader from the read bytes and no SDK runtime; fulfill `OPA Engine.policy.path` from `data-model.md` § OPA Engine and emit the once-only `local_mode` record through `telemetry.go` (T047, research R15, FR-003).
- [ ] T051 [US3] Implement the `aib_bearer_token_file` `rest.HTTPAuthPlugin` and its `plugins.Factory` in `internal/adapters/policy/opa/tokenplugin.go`, registered in `sdk.Options.Plugins` only when the bootstrap configures it; `NewClient` wraps its transport with `telemetryhttp.NewTransport`, and `Prepare` reads the token through `internal/fscontain` before every bundle request; first confirm OPA v1.21.4's `credentials.plugin` lookup in a focused test (T048, research R16).
- [ ] T052 [US3] Map invalid decisions and evaluation failures to terminal OAuth server errors in `internal/domain/impersonation/errors.go` and `service.go` and add `opa_evaluation_failed` and `opa_decision_invalid` to `internal/domain/tokenexchange/diagnostic.go`; preserve the caller-cancellation override and prevent later-rule fallthrough (`contracts/policy-input-decision.md` § 3, `contracts/observability.md` § Diagnostics, T046, FR-007, SR-002).
- [ ] T053 [US3] Implement the global-scope tracer and meter, the revisions counter and age-gauge callbacks with cleanup, and sanitized lifecycle logs in `internal/adapters/policy/opa/telemetry.go`; add `authorization_engine`, `policy_revision`, and `policy_admission_id` to the `impersonation_decision` audit in `internal/domain/impersonation/service.go` and `internal/adapters/http/enduser/oauth2_token.go`, copying the deciding revision's values even when labels are equal or empty (`contracts/observability.md`, T046, SR-003, SR-004, Principle XV).
- [ ] T054 [US3] Complete Scenarios 3.1–3.5 in `tests/e2e/opa_impersonation_authorization_test.go` with T016's helper: deterministic admission polling; the 3.1 token-rotation variant (`T1` to `T2`, emptied file keeps `r2`); the 3.2 canceled-client variant with a synthetic trace context, a test-local span processor waiting for evaluation start, and correlated-span, no-issuance, and no-fallthrough assertions; the 3.3 two-bundle partial-usable, complete-rejected startup variant; old allow and new-request deny after 3.4 failures; and zero contacts with the 3.5 `http.send` target (`spec.md` Journey 2, Scenarios 3.1–3.5, SC-003, SC-004, SC-007).
- [ ] T055 [US3] Run `just check`, `just test` (including race tests), focused Ginkgo for Scenarios 3.1–3.5 in `tests/e2e/opa_impersonation_authorization_test.go`, and `just test-e2e-backend`; verify deadlines with scheduling tolerance, not fixed sleeps (SC-003, SC-004, SC-006, SC-007, Principles VIII, XIII).

## Phase 6: User Story 4 - Adopt OPA one rule at a time (Priority: P2)

**Goal**: Explicit engine selection with unchanged valid CEL-only and ordinary-exchange behavior; explicit ordinary-exchange `type: opa` fails startup.
**Independent Test**: Scenarios 4.1–4.3 prove an explicit OPA deny falls through to a later CEL rule, a CEL-only deployment needs no OPA, and ambiguous or misplaced engine configuration fails startup.

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [ ] T056 [P] [US4] Write configuration tests in `internal/config/validator_test.go`, `internal/config/loader_test.go`, and `internal/domain/impersonation/rule_test.go`: unknown, missing, and conflicting engines; `impersonation.opa` required exactly when a rule selects OPA and empty otherwise, checked on raw values so a CEL-only configuration stays valid; exactly one of `policy.config_file` and `policy.path`; defaults applied at use (`package` `aib.broker.impersonation`, `decision` `result`, `evaluation_timeout` `100ms`, `startup_timeout` `30s`) and their bounds; raw-map rejection of every other rule `opa` key including `policy_url`, `policy_path`, `policy`, and `startup_timeout`; indexed credential-free diagnostics; and ordinary-exchange `type: opa` rejection through both the loader and loader-bypassing domain construction with valid CEL defaults unchanged (`contracts/impersonation-opa-config.yaml`, research R10, Scenarios 4.2–4.3, FR-001–FR-003, FR-009).
- [ ] T057 [P] [US4] Write mixed-rule tests in `internal/domain/impersonation/opa_mixed_rules_test.go`: per-rule validation, explicit-deny fallthrough across two OPA rules and a later CEL rule against one revision read per request (a swap during the request does not change later rules' revision), all-or-nothing admission when one rule's decision is undefined, terminal failure and delegation without fallthrough, and CEL-equivalent outcomes (Scenarios 4.1–4.3, FR-001, FR-008).

### Implementation for User Story 4

- [ ] T058 [US4] Complete configuration validation in `internal/ports/config.go`, `internal/config/loader.go` (raw-map rejection of unsupported rule `opa` keys), `internal/config/validator.go`, `internal/domain/tokenexchange/service.go` (ordinary-exchange `type: opa` rejection), and `internal/domain/impersonation/rule.go` (defaults at use): enforce `Impersonation Rule.authorization.type`, `Impersonation Rule.authorization.cel`, and `Impersonation Rule.authorization.opa` from `data-model.md` § Impersonation Rule (extended), `OPA Rule Decision.package`, `OPA Rule Decision.decision`, and `OPA Rule Decision.evaluation_timeout` from `data-model.md` § OPA Rule Decision, and `OPA Engine.policy.config_file`, `OPA Engine.policy.path`, and `OPA Engine.startup_timeout` from `data-model.md` § OPA Engine (`contracts/impersonation-opa-config.yaml`, T056, FR-001, FR-002, FR-009, FR-010).
- [ ] T059 [US4] Complete ordered mixed-engine behavior in `internal/domain/impersonation/rule.go`, `authorizer.go`, and `service.go`: a missing factory or revision fails closed, and CEL-only construction never starts an OPA engine and rejects a configured `impersonation.opa` (`contracts/internal-contracts.md` § 3, T057, FR-001, FR-008, FR-009).
- [ ] T060 [US4] Complete Scenarios 4.1–4.3 in `tests/e2e/opa_impersonation_authorization_test.go`, including allowed and denied ordinary third-party exchange, zero bundle-server contact for CEL-only deployments, and each 4.3 startup diagnostic, without changing existing CEL assertions (`spec.md` Journey 3, SC-005).
- [ ] T061 [US4] Run `just check`, `just test`, focused Ginkgo for Scenarios 4.1–4.3 in `tests/e2e/opa_impersonation_authorization_test.go`, and `just test-e2e-backend`; existing CEL-only acceptance suites stay unchanged (SC-005, SC-008, Principles VIII, XIII).

## Phase 7: Feature Documentation & Deployment Validation

- [ ] T062 Update `docs/configuration.md`, `docs/api/enduser-api.md`, and `docs/reference/token-exchange.md` (OPA impersonation section with equivalent CEL/Rego examples): config keys, defaults, and bounds; the full input and decision contract with the seven excluded credential parameters and a warning that other headers and form fields are unvalidated caller data; that exceptional modes on OPA rules rely solely on the operator's policy, with examples constraining `context.subject_token.sub`, `context.subject_token.email`, and each audience-absent role; the upgrade note that an explicit ordinary-exchange `token_exchange.authorization.type: opa` now fails startup and must be removed or set to `cel`; managed activation, per-instance convergence, all-or-nothing admission, and retained-revision staleness with no maximum age; alerting on the rejected outcome of `tokenexchange.impersonation.opa.revisions`; memory held about twice and bounding bundles with `bundles.<name>.size_limit_bytes`; local mode's restart-only updates; the token-file plugin, its rotation, and its traced client, with OPA inline credential forms untraced; recommended HTTPS and signed bundles; diagnostic closed values; correlated cancellation; and admission IDs (FR-004, FR-009, FR-011, SR-003–SR-006, Principles IV, VII, XV).
- [ ] T063 Update the package maps in `internal/AGENTS.md`, the port catalogue in `internal/ports/AGENTS.md` (`ImpersonationPolicySource`), and the adapter map in `internal/adapters/AGENTS.md` (`policy/opa` driven adapter and the shared `internal/fscontain` helper used by `jwks` and `policy/opa`) (`plan.md` § Source Code, Principles II, VI).
- [ ] T064 Validate CEL and OPA examples and rejection of mixed or misplaced blocks through the Helm schema in `charts/agentic-identity-broker/`; run `just helm-lint`, `just helm-template`, and `just helm-template-values` with synthetic valid and invalid values in a temporary directory, verifying existing mounts and no new chart key (`contracts/impersonation-opa-config.yaml` `helm`, FR-011, Principle VII).
- [ ] T065 Walk the automated and manual instructions in `specs/20261120-153502-opa-impersonation-authorization/quickstart.md` against the committed `examples/config/opa/impersonation/`; correct routine instructions only and keep observed evidence in the session result (FR-011, Principle XIV).
- [ ] T066 Run `just telemetry-doc` and commit `docs/reference/telemetry.md` if it changed, with the span and attribute descriptions added to `docs/reference/telemetry-descriptions.yaml` (`AGENTS.md` § Development Workflow, Principle XV).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

**These tasks MUST be included in every generated tasks.md file. They verify that all constitution principles have been followed.**

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist - verify Phase 2 tasks were completed correctly.

- [ ] T067 Verify every task traces to FR-001–FR-013, SR-001–SR-006, a scenario, or a binding constraint, and that every approval item in `plan.md` § Approval Items is recorded before its work in `specs/20261120-153502-opa-impersonation-authorization/tasks.md` (Feature Scope & Deferred Findings, Principles II, X).
- [ ] T068 Verify the domain model and glossary in `ARCHITECTURE.md` and the root `AGENTS.md`, the examples and index in `examples/config/`, the broker Helm schema and mount contract, and the confirmed End-User API design in `api/enduser/openapi.yaml` (Principles II, IV, V, VII, X).
- [ ] T069 Verify no migration, repository, persisted entity, or frontend, Wireloom, or Playwright obligation was introduced beyond `specs/20261120-153502-opa-impersonation-authorization/data-model.md` and `spec.md` (Principles IX, XI, XIII, XIV).
- [ ] T070 Verify all concrete journeys in `specs/20261120-153502-opa-impersonation-authorization/spec.md` match requirements and configuration, HTTP, and decoded-JWT examples, and that the 17 acceptance scenarios map 1:1 to `tests/e2e/opa_impersonation_authorization_test.go`, written before implementation with realistic expectations and no placeholders, pending markers, or red-phase comments; red-run evidence is not required (Principles VIII, XIII, XIV).
- [ ] T071 Verify `specs/20261120-153502-opa-impersonation-authorization/contracts/observability.md` covers every added log event, span, metric, and audit attribute with its value set and safety test (Principle XV).

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist - verify all principles followed during implementation.

**Feature Scope** (Development Requirements):
- [ ] T072 Verify unrelated remediation is absent from the implementation diff and user-approved issue URLs, if any, are Future Work notes, not tasks, in `specs/20261120-153502-opa-impersonation-authorization/tasks.md` (Feature Scope & Deferred Findings).

**API & Documentation** (Principles IV, X):
- [ ] T073 Verify `api/enduser/openapi.yaml` (version `1.4.0`), `docs/api/enduser-api.md`, `docs/reference/token-exchange.md`, and the HTTP implementation match the confirmed decision and error contract exactly, with unchanged request parameters, reserved claims, Admin API, and recovery behavior (Principles IV, X; FR-007–FR-009).

**Architecture & Documentation** (Principle II):
- [ ] T074 Verify `ARCHITECTURE.md` and the root `AGENTS.md` ADR index reference `adrs/20261121-061324-opa-impersonation-authorization.md` by full file stem, that the ADR cites the recorded approval and its final supersession scope, that the root `AGENTS.md` Security-First summary matches constitution v4.8.0, and that default-branch ADRs are untouched (Principle II).

**Configuration** (Principle VII):
- [ ] T075 Verify `internal/ports/config.go`, `internal/config/loader.go`, and `internal/config/validator.go` use broker-owned configuration that matches examples, docs, and the Helm schema in `charts/agentic-identity-broker/` for every changed parameter, with no ExtProc setting and no ordinary-exchange change beyond the explicit `type: opa` startup rejection (Principle VII).

**Database & Persistence** (Principle IX):
- [ ] T076 Verify no schema, migration, repository, or persistence path in the implementation diff relative to `specs/20261120-153502-opa-impersonation-authorization/data-model.md`; new migration or PostgreSQL integration tests are not applicable (Principle IX).

**Security** (Principles I, III):
- [ ] T077 Verify in `internal/domain/impersonation/` that every authentication, audience-absence, category, cross-agent, delegation, scope, and claim control stays mandatory for OPA rules; that CEL rules keep their reference and field-binding checks; that OPA is opt-in per rule; that every request error fails closed; and that no cached allow, implicit readiness, or policy-time network built-in exists (Principle I as amended in v4.8.0; SR-001–SR-006).
- [ ] T078 Verify only established OPA, jwx, CEL, and Go libraries implement policy and crypto in `internal/adapters/policy/opa/`, `internal/fscontain/`, and `internal/domain/impersonation/`, with no custom Rego parser or crypto, no `internal/extproc` import, and upgrade-safe whole-module built-in restriction (Principle III).
- [ ] T079 Verify security-critical allow, deny, error, startup, and update paths emit sanitized structured logs through injected logger plumbing in `internal/adapters/policy/opa/telemetry.go` and `internal/adapters/http/enduser/oauth2_token.go`, with no credential, native OPA error, or claim payload recorded (Principles I, XV).

**Coding Standards** (Principle XV):
- [ ] T080 Verify changed logs, spans, instruments, and attributes in `internal/adapters/policy/opa/`, `internal/domain/impersonation/`, and `internal/adapters/http/enduser/` satisfy both coding-standard review checklists: global tracer and meter lookup, canonical `error.type`, caller-canceled status, log-only revision labels and admission IDs, instrument-creation failures not affecting issuance, and callback cleanup (Principle XV).
- [ ] T081 Verify `internal/adapters/policy/opa/telemetry_test.go`, `internal/ports/impersonation_policy_test.go`, and the affected handler and domain tests assert credential exclusion on every sensitive path, metric attribute sets, span name, kind, and status, sensitive-input redaction, and admission-ID-to-deciding-snapshot pairing, with no lint rule suppressed or disabled (Principle XV).

**Architecture Patterns** (Principles VI, XII):
- [ ] T082 Verify domain admission, classification, email, and decision orchestration in `internal/domain/impersonation/` depend only on ports, OPA I/O stays in `internal/adapters/policy/opa/`, wiring and shutdown live in `internal/app/builder.go`, routing only receives handlers, and the domain request DTO has no `net/http` dependency (Principles VI, XII).

**Testing** (Principle VIII - Unit & Integration Tests):
- [ ] T083 Verify unit tests in `internal/domain/impersonation/`, `internal/adapters/policy/opa/`, `internal/fscontain/`, `internal/config/`, and `internal/adapters/http/enduser/` drove the implementation in red-green-refactor cycles and changed minimally, with no test-only production API or Bash correctness check (Principle VIII).
- [ ] T084 Run `just check` and `just test` against the final diff, including the revision-swap race tests in `internal/adapters/policy/opa/`, and resolve change-caused failures without reducing validation (`justfile`, Principles VIII, XV).

**E2E Acceptance Testing** (Principle XIII):
- [ ] T085 Verify all 17 `It()` blocks in `tests/e2e/opa_impersonation_authorization_test.go` use Ginkgo/Gomega, hierarchical contexts, scenario comments, the production bootstrap, fresh fixture, storage, and server lifecycle, `GinkgoT().TempDir()` files, checked request errors, and response cleanup, and that existing CEL assertions are unchanged (Principle XIII).
- [ ] T086 Run `just test-e2e-backend` and the focused command from `specs/20261120-153502-opa-impersonation-authorization/quickstart.md` § Automated validation; all 17 scenarios pass, including decoded claims and update, failure, and control variants (Principle XIII; SC-001–SC-008).

**Frontend** (Principle XI - if applicable):
- [ ] T087 Confirm no styled component, semantic token, universal component, accessibility, or frontend acceptance obligation exists in `web/` for this backend-only feature (Principles XI, XIII).

**Full Repository Verification & Completion** (Principles VIII, XIII, XV):
- [ ] T088 Run the container-free recipes of `just verify` from `justfile`: `just check`, the security scan, `just test`, `just web-test`, `just cdk-test`, `just mock-sample-agent-test`, `just mock-upstream-oauth2-test`, `just test-integration`, `just test-e2e-backend`, `ginkgo -v --procs=1 --skip='Agentgateway' ./tests/e2e/extproc/`, and `just test-e2e-frontend`; report container-backed suites, including the full `just test-e2e-extproc`, as deferred to pushed-commit CI, never as locally validated (`quickstart.md` § Automated validation, Principles VIII, XIII, XV).
- [ ] T089 Verify every task and checklist in `specs/20261120-153502-opa-impersonation-authorization/tasks.md` is complete or reported blocked with its reason; tick only completed tasks and commit the intended files with the repository's DCO sign-off (Principles II, VIII, XIII).

## Requirement & Acceptance Traceability

| Requirement / outcome | Implementing tasks | Acceptance / verification |
| --- | --- | --- |
| FR-001, FR-002 | T015, T027–T028, T058–T059 | T056–T061; Scenarios 4.1–4.3 |
| FR-003, SR-002 | T005, T025, T033, T035, T049–T051 | T023, T030, T045, T047–T048; Scenarios 1.1, 3.3–3.4 |
| FR-004, FR-012, SR-006 | T026, T032–T033, T049, T062 | T024, T045, T054–T055; Scenarios 3.1, 3.4–3.5 |
| FR-005, FR-006, SR-003 | T022, T034, T039–T040, T053 | T021, T037, T046; Scenarios 2.1–2.2; T080–T081 |
| FR-007, SR-004 | T032, T052–T053 | T030, T046; Scenarios 1.2, 3.2; T073, T079–T081 |
| FR-008, FR-009, SR-001 | T004, T034–T035, T041, T058–T059 | T031, T038, T057; Scenarios 1.1–1.4, 2.3–2.5, 4.1–4.2 |
| FR-010 | T049, T058 | T045, T056; Scenarios 3.2–3.3; SC-006 |
| FR-011 | T010–T012, T062–T065 | T064, T068, T073–T075 |
| FR-013, SR-005 | T028, T041–T042, T062 | T038, T077; Scenarios 2.3–2.5 |
| SC-001–SC-002 | US1 and US2 tasks | Scenarios 1.1–1.4, 2.1–2.2 |
| SC-003–SC-004, SC-006–SC-007 | US3 tasks | Scenarios 3.1–3.5 |
| SC-005, SC-008 | US2 and US4 tasks | Scenarios 2.3–2.5, 4.1–4.3 |

The exact scenario-to-`It()` mapping is `plan.md` § Scenario Mapping. T017 writes all 17 tests; T036, T044, T055, and T061 validate the 4, 5, 5, and 3 tests of US1–US4.

## Dependencies & Execution Order

### Phase Dependencies

1. Setup T001–T002 verifies approvals and prepares the ADR move; it commits no feature behavior.
2. Phase 0 T003–T006 runs existing tests, extracts the authorizer seam and `internal/fscontain`, and commits that behavior-preserving refactor first. Then commit the prepared approved ADR and document changes.
3. Phase 2 T007–T019 blocks feature logic. T015 is compile scaffolding and mechanical migration only; T015 precedes T016, which precedes T017, which precedes T018. Phase 2.7 is skipped.
4. Foundation T020–T029 follows Phase 2. T021 and T022 follow T020 in red-green order; T023, T024, and T027 can be authored concurrently; T025 follows T023, T026 follows T024, T028 follows T027, and T029 closes the foundation.
5. User stories run in priority order: **US1 (T030–T036) → US2 (T037–T044) → US3 (T045–T055) → US4 (T056–T061)**. Their tests are isolated, but their implementation shares domain and source files, so stories must not run concurrently.
6. Documentation and deployment T062–T066 follow final behavior. Phase N T067–T089 verifies every story, not only the MVP.

### User Story Dependencies

- **US1** depends on the approved configuration declarations, admission, bootstrap and capability restrictions, and all acceptance tests. It is the smallest signed governance increment; it does not complete the feature.
- **US2** depends on US1's active revision and factory, then completes input parity and the exceptional credential modes.
- **US3** depends on US1's source and revision, then completes the update, failure, local-mode, token-plugin, and telemetry matrix.
- **US4** depends on the source and error semantics; its configuration tests may be written earlier but pass only once the startup invariants are complete.

### Within Each User Story

Tests first (semantic red) → implementation → focused journey green → existing regression suite. Run `just check` after changed Go or test code, then the smallest matching `just` recipe. Use focused Ginkgo in addition to, not instead of, the maintained E2E recipe. Do not change assertions merely to make an implementation pass.

### Parallel Opportunities

- Phase 0: T004 (`internal/domain/impersonation/`) and T005 (`internal/fscontain/`, `internal/adapters/jwks/`) touch disjoint files.
- Foundation: T021, T023, T024, and T027 touch disjoint test files.
- US1: T030 (adapter tests) and T031 (domain and builder tests).
- US2: T037 (input and handler tests) and T038 (exceptional-mode domain tests).
- US3: T045 (source and revision tests), T046 (failure and telemetry tests), T047 (local bundle tests), and T048 (token plugin tests).
- US4: T056 (configuration tests) and T057 (mixed-rule tests).
- Do not parallelize tasks that edit the same rule, service, or configuration file.

## Parallel Examples by User Story

| Story | Concurrent test-authoring tasks | Files |
| --- | --- | --- |
| US1 | T030 + T031 | `internal/adapters/policy/opa/{revision,source}_test.go` vs `internal/domain/impersonation/service_test.go`, `internal/app/builder_test.go` |
| US2 | T037 + T038 | `internal/domain/impersonation/policy_input_test.go`, `internal/adapters/http/enduser/oauth2_token_test.go`, `tests/testdata/opa-envoy-input/` vs `internal/domain/impersonation/opa_security_test.go` |
| US3 | T045 + T046 + T047 + T048 | `internal/adapters/policy/opa/{source,revision}_test.go` vs `internal/domain/impersonation/opa_failure_test.go`, `internal/adapters/policy/opa/telemetry_test.go` vs `internal/adapters/policy/opa/local_test.go` vs `internal/adapters/policy/opa/tokenplugin_test.go` |
| US4 | T056 + T057 | `internal/config/{validator,loader}_test.go`, `internal/domain/impersonation/rule_test.go` vs `internal/domain/impersonation/opa_mixed_rules_test.go` |

## Implementation Strategy

### MVP First (User Story 1)

Verify approvals → isolated behavior-preserving refactor → all design preconditions and semantic E2E red → tested foundation → US1 signed governance. Demonstrate normal claims, governance denial, human delegation recovery, the local bundle variant, and a signed self-agent without human state. This is an internal checkpoint, not approval to drop the remaining stories.

### Incremental Delivery

Add US2 input parity and exceptional modes, then US3 managed revisions, failures, local mode, and the token plugin, then US4 explicit selection and CEL compatibility. Revalidate earlier stories after each increment. Finish documentation, Helm validation, telemetry reference, coding-standard and security checks, the container-free repository gates, and the final task review. Container suites are downstream validation, never local evidence. Keep partial tasks unchecked and describe remaining work in the session result.
