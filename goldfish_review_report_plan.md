# Goldfish Annotated Review Report — Round 1

Status: resolved

## Round 1 decisions (interactive breakdown)

- Applied: P1-1 (also contract and tasks.md T011), P1-2, P1-3, P1-4, P1-5, P1-6 (also internal-contracts.md), P1-7, P1-8, P1-9, P1-10, P1-12 (also internal-contracts.md), P1-13, P1-14, P1-15, P1-16.
- P1-11: contest upheld; a pointer to tasks.md § Phase Dependencies was added.
- P1-17 (escalated): the user chose to trace bundle fetches. Applied in the plan, research R16, and contracts/observability.md, limited to the broker-owned `aib_bearer_token_file` client; OPA's inline credential forms use OPA's own client and stay uninstrumented.
- Declined nits: P1-18, P1-19.

**Target:** `specs/20261120-153502-opa-impersonation-authorization/plan.md` (full file, lines 1–334)

**Reviewers:** `implementation_readiness` (completed: 1 Blocker, 8 Majors, 3 Nits, against `implementation-readiness-standard`) and `codebase_details` (completed: 1 Blocker, 5 Majors, 6 Nits, no standard).

After removing duplicates, there are 19 findings: 2 Blocker, 9 Major and 8 Nit. Both reviewers independently found the two Blockers and one Major.

## Prioritized Findings & AI Suggestions

### [P1-1] [`codebase_details`-1-1, `implementation_readiness`-1-M2] - Severity: Blocker
*   **Affected Scope:** [Lines 93, 330](specs/20261120-153502-opa-impersonation-authorization/plan.md#L93) (Constitution Check; API-DECISION)
*   **Discrepancy Description:** The plan bumps the End-User API version "`1.2.0` → `1.3.0`". However, `api/enduser/openapi.yaml:104` is already `1.3.0`: feature 054 made that bump. The same error appears in contracts/openapi-token-opa.yaml:11 and tasks.md T011.
*   **Rationale:** The approval item asks for a version change that has already happened.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** I confirmed it: `api/enduser/openapi.yaml:104` reads `version: 1.3.0` and contains no OPA wording.
*   ***Proposed Edit:***
    *   Location: plan lines 93 and 330, contracts/openapi-token-opa.yaml:11 and tasks.md T011
    *   Change: `1.3.0` → `1.4.0` (from `1.3.0`). The contract comment becomes `# from 1.3.0: …`.

### [P1-2] [`implementation_readiness`-1-B1, `codebase_details`-1-M1, `codebase_details`-1-M2] - Severity: Blocker
*   **Affected Scope:** [Lines 161-195, 262-263, 309](specs/20261120-153502-opa-impersonation-authorization/plan.md#L161-L195) (Source Code tree; Testing Strategy)
*   **Discrepancy Description:** Two planned changes break existing code that the plan does not list:
    *   Changing `ImpersonationRuleConfig.Authorization` to `ImpersonationAuthorizationConfig` breaks every impersonation-rule literal.
    *   Adding parameters to `NewService` breaks about 21–25 test call sites.

    Neither set of files is listed, yet lines 263 and 309 say existing suites run "unchanged". Several other required files are also missing:
    *   `internal/config/loader.go`, which does the raw-map rejection
    *   `internal/domain/tokenexchange/service.go`, which rejects ordinary-exchange `type: opa` (tasks.md T050 names it; the plan does not)
    *   `internal/domain/impersonation/cel.go`, which becomes the static CEL revision
    *   the package maps in `internal/{,ports/,adapters/}AGENTS.md`

    The tree also doesn't mark which files are created and which are modified.
*   **Rationale:** A write allowlist built from the tree would block required edits, and "unchanged" cannot be verified as written.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** I confirmed it: `NewService` is positional (service.go:34-42), and `ports.AuthorizationConfig` is used directly in the test fixtures that both reviewers cite.
*   ***Proposed Edit:***
    *   Location: the Source Code tree and lines 263 and 309
    *   Change:
        *   Prefix each entry with `(new)` or `(modify)`.
        *   Add `internal/config/loader.go`, `internal/domain/tokenexchange/service.go`, `internal/domain/impersonation/cel.go`, `internal/AGENTS.md`, `internal/ports/AGENTS.md` and `internal/adapters/AGENTS.md`.
        *   Add a "Mechanical fixture migration (modify)" block listing:
            *   `tests/e2e/impersonation_test.go`
            *   `tests/e2e/fixtures/impersonation_rules.go`
            *   `tests/e2e/agent_subject_impersonation_test.go`
            *   `internal/domain/impersonation/{service,service_category,service_security,subject_category,audience,agent_extraction_security}_test.go`
            *   `internal/config/impersonation_validator_test.go`
            *   `internal/app/builder_test.go` and `builder_trust_anchor_test.go`
            *   `internal/adapters/http/enduser/oauth2_token_test.go` and `oauth2_token_audit_test.go`
        *   Replace "unchanged" with "behaviorally unchanged; their rule literals and `NewService` calls migrate mechanically in the same commit as the type change".

### [P1-3] [`implementation_readiness`-1-M1] - Severity: Major
*   **Affected Scope:** [Lines 37, 327](specs/20261120-153502-opa-impersonation-authorization/plan.md#L327) (startup bound; API-CFG)
*   **Discrepancy Description:** The plan doesn't say where defaults are applied. A Viper default for `impersonation.opa.startup_timeout` would make the block non-empty, so every CEL-only deployment would fail the "must be empty when no OPA rule" check. Per-rule list entries cannot take Viper defaults at all.
*   **Rationale:** This would break SC-005 and Scenario 4.2.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. The existing pattern applies defaults at the point of use (`defaultEvaluationTimeout`, cel.go).
*   ***Proposed Edit:***
    *   Location: after line 37
    *   Change: "All four OPA defaults (`startup_timeout`, `package`, `decision`, `evaluation_timeout`) are applied at use in the domain or adapter, never as loader or Viper defaults. The empty-block check runs on the raw configured values."

### [P1-4] [`implementation_readiness`-1-M4] - Severity: Major
*   **Affected Scope:** [Lines 18-23, 178-180, 332, 334](specs/20261120-153502-opa-impersonation-authorization/plan.md#L18-L23)
*   **Discrepancy Description:** The plan doesn't say which compiler does what. Specifically:
    *   which compiler enforces the restricted capabilities
    *   which one supplies `Graph.Dependencies` for evidence
    *   how prepared queries are tied to that same compiler
    *   how a `prohibited_builtin` rejection is told apart from `compile_failed`

    Research R2 copies the modules from OPA's full-capability activation compiler.
*   **Rationale:** SR-005 coupling requires that the evidence and the evaluation come from one compiled candidate.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. R2 step 2 and R5 step 2 imply one compiler but never say so.
*   ***Proposed Edit:***
    *   Location: the "Revisions are broker-owned" bullet (lines 18-23)
    *   Change: append:
        *   "Per candidate, the adapter compiles the copied parsed modules in one broker-owned `ast.NewCompiler().WithCapabilities(restricted)`. Each rule's query is prepared with `rego.Compiler(thatCompiler)` and the candidate's store. Evidence reads that compiler's `Graph`.
        *   A compile error that is an undefined-function error naming a removed built-in is classified `prohibited_builtin`; every other compile error is `compile_failed`."

### [P1-5] [`implementation_readiness`-1-M5] - Severity: Major
*   **Affected Scope:** [Lines 189-191, 294-296](specs/20261120-153502-opa-impersonation-authorization/plan.md#L294-L296)
*   **Discrepancy Description:** The shared broker/ExtProc input vectors have no defined file format, consumer functions or compared fields. ExtProc's builder takes only `map[string]string` headers.
*   **Rationale:** A cross-binary test with no schema cannot be implemented consistently.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** tasks.md T033 names the consuming test files and the compared areas, but there is still no vector schema.
*   ***Proposed Edit:***
    *   Location: "Shared input vectors" (line 294)
    *   Change: "One JSON file per case, `tests/testdata/opa-envoy-input/<case>.json`, of the form `{"name", "request": {"method", "path", "raw_query", "headers": {name: value}}, "expected": {"attributes.request.http", "parsed_path", "parsed_query"}, "broker_excluded_headers": [...]}`. Headers are single-valued, matching ExtProc's builder. Broker-only `parsed_body`, `version` and `context` are tested only on the broker side (T033)."

### [P1-6] [`implementation_readiness`-1-M6] - Severity: Major
*   **Affected Scope:** [Lines 116, 172](specs/20261120-153502-opa-impersonation-authorization/plan.md#L116)
*   **Discrepancy Description:** The exact new `NewService` signature is not given. That signature decides how roughly 25 call sites change.
*   **Rationale:** Implementers would choose an incompatible shape.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The finding is valid. internal-contracts.md §3 only says the function "gains" a parameter.
*   ***Proposed Edit:***
    *   Location: the Hexagonal Architecture line (line 112), plus internal-contracts.md §3
    *   Change: "`NewService` appends two trailing parameters, `policySources PolicySourceFactory, publicURL string`. A `nil` factory is valid when no rule selects OPA, so CEL-only call sites pass `nil, ""`."

### [P1-7] [`implementation_readiness`-1-M7, `codebase_details`-1-M4] - Severity: Major
*   **Affected Scope:** [Lines 265-267](specs/20261120-153502-opa-impersonation-authorization/plan.md#L265-L267)
*   **Discrepancy Description:** Committed E2E tests would write under the sandbox-only path `/tmp/gh-aw/agent/`. Every existing E2E file fixture uses `GinkgoT().TempDir()` instead.
*   **Rationale:** The fixed path creates a hidden CI dependency and breaks isolation between parallel Ginkgo processes.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** I confirmed `ci.yml` does not create that path. tasks.md T055 has the same issue but only for local Helm checks, which is fine.
*   ***Proposed Edit:***
    *   Location: line 266-267
    *   Change: "…bootstrap, token, and local bundle files in `GinkgoT().TempDir()` (absolute and never `/`), as existing file-backed E2E fixtures do."

### [P1-8] [`implementation_readiness`-1-M8] - Severity: Major
*   **Affected Scope:** [Line 172](specs/20261120-153502-opa-impersonation-authorization/plan.md#L172); rule.go:55
*   **Discrepancy Description:** CEL principal and email extraction take their timeout from `cfg.Authorization.CEL.EvaluationTimeout`. For OPA rules that block is empty, so extraction silently falls back to 100 ms. The plan doesn't say whether that is intended.
*   **Rationale:** This behavior is undecided.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** tasks.md T050 says "preserving CEL extraction defaults", which implies the 100 ms default, but the plan doesn't state it.
*   ***Proposed Edit:***
    *   Location: Unit Tests, `internal/domain/impersonation` bullet
    *   Change: add "OPA rules' CEL principal and email extraction use the existing default extraction timeout (100 ms); `authorization.opa.evaluation_timeout` bounds only the OPA decision."

### [P1-9] [`codebase_details`-1-M3] - Severity: Major
*   **Affected Scope:** [Lines 47-48, 175-177](specs/20261120-153502-opa-impersonation-authorization/plan.md#L175-L177)
*   **Discrepancy Description:** The plan specifies `os.Root` containment from scratch. `internal/adapters/jwks/trust_directory.go` already implements the same pattern:
    *   `EvalSymlinks`, then `OpenRoot`, rejecting `/`
    *   a regular-file check on the same descriptor
    *   a bounded read
    *   escape classification

    The ban on importing between adapters prevents direct reuse.
*   **Rationale:** Two copies of security-critical containment code tend to drift apart.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** I confirmed the file exists. research.md R3, R15 and R16 never mention it.
*   ***Proposed Edit:***
    *   Location: the Source Code tree and Complexity Tracking
    *   Change: add a `(new)` shared package, `internal/fscontain/`, extracted from `internal/adapters/jwks/trust_directory.go`. It provides a contained open plus the regular-file, size and escape checks. Both `jwks` and `policy/opa` use it, and the jwks tests guard the extraction as a behavior-preserving refactor in Phase 0.

### [P1-10] [`codebase_details`-1-M5, `codebase_details`-1-N2] - Severity: Major
*   **Affected Scope:** [Lines 193-194, 255-260, 269-270](specs/20261120-153502-opa-impersonation-authorization/plan.md#L255-L270)
*   **Discrepancy Description:** The plan doesn't reuse existing test helpers:
    *   `tests/e2e/helpers/local_jwks.go` (`helpers.TrustDir`) already provides Kubernetes Secret rotation and escape fixtures.
    *   A custom bundle server is proposed with no comparison to the existing `sdktest` server.
*   **Rationale:** The new fixtures would duplicate existing ones.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** I confirmed `SwapKubernetesLayout` and `LinkRelativeEscape` exist (local_jwks.go:259, 291).
*   ***Proposed Edit:***
    *   Location: Helper Utilities and Test Data Strategy
    *   Change:
        *   Add: "Reuse `helpers.TrustDir` (`SwapKubernetesLayout`, `LinkRelativeEscape`, `LinkAbsolute`, `WriteOutsideFile`) for token rotation and containment cases."
        *   Add: "The bundle server is custom because `sdktest` cannot inject fetch failures, latency, bearer-token enforcement or an outbound-request recorder."

### [P1-11] [`implementation_readiness`-1-M3] - Severity: Major
*   **Affected Scope:** [Lines 200-213](specs/20261120-153502-opa-impersonation-authorization/plan.md#L200-L213)
*   **Discrepancy Description:** The phases have no file lists or exit criteria, and their order is ambiguous: Phase 1 is listed before Phase 0.
*   **Rationale:** The implementation-readiness standard requires a clear execution order.
*   ***AI Suggestion:*** CONTEST
*   ***Justification:*** The plan's phase table is the template's overview. tasks.md, a file the reviewer could read, gives the execution order, per-task files and checkpoints (tasks.md:281-300, "Phase Dependencies": Setup → Phase 0 committed first → Phase 2 → Foundation → US1–US4 → docs → Phase N). The fixture-migration part of the finding remains open under P1-2.
*   ***Proposed Edit:*** None. Optionally, add a pointer under the table: "Execution order and checkpoints: tasks.md § Phase Dependencies."

### [P1-12] [`implementation_readiness`-1-N2] - Severity: Nit
*   **Affected Scope:** [Lines 29-31, 182](specs/20261120-153502-opa-impersonation-authorization/plan.md#L29-L31)
*   **Discrepancy Description:** It is unclear whether `parsed_body` comes from the raw `url.ParseQuery` result or the map after `dropOmittedValues` has mutated it (oauth2_token.go:88-103).
*   **Rationale:** The two differ for empty values.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Envoy's semantics match the raw parse.
*   ***Proposed Edit:***
    *   Location: the Input bullet
    *   Change: "`parsed_body` comes from a copy of the raw `url.ParseQuery` result taken before omitted-value normalization."

### [P1-13] [`codebase_details`-1-N1] - Severity: Nit
*   **Affected Scope:** [Lines 63-65](specs/20261120-153502-opa-impersonation-authorization/plan.md#L63-L65)
*   **Discrepancy Description:** The OPA package list leaves out `v1/storage`, `v1/plugins/rest` and `v1/logging`, plus `v1/sdk/test` for tests.
*   **Rationale:** research R2 and R16 rely on them.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The fix is trivial.
*   ***Proposed Edit:***
    *   Location: line 64-65
    *   Change: add the missing packages to the list.

### [P1-14] [`codebase_details`-1-N3] - Severity: Nit
*   **Affected Scope:** [Line 116](specs/20261120-153502-opa-impersonation-authorization/plan.md#L116)
*   **Discrepancy Description:** The shutdown mechanism is not named. The builder already has `App.chainShutdown` (builder.go:146).
*   **Rationale:** Naming it avoids yet another hand-rolled shutdown chain.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The fix is trivial.
*   ***Proposed Edit:***
    *   Location: line 116
    *   Change: "…registers `Service.Close` through `App.chainShutdown`."

### [P1-15] [`codebase_details`-1-N5] - Severity: Nit
*   **Affected Scope:** [Line 15](specs/20261120-153502-opa-impersonation-authorization/plan.md#L15)
*   **Discrepancy Description:** The plan uses the shortened path `impersonation.opa` without first giving the full prefix.
*   **Rationale:** The spec stresses the singular full prefix.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The fix is trivial.
*   ***Proposed Edit:***
    *   Location: line 15
    *   Change: "configured once under `oauth2_authorization_server.impersonation.opa` (`impersonation.opa` below)".

### [P1-16] [`implementation_readiness`-1-N3] - Severity: Nit
*   **Affected Scope:** [Lines 110, 158](specs/20261120-153502-opa-impersonation-authorization/plan.md#L158)
*   **Discrepancy Description:** The tree comment "tasks.md — created by /speckit-tasks" is stale. The home of the "OPA impersonation section" is not named.
*   **Rationale:** Both are stale or vague.
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The fix is trivial.
*   ***Proposed Edit:***
    *   Location: line 158 and line 110
    *   Change: line 158 becomes `tasks.md # task list`. Line 110 becomes "an OPA section in `docs/reference/token-exchange.md`".

### [P1-17] [`codebase_details`-1-N6] - Severity: Nit
*   **Affected Scope:** Plan as a whole; contracts/observability.md
*   **Discrepancy Description:** The plan doesn't say whether bundle downloads use `telemetryhttp.NewTransport` or are left uninstrumented.
*   **Rationale:** The repository instruments outbound HTTP only through `internal/telemetryhttp`.
*   ***AI Suggestion:*** ESCALATE
*   ***Justification:*** This is a design choice with no evidence either way in the artifacts. Instrumenting the SDK's downloader requires wiring a custom client into the credential plugin, which R16's `NewClient` could do. Do you want bundle fetches traced?
*   ***Proposed Edit:*** None until you decide.

### [P1-18] [`implementation_readiness`-1-N1] - Severity: Nit
*   **Affected Scope:** [Lines 186, 190, 193](specs/20261120-153502-opa-impersonation-authorization/plan.md#L186)
*   **Discrepancy Description:** Some tree entries name directories rather than files, and some new test files are not listed.
*   **Rationale:** The standard allows globs only for generated output.
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** tasks.md names the concrete files (for example T033), and P1-2 already expands the tree.
*   ***Proposed Edit:*** None.

### [P1-19] [`codebase_details`-1-N4] - Severity: Nit
*   **Affected Scope:** [Line 74](specs/20261120-153502-opa-impersonation-authorization/plan.md#L74)
*   **Discrepancy Description:** Nothing enforces the "no `internal/extproc` import" constraint; the existing depguard rule covers only the opposite direction.
*   **Rationale:** The constraint could be enforced by lint.
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** This is worthwhile, but it is a lint-configuration improvement outside this feature's scope.
*   ***Proposed Edit:*** None.
