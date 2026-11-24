# Tasks: OPA policies for impersonation authorization

**Input**: Design documents from `specs/20261120-153502-opa-impersonation-authorization/`.
**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, and `contracts/`.
**Tests**: Mandatory test-first red-green-refactor (Principles VIII, XIII). Keep run evidence in the session result, never in feature documents. Check a task only after its work is complete.
**Organization**: Shared design/foundation first, then independently testable user-story increments. All proposed API, ADR, and consequential decisions require the supplied maintainer approval before their implementation.

## Format: `[ID] [P?] [Story] Description`

- `[P]` marks disjoint files with no dependency on another incomplete task in the same group.
- `[US1]`–`[US4]` map to the stories in `spec.md:102-163`.
- Paths are relative to the repository root. Design citations without the feature prefix resolve inside this feature directory; e.g. `data-model.md:29` resolves to `specs/20261120-153502-opa-impersonation-authorization/data-model.md:29`.
- Each implementation task cites the governing requirements/contracts; read those sections before changing code. Read the nearest `AGENTS.md` and the applicable binding standards too.

## 🔒 Feature Scope [MANDATORY]

- Every generated task MUST cite an approved feature requirement or acceptance scenario (ID if available, otherwise title), or a binding constraint applicable to the affected feature path. Include work necessary to make that path functional, secure, and compliant; do not add independent remediation or optional improvements.
- If a finding blocks safe or compliant delivery, include only the minimum necessary correction. For unrelated findings, ask the user before filing an issue; record only user-approved issue URLs in a non-checkbox `Future Work` note in the feature planning or implementation record, never as feature tasks.
- Expanding feature scope requires explicit user approval and corresponding updates to `spec.md` and `tasks.md` before implementation.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the approval boundary without granting approval ourselves. T001 is the first implementation task. Prepare its document changes, but commit the behavior-preserving refactor before committing any feature/ADR changes.

- [ ] T001 Record only the approval supplied by `context.approval` in each covered ADR's Status line as `Accepted (approved by <login> on <date>: <url>)`; move `specs/20261120-153502-opa-impersonation-authorization/adr-20261121-061324-opa-impersonation-authorization.md` to `adrs/20261121-061324-opa-impersonation-authorization.md`, fix its relative supersedes links and feature references, and retain the approval-comment reference in the implementation result. Verify coverage of all nine items in `plan.md:316-331` before dependent work; leave uncovered work unchecked. Do not edit a default-branch ADR, `spec.md`, or `plan.md` (Principles II, X).
- [ ] T002 Confirm `go.mod` already pins the OPA packages required by `plan.md:57-75` and that `internal/adapters/policy/opa/` needs no new module or ExtProc import; inspect `justfile` recipes and the relevant source instructions before implementing (FR-009, Principle III). Do not change tool versions or install tools unless the feature requires it.

## Phase 0: Pre-implementation Refactoring

**Purpose**: Isolate the minimum existing-domain restructuring required by `plan.md:197-210` and `research.md:193-211`. No observable behavior change, configuration change, or OPA implementation belongs in this commit.

- [ ] T003 Run existing `internal/domain/impersonation/*_test.go` through `just test` and existing HTTP impersonation acceptance scenarios through `just test-e2e-backend` before refactoring; keep these test assertions unchanged (FR-008, FR-009, SC-005, Principle VIII).
- [ ] T004 Extract the static CEL `authorizer`/`authorization` seam into `internal/domain/impersonation/authorizer.go`, shared CEL-only admission into `internal/domain/impersonation/binding.go`, and one authorization revision read in `rule.go` and `service.go`; use it for classification, email binding, and evaluation while preserving CEL error text and pipeline order (`contracts/internal-contracts.md:120-144`, FR-008, FR-013, SR-005). Keep these internal types independent of the later OPA port.
- [ ] T005 Run `just check`, `just test`, and `just test-e2e-backend` against the refactor; commit only the refactor paths from T004 in their own behavior-preserving commit, before the prepared T001 ADR changes or any feature code, with the required Copilot co-author trailer (Principles II, VIII).

**Checkpoint**: Existing tests unchanged and passing; structural refactor isolated ahead of feature commits.

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation.

**⚠️ CRITICAL**: No feature business implementation can begin until this entire phase is complete. Test-enabling declarations may be added in T014, but may not implement an implicit allow or weaken assertions.

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management).

- [ ] T006 Verify the value objects and invariants in `specs/20261120-153502-opa-impersonation-authorization/data-model.md:8-133`, including unchanged Agent/UserGrant boundaries and no new typed ID, entity, repository, or persisted event; reconcile only routine gaps within the approved contract (FR-008, FR-013, Principles II, V, IX).
- [ ] T007 Add Impersonation Policy Engine, Impersonation OPA Engine, Policy/Data Revision, Binding Evidence, and Policy Bootstrap to the `ARCHITECTURE.md` glossary; also restate existing Impersonation Rule, Subject Category, and Agent-Subject Token entries in terms of Binding Evidence, distinguishing CEL detection from engine-neutral issuance; update the root `AGENTS.md` Domain Glossary and update its impersonation boundary and ADR index to the newly approved superseding ADR without editing any old ADR (`data-model.md:135-149`, Principles II, V).

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design).

- [ ] T008 Verify every new key, default, selected-block rule, timeout bound, bootstrap restriction, and indexed diagnostic against `specs/20261120-153502-opa-impersonation-authorization/contracts/impersonation-opa-config.yaml:14-165`; identify loader/domain responsibilities including the explicit ordinary-exchange OPA startup rejection without adding usable ordinary-exchange OPA keys (FR-001–FR-003, FR-010, FR-011).
- [ ] T009 Create approved YAML/Rego/data examples in `examples/config/impersonation-opa.yaml` and `examples/config/opa/impersonation/`, including the shared `impersonation.opa` engine block, a minimal bootstrap with the `aib_bearer_token_file` credential plugin, a local bundle-directory variant, equivalent CEL/Rego governance predicates, and exceptional-mode binding forms; index them in `examples/config/README.md` (`contracts/impersonation-opa-config.yaml:173-226`, `contracts/policy-input-decision.md:139-224`, FR-011, Principle VII).
- [ ] T010 Design and apply the `impersonation.opa` engine block (`policy` oneOf `config_file`/`path`, `startup_timeout`) and the mutually exclusive CEL/OPA rule branches (OPA rule: `package`, `decision`, `evaluation_timeout` only) to `charts/agentic-identity-broker/values.schema.json`, explain unchanged snake_case passthrough in `values.yaml`, and document bootstrap, local bundle, and token-file Secret mounts using existing volume values in `README.md`; keep ExtProc configuration out of the broker chart (`contracts/impersonation-opa-config.yaml:227-241`, FR-011, Principle VII).

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development).

- [ ] T011 Verify supplied API sign-off and publish the approved token-operation description delta including the full Agent subjects paragraph replacement (signed or revision-bound unsigned exact-target classification, unbound-human fallback and cross-agent rejection), 500 example, and version 1.3.0 in `api/enduser/openapi.yaml` before endpoint implementation; confirm no Admin API, request/response envelope, or token-claim change (`contracts/openapi-token-opa.yaml:1-83`, FR-007–FR-009, Principles IV, X).

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management).

- [ ] T012 Confirm the no-database-change design in `specs/20261120-153502-opa-impersonation-authorization/data-model.md:5-6`; retain only the shared engine's in-memory snapshots and existing Agent/UserGrant access, with no migration or repository scaffolding (FR-008, Principle IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency).

- [ ] T013 Confirm `specs/20261120-153502-opa-impersonation-authorization/spec.md:594-621` introduces no UI, new consent flow, or frontend component; document applicability as no frontend implementation, Wireloom, Playwright, or screenshot work (Principles XI, XIII, XIV).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principles XIII (End-to-End Acceptance Testing & Spec Traceability), XIV (Specification by Example & Reviewable Journeys).

- [ ] T014 Add only the compile-enabling OPA configuration declarations in `internal/ports/config.go` and mechanically migrate impersonation configuration literals in affected `*_test.go` and `tests/e2e/fixtures/` to the dedicated type; keep ordinary exchange types unchanged and OPA unsupported until feature implementation (`contracts/internal-contracts.md:11-52`, FR-001, FR-009, Principle XIII).
- [ ] T015 Create `tests/e2e/helpers/opa_bundle_server.go` and reusable `tests/e2e/fixtures/opa/impersonation/` policies/data using `bundle.Write`, controllable ETags, source errors/delays, and a network-attempt recorder; use fresh mutable fixtures and thin production-bootstrap wrappers in `tests/e2e/bootstrap/` (`plan.md:251-267`, `research.md:412-423`, Principles VIII, XIII).
- [ ] T016 Write all 19 scenario-mapped `It()` blocks in `tests/e2e/opa_impersonation_authorization_test.go` before feature logic, with one `Context` per story, setup/cleanup, spec path and scenario ID beside each test, real HTTP status/error/decoded-claims/audit assertions, and no placeholders or pending markers. Cover valid credential combinations and variants within their owning scenario, following `plan.md:214-267` and `spec.md:102-163` (Principle XIII).
- [ ] T017 Review Journeys 1–4 in `specs/20261120-153502-opa-impersonation-authorization/spec.md:176-506` against the complete acceptance mapping, configuration/HTTP contracts, and decoded JWT claims; run `just web-build` then focused Ginkgo with `--label-filter="!performance && !docker" --focus "OPA Impersonation Authorization" ./tests/e2e/` and confirm semantic failures for absent OPA support, not compilation failures (Principles VIII, XIII, XIV).

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

**Constitution Reference**: Principle XV (Binding Coding Standards).

- [ ] T018 Read `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`; verify `specs/20261120-153502-opa-impersonation-authorization/contracts/observability.md:1-100` lists new diagnostics, lifecycle events, span/error/cancellation value sets, metric scope/units/attributes, and safety tests. Reuse the existing `impersonation` namespace, global tracer/meter plumbing, and central principal policy; no standard amendment or new namespace is needed (SR-003, SR-004, Principle XV).

**Checkpoint**: Approved designs published, test scaffolding compiles, all acceptance tests written and locally exercised for semantic red behavior. No frontend or persistence scaffold applies.

## Phase 2.5: Foundational Infrastructure

**Purpose**: Establish the runtime boundary, restricted bootstrap/capabilities, and revision evidence before any OPA issuance.

- [ ] T019 Define the policy source/revision/admission interfaces, evidence DTOs, closed role/action types, and safe sentinel errors in `internal/ports/impersonation_policy.go` (`contracts/internal-contracts.md:54-118`, FR-007, SR-005). Implement the constrained `Binding Evidence.References` and `Binding Evidence.Field bindings` as defined in `data-model.md:80-85`; callers must not mutate revision evidence.
- [ ] T080 Write semantic red tests in `internal/ports/impersonation_policy_test.go` after T019's compile-enabling declarations: exercise runtime interface assertions for `LogValue`, `String`, `GoString` (so absent methods fail semantically, not compilation), value/pointer `%s`, `%v`, `%+v`, `%#v`, nil/empty/populated input, and `slog.Any` through JSON/text handlers with distinctive synthetic identity/transport values; assert only `[REDACTED]` renders while policy evaluation still sees complete typed data (`contracts/internal-contracts.md` § 2–3, `contracts/policy-input-decision.md` § 2; C1, SR-003, LOGGING § 6.4, Principle VIII).
- [ ] T081 Implement constant-redacted `LogValue`, `String`, and `GoString` for the sensitive `ImpersonationPolicyInput` type in `internal/ports/impersonation_policy.go`, driven by T080; do not convert to raw maps for formatting/logging and preserve full input only for evaluation. Run `just check` and `just test` before dependent runtime work (C1, SR-003, Principle XV).
- [ ] T020 [P] Write bootstrap rejection and redaction tests in `internal/adapters/policy/opa/bootstrap_test.go` for missing/unreadable/invalid files, every unsupported top-level key and every plugin other than `aib_bearer_token_file`, no bundles, persistence, YAML/JSON, and native parser failures, os.Root containment over the file's own directory (relative `..data` links accepted; escaping and absolute symlinks, relative paths, and a parent directory of `/` rejected), oversized/nonregular files, no use of `security.trust_material_dir`, and each SDK-local-file credential/TLS/bundle reference rejection (`contracts/impersonation-opa-config.yaml:113-165`, FR-003, SR-003).
- [ ] T021 [P] Write whole-module capability and decision-graph evidence tests in `internal/adapters/policy/opa/capabilities_test.go` and `evidence_test.go`, covering every removed builtin, allowed time, unreachable modules, imports, aliases, nested comprehensions, cross-package helpers, functions, dynamic keys, decision absence, and every reachable `with` modifier including data-rule/function/built-in replacements; exercise overridden email/sub-binding helpers and mandatory-reference admission rejection, nested scopes, and unreachable modifiers; only subject sub/email fields count, all three role references remain independent (`contracts/policy-input-decision.md:175-224`, FR-012, SR-005).
- [ ] T022 Implement startup-only contained bootstrap reading through an adapter-owned `os.Root` over the configured file's own directory, same-descriptor regular-file/1 MiB checks, and nested SDK-local-file rejection before `sdk.New` with sanitized reason classes in `internal/adapters/policy/opa/bootstrap.go`; fulfill `OPA Engine.policy.config_file` from data-model.md § OPA Engine via the configuration contract, without reflecting native parser errors (T020, FR-003, SR-003).
- [ ] T023 Implement restricted OPA compiler capabilities in `internal/adapters/policy/opa/capabilities.go` and conservative decision-reachable evidence extraction in `evidence.go`, using OPA compiler/AST APIs and disabled printing; fulfill `Binding Evidence.References` and `Binding Evidence.Field bindings` from `data-model.md:80-85` (`research.md:132-191`, T021, FR-012, SR-005).
- [ ] T024 Write tests in `internal/domain/impersonation/binding_test.go` for derived requirements, CEL error-text parity, and candidate admission with voided or missing evidence; then extend `binding.go`/`authorizer.go` to adapt port evidence and enforce `Impersonation Rule.Binding requirements` from `data-model.md:17` (`contracts/internal-contracts.md:120-144`, FR-013, SR-005).
- [ ] T025 Run `just check` and `just test` for the foundation, verifying no source constructor enters readiness without domain admission and no new adapter imports another adapter or `internal/extproc` (`internal/adapters/policy/opa/`, `internal/ports/impersonation_policy.go`, FR-009, Principles VI, VIII).

**Checkpoint**: Bootstrap/capabilities/evidence and domain admission tested. Runtime/issuance is not yet complete.

## Phase 3: User Story 1 - Enforce organizational governance before issuing a token (Priority: P1) 🎯 MVP

**Goal**: One OPA rule authorizes governed delegated humans and eligible signed self-agents while preserving all issuance controls.
**Independent Test**: Scenarios 1.1–1.4: allowed human and self-agent requests mint the normal claims; unregistered governance denies; absent human delegation returns the existing recovery URL.

### Tests for User Story 1 [MANDATORY - Principle VIII]

- [ ] T026 [P] [US1] Write prepared-query decision tests in `internal/adapters/policy/opa/revision_test.go` for allow/deny, exactly one object result, ignored extra decision members, strict builtin errors, and undefined/malformed/unsupported actions; write minimal source readiness/activation tests including a usable first-bundle subset followed by a rejected complete two-bundle candidate (neither admits or becomes ready) in `source_test.go` (`contracts/policy-input-decision.md:122-173`, Scenarios 1.1–1.2, FR-007, SR-002).
- [ ] T027 [P] [US1] Write OPA-rule service and builder tests in `internal/domain/impersonation/service_test.go` and `internal/app/builder_test.go` for ordinary signed claims, governed denial, delegation ordering, signed self-agent issuance, scope/reserved claims, input construction and handler transport metadata needed by Journey 1, and factory/shutdown wiring; use domain-local port doubles, not concrete adapters (Scenarios 1.1–1.4, `contracts/internal-contracts.md:120-206`, FR-008, SR-001).

### Implementation for User Story 1

- [ ] T028 [US1] Implement immutable prepared-query/data/evidence revisions and action parsing in `internal/adapters/policy/opa/revision.go`; fulfill `Policy/Data Revision.Prepared query`, `Policy/Data Revision.Data snapshot`, and `Policy/Data Revision.Evidence` from `data-model.md:62-78`, and the decision value/effect mapping from `data-model.md:53-60` (`research.md:50-91`, T026, FR-004, FR-007).
- [ ] T029 [US1] Implement the shared OPA engine source (one SDK instance for every OPA rule) with commit trigger, restricted candidate construction with one prepared query and evidence set per OPA rule, one admission call per candidate (all-or-nothing), all-configured-bundle completeness from the candidate transaction before domain admission/atomic replacement, and initial readiness on that same admitted snapshot; allocate a process-instance UUID/engine sequence admission ID only on successful admission in `internal/adapters/policy/opa/source.go`; implement `Policy/Data Revision.Label` and `Policy/Data Revision.Admitted at` from `data-model.md:72-75` (`research.md:50-91`, `contracts/internal-contracts.md:173-200`, T026, FR-003, FR-004).
- [ ] T030 [US1] Wire the OPA authorizer/factory into `internal/domain/impersonation/rule.go` and `service.go`, retaining CEL extraction and one revision for classification/email/evaluation, normal human delegation and target-derived agent minting. Implement the initial full input builder, including `type: "impersonation"` and broker-resolved `context.agent {id, canonical_id?, external_id?}` with unset IDs omitted, independent of caller claims/headers and without UUID fallback in `policy_input.go`, transport DTO in `request.go`, and handler metadata in `internal/adapters/http/enduser/oauth2_token.go` so Journey 1's HTTP predicates work from actual request facts; US2 extends edge-case and exceptional-mode coverage. Fulfill Impersonation Policy Input from `data-model.md:43-51` via `contracts/policy-input-decision.md:18-120` (`contracts/internal-contracts.md:120-171`, T027, FR-005, FR-006, FR-008, FR-009, FR-013, SR-001).
- [ ] T031 [US1] Construct the policy-source factory only in `internal/app/builder.go` with the injected logger, pass configured end-user public URL to the domain service (the adapter needs no directory setting), and chain `Service.Close` before telemetry shutdown; update thin bootstrap wrappers and mechanical constructor call sites, not HTTP routing logic (`contracts/internal-contracts.md:202-206`, T027, FR-003, Principles VI, XII).
- [ ] T032 [US1] Run `just check`, `just test`, and focused Ginkgo for US1's four scenarios in `tests/e2e/opa_impersonation_authorization_test.go`, then `just test-e2e-backend`; confirm normal claims, consent recovery, and no human grant/session creation for signed self-agent (SC-001, SC-002, Principles VIII, XIII).

**Checkpoint**: Signed governance MVP works. Full input parity, exceptional credentials, failure/update matrix, and migration checks remain mandatory before feature completion.

## Phase 4: User Story 2 - Reuse policy input conventions without losing identity context (Priority: P1)

**Goal**: Complete Envoy-compatible transport facts and CEL-equivalent trusted context, including exceptional credential modes.
**Independent Test**: Scenarios 2.1–2.5 exercise parity, spoof-resistant context, bound unsigned human email, unsigned self-agent/cross-agent controls, and audience-absent signed-role combinations.

### Tests for User Story 2 [MANDATORY - Principle VIII]

- [ ] T033 [P] [US2] Write input/handler tests in `internal/domain/impersonation/policy_input_test.go` and `internal/adapters/http/enduser/oauth2_token_test.go` for all input members, optional claims, exact numbers/arrays/nested objects, repeated and filtered headers, public-URL host/scheme, parsed path/query, and no raw credentials/body, `type: "impersonation"`, broker-authoritative `context.agent` (`id`, Canonical ID, External ID) including unset (omitted, not `""`)/unknown/spoofed variants, and absence of every reserved ExtProc grant-context member; create shared Envoy input vectors under `tests/testdata/opa-envoy-input/` and consume them in both broker and `internal/extproc/authorization/input_builder_test.go`, comparing common fields, the `context.agent` JSON shape against ExtProc's `AgentContext` serialization, and declared broker exclusions without production cross-binary imports (`contracts/policy-input-decision.md:18-120`, Scenarios 2.1–2.2, FR-005, FR-006, SR-003).
- [ ] T034 [P] [US2] Write exceptional-mode tests in `internal/domain/impersonation/opa_security_test.go` for unsigned human/email and agent/sub binding, absent field binding, cross-agent raw/extracted identities, human fallback/delegation, and each audience-absent role/combination with bad signature/issuer/algorithm/expiry/present-aud negative controls (`spec.md:436-506`, Scenarios 2.3–2.5, FR-013, SR-001, SR-005).

### Implementation for User Story 2

- [ ] T035 [US2] Complete type-preserving and edge-case input construction and External ID projection in `internal/domain/impersonation/policy_input.go` and refine the transport-only `RequestAttributes` with `Headers map[string][]string` in `request.go`; fulfill the constrained Impersonation Policy Input document in `data-model.md:43-51` by its canonical `contracts/policy-input-decision.md:18-120` fields, without importing `net/http` into the domain (T033, FR-005, FR-006, SR-003).
- [ ] T036 [US2] Complete repeated-header, query, protocol, and metadata-cloning behavior in `internal/adapters/http/enduser/oauth2_token.go` and pass it through the existing domain service, keeping claim derivation and all policy decisions outside the handler (`contracts/internal-contracts.md:145-149,202-206`, T033, FR-006, Principle VI).
- [ ] T037 [US2] Complete exceptional credential-mode integration in `internal/domain/impersonation/service.go`, `authorizer.go`, and `binding.go`: classification and unverified email use only the captured revision evidence, mandatory role references gate admission, and existing cryptographic/category/delegation guards remain authoritative (`data-model.md:17,80-85,110-133`, `contracts/policy-input-decision.md:205-217`, T034, FR-013, SR-005).
- [ ] T038 [US2] Add explicit paired CEL/OPA policy fixture variants for exceptional modes in `tests/e2e/fixtures/opa/impersonation/` and reusable configuration builders in `tests/e2e/fixtures/`; ensure the 2.3–2.5 scenarios compare identical observable outcomes and cover valid combinations without broadening scope (`spec.md:436-506`, SC-008).
- [ ] T039 [US2] Assert captured OPA inputs, token claims, and zero grant/session effects for agent variants through the production bootstrap in `tests/e2e/opa_impersonation_authorization_test.go`; do not use test-only production getters (`plan.md:251-305`, Scenarios 2.1–2.5, Principles VI, XIII).
- [ ] T040 [US2] Run `just check`, `just test`, focused Ginkgo for the five US2 scenarios in `tests/e2e/opa_impersonation_authorization_test.go`, and `just test-e2e-backend`; verify context and exceptional credential-mode parity (SC-002, SC-008, Principles VIII, XIII).

## Phase 5: User Story 3 - Apply managed policy and data changes safely (Priority: P2)

**Goal**: Safe live activation, last admitted revision retention, bounded startup/evaluation, and credential-free diagnostics.
**Independent Test**: Scenarios 3.1–3.6 revoke through an ETag update, make runtime failures terminal, reject unusable startup/networking/missing bindings, and prove retained revision outcomes and revision-coupled field evidence under concurrent updates.

### Tests for User Story 3 [MANDATORY - Principle VIII]

- [ ] T041 [P] [US3] Extend `internal/adapters/policy/opa/source_test.go` and `revision_test.go` with multiple bundles, same-snapshot all-bundle completeness/bounds, especially usable partial initial candidate followed by rejected complete candidate with no readiness, equal/empty manifest labels producing distinct admission IDs, rejected/fetch-failed/partial updates, cancellation/timeouts, in-flight snapshots, per-rule isolation, shutdown cleanup, and race stress during revision swaps (`research.md:50-91,213-232,345-362`, `data-model.md:87-133`, Scenarios 3.1–3.6, FR-003, FR-004, FR-010, FR-012).
- [ ] T042 [P] [US3] Write service abort/diagnostic tests in `internal/domain/impersonation/opa_failure_test.go` and telemetry leakage tests in `internal/adapters/policy/opa/telemetry_test.go`; inject distinctive synthetic token/claim/decision/bootstrap values through allow, deny, invalid, error, rejection, and startup paths; assert span kind/error.type/canceled outcome and metric attribute sets with test providers/manual readers; domain cancellation maps internally to server_error and never falls through; HTTP cancellation acceptance uses correlated ended spans rather than receipt of HTTP 500; no admission ID may enter spans/metrics (`contracts/observability.md:1-100`, Scenarios 3.2–3.3, SR-003, SR-004, Principle XV).

### Implementation for User Story 3

- [ ] T043 [US3] Complete bounded same-snapshot all-configured-bundle readiness (partial candidates never admitted), the single shared startup wait, engine cleanup on failure, cancellation-aware deadlines, and immutable last-admitted revision retention in `internal/adapters/policy/opa/source.go`, `revision.go`, and `internal/domain/impersonation/service.go`; enforce `OPA Rule Decision.evaluation_timeout` and `OPA Engine.startup_timeout` from data-model.md § OPA Engine and § OPA Rule Decision (`contracts/internal-contracts.md:128-138`, T041, FR-003, FR-010, FR-012).
- [ ] T082 [P] [US3] Write local bundle-directory tests in `internal/adapters/policy/opa/local_test.go`: valid directory admitted through the same capability/evidence/admission path, policy_revision from `.manifest`, unsupported file types, symlinks escaping the bundle directory, a relative path, the bundle directory resolving to `/`, more than 1,000 files or 16 MiB, and no polling or later re-read (`contracts/impersonation-opa-config.yaml` engine `policy.path`, `research.md` R15, FR-003).
- [ ] T083 [US3] Implement local bundle-directory loading in `internal/adapters/policy/opa/local.go` through the adapter-owned `os.Root`, building one candidate with OPA's bundle reader from the read bytes and no SDK runtime (T082, R15, FR-003).
- [ ] T084 [P] [US3] Write `aib_bearer_token_file` plugin tests in `internal/adapters/policy/opa/tokenplugin_test.go`: startup validation (missing, relative path, escaping or absolute symlink, empty, over 64 KiB), a rotation that replaces the file with a link outside its directory failing that download, the token re-read before every bundle request against an `httptest` server, rotation without restart, a later read failure keeping the active revision and emitting rate-limited `token_unavailable` without contents, and `credentials.plugin` naming any other plugin rejected (`contracts/impersonation-opa-config.yaml` `bootstrap_subset.plugins`, `research.md` R16, `contracts/observability.md`).
- [ ] T085 [US3] Implement the `aib_bearer_token_file` `rest.HTTPAuthPlugin` and its `plugins.Factory` in `internal/adapters/policy/opa/tokenplugin.go`, registered in `sdk.Options.Plugins` only when the bootstrap configures it; first confirm OPA v1.21.4's `credentials.plugin` lookup through the plugin manager in a focused test (T084, R16).
- [ ] T044 [US3] Map invalid decisions and evaluation failures to terminal OAuth server errors in `internal/domain/impersonation/errors.go`/`service.go` and add diagnostic constants in `internal/domain/tokenexchange/diagnostic.go`; preserve caller cancellation override and prevent later-rule fallthrough (`contracts/policy-input-decision.md:122-137`, `contracts/observability.md:12-30`, T042, FR-007, SR-002).
- [ ] T045 [US3] Implement global-scope tracer/meter, metric callbacks/lifecycle cleanup, and sanitized lifecycle logging in `internal/adapters/policy/opa/telemetry.go`; add engine/revision/admission-ID audit metadata, copying the captured revision ID to deciding audits and admission logs even when manifest labels are equal or empty in `internal/domain/impersonation/service.go` and `internal/adapters/http/enduser/oauth2_token.go`; keep revision labels log-only, preserve existing typed principals, and never emit native errors or copied claim values (`contracts/observability.md:32-100`, T042, SR-003, SR-004, Principle XV).
- [ ] T046 [US3] Complete update variants in `tests/e2e/opa_impersonation_authorization_test.go` using T015's helper: poll admission deterministically; within Scenario 3.2 send a synthetic trace context, wait for evaluation start with a test-local span processor, cancel the client, wait for correlated request completion, and assert canceled telemetry/no issuance/no later-rule authorization rather than an HTTP response; within Scenario 3.3 include the two-bundle partial-usable/complete-rejected startup variant, assert the first post-admission request changes, prove old allow and new-request deny after fetch/rejection failures, record zero prohibited-network calls, and alternate email/sub bindings across concurrent updates without mixing evidence (`spec.md:386-414,500-506`, Scenarios 3.1–3.6, SC-003, SC-007, SC-008).
- [ ] T047 [US3] Run `just check`, `just test` (including race tests), focused Ginkgo for all six US3 scenarios in `tests/e2e/opa_impersonation_authorization_test.go`, and `just test-e2e-backend`; verify deadlines with scheduling tolerance rather than fixed sleeps (SC-003, SC-004, SC-006–SC-008, Principles VIII, XIII).

## Phase 6: User Story 4 - Adopt OPA one rule at a time (Priority: P2)

**Goal**: Safe explicit engine selection with unchanged valid CEL-only and ordinary exchange behavior; explicit ordinary-exchange OPA selection now fails startup.
**Independent Test**: Scenarios 4.1–4.4 prove explicit deny can fall through to CEL, CEL-only requires no OPA, ambiguous selection fails, and mandatory OPA references gate initial readiness.

### Tests for User Story 4 [MANDATORY - Principle VIII]

- [ ] T048 [P] [US4] Write loader/domain config tests in `internal/config/validator_test.go` and `internal/domain/impersonation/rule_test.go` for unknown/missing/conflicting engines, selected block validation, identifiers/defaults (`package` `aib.broker.impersonation`, `decision` `result`)/timeouts, `impersonation.opa` required exactly when a rule selects OPA, exactly one of `policy.config_file`/`policy.path`, rejection of every other rule `opa` key including `policy_url`, `policy_path`, `policy`, and `startup_timeout`, indexed safe diagnostics, and ordinary-exchange explicit type opa startup rejection through both loader and loader-bypassing domain construction, with reserved field declarations and valid CEL defaults preserved (`contracts/impersonation-opa-config.yaml:14-165`, `research.md:317-343`, Scenarios 4.2–4.4, FR-001–FR-003, FR-009).
- [ ] T049 [P] [US4] Write mixed-rule regression tests in `internal/domain/impersonation/opa_mixed_rules_test.go` for independent per-rule validation, explicit-deny fallthrough across two OPA rules evaluated against one revision read per request (a swap during the request does not change the second rule's revision), all-or-nothing admission when only one rule's requirements fail, terminal failure/delegation, missing initial references even with a later allowing rule, and CEL-equivalent outcomes (`spec.md:150-163,416-434`, Scenarios 4.1–4.4, FR-008, FR-013).

### Implementation for User Story 4

- [ ] T050 [US4] Complete broker config defaults/validation in `internal/ports/config.go`, `internal/config/validator.go`, `internal/domain/tokenexchange/service.go`, and `internal/domain/impersonation/rule.go` while preserving CEL extraction defaults: enforce `Impersonation Rule.authorization.type`, `Impersonation Rule.authorization.cel`, and `Impersonation Rule.authorization.opa` from `data-model.md:14-16`, `OPA Rule Decision.package`/`.decision`/`.evaluation_timeout` and the `OPA Engine` block from data-model.md, including the raw-map rejection of other rule `opa` keys; reject explicit `token_exchange.authorization.type: opa` in loader and ordinary-exchange domain service construction while keeping valid CEL behavior and reserved declarations unchanged (`contracts/impersonation-opa-config.yaml:14-112`, T048, FR-001, FR-002, FR-009).
- [ ] T051 [US4] Complete ordered mixed-engine behavior and startup binding enforcement in `internal/domain/impersonation/rule.go`, `authorizer.go`, and `service.go`; read the engine revision once per request before the first rule; missing factory/revision fails closed, and CEL-only construction never creates an OPA engine and rejects a configured `impersonation.opa` (`contracts/internal-contracts.md:120-171`, T049, FR-008, FR-009, FR-013).
- [ ] T052 [US4] Complete Scenarios 4.1–4.4 in `tests/e2e/opa_impersonation_authorization_test.go`, including allowed/denied ordinary third-party exchange and zero OPA bootstrap contact for CEL-only deployments, without changing existing CEL assertions (`spec.md:150-163,416-434`, SC-005).
- [ ] T053 [US4] Run `just check`, `just test`, focused Ginkgo for the four US4 scenarios in `tests/e2e/opa_impersonation_authorization_test.go`, and `just test-e2e-backend`; keep existing CEL-only acceptance suites unchanged (SC-005, SC-008, Principles VIII, XIII).

## Phase 7: Feature Documentation & Deployment Validation

- [ ] T054 Update `docs/configuration.md`, `docs/api/enduser-api.md`, and `docs/reference/token-exchange.md` with approved config defaults/bounds, the full input/decision contract, diagnostic closed values, no policy-time networking, managed activation/readiness, retained revision staleness, per-rule migration cautions, the shared engine and its all-or-nothing admission, local bundle-directory mode, the contained token-file plugin and its rotation, ExtProc bootstrap reuse limits, the explicit replacement Agent subjects paragraph and guide text, broker External ID governance lookup, contained bootstrap/inline credential restart behavior, correlated cancellation and admission IDs, and unchanged claims/delegation (`contracts/`, FR-011, SR-006, Principles IV, VII, XV).
- [ ] T055 Validate CEL and OPA examples plus rejection of mixed/missing blocks through the Helm schema/template mechanism in `charts/agentic-identity-broker/`; run `just helm-lint`, `just helm-template`, and `just helm-template-values` with synthetic valid and invalid values under `/tmp/gh-aw/agent/`, verifying existing bootstrap mounts and no new chart key (`contracts/impersonation-opa-config.yaml:227-241`, FR-011, Principle VII).
- [ ] T056 Walk the automated and manual instructions in `specs/20261120-153502-opa-impersonation-authorization/quickstart.md` against committed `examples/config/opa/impersonation/` and documented HTTP-bundle polling behavior; correct routine validation instructions only, and keep all observed evidence in the session result (FR-011, Principle XIV).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance.

**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be included in every tasks.md file. The compliance tasks map directly to the constitution Implementation Phase checklist and MUST be completed before considering the feature done.

### 🔒 Constitution Compliance Verification [MANDATORY]

**These tasks MUST be included in every generated tasks.md file. They verify that all constitution principles have been followed.**

#### Design Phase Verification [MANDATORY]

**Constitution Reference**: PRECONDITIONS checklist - verify Phase 2 tasks were completed correctly.

- [ ] T057 Verify every task traces to FR-001–FR-013, SR-001–SR-006, a scenario, or a binding constraint; ensure all nine approval items are covered and required work is complete in `specs/20261120-153502-opa-impersonation-authorization/tasks.md` and the implementation result (Feature Scope & Deferred Findings, Principles II, X).
- [ ] T058 Verify documented value objects, glossary/boundary updates in `ARCHITECTURE.md`, examples/index in `examples/config/`, broker Helm schema/mount contract, and confirmed End-User API design in `api/enduser/openapi.yaml` (Principles II, IV, V, VII, X).
- [ ] T059 Verify no migration/repository/persisted entity or frontend/Wireloom/Playwright obligation was introduced beyond `specs/20261120-153502-opa-impersonation-authorization/data-model.md` and `spec.md`; preserve recorded no-DB/no-UI scope (Principles IX, XI, XIII, XIV).
- [ ] T060 Verify all concrete journeys match requirements, configuration/HTTP/decoded-JWT examples, and all 19 acceptance scenarios map 1:1 to `tests/e2e/opa_impersonation_authorization_test.go`; verify tests were written before implementation with realistic expectations, no placeholders, pending markers, or red-phase comments; recorded red-run evidence is not required (Principles VIII, XIII, XIV).
- [ ] T061 Verify `specs/20261120-153502-opa-impersonation-authorization/contracts/observability.md` covers every added signal and its value set/safety test, uses the existing namespace, and matches the binding standards without an unapproved amendment (Principle XV).

#### Implementation Phase Verification [MANDATORY]

**Constitution Reference**: Implementation Phase checklist - verify all principles followed during implementation.

**Feature Scope** (Development Requirements):
- [ ] T062 Verify unrelated remediation is absent from the implementation diff and user-approved issue URLs, if any, are future-work notes rather than implementation tasks in `specs/20261120-153502-opa-impersonation-authorization/tasks.md` (Feature Scope & Deferred Findings).

**API & Documentation** (Principles IV, X):
- [ ] T063 Verify `api/enduser/openapi.yaml`, `docs/api/enduser-api.md`, `docs/reference/token-exchange.md`, and the HTTP implementation match the confirmed engine decision/error contract exactly, with unchanged request parameters, reserved claims, Admin API, and recovery behavior (Principles IV, X; FR-007–FR-009).

**Architecture & Documentation** (Principle II):
- [ ] T064 Verify `ARCHITECTURE.md` and the root `AGENTS.md` ADR index reference `adrs/20261121-061324-opa-impersonation-authorization.md` by full stem, updating that index if needed; verify the moved ADR cites the supplied approval and final supersession scope, with old default-branch ADRs untouched (Principle II).

**Configuration** (Principle VII):
- [ ] T065 Verify `internal/ports/config.go`/`internal/config/validator.go` use broker-owned configuration, matching examples/docs/Helm for every changed parameter; verify no ExtProc setting or ordinary-exchange change beyond the approved explicit OPA type startup rejection in `charts/agentic-identity-broker/` (Principle VII).

**Database & Persistence** (Principle IX):
- [ ] T066 Verify no new schema, migration, repository, or persistence path in the implementation diff relative to `specs/20261120-153502-opa-impersonation-authorization/data-model.md`; existing Agent/UserGrant invariants and access paths remain unchanged, so new migration/PostgreSQL integration tests are inapplicable (Principle IX).

**Security** (Principles I, III):
- [ ] T067 Verify every authentication/category/delegation/scope/claim control remains mandatory in `internal/domain/impersonation/`, OPA is opt-in only as an engine, every request error fails closed, and no cached allow, implicit readiness, stale field evidence, or policy-time network builtin exists (Principle I; SR-001–SR-006).
- [ ] T068 Verify only established OPA/jwx/CEL/Go libraries implement policy/crypto primitives in `internal/adapters/policy/opa/` and `internal/domain/impersonation/`, no custom Rego parser/crypto or ExtProc import was introduced, and whole-module builtin restrictions remain upgrade-safe (Principle III).
- [ ] T069 Verify security-critical allow/deny/error/startup/update paths emit sanitized structured logs through injected logger plumbing in `internal/adapters/policy/opa/telemetry.go` and `internal/adapters/http/enduser/oauth2_token.go`; existing principals stay typed and no credential/native error/claim payload is recorded (Principles I, XV).

**Coding Standards** (Principle XV):
- [ ] T070 Verify changed logs, spans, instruments, and attributes in `internal/adapters/policy/opa/` and `internal/adapters/http/enduser/` satisfy both coding-standard review checklists; verify global tracer/meter lookup, canonical error.type, caller-canceled status, log-only revision labels, instrument creation failures handled without affecting issuance, and callback cleanup (Principle XV).
- [ ] T071 Verify `internal/adapters/policy/opa/telemetry_test.go` and affected handler/domain safety tests assert credential exclusion for every sensitive path, expected metric attribute sets, span name/kind/status, and central principal-policy output where logged, sensitive input formatting/slog redaction, and admission-ID-to-deciding-snapshot pairing; no standard enforcement is suppressed or disabled (Principle XV).

**Architecture Patterns** (Principles VI, XII):
- [ ] T072 Verify domain admission/classification/email/decision orchestration depends only on ports, OPA I/O stays in its driven adapter, all dependency wiring and shutdown belong to `internal/app/builder.go`, routing only receives handlers, and the domain request DTO has no net/http/framework dependency (Principles VI, XII).

**Testing** (Principle VIII - Unit & Integration Tests):
- [ ] T073 Verify unit tests in `internal/domain/impersonation/`, `internal/adapters/policy/opa/`, `internal/config/`, and `internal/adapters/http/enduser/` drove implementation in red-green-refactor cycles and were changed minimally; no test-only production APIs or Bash correctness checks were added (Principle VIII).
- [ ] T074 Run `just check` and `just test` against the final diff, including the OPA revision-swap race tests; resolve change-caused failures without reducing validation (Principles VIII, XV; `justfile:71-73,602-604`).

**E2E Acceptance Testing** (Principle XIII):
- [ ] T075 Verify all 19 `It()` blocks in `tests/e2e/opa_impersonation_authorization_test.go` use Ginkgo/Gomega, hierarchical contexts, exact scenario comments, production bootstrap, fresh fixture/storage/server lifecycle, checked request errors, and response cleanup; preserve existing CEL assertions (Principle XIII).
- [ ] T076 Run `just test-e2e-backend` and the focused all-19-scenario Ginkgo command from `specs/20261120-153502-opa-impersonation-authorization/quickstart.md`; all complete journeys must pass, including decoded claims and update/failure/control variants (Principle XIII; SC-001–SC-008).

**Frontend** (Principle XI - if applicable):
- [ ] T077 Confirm no styled component, semantic-token, universal-component, accessibility, or new frontend acceptance obligation exists in `web/` for this backend-only feature; any new UI scope needs renewed design/sign-off rather than unchecked applicability assumptions (Principles XI, XIII).

**Full Repository Verification & Completion** (Principles VIII, XIII, XV):
- [ ] T078 Run all container-free recipes of `just verify` from `justfile`: `just check`, prepared security scan `bash "$SECURITY_SCAN_SCRIPT" .`, `just test`, `just web-test`, `just cdk-test`, `just mock-sample-agent-test`, `just mock-upstream-oauth2-test`, `just test-integration`, `just test-e2e-backend`, the direct ExtProc subset `ginkgo -v --procs=1 --skip='Agentgateway' ./tests/e2e/extproc/`, and `just test-e2e-frontend`. The full `just test-e2e-extproc` requires containers and is deferred, not locally validated. Do not run container-backed suites or change CI; report them as deferred to pushed-commit container tests, never as locally validated (Principles VIII, XIII, XV).
- [ ] T079 Verify all feature tasks/checklists and local validation obligations (including the direct ExtProc subset, not the full container-dependent recipe) are complete in `specs/20261120-153502-opa-impersonation-authorization/tasks.md`; tick only completed tasks, commit exact intended files with the Copilot co-author trailer, and publish progress and all run evidence in the implementation session result. If anything is unfinished, identify each task/reason and report blocked, not complete (Principles II, VIII, XIII).

## Requirement & Acceptance Traceability

| Requirement / outcome | Implementing tasks | Acceptance / verification |
| --- | --- | --- |
| FR-001, FR-002 | T014, T050–T051 | T048–T053; Scenarios 4.2–4.4 |
| FR-003, SR-002 | T022, T029, T031, T043, T083, T085 | T020, T026, T041–T042, T082, T084; 3.2–3.4, 4.4 |
| FR-004, FR-012, SR-006 | T023, T028–T029, T043, T046, T054 | T021, T041, T046–T047; 3.1, 3.4–3.6 |
| FR-005, FR-006, SR-003 | T035–T036, T045, T081 | T033, T042, T080; 2.1–2.2; T070–T071 |
| FR-007, SR-004 | T028, T044–T045 | T026, T042; 1.2, 3.2; T063, T069–T071 |
| FR-008, FR-009, SR-001 | T004, T030–T031, T037, T051 | T027, T034, T049; 1.1–1.4, 2.3–2.5, 4.1–4.2 |
| FR-010 | T043, T050 | T041, T048; 3.2–3.3; SC-006 |
| FR-011 | T009–T011, T054–T056 | T055, T058, T063–T065 |
| FR-013, SR-005 | T019, T023–T024, T030, T037, T043, T051 | T021, T034, T041, T049; 2.3–2.5, 3.6, 4.4 |
| SC-001–SC-002 | US1 + US2 tasks | 1.1–1.4, 2.1–2.2 |
| SC-003–SC-004, SC-006–SC-007 | US3 tasks | 3.1–3.6 |
| SC-005, SC-008 | US2 + US3 + US4 tasks | 2.3–2.5, 3.6, 4.1–4.4 |

The exact 19-scenario to `It()` mapping is `plan.md:230-250`; T016 writes all tests, and T032/T040/T047/T053 validate the 4/5/6/4 story groups respectively.

## Dependencies & Execution Order

### Phase Dependencies

1. Setup T001–T002 prepares approved documents; it does not commit feature behavior.
2. Phase 0 T003–T005 runs existing tests, extracts the required seam, and commits that no-behavior-change refactor first. Then commit the prepared approved ADR/document changes.
3. Phase 2 T006–T018 blocks feature logic. T014 is compilation scaffolding only; T015 precedes T016, which precedes T017. Finish the approved API/config/design surfaces before implementation. No entity boilerplate is needed.
4. Foundation T019–T025 follows Phase 2. T080/T081 follow T019 in red-green order and precede runtime use; T020 and T021 can be authored concurrently, T022 follows T020, T023 follows T021, T024 follows T019 and T023, and T025 follows the complete foundation.
5. User stories run in priority order: **US1 (T026–T032) → US2 (T033–T040) → US3 (T041–T047, T082–T085) → US4 (T048–T053)**. Their test cases are independently isolated, but their implementation shares the domain/source files and must not race across stories.
6. Documentation/deployment T054–T056 follows final behavior. Phase N T057–T079 verifies every story, not only the MVP.

### User Story Dependencies

- US1 depends on approved config declarations, admission/evidence, bootstrap restrictions, and all acceptance tests. It supplies the smallest signed governance MVP; no delivery may claim the whole feature done at this checkpoint.
- US2 depends on US1's active revision and factory, then completes input parity and exceptional credential modes.
- US3 depends on US1/US2 revisions and field bindings, then completes the update/error/telemetry matrix. Its foundational capability and binding tests already exist.
- US4 depends on prior source and error semantics; its config tests may be authored earlier but must pass only once the runtime boundary and startup invariants are complete.

### Within Each User Story

Tests first (semantic red) → implementation → focused journey green → existing regression suite. Run `just check` after changed Go/test code and the smallest matching `just` suite. Use focused Ginkgo in addition to, not instead of, the matching maintained E2E recipe. Do not change assertions merely to make implementation pass.

### Parallel Opportunities

- Foundation: T020 (bootstrap tests) and T021 (capability/evidence tests) touch disjoint files.
- US1: T026 (adapter tests) and T027 (domain/builder tests).
- US2: T033 (input/HTTP tests) and T034 (exceptional-mode domain tests).
- US3: T041 (source/revision tests) and T042 (failure/telemetry tests); T082 (local bundle tests) and T084 (token plugin tests).
- US4: T048 (validator/rule tests) and T049 (mixed-rule tests).
- Do not parallelize tasks editing the same rule/service/configuration file; unattended implementation hand-offs remain one group at a time.

## Parallel Examples by User Story

| Story | Concurrent test-authoring tasks | Files |
| --- | --- | --- |
| US1 | T026 + T027 | `internal/adapters/policy/opa/{revision,source}_test.go` vs `internal/domain/impersonation/service_test.go`, `internal/app/builder_test.go` |
| US2 | T033 + T034 | `internal/domain/impersonation/policy_input_test.go`, handler tests vs `internal/domain/impersonation/opa_security_test.go` |
| US3 | T041 + T042 | OPA source/revision tests vs domain failure and OPA telemetry tests |
| US4 | T048 + T049 | Config validator/rule tests vs `internal/domain/impersonation/opa_mixed_rules_test.go` |

## Implementation Strategy

### MVP First

Complete approval preparation → isolated behavior-preserving refactor → all design preconditions and semantic E2E red → tested foundation → US1 signed governance. Demonstrate normal claims, governance denial, human delegation recovery, and signed self-agent without human state. This is an internal checkpoint, not approval to drop the remaining stories.

### Incremental Delivery

Add US2 input/exceptional-mode parity, then US3 managed revision and failure controls, then US4 explicit selection/CEL compatibility. Revalidate earlier stories after each increment. Finish docs, Helm validation, coding-standard/security checks, all container-free repository gates, and the final task/checklist review. Container suites are downstream validation, never local evidence. Save only actual completed work; keep partial tasks unchecked and describe remaining work in the session result.
