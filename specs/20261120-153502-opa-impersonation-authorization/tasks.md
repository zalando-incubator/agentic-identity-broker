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

- [X] T001 Verify recorded maintainer approval for every Proposed row of `plan.md` § Approval Items (ADR-OPA, API-CFG, API-BOOT, API-INPUT, API-DECISION, API-DIAG, DEC-BUILTINS) and that the CONST-AMEND text is present in `.specify/memory/constitution.md` v4.8.0; move `specs/20261120-153502-opa-impersonation-authorization/adr-20261121-061324-opa-impersonation-authorization.md` to `adrs/20261121-061324-opa-impersonation-authorization.md` with status `Accepted (approved by <login> on <date>: <url>)` from the recorded approval only, and fix its relative supersedes links and feature references. Leave work for any unapproved item unchecked; do not edit any ADR already on the default branch (Principles II, X).
- [X] T002 Confirm `go.mod` already pins `github.com/open-policy-agent/opa` v1.21.4 with the packages listed in `plan.md` § Technical Context and that no new module is needed; read `justfile` recipes and `internal/AGENTS.md`, `internal/adapters/AGENTS.md`, and `tests/e2e/AGENTS.md` before implementing (Principle III, FR-009).

## Phase 0: Pre-implementation Refactoring

**Purpose**: The behavior-preserving restructuring required by `plan.md` § Implementation Phase Overview. No configuration change or OPA implementation belongs in this commit.

- [X] T003 Run existing `internal/domain/impersonation/*_test.go` and `internal/adapters/jwks/*_test.go` through `just test` and existing impersonation acceptance scenarios through `just test-e2e-backend` before refactoring; keep their assertions unchanged (FR-008, SC-005, Principle VIII).
- [X] T004 [P] Introduce the `authorization` interface and `policyInputs` struct exactly as declared in `contracts/internal-contracts.md` § 3 (`label()`, `engine()`, `evaluate(ctx, policyInputs) (bool, error)`, `bindsSubjectField(field string) bool`) in `internal/domain/impersonation/authorizer.go`; make `internal/domain/impersonation/cel.go`'s `authorizationProgram` the CEL implementation (`bindsSubjectField` returns the compiled predicate's `sub`/`email` binding) and replace the three `rule.authz` reads per evaluation in `service.go` (`classifySubject`, email minting, and evaluation) with one authorization read, keeping the startup `References` checks in `rule.go` on the concrete CEL program before it is wrapped; keep the `Impersonation Rule.Binding requirements` of `data-model.md` § Impersonation Rule (extended), today's CEL error text, and the pipeline order unchanged (FR-008, FR-013).
- [X] T005 [P] Extract `internal/fscontain/` with the API of `contracts/internal-contracts.md` § 6 (`OpenDir`, `Dir.ReadFile(name, max)`, `Dir.Walk(maxFiles, maxBytes, accept, fn)`, `Dir.Close`, `*Error` with `Reason`) in `internal/fscontain/fscontain.go`, `open_unix.go`, and `open_other.go`, moving `openFlags` from `internal/adapters/jwks/trust_directory_unix.go` and `trust_directory_other.go`; write `internal/fscontain/fscontain_test.go` and `internal/fscontain/open_unix_test.go` (FIFO, escape, `/`, relative path, size cases); make `internal/adapters/jwks/trust_directory.go` call `ReadFile(name, trustanchor.MaxKeySetBytes)` and map each `Reason` to its existing `trustanchor.Category`, so jwks error text and categories stay byte-for-byte unchanged and the existing jwks tests pass unmodified (ADR `040-trust-anchor-local-key-sets`, FR-003).
- [X] T006 Run `just check`, `just test`, and `just test-e2e-backend` against the refactor; commit only the T004–T005 paths (`internal/domain/impersonation/`, `internal/fscontain/`, `internal/adapters/jwks/`) in their own behavior-preserving commit, ahead of the prepared T001 ADR changes and any feature code (Phase 0 rule, Principles II, VIII).

**Checkpoint**: Refactoring complete, all existing tests pass, no behavior changes introduced

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E
design before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

**🔒 CONSTITUTION REQUIREMENT**: This phase maps directly to the constitution PRECONDITIONS checklist. All sub-phases (2a-2f) MUST be included in every tasks.md, though specific task details should be adapted to the feature. Phase 2g MUST also be included when the feature adds or changes log events, spans, or metric instruments.

T015 adds only compile-enabling declarations and the fixture migration that keeps the tree building (`plan.md` § Source Code); it implements no OPA behavior and weakens no assertion.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management).

- [X] T007 Verify the value objects, state transitions, and per-request use in `specs/20261120-153502-opa-impersonation-authorization/data-model.md`, including unchanged Agent/UserGrant boundaries and no new typed ID, entity, repository, or persisted event; reconcile only routine gaps within the approved contract (FR-008, FR-013, Principles II, V, IX).
- [X] T008 Add Impersonation Policy Engine, Impersonation OPA Engine, Policy/Data Revision, and Policy Bootstrap to the `ARCHITECTURE.md` glossary and restate Impersonation Rule, Subject Category, and Agent-Subject Token as `data-model.md` § Glossary additions describes; update the impersonation boundary text in `ARCHITECTURE.md` § 3.1.y and its § 10 ADR table (a `20261121-061324-opa-impersonation-authorization` row and the partial-supersession notes on the ADR 009, 031, 034, and 045 rows); update the same glossary entries, the impersonation boundary text, and the ADR index row in the root `AGENTS.md` (Principles II, V).

**Checkpoint**: Domain model complete and documented

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design).

- [X] T009 Verify every new key, default, selected-block rule, timeout bound, bootstrap allowlist member, and indexed diagnostic against `specs/20261120-153502-opa-impersonation-authorization/contracts/impersonation-opa-config.yaml`; assign loader versus domain responsibilities, including that `OPA Engine.startup_timeout`, `OPA Rule Decision.package`, `OPA Rule Decision.decision`, and `OPA Rule Decision.evaluation_timeout` (`data-model.md` § OPA Engine, § OPA Rule Decision) default at use and never as loader or Viper defaults, that an absent or `{}` `impersonation.opa` is accepted without an OPA rule, and the explicit ordinary-exchange `type: opa` startup rejection (`plan.md` § Summary, FR-001–FR-003, FR-009, FR-010).
- [X] T010 Create approved examples in `examples/config/impersonation-opa.yaml` and `examples/config/opa/impersonation/` (`bootstrap.yaml` with the `aib_bearer_token_file` plugin, `local/governance.rego`, `local/data.json`, `local/.manifest`, and `exceptional-modes.rego`), each policy defining a `default` decision, showing the shared `impersonation.opa` engine block, equivalent CEL/Rego governance predicates, and policies that constrain `context.subject_token.sub`, `context.subject_token.email`, and each audience-absent role's token map; index them in `examples/config/README.md` (`contracts/impersonation-opa-config.yaml` `example`, `example_bootstrap`, `example_local`; `contracts/policy-input-decision.md` § 4; FR-007, FR-011, SR-005, Principle VII).
- [X] T011 Apply the `impersonation.opa` engine block (`policy` oneOf `config_file`/`path`, `startup_timeout`) and mutually exclusive CEL/OPA rule branches (OPA rule: `package`, `decision`, `evaluation_timeout` only) to `charts/agentic-identity-broker/values.schema.json`, explain the unchanged snake_case passthrough in `charts/agentic-identity-broker/values.yaml`, and document bootstrap, local bundle, and token-file Secret mounts through existing volume values in `charts/agentic-identity-broker/README.md`; keep ExtProc configuration out of the broker chart (`contracts/impersonation-opa-config.yaml` `helm`, FR-011, Principle VII).

**Checkpoint**: Configuration requirements designed with YAML examples and the applicable deployment contract documented

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development).

- [X] T012 Verify the recorded API sign-off and publish the approved `POST /oauth2/token` description delta, including the Agent subjects paragraph (CEL rules need the `sub` binding; on OPA rules an unsigned exact-target subject is agent category and the policy must constrain `sub`; cross-agent rejection), the 500 example, and version `1.4.0` (from `1.3.0`) in `api/enduser/openapi.yaml` before endpoint implementation; correct the existing `impersonation_consent_required` example's `error_description` to the endpoint's returned `User authorization is insufficient. Please re-consent.`, because the OPA delegation journey (Scenario 1.3) documents that response; confirm no Admin API, request/response envelope, or token-claim change (`contracts/openapi-token-opa.yaml`, FR-007–FR-009, Principles IV, X).

**Checkpoint**: APIs designed and confirmed by user/stakeholder

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management).

- [X] T013 Confirm the no-database-change design in `specs/20261120-153502-opa-impersonation-authorization/data-model.md` (in-memory engine snapshots only, existing Agent/UserGrant access); add no migration or repository scaffolding (FR-008, Principle IX).

**Checkpoint**: Database schema designed, migrations documented

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency).

- [X] T014 Confirm `specs/20261120-153502-opa-impersonation-authorization/spec.md` § Key Entities introduces no UI, consent flow, or frontend component; record that no frontend, Wireloom, Playwright, or screenshot work applies (Principles XI, XIII, XIV).

**Checkpoint**: Design system usage planned (if applicable)

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII (End-to-End Acceptance Testing & Spec Traceability), XIV (Specification by Example & Reviewable Journeys).

- [X] T015 Add only the compile-enabling OPA configuration declarations of `contracts/internal-contracts.md` § 1 to `internal/ports/config.go` and append the trailing `policySources PolicySourceFactory, publicURL string` parameters to `impersonation.NewService` in `internal/domain/impersonation/service.go`; in the same commit migrate every file listed under "Mechanical fixture migration" in `plan.md` § Source Code (rule literals to the new type, including `internal/ports/config_test.go`; `internal/app/builder.go` and CEL-only test call sites pass `nil, ""`), fix every remaining compile error from the type and signature change found by `go build ./... && go vet ./...`, and change only the intent of the two listed `type: "opa"` rejection cases in `internal/config/impersonation_validator_test.go` and `internal/domain/impersonation/service_test.go` to the failures they now hit; OPA stays unsupported until feature implementation (FR-001, FR-009, Principle XIII).
- [X] T016 Create every acceptance fixture before the tests: `tests/e2e/helpers/opa_bundle_server.go` (tarballs built with `bundle.Write`, controllable ETag, fetch failure and latency, optional bearer-token enforcement with a switchable expected token, and a recorder proving an `http.send` target is never contacted); the Rego and data fixtures `tests/e2e/fixtures/opa/impersonation/governance.rego`, `data.json`, `data_revoked.json`, `exceptional.rego`, `undefined.rego`, `malformed.rego`, `slow.rego`, `http_send.rego`, and `cel_equivalent.rego`, each defining a `default` decision except the deliberate undefined and malformed cases. `undefined.rego` and `malformed.rego` MUST still define the decision rule `result`, so admission accepts them: in `undefined.rego` its body is false for the Scenario 3.2 request and there is no `default`; in `malformed.rego` it returns a non-`allow`/`deny` value. Their failure must be the runtime `500` of Scenario 3.2, not the `decision_absent` startup rejection of Scenario 3.3; paired CEL and OPA rule builders for every credential mode in `tests/e2e/fixtures/impersonation_rules.go`; the production-bootstrap wrapper for the OPA policy-source factory in `tests/e2e/bootstrap/server_factory.go`; and a test-local span processor that signals evaluation start in `tests/e2e/bootstrap/telemetry.go`. Reuse `helpers.TrustDir` from `tests/e2e/helpers/local_jwks.go` for token rotation and containment, and place every file in `GinkgoT().TempDir()` (`plan.md` § Testing Strategy, research R14, Principles VIII, XIII).
- [X] T017 Write the 17 scenario-mapped `It()` blocks of `plan.md` § Scenario Mapping in `tests/e2e/opa_impersonation_authorization_test.go` before feature logic, with every variant each scenario lists: `Describe("OPA Impersonation Authorization")`, one `Context` per story, the spec scenario ID beside each `It`, table-driven variants inside the `It` for composite scenarios 2.5, 3.2, and 4.3, real HTTP status, OAuth error body, decoded-claims, startup-error, and captured-log assertions, and no placeholders, `Skip`, `XIt`, or `PIt`. Include the 1.1 local-bundle variant, the 3.1 token-rotation variant (`T1` to `T2`, emptied file keeps `r2`), the 3.2 canceled-client variant with a synthetic trace context and correlated-span, no-issuance, and no-fallthrough assertions, the 3.2 undefined and malformed variants asserting that the broker became ready (the fixture was admitted) before asserting the `500`, the 3.3 two-bundle partial-usable, complete-rejected startup variant, and zero contacts with the 3.5 `http.send` target (Scenarios 1.1–4.3, Principle XIII).
- [X] T018 Review Journeys 1–4 in `specs/20261120-153502-opa-impersonation-authorization/spec.md` § Concrete Example Journeys against the 17 scenarios, the configuration and HTTP contracts, and decoded JWT claims; run `just web-build`, then `ginkgo -v --label-filter="!performance && !docker" --focus "OPA Impersonation Authorization" ./tests/e2e/` and confirm semantic failures for absent OPA support, not compile failures (Principles VIII, XIII, XIV).

**Checkpoint**: Concrete spec journeys reviewed against scenarios and applicable configuration,
HTTP, Wireloom UI, and JWT examples; E2E tests written before implementation and, where they run
locally, seen failing semantically (container-backed suites without a local runtime first run in CI);
frontend Playwright tests added/amended and screenshots configured (if applicable)

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

**Constitution Reference**: Principle XV (Binding Coding Standards).

- [X] T019 Read `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`; verify `specs/20261120-153502-opa-impersonation-authorization/contracts/observability.md` lists the new diagnostics, the evaluation span and its value sets, the revisions counter and age gauge, the bundle download counter and download-age gauge with their bounded `bundle` attribute, the lifecycle log events including `bundle_download_failed`, the `impersonation.decision.recorded` audit attributes, the traced `aib_bearer_token_file` client through `telemetryhttp.NewTransport`, and the safety tests; reuse the existing `tokenexchange` scope and `impersonation` attribute namespace, so no standard amendment is needed (SR-003, SR-004, Principle XV).

**Checkpoint**: Observability design is listed against the binding coding standards, and each new
instrument has an observability contract entry (if applicable)

## Phase 2.7: Entity Boilerplate

Skipped: no new entity, handler, or repository (`plan.md` § Implementation Phase Overview).

## Phase 2.5: Foundational Infrastructure

**Purpose**: The runtime boundary, contained bootstrap with its closed allowlist, restricted capabilities, and decision-presence admission before any OPA issuance.

- [X] T020 Define the port of `contracts/internal-contracts.md` § 2 in `internal/ports/impersonation_policy.go`: `ImpersonationPolicyAction`, the sensitive `ImpersonationPolicyInput`, `ImpersonationPolicyDecisionRef`, `ImpersonationPolicyRevision`, `ImpersonationPolicySource`, `ImpersonationPolicyAdmission func(decisionPresent map[string]bool) error`, `ImpersonationPolicyAdmissionError`, and the two sentinel errors (FR-007, SR-002).
- [X] T021 Write semantic red tests in `internal/ports/impersonation_policy_test.go` for `LogValue`, `String`, and `GoString` through runtime interface assertions, value/pointer `%s`, `%v`, `%+v`, `%#v`, nil/empty/populated input, and `slog.Any` with JSON and text handlers using distinctive synthetic values; only `[REDACTED]` may render while evaluation still receives the full typed document (`contracts/internal-contracts.md` § 3, `contracts/policy-input-decision.md` § 2, SR-003, LOGGING § 6.4).
- [X] T022 Implement constant-redacted `LogValue`, `String`, and `GoString` on `ImpersonationPolicyInput` in `internal/ports/impersonation_policy.go`, driven by T021; never convert the input to a raw map for logging (SR-003, Principle XV).
- [X] T023 [P] Write bootstrap tests in `internal/adapters/policy/opa/bootstrap_test.go`: missing, unreadable, invalid, YAML/JSON, and oversized or non-regular files; every unsupported top-level key and every plugin other than `aib_bearer_token_file`; no bundles; `persist: true`; containment through `internal/fscontain` over the file's own directory (relative `..data` links accepted; escaping and absolute symlinks, relative paths, and a parent of `/` rejected); every accepted allowlist member accepted and one case per rejected member class of `bootstrap_subset.allowlist` (bearer `token_path`, OAuth2 assertion, signing-key, and `client_assertion_path` members, `s3_signing` web-identity, profile, and SSO credentials, service `tls`, `client_tls`, `allow_insecure_tls`, `type`, `keys.<id>.private_key`, `signing.allow_unsigned_delta_bundles`, and `file://` bundles); accepted `s3_signing` environment, metadata, and assume-role forms, `gcp_metadata`, and `azure_managed_identity`; diagnostics name keys and reason classes, never values (`contracts/impersonation-opa-config.yaml` `file_containment`, `bootstrap_subset`, `startup_diagnostics`; FR-003, SR-003, SR-006).
- [X] T024 [P] Write capability and decision-presence tests in `internal/adapters/policy/opa/capabilities_test.go`: each of the 9 removed built-ins (`http.send`, `net.lookup_ip_addr`, `opa.runtime`, `rand.intn`, `uuid.rfc4122`, `io.jwt.encode_sign`, `io.jwt.encode_sign_raw`, `json.verify_schema`, `json.match_schema`) is rejected as `prohibited_builtin` even in unreachable modules; `time.now_ns` and `io.jwt.decode_verify` stay allowed; print is disabled; other compile errors classify as `compile_failed`; and the decision-presence flag is true only when `GetRulesExact` finds a rule for `data.<package>.<decision>`, false for a data-only value or only nested rules below the ref (`contracts/policy-input-decision.md` § 5, research R4, R6, FR-012, SC-007).
- [X] T025 Implement startup-only bootstrap reading in `internal/adapters/policy/opa/bootstrap.go` through `internal/fscontain` with a same-descriptor 1 MiB bound and validation against the closed allowlist before `sdk.New`, with sanitized reason classes; fulfill `OPA Engine.policy.config_file` from `data-model.md` § OPA Engine (T023, FR-003, SR-003).
- [X] T026 Implement restricted capabilities in `internal/adapters/policy/opa/capabilities.go` (`ast.CapabilitiesForThisVersion()` minus every nondeterministic built-in except `time.now_ns` and `io.jwt.decode_verify`), one broker-owned `ast.NewCompiler().WithCapabilities(restricted)` per candidate whose compiler also prepares every rule's query through `rego.Compiler`, the `GetRulesExact` decision-presence flags, and classification of an undefined-function error naming a removed built-in as `prohibited_builtin` (`plan.md` § Summary, research R4, R6, T024, FR-012).
- [X] T027 [P] Write admission tests in `internal/domain/impersonation/admission_test.go`: a candidate is admitted only when every OPA rule's decision rule exists, rejection names the first failing rule in rule order through `ImpersonationPolicyAdmissionError`, and CEL rules are never passed to OPA admission (`contracts/internal-contracts.md` § 3, research R6, R11, FR-001).
- [X] T028 Implement the OPA admission function in `internal/domain/impersonation/admission.go`, driven by T027, with no reference or field-binding requirement for OPA rules (`contracts/policy-input-decision.md` § 4, SR-005).
- [X] T029 Run `just check` and `just test` for the foundation; verify no source enters readiness without domain admission and that `internal/adapters/policy/opa/` imports neither another adapter nor `internal/extproc` (FR-009, Principles VI, VIII).

**Checkpoint**: Bootstrap, capabilities, and admission are tested. Runtime and issuance are not yet complete.

## Phase 3: User Story 1 - Enforce organizational governance before issuing a token (Priority: P1) 🎯 MVP

**Goal**: One OPA rule, managed or local, authorizes governed delegated humans and eligible signed self-agents while every existing issuance control stays authoritative.
**Independent Test**: Scenarios 1.1–1.4: an approved human request and a signed self-agent request mint the normal claims; governance marking the agent unregistered denies with `403 access_denied`; absent human delegation returns the existing recovery URL; the local bundle variant of 1.1 gives the same `200`.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

- [X] T030 [P] [US1] Write decision and readiness tests in `internal/adapters/policy/opa/revision_test.go` and `internal/adapters/policy/opa/source_test.go`: allow, deny, exactly one object result, ignored extra members, strict built-in errors, a decision rule that exists but is undefined for the input and malformed or unsupported actions mapping to `ErrPolicyDecisionInvalid`, a decision rule absent from the candidate rejecting admission, and readiness on the first admitted complete snapshot through `AwaitReady` (`contracts/policy-input-decision.md` § 3, Scenarios 1.1–1.2, FR-007, SR-002).
- [X] T031 [P] [US1] Write local bundle-directory tests in `internal/adapters/policy/opa/local_test.go`: a valid directory admitted through the same capability and admission path, `policy_revision` from `.manifest`, unsupported file types, symlinks escaping the directory, a relative path, a directory resolving to `/`, more than 1,000 files or 16 MiB through `fscontain.Dir.Walk`, no polling or later re-read, and the once-only `oauth2_server.impersonation_policy.local_mode` record without file contents (`contracts/impersonation-opa-config.yaml` `engine`, research R15, Scenario 1.1 local variant, FR-003).
- [X] T032 [P] [US1] Write OPA-rule service and builder tests in `internal/domain/impersonation/service_test.go`, `internal/domain/impersonation/rule_test.go`, and `internal/app/builder_test.go` with domain-local port doubles: `validateRuleInvariants` accepting `type: opa`, the four defaults applied at use, `NewService` waiting with `AwaitReady` under `startup_timeout` and stopping the source on failure, a nil revision failing closed, normal signed claims, governance denial, delegation after the OPA allow, signed self-agent issuance with no human state, scope and reserved claims, the input and transport metadata Journey 1 needs, interim failures returning the existing terminal `server_error`, and factory and `App.chainShutdown` wiring (Scenarios 1.1–1.4, `contracts/internal-contracts.md` § 3, § 5, FR-003, FR-008, SR-001).

### Implementation for User Story 1

- [X] T033 [US1] Implement immutable revisions and decision parsing in `internal/adapters/policy/opa/revision.go`, fulfilling `Policy/Data Revision.Prepared queries` and `Policy/Data Revision.Data snapshot` from `data-model.md` § Policy/Data Revision and the value mapping of `data-model.md` § Impersonation Policy Decision; `Evaluate` applies the rule's `OPA Rule Decision.evaluation_timeout` (research R2, T030, FR-004, FR-007).
- [X] T034 [US1] Implement the managed shared engine in `internal/adapters/policy/opa/source.go`: one `sdk.New` with an in-memory store and `OnCommit` trigger, candidates built from one transaction with one prepared query per OPA rule, all-configured-bundle completeness before admission, one admission call per candidate (all-or-nothing), an `atomic.Pointer` swap, and `AwaitReady` on that same admitted snapshot; fulfill `Policy/Data Revision.Label`, `Policy/Data Revision.Admission ID` (assigned only on successful admission), `Policy/Data Revision.Bundle completeness`, and `Policy/Data Revision.Admitted at` from `data-model.md` § Policy/Data Revision (`contracts/internal-contracts.md` § 4, T030, FR-003, FR-004).
- [X] T035 [US1] Create `internal/adapters/policy/opa/telemetry.go` with the global-scope logger helpers and the lifecycle log records of `contracts/observability.md` § Log records, then implement local bundle-directory loading in `internal/adapters/policy/opa/local.go` through `fscontain.Dir.Walk`, building one candidate with OPA's bundle reader from the read bytes and no SDK runtime, and emitting the once-only `local_mode` record; fulfill `OPA Engine.policy.path` from `data-model.md` § OPA Engine (T031, research R15, FR-003).
- [X] T036 [US1] Wire OPA rules into `internal/domain/impersonation/rule.go` and `service.go`: accept `type: opa` in `validateRuleInvariants`, apply the startup `References` checks for unsigned-subject and audience-absent roles in `rule.go` only to CEL rules (SR-005), apply the four OPA defaults at use, start the source and call `AwaitReady` with `context.WithTimeout(context.Background(), startup_timeout)` in `NewService`, fail closed when the factory or revision is nil, read `source.Current()` once per request before the first rule, keep CEL extraction with `defaultEvaluationTimeout`, and keep human delegation and target-derived agent minting unchanged; until T050, `ErrPolicyDecisionInvalid` and `ErrPolicyEvaluationFailed` return the existing terminal `server_error` path without fallthrough. Build the initial input in `internal/domain/impersonation/policy_input.go` with `type: "impersonation"` and broker-resolved `context.agent {id, canonical_id?, external_id?}` without UUID fallback, add the transport DTO in `request.go`, and set `req.HTTP` after `impersonation.ParseRequest` in `internal/adapters/http/enduser/oauth2_token.go`; fulfill `data-model.md` § Impersonation Policy Input through `contracts/policy-input-decision.md` § 1 (T032, FR-005, FR-006, FR-008, SR-001).
- [X] T037 [US1] Construct the policy-source factory only in `internal/app/builder.go` with the injected logger, pass the configured end-user public URL as `NewService`'s trailing `publicURL`, and register `Service.Close(ctx) error` through `App.chainShutdown`; complete the T016 wrapper in `tests/e2e/bootstrap/server_factory.go`, not routing logic (`contracts/internal-contracts.md` § 5, T032, FR-003, Principles VI, XII).
- [X] T038 [US1] Make Scenarios 1.1–1.4 in `tests/e2e/opa_impersonation_authorization_test.go` pass, with fixture adjustments only; run `just check`, `just test`, focused Ginkgo for Scenarios 1.1–1.4, then `just test-e2e-backend`; confirm normal claims, consent recovery, the local bundle variant, and no human grant or session for the signed self-agent (SC-001, SC-002, Principles VIII, XIII).

**Checkpoint**: The signed governance MVP works in managed and local mode. Input parity, exceptional modes, the update and failure matrix, and migration checks remain mandatory before completion.

## Phase 4: User Story 2 - Reuse policy input conventions without losing identity context (Priority: P1)

**Goal**: Complete Envoy-compatible transport facts and CEL-equivalent trusted context, including every exceptional credential mode on OPA rules.
**Independent Test**: Scenarios 2.1–2.5 exercise context parity, spoof-resistant context, unsigned human with email, unsigned exact-target self-agent with cross-agent rejection, and audience-absent signed-role combinations.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

- [X] T039 [P] [US2] Write input and handler tests in `internal/domain/impersonation/policy_input_test.go` and `internal/adapters/http/enduser/oauth2_token_test.go`: every input member, optional claims, the same values and types as the CEL activation for numbers, arrays, and nested objects, repeated and filtered headers, public-URL host and scheme, `version`, `parsed_path`, `parsed_query`, `parsed_body` from the raw parse copy (repeated keys as arrays, `null` for other content types, and `client_assertion`, `actor_token`, `subject_token`, `client_secret`, `refresh_token`, `code`, and `code_verifier` absent even when the handler passes them), `truncated_body: false`, no raw body or token string anywhere, `type: "impersonation"`, broker-authoritative `context.agent` with unset IDs omitted, and every reserved ExtProc member absent; create the vectors `tests/testdata/opa-envoy-input/basic_post.json`, `repeated_headers.json`, `query_string.json`, and `filtered_headers.json` in the schema of `plan.md` § Unit & Integration Tests (with `expected_path` per builder) and consume them in both the broker test and `internal/extproc/authorization/input_builder_test.go` without production cross-binary imports (`contracts/policy-input-decision.md` § 1, Scenarios 2.1–2.2, FR-005, FR-006, SR-003).
- [X] T040 [P] [US2] Write exceptional-mode tests in `internal/domain/impersonation/opa_security_test.go`: on an OPA rule the unsigned human's extracted email is minted when the decision allows, an unsigned raw `sub` equal to the target UUID is agent category with no human grant or session, another registered agent's raw `sub` or extracted principal is a rejected cross-agent request without consent link or human fallback, and each audience-absent role and valid combination succeeds while a present `aud`, bad signature, wrong issuer or algorithm, or expiry fails even when OPA allows; on a CEL rule the existing reference and field-binding checks and outcomes are unchanged (`contracts/policy-input-decision.md` § 4, Scenarios 2.3–2.5, FR-013, SR-001, SR-005).

### Implementation for User Story 2

- [X] T041 [US2] Complete type-preserving and edge-case input construction in `internal/domain/impersonation/policy_input.go` and `RequestAttributes{Method, Path, RawQuery, Protocol, ContentType, Headers, Form}` in `internal/domain/impersonation/request.go`, building `parsed_body` from the raw parse copy and deleting the seven credential-bearing OAuth parameters in the domain, without importing `net/http` into the domain (`contracts/internal-contracts.md` § 3, T039, FR-005, FR-006, SR-003).
- [X] T042 [US2] Complete repeated-header, query, protocol, content-type, and raw-form capture (a copy taken before `dropOmittedValues`, passed through `handleTokenExchange`) in `internal/adapters/http/enduser/oauth2_token.go`, keeping claim derivation and policy decisions out of the handler (`contracts/internal-contracts.md` § 3, § 5, T039, FR-006, Principle VI).
- [X] T043 [US2] Implement the OPA `authorization` in `internal/domain/impersonation/authorizer.go` (`engine()` `opa`, `bindsSubjectField` always true, `evaluate` through the request's revision) and the exceptional-mode handling in `service.go`: exact-target classification of unsigned subjects, minting of the extracted email when the decision allows, cross-agent rejection on raw and extracted identities, and unchanged cryptographic, audience-absence, category, and delegation guards; CEL rules keep their `Impersonation Rule.Binding requirements` from `data-model.md` § Impersonation Rule (extended) (`contracts/internal-contracts.md` § 3, `contracts/policy-input-decision.md` § 4, T040, FR-013, SR-001, SR-005).
- [X] T044 [US2] Make Scenarios 2.1–2.5 in `tests/e2e/opa_impersonation_authorization_test.go` pass through the production bootstrap, with fixture adjustments only and no test-only production getter; run `just check`, `just test`, focused Ginkgo for Scenarios 2.1–2.5, and `just test-e2e-backend` (SC-002, SC-008, Principles VI, VIII, XIII).

## Phase 5: User Story 3 - Apply managed policy and data changes safely (Priority: P2)

**Goal**: Live activation, last-admitted-revision retention, bounded startup and evaluation, credential-free diagnostics, observable bundle downloads, and a rotating, traced bundle token.
**Independent Test**: Scenarios 3.1–3.5 revoke through an ETag update and token rotation, make runtime failures terminal without fallthrough, fail unusable startup, retain `r1` after failed updates, and reject `http.send` revisions without contacting the destination.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

- [X] T045 [P] [US3] Extend `internal/adapters/policy/opa/source_test.go` and `revision_test.go` with multiple bundles, same-snapshot completeness, a usable first-bundle subset followed by a rejected complete two-bundle candidate (neither admitted nor ready), equal or empty manifest labels producing distinct admission IDs, rejected, fetch-failed, and partial updates keeping the active revision, a rejected bundle blocking later updates to other bundles until republished, one shared revision for every OPA rule with all-or-nothing rejection when one rule's decision rule is absent, cancellation and timeouts, a request completing within its `evaluation_timeout` against the active revision while a bundle fetch is stalled by the bundle server's latency control, in-flight snapshots across swaps, shutdown cleanup, and `-race` stress during revision swaps (research R2, R7, R11; Scenarios 3.1, 3.3–3.5; FR-003, FR-004, FR-010, FR-012, SC-006).
- [X] T046 [P] [US3] Write abort and diagnostic tests in `internal/domain/impersonation/opa_failure_test.go` and telemetry tests in `internal/adapters/policy/opa/telemetry_test.go`: distinctive synthetic token, claim, decision, and bootstrap values never appear in logs, spans, or metrics for allow, deny, invalid, error, rejection, download failure, and startup; span name, kind, `error.type`, and `canceled` outcome; revisions counter `outcome`/`reason` and age gauge; bundle download counter outcomes, the download-age gauge growing while downloads fail, and the rate-limited `bundle_download_failed` log without URL, body, or token, all with a manual reader against the in-process bundle server; domain cancellation maps to terminal `server_error` without fallthrough; no admission ID or revision label in a span or metric (`contracts/observability.md`, Scenarios 3.2–3.4, SR-003, SR-004, Principle XV).
- [X] T047 [P] [US3] Write `aib_bearer_token_file` tests in `internal/adapters/policy/opa/tokenplugin_test.go`: startup validation (missing, relative, escaping or absolute symlink, empty, over 64 KiB), rotation to a link outside the directory failing that download, the token re-read before every bundle request against an `httptest` server, rotation without restart, a later read failure keeping the active revision with rate-limited `token_unavailable` and no contents, `credentials.plugin` naming another plugin rejected, a focused check that OPA v1.21.4 resolves `credentials.plugin` through `sdk.Options.Plugins`, and a client span from the `telemetryhttp.NewTransport`-wrapped transport without the token as an attribute (research R16, `contracts/observability.md` § Span, Scenario 3.1).

### Implementation for User Story 3

- [X] T048 [US3] Complete bounded same-snapshot readiness (partial candidates never admitted), engine cleanup on failure, cancellation-aware deadlines, and immutable last-admitted-revision retention in `internal/adapters/policy/opa/source.go`, `revision.go`, and `internal/domain/impersonation/service.go`; enforce `OPA Engine.startup_timeout` and `OPA Rule Decision.evaluation_timeout` from `data-model.md` § OPA Engine and § OPA Rule Decision (`contracts/internal-contracts.md` § 3–4, T045, FR-003, FR-010, FR-012).
- [X] T049 [US3] Implement the `aib_bearer_token_file` `rest.HTTPAuthPlugin` and its `plugins.Factory` in `internal/adapters/policy/opa/tokenplugin.go`, registered in `sdk.Options.Plugins` only when the bootstrap configures it; `NewClient` wraps its transport with `telemetryhttp.NewTransport`, and `Prepare` reads the token through `fscontain.Dir.ReadFile` before every bundle request. If T047's lookup check fails, fall back to inline credentials only and reject the plugin key in `bootstrap.go` (T047, research R16).
- [X] T050 [US3] Map invalid decisions and evaluation failures to terminal OAuth server errors in `internal/domain/impersonation/errors.go` and `service.go` and add `opa_evaluation_failed` and `opa_decision_invalid` to `internal/domain/tokenexchange/diagnostic.go`, replacing T036's interim mapping; preserve the caller-cancellation override and prevent later-rule fallthrough (`contracts/policy-input-decision.md` § 3, `contracts/observability.md` § Diagnostics, T046, FR-007, SR-002).
- [X] T051 [US3] Extend `internal/adapters/policy/opa/telemetry.go` with the global-scope tracer and meter, the evaluation span, the revisions counter and age-gauge callbacks with cleanup, and the bundle status listener registered through `sdk.OPA.Plugin("bundle")` and `bundle.Plugin.Register` that feeds `tokenexchange.impersonation_opa.bundle_downloads`, `tokenexchange.impersonation_opa.bundle_download_age`, and the rate-limited `bundle_download_failed` log; add `authorization_engine`, `policy_revision`, and `policy_admission_id` to the `impersonation.decision.recorded` audit in `internal/domain/impersonation/service.go` and `internal/adapters/http/enduser/oauth2_token.go`, copying the deciding revision's values even when labels are equal or empty (`contracts/observability.md`, T046, SR-003, SR-004, Principle XV).
- [X] T052 [US3] Add the span, metric, and attribute descriptions to `docs/reference/telemetry-descriptions.yaml`, run `just telemetry-doc`, and include `docs/reference/telemetry.md` in the change, so `TestReferenceIsCurrent` in `tools/telemetrydoc/` keeps `just test` green (`AGENTS.md` § Development Workflow, T051, Principle XV).
- [X] T053 [US3] Make Scenarios 3.1–3.5 in `tests/e2e/opa_impersonation_authorization_test.go` pass with T016's helper and span processor, with fixture adjustments only; run `just check`, `just test` (including race tests), focused Ginkgo for Scenarios 3.1–3.5, and `just test-e2e-backend`; verify deadlines with scheduling tolerance, not fixed sleeps (`spec.md` Journey 2, SC-003, SC-004, SC-006, SC-007, Principles VIII, XIII).

## Phase 6: User Story 4 - Adopt OPA one rule at a time (Priority: P2)

**Goal**: Explicit engine selection with unchanged valid CEL-only and ordinary-exchange behavior; explicit ordinary-exchange `type: opa` fails startup.
**Independent Test**: Scenarios 4.1–4.3 prove an explicit OPA deny falls through to a later CEL rule, a CEL-only deployment needs no OPA, and ambiguous or misplaced engine configuration fails startup.

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

- [X] T054 [P] [US4] Write configuration tests in `internal/config/validator_test.go`, `internal/config/loader_test.go`, and `internal/domain/impersonation/rule_test.go`: unknown, missing, and conflicting engines; `impersonation.opa` required when a rule selects OPA, and absent or `{}` accepted but any populated key rejected when none does, checked on raw values so a CEL-only configuration stays valid; exactly one of `policy.config_file` and `policy.path`; defaults applied at use (`package` `aib.broker.impersonation`, `decision` `result`, `evaluation_timeout` `100ms`, `startup_timeout` `30s`) and their bounds; an invalid `package` path (empty segment, non-identifier segment) and an invalid `decision` identifier rejected with the indexed diagnostic (`data-model.md` § OPA Rule Decision); decode-hook rejection of every other rule `opa` key including `policy_url`, `policy_path`, `policy`, and `startup_timeout` with the indexed `rules[<i>] (<name>)` diagnostic; and ordinary-exchange `type: opa` rejection through both the loader and loader-bypassing domain construction with valid CEL defaults unchanged (`contracts/impersonation-opa-config.yaml`, `contracts/internal-contracts.md` § 1, research R10, Scenarios 4.2–4.3, FR-001–FR-003, FR-009).
- [X] T055 [P] [US4] Write mixed-rule tests in `internal/domain/impersonation/opa_mixed_rules_test.go`: per-rule validation, explicit-deny fallthrough across two OPA rules and a later CEL rule against one revision read per request (a swap during the request does not change later rules' revision), all-or-nothing admission when one rule's decision rule is absent, terminal failure and delegation without fallthrough, and CEL-equivalent outcomes (Scenarios 4.1–4.3, FR-001, FR-008).

### Implementation for User Story 4

- [X] T056 [US4] Complete configuration validation in `internal/ports/config.go`, `internal/config/loader.go` (a decode hook for `ImpersonationRuleOPADecision` following `rejectNonSequenceSubjectCategoriesHook`), `internal/config/validator.go`, `internal/domain/tokenexchange/service.go` (ordinary-exchange `type: opa` rejection), and `internal/domain/impersonation/rule.go`: enforce `Impersonation Rule.authorization.type`, `Impersonation Rule.authorization.cel`, and `Impersonation Rule.authorization.opa` from `data-model.md` § Impersonation Rule (extended), `OPA Rule Decision.package`, `OPA Rule Decision.decision`, and `OPA Rule Decision.evaluation_timeout` from `data-model.md` § OPA Rule Decision, and `OPA Engine.policy.config_file`, `OPA Engine.policy.path`, and `OPA Engine.startup_timeout` from `data-model.md` § OPA Engine (`contracts/impersonation-opa-config.yaml`, T054, FR-001, FR-002, FR-009, FR-010).
- [X] T057 [US4] Complete ordered mixed-engine behavior in `internal/domain/impersonation/rule.go`, `authorizer.go`, and `service.go`: CEL-only construction never starts an OPA engine and rejects a populated `impersonation.opa` (`contracts/internal-contracts.md` § 3, T055, FR-001, FR-008, FR-009).
- [X] T058 [US4] Make Scenarios 4.1–4.3 in `tests/e2e/opa_impersonation_authorization_test.go` pass, including allowed and denied ordinary third-party exchange, zero bundle-server contact for CEL-only deployments, and each 4.3 startup diagnostic, with fixture adjustments only and existing CEL assertions unchanged; run `just check`, `just test`, focused Ginkgo for Scenarios 4.1–4.3, and `just test-e2e-backend` (`spec.md` Journey 3, SC-005, SC-008, Principles VIII, XIII).

## Phase 7: Feature Documentation & Deployment Validation

- [ ] T059 Update `docs/configuration.md`, `docs/api/enduser-api.md`, and `docs/reference/token-exchange.md` (OPA impersonation section with equivalent CEL/Rego examples): config keys, defaults, bounds, and the closed bootstrap allowlist including cloud credentials that read no file; the full input and decision contract with the seven excluded credential parameters and a warning that other headers and form fields are unvalidated caller data; that every policy needs a `default` decision because an undefined decision is a terminal 500 that also blocks later rules; the built-ins removed and kept (`time.now_ns`, `io.jwt.decode_verify`); that exceptional modes on OPA rules rely solely on the operator's policy, with examples constraining `context.subject_token.sub`, `context.subject_token.email`, and each audience-absent role; the upgrade note that an explicit ordinary-exchange `token_exchange.authorization.type: opa` now fails startup and must be removed or set to `cel`; managed activation, per-instance convergence, all-or-nothing admission, a rejected bundle blocking other bundles until republished, and retained-revision staleness with no maximum age; alerting on the rejected outcome of `tokenexchange.impersonation_opa.revisions` and on failing bundle downloads; that broker revision telemetry, not OPA bundle status, is authoritative for the active revision; that a request falling through N OPA rules can wait up to the sum of their `evaluation_timeout` values; that a universal governance restriction must be applied to every rule that could permit the request, because a later rule can authorize after an earlier OPA deny; memory held about twice, the fleet-wide out-of-memory and crash-loop risk, and bounding bundles with `bundles.<name>.size_limit_bytes`; local mode's restart-only updates; the token-file plugin, its rotation, and its traced client, with OPA inline credential forms untraced; recommended HTTPS and signed bundles; diagnostic closed values; correlated cancellation; and admission IDs (FR-004, FR-007, FR-009, FR-011, SR-003–SR-006, Principles IV, VII, XV).
- [ ] T060 Update the package maps in `internal/AGENTS.md`, the port catalogue in `internal/ports/AGENTS.md` (`ImpersonationPolicySource`), and the adapter map in `internal/adapters/AGENTS.md` (`policy/opa` driven adapter and the shared `internal/fscontain` helper used by `jwks` and `policy/opa`) (`plan.md` § Source Code, Principles II, VI).
- [ ] T061 Validate CEL and OPA examples and rejection of mixed or misplaced blocks through the Helm schema in `charts/agentic-identity-broker/`; run `just helm-lint`, `just helm-template`, and `just helm-template-values` with synthetic valid and invalid values in a temporary directory, verifying existing mounts and no new chart key (`contracts/impersonation-opa-config.yaml` `helm`, FR-011, Principle VII).
- [ ] T062 Walk the automated and manual instructions in `specs/20261120-153502-opa-impersonation-authorization/quickstart.md` against the committed `examples/config/opa/impersonation/`; correct routine instructions only and keep observed evidence in the session result (FR-011, Principle XIV).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

**These tasks MUST be included in every generated tasks.md file. They verify that all constitution principles have been followed.**

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist - verify Phase 2 tasks were completed correctly.

- [ ] T063 Verify every task traces to FR-001–FR-013, SR-001–SR-006, a scenario, or a binding constraint, and that every approval item in `plan.md` § Approval Items is recorded before its work in `specs/20261120-153502-opa-impersonation-authorization/tasks.md` (Feature Scope & Deferred Findings, Principles II, X).
- [ ] T064 Verify the domain model and glossary in `ARCHITECTURE.md` and the root `AGENTS.md`, the examples and index in `examples/config/`, the broker Helm schema and mount contract, and the confirmed End-User API design in `api/enduser/openapi.yaml` (Principles II, IV, V, VII, X).
- [ ] T065 Verify no migration, repository, persisted entity, or frontend, Wireloom, or Playwright obligation was introduced beyond `specs/20261120-153502-opa-impersonation-authorization/data-model.md` and `spec.md` (Principles IX, XI, XIII, XIV).
- [ ] T066 Verify all concrete journeys in `specs/20261120-153502-opa-impersonation-authorization/spec.md` match requirements and configuration, HTTP, and decoded-JWT examples, and that the 17 acceptance scenarios map 1:1 to `tests/e2e/opa_impersonation_authorization_test.go`, written before implementation with realistic expectations and no placeholders, pending markers, or red-phase comments; red-run evidence is not required (Principles VIII, XIII, XIV).
- [ ] T067 Verify `specs/20261120-153502-opa-impersonation-authorization/contracts/observability.md` covers every added log event, span, metric, and audit attribute with its value set and safety test (Principle XV).

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist - verify all principles followed during implementation.

**Feature Scope** (Development Requirements):
- [ ] T068 Verify unrelated remediation is absent from the implementation diff and user-approved issue URLs, if any, are Future Work notes, not tasks, in `specs/20261120-153502-opa-impersonation-authorization/tasks.md` (Feature Scope & Deferred Findings).

**API & Documentation** (Principles IV, X):
- [ ] T069 Verify `api/enduser/openapi.yaml` (version `1.4.0`), `docs/api/enduser-api.md`, `docs/reference/token-exchange.md`, and the HTTP implementation match the confirmed decision and error contract exactly, with unchanged request parameters, reserved claims, Admin API, and recovery behavior (Principles IV, X; FR-007–FR-009).

**Architecture & Documentation** (Principle II):
- [ ] T070 Verify `ARCHITECTURE.md` § 3.1.y and § 10 and the root `AGENTS.md` ADR index reference `adrs/20261121-061324-opa-impersonation-authorization.md` by full file stem, that the ADR cites the recorded approval and its final supersession scope, that the root `AGENTS.md` Security-First summary matches constitution v4.8.0, and that default-branch ADRs are untouched (Principle II).

**Configuration** (Principle VII):
- [ ] T071 Verify `internal/ports/config.go`, `internal/config/loader.go`, and `internal/config/validator.go` use broker-owned configuration that matches examples, docs, and the Helm schema in `charts/agentic-identity-broker/` for every changed parameter, with no ExtProc setting and no ordinary-exchange change beyond the explicit `type: opa` startup rejection (Principle VII).

**Database & Persistence** (Principle IX):
- [ ] T072 Verify no schema, migration, repository, or persistence path in the implementation diff relative to `specs/20261120-153502-opa-impersonation-authorization/data-model.md`; new migration or PostgreSQL integration tests are not applicable (Principle IX).

**Security** (Principles I, III):
- [ ] T073 Verify in `internal/domain/impersonation/` that every authentication, audience-absence, category, cross-agent, delegation, scope, and claim control stays mandatory for OPA rules; that CEL rules keep their reference and field-binding checks; that OPA is opt-in per rule; that every request error fails closed; and that no cached allow, implicit readiness, or policy-time network built-in exists (Principle I as amended in v4.8.0; SR-001–SR-006).
- [ ] T074 Verify only established OPA, jwx, CEL, and Go libraries implement policy and crypto in `internal/adapters/policy/opa/`, `internal/fscontain/`, and `internal/domain/impersonation/`, with no custom Rego parser or crypto, no `internal/extproc` import, upgrade-safe whole-module built-in restriction, and a closed bootstrap allowlist (Principle III).
- [ ] T075 Verify security-critical allow, deny, error, startup, update, and download-failure paths emit sanitized structured logs through injected logger plumbing in `internal/adapters/policy/opa/telemetry.go` and `internal/adapters/http/enduser/oauth2_token.go`, with no credential, native OPA error, URL, or claim payload recorded (Principles I, XV).

**Coding Standards** (Principle XV):
- [ ] T076 Verify changed logs, spans, instruments, and attributes in `internal/adapters/policy/opa/`, `internal/domain/impersonation/`, and `internal/adapters/http/enduser/` satisfy both coding-standard review checklists: global tracer and meter lookup, canonical `error.type`, caller-canceled status, log-only revision labels and admission IDs, the bounded `bundle` attribute, instrument-creation failures not affecting issuance, and callback cleanup (Principle XV).
- [ ] T077 Verify `internal/adapters/policy/opa/telemetry_test.go`, `internal/ports/impersonation_policy_test.go`, and the affected handler and domain tests assert credential exclusion on every sensitive path, metric attribute sets, span name, kind, and status, sensitive-input redaction, and admission-ID-to-deciding-snapshot pairing, with no lint rule suppressed or disabled (Principle XV).

**Architecture Patterns** (Principles VI, XII):
- [ ] T078 Verify domain admission, classification, email, and decision orchestration in `internal/domain/impersonation/` depend only on ports, OPA I/O stays in `internal/adapters/policy/opa/`, wiring and shutdown live in `internal/app/builder.go`, routing only receives handlers, and the domain request DTO has no `net/http` dependency (Principles VI, XII).

**Testing** (Principle VIII - Unit & Integration Tests):
- [ ] T079 Verify unit tests in `internal/domain/impersonation/`, `internal/adapters/policy/opa/`, `internal/fscontain/`, `internal/config/`, and `internal/adapters/http/enduser/` drove the implementation in red-green-refactor cycles and changed minimally, with no test-only production API or Bash correctness check (Principle VIII).
- [ ] T080 Run `just check` and `just test` against the final diff, including the revision-swap race tests in `internal/adapters/policy/opa/` and `TestReferenceIsCurrent`, and resolve change-caused failures without reducing validation (`justfile`, Principles VIII, XV).

**E2E Acceptance Testing** (Principle XIII):
- [ ] T081 Verify all 17 `It()` blocks in `tests/e2e/opa_impersonation_authorization_test.go` use Ginkgo/Gomega, hierarchical contexts, scenario comments, the production bootstrap, fresh fixture, storage, and server lifecycle, `GinkgoT().TempDir()` files, checked request errors, and response cleanup, and that existing CEL assertions are unchanged (Principle XIII).
- [ ] T082 Run `just test-e2e-backend` and the focused command from `specs/20261120-153502-opa-impersonation-authorization/quickstart.md` § Automated validation; all 17 scenarios pass, including decoded claims and update, failure, and control variants (Principle XIII; SC-001–SC-008).

**Frontend** (Principle XI - if applicable):
- [ ] T083 Confirm no styled component, semantic token, universal component, accessibility, or frontend acceptance obligation exists in `web/` for this backend-only feature (Principles XI, XIII).

**Full Repository Verification & Completion** (Principles VIII, XIII, XV):
- [ ] T084 Run the container-free recipes of `just verify` from `justfile`: `just check`, `just security`, `just test`, `just lint-plugin-test`, `just web-test`, `just cdk-test`, `just mock-sample-agent-test`, `just mock-upstream-oauth2-test`, `just test-integration`, `just test-e2e-backend`, `ginkgo -v --procs=1 --skip='Agentgateway' ./tests/e2e/extproc/`, and `just test-e2e-frontend`; report container-backed suites, including the full `just test-e2e-extproc`, as deferred to pushed-commit CI, never as locally validated (`quickstart.md` § Automated validation, Principles VIII, XIII, XV).
- [ ] T085 Verify every task and checklist in `specs/20261120-153502-opa-impersonation-authorization/tasks.md` is complete or reported blocked with its reason; tick only completed tasks and commit the intended files with the repository's DCO sign-off (Principles II, VIII, XIII).

## Requirement & Acceptance Traceability

| Requirement / outcome | Implementing tasks | Acceptance / verification |
| --- | --- | --- |
| FR-001, FR-002 | T015, T027–T028, T036, T056–T057 | T032, T054–T055, T058; Scenarios 4.1–4.3 |
| FR-003, SR-002 | T005, T025, T034–T037, T048–T049 | T023, T030–T032, T045, T047; Scenarios 1.1, 3.3–3.4 |
| FR-004, FR-012, SR-006 | T026, T033–T034, T048, T059 | T024, T045, T053; Scenarios 3.1, 3.4–3.5 |
| FR-005, FR-006, SR-003 | T022, T036, T041–T042, T051 | T021, T039, T046; Scenarios 2.1–2.2; T076–T077 |
| FR-007, SR-004 | T033, T036, T050–T052 | T030, T046; Scenarios 1.2, 3.2; T069, T075–T077 |
| FR-008, FR-009, SR-001 | T004, T036–T037, T043, T056–T057 | T032, T040, T055; Scenarios 1.1–1.4, 2.3–2.5, 4.1–4.2 |
| FR-010 | T048, T056 | T045, T054; Scenarios 3.2–3.3; SC-006 |
| FR-011 | T010–T012, T059–T062 | T061, T064, T069–T071 |
| FR-013, SR-005 | T028, T043, T059 | T040, T073; Scenarios 2.3–2.5 |
| SC-001–SC-002 | US1 and US2 tasks | Scenarios 1.1–1.4, 2.1–2.2 |
| SC-003–SC-004, SC-006–SC-007 | US3 tasks | Scenarios 3.1–3.5 |
| SC-005, SC-008 | US2 and US4 tasks | Scenarios 2.3–2.5, 4.1–4.3 |

The exact scenario-to-`It()` mapping is `plan.md` § Scenario Mapping. T017 writes all 17 tests with every variant; T038, T044, T053, and T058 make the 4, 5, 5, and 3 tests of US1–US4 pass.

## Dependencies & Execution Order

### Phase Dependencies

1. Setup T001–T002 verifies approvals and prepares the ADR move; it commits no feature behavior.
2. Phase 0 T003–T006 runs existing tests, introduces the authorizer seam and `internal/fscontain`, and commits that behavior-preserving refactor first. Then commit the prepared approved ADR and document changes.
3. Phase 2 T007–T019 blocks feature logic. T015 is compile scaffolding and fixture migration only; T015 precedes T016, which precedes T017, which precedes T018. Phase 2.7 is skipped.
4. Foundation T020–T029 follows Phase 2. T021 follows T020 and T022 follows T021; T023, T024, and T027 can be authored concurrently once T020 exists; T025 follows T023, T026 follows T024, T028 follows T027, and T029 closes the foundation.
5. User stories run in priority order: **US1 (T030–T038) → US2 (T039–T044) → US3 (T045–T053) → US4 (T054–T058)**. Their tests are isolated, but their implementation shares domain and source files, so stories must not run concurrently.
6. Documentation and deployment T059–T062 follow final behavior. Phase N T063–T085 verifies every story, not only the MVP.

### User Story Dependencies

- **US1** depends on the approved configuration declarations, admission, bootstrap and capability restrictions, and all acceptance tests. It delivers managed and local signed governance; it does not complete the feature.
- **US2** depends on US1's active revision and factory, then completes input parity and the exceptional credential modes.
- **US3** depends on US1's source, revision, and `telemetry.go`, then completes the update, failure, token-plugin, download-observability, and telemetry matrix.
- **US4** depends on the source and error semantics; its configuration tests may be written earlier but pass only once the startup invariants are complete.

### Within Each User Story

Tests first (semantic red) → implementation → focused journey green → existing regression suite. Run `just check` after changed Go or test code, then the smallest matching `just` recipe. Use focused Ginkgo in addition to, not instead of, the maintained E2E recipe. Do not change assertions merely to make an implementation pass.

### Parallel Opportunities

- Phase 0: T004 (`internal/domain/impersonation/`) and T005 (`internal/fscontain/`, `internal/adapters/jwks/`) touch disjoint files.
- Foundation: T023, T024, and T027 touch disjoint test files.
- US1: T030 (revision and source tests), T031 (local bundle tests), and T032 (domain and builder tests).
- US2: T039 (input and handler tests) and T040 (exceptional-mode domain tests).
- US3: T045 (source and revision tests), T046 (failure and telemetry tests), and T047 (token plugin tests).
- US4: T054 (configuration tests) and T055 (mixed-rule tests).
- Do not parallelize tasks that edit the same rule, service, or configuration file.

## Parallel Examples by User Story

| Story | Concurrent test-authoring tasks | Files |
| --- | --- | --- |
| US1 | T030 + T031 + T032 | `internal/adapters/policy/opa/{revision,source}_test.go` vs `internal/adapters/policy/opa/local_test.go` vs `internal/domain/impersonation/{service,rule}_test.go`, `internal/app/builder_test.go` |
| US2 | T039 + T040 | `internal/domain/impersonation/policy_input_test.go`, `internal/adapters/http/enduser/oauth2_token_test.go`, `tests/testdata/opa-envoy-input/*.json` vs `internal/domain/impersonation/opa_security_test.go` |
| US3 | T045 + T046 + T047 | `internal/adapters/policy/opa/{source,revision}_test.go` vs `internal/domain/impersonation/opa_failure_test.go`, `internal/adapters/policy/opa/telemetry_test.go` vs `internal/adapters/policy/opa/tokenplugin_test.go` |
| US4 | T054 + T055 | `internal/config/{validator,loader}_test.go`, `internal/domain/impersonation/rule_test.go` vs `internal/domain/impersonation/opa_mixed_rules_test.go` |

## Implementation Strategy

### MVP First (User Story 1)

Verify approvals → isolated behavior-preserving refactor → all design preconditions, fixtures, and semantic E2E red → tested foundation → US1 signed governance in managed and local mode. Demonstrate normal claims, governance denial, human delegation recovery, the local bundle variant, and a signed self-agent without human state. This is an internal checkpoint, not approval to drop the remaining stories.

### Incremental Delivery

Add US2 input parity and exceptional modes, then US3 managed revisions, failures, download observability, and the token plugin, then US4 explicit selection and CEL compatibility. Revalidate earlier stories after each increment. Finish documentation, Helm validation, coding-standard and security checks, the container-free repository gates, and the final task review. Container suites are downstream validation, never local evidence. Keep partial tasks unchecked and describe remaining work in the session result.
