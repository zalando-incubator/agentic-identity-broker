# Implementation Plan: OPA policies for impersonation authorization

**Branch**: `speckit/issue-518` | **Date**: 2026-11-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/20261120-153502-opa-impersonation-authorization/spec.md`

**Sign-off**: Implementation needs the maintainer's 👍 on the pipeline approval comment for every
row in [Approval Items](#approval-items). Clarification Q1 is accepted (mjungsbluth, 2026-11-21).

## Summary

Let each local-mode impersonation rule select `authorization.type: opa` instead of CEL. The broker
embeds the OPA Go SDK, which OPA already provides to ExtProc, as one engine shared by every OPA rule
and configured once under `oauth2_authorization_server.impersonation.opa` (`impersonation.opa` below). The engine pulls operator-managed policy/data bundles
through a restricted OPA bootstrap file, or loads a local bundle directory once at startup. Each OPA
rule names only its decision (`package`, `decision`, `evaluation_timeout`).

- **Revisions are broker-owned.** After each OPA activation, a new adapter
  (`internal/adapters/policy/opa/`) builds an immutable candidate from the compiled modules and a data
  copy. It compiles the candidate without nondeterministic built-ins other than `time.now_ns` and
  `io.jwt.decode_verify` (no `http.send`), checks that every OPA rule's decision rule exists
  (`GetRulesExact` on `data.<package>.<decision>`; a data-only value or only nested rules do not
  count), and asks the domain to admit it. Only
  admitted candidates replace the active revision. Failed fetches and rejected candidates keep the
  last admitted revision (Q1). ([research R2, R4](research.md)) Each candidate compiles the copied
  parsed modules in one broker-owned `ast.NewCompiler().WithCapabilities(restricted)`; every OPA rule's
  query is prepared with `rego.Compiler(thatCompiler)` and the candidate's store. An undefined-function compile error naming a removed built-in is
  classified `prohibited_builtin`; every other compile error is `compile_failed`. A decision rule that
  exists but is undefined for a request is a terminal runtime failure (500), not a deny; docs and
  examples require a `default` decision (2026-11-26).
- **Binding controls stay CEL-only.** CEL rules keep today's startup checks: mandatory references
  for unsigned-subject and audience-absent roles, and the email and `sub` field bindings. OPA rules
  have no static reference or field-binding checks (2026-11-25 decision, SR-005, CONST-AMEND); the
  operator's policy is the only control for their exceptional credential modes. A candidate is
  admitted only if every OPA rule's decision rule exists. Each request reads one revision before the first
  rule and uses it for every OPA rule's decision. ([R11](research.md#r11--one-shared-opa-engine-revised-2026-11-24-maintainer-choice-option-b)) ([R6](research.md#r6--admission-and-requirements-stay-in-the-domain))
- **Input and decision.** The input is the Envoy-compatible request envelope, including Envoy's
  `version`, `parsed_query`, and `parsed_body` (the parsed form body without the seven credential-bearing OAuth
  parameters, from a copy of the raw `url.ParseQuery` result taken before omitted-value normalization),
  without credential headers, `type: "impersonation"`, and `context` with CEL-equivalent values and the
  broker-resolved `agent` object (`id`, optional `canonical_id` and `external_id`), aligned with
  ExtProc's AuthZEN Grant Context (ADR `048-authzen-delegation-pip`). Grant-context members are
  reserved and absent. `allow`/`deny` map to match/non-match; failures map internally to terminal
  `server_error` with new diagnostics (HTTP 500 for connected callers).
  ([contracts/policy-input-decision.md](contracts/policy-input-decision.md))
- **Startup is bounded.** A new `impersonation.opa.startup_timeout` (default `30s`) bounds readiness;
  without an admitted snapshot containing every configured bundle the broker does not start.
  Completeness is read from that same snapshot, not combined from historical activations.
- **Defaults apply at use.** All four OPA defaults (`startup_timeout`, `package`, `decision`,
  `evaluation_timeout`) are applied in the domain or adapter at use, as the CEL
  `defaultEvaluationTimeout` is, never as loader or Viper defaults. The empty-block check runs on
  the raw configured values, so CEL-only deployments stay valid: an absent block or `{}` is
  accepted when no rule selects OPA, and any populated key fails.
- **Bundle downloads are observable.** In managed mode a listener registered with OPA's bundle
  plugin (`bundle.Plugin.Register`) feeds a per-bundle download counter, a download-age gauge, and a
  rate-limited download-failure log, so a failing bundle service is visible although it builds no
  candidate ([contracts/observability.md](contracts/observability.md)).
- **Governance.** A Proposed ADR ([draft](adr-20261121-061324-opa-impersonation-authorization.md))
  partly supersedes ADR `009-cel-for-authorization-policies` and the CEL-specific mechanisms of ADRs
  `031-unverified-subject-unsigned-jwt`, `034-audience-absent-impersonation-credentials`, and
  `045-agent-subject-impersonation`. The unsigned-subject exception is not widened.

### Accepted clarification and planning decisions

- 2026-11-24 containment revision (maintainer): OPA files no longer use `security.trust_material_dir`;
  each is read through an `os.Root` over its own directory (R3).
- 2026-11-24 revision (maintainer): one shared OPA engine (option B, R11); rule keys other than
  `package`, `decision`, and `evaluation_timeout` rejected, including reserved `policy_url` and
  `policy_path`; local bundle directory mode (R15); contained, rotating bundle token file (R16);
  `package` default `aib.broker.impersonation`; ExtProc bootstrap reuse documented.
- Q1 (managed bundles only, no policy-time networking, last complete revision retained, no maximum
  age) was accepted by mjungsbluth on 2026-11-21 and is recorded in [spec.md](spec.md) Clarifications.
- Planning added the startup bound (now `impersonation.opa.startup_timeout`) to the spec's configuration table (the spec
  left the startup bound to planning) and resolved the spec's planning obligations for SR-005
  (enforcement mechanism and superseding ADR). Both are recorded under the spec's Clarifications
  as a planning session entry.

## Technical Context

**Language/Version**: Go (version in `go.mod`)
**Primary Dependencies**: `github.com/open-policy-agent/opa` v1.21.4 (existing module dependency, newly
linked into the broker binary: `v1/sdk`, `v1/rego`, `v1/ast`, `v1/storage`, `v1/storage/inmem`,
`v1/plugins`, `v1/plugins/rest`, `v1/plugins/bundle` (status listener), `v1/bundle`, `v1/logging`; tests also use `v1/sdk/test`); cel-go (unchanged); chi, Viper, OpenTelemetry (unchanged). No new module.
**Storage**: N/A. No migration; policy state is in memory in the shared OPA engine.
**Testing**: Go `testing` + testify for units; Ginkgo/Gomega E2E with an in-process `httptest` bundle
server; `go test -race` for revision swaps.
**Target Platform**: Linux container (broker), local mode only.
**Project Type**: Backend service (single Go module), plus Helm chart and documentation.
**Performance Goals**: Each OPA decision completes or fails within its rule's `evaluation_timeout`
(FR-010, SC-006); no network on the request path; activation work off the request path.
**Constraints**: Fail closed on every error; no claim data in telemetry; bounded startup; one
revision per request for all OPA rules; no `internal/extproc` import.
**Scale/Scope**: A handful of OPA rules per deployment sharing one engine; governance data held
twice in memory (OPA store and active snapshot), independent of the number of rules.

No NEEDS CLARIFICATION remains; [research.md](research.md) resolves every open design point.

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Value objects documented in [data-model.md](data-model.md); no persisted entity or aggregate.
- [x] **Domain Concepts**: Impersonation Policy Engine, Impersonation OPA Engine, Policy/Data Revision, and Policy Bootstrap go to the `ARCHITECTURE.md` glossary ([data-model.md](data-model.md#glossary-additions-architecturemd)).
- [x] **Entity IDs**: N/A. No new UUID entity.
- [x] **Configuration Design**: [contracts/impersonation-opa-config.yaml](contracts/impersonation-opa-config.yaml) with YAML examples and validation.
- [x] **Config Examples**: New `examples/config/impersonation-opa.yaml`, bootstrap and Rego examples under `examples/config/opa/impersonation/`, and an index entry in `examples/config/README.md`.
- [x] **Helm Deployment Contract**: The broker chart deploys impersonation. `values.schema.json` gains the `impersonation.opa` engine block and the OPA rule shape; the bootstrap, local bundle directory, and token file mount through existing `extraVolumes`/`extraVolumeMounts`; the chart README documents both. ExtProc's chart surface is untouched.
- [x] **API Design First**: [contracts/openapi-token-opa.yaml](contracts/openapi-token-opa.yaml) documents the `POST /oauth2/token` description delta before implementation.
- [x] **API Documentation**: `api/enduser/openapi.yaml` (description, example, version `1.4.0`, from the current `1.3.0`) and `docs/api/enduser-api.md`.
- [x] **API Changes**: Registered as API-INPUT, API-DECISION, API-CFG, API-BOOT, API-DIAG; implementation waits for sign-off.
- [x] **Database Design**: N/A. No schema change.
- [x] **E2E Acceptance Tests**: All 17 scenarios mapped below; written before implementation.
- [x] **E2E Test Mapping**: One `It()` per scenario in `tests/e2e/opa_impersonation_authorization_test.go`.
- [x] **E2E Red Phase**: Tests assert real status codes, decoded claims, and log records against the not-yet-supported `type: opa`.
- [x] **Frontend Playwright E2E**: N/A. No UI change.
- [x] **Frontend Screenshots**: N/A.

**Implementation Considerations**:

- [x] **Security-First (I)**: OPA is opt-in per rule; every failure is terminal or keeps the last admitted revision; existing credential, category, delegation, and minting controls stay authoritative. The ADR 031 exception keeps its scope. For OPA rules, its startup-verified `subject_token` reference and email-binding controls are dropped, so Principle I needed amendment CONST-AMEND, now applied to the constitution as v4.8.0 on 2026-11-25 (commit `fb84c31`); it takes effect for OPA rules once ADR-OPA is Accepted ([research R13](research.md#r13--superseding-adr-sign-off-adr-opa)).
- [x] **Architecture Docs (II)**: `ARCHITECTURE.md` impersonation section, glossary, and ADR index record the partial supersession. The new ADR records its supersedes links; no ADR already on the default branch is edited.
- [x] **ADRs (II)**: Draft `adr-20261121-061324-opa-impersonation-authorization.md`, status Proposed; moved to `adrs/` with the recorded approval by the first implementation task.
- [x] **Library-First Security (III)**: OPA SDK, compiler, and capabilities do the policy work; no custom parser or crypto.
- [x] **Zalando Guidelines**: No new resource or field; the OpenAPI change is descriptive.
- [x] **End-User Docs**: `docs/api/enduser-api.md`, `docs/reference/token-exchange.md`, `docs/configuration.md`, and an OPA impersonation section with equivalent CEL/Rego examples in `docs/reference/token-exchange.md` (FR-011).
- [x] **Migration Testing**: N/A.
- [x] **Hexagonal Architecture (VI)**: New port `ports.ImpersonationPolicySource`; `impersonation.NewService` appends two trailing parameters, `policySources PolicySourceFactory, publicURL string` (`nil, ""` is valid when no rule selects OPA), takes no context, and waits with `AwaitReady(context.WithTimeout(context.Background(), startup_timeout))`; `Service.Close(ctx context.Context) error`; the handler sets `req.HTTP` after `impersonation.ParseRequest`; the `authorization` seam and `internal/fscontain` API are declared in [internal contracts § 3 and § 6](contracts/internal-contracts.md); adapter in `internal/adapters/policy/opa/`; binding rules and admission in the domain; handler only passes transport facts.
- [x] **Persistence Patterns (IX)**: N/A.
- [x] **Configuration (VII)**: Broker-only keys under `internal/ports/config.go`; validator, docs, examples, and Helm schema updated together.
- [x] **TDD (VIII)**: Unit and E2E tests precede each implementation task.
- [x] **DI via Builder (XII)**: `internal/app/builder.go` builds the policy-source factory and registers `Service.Close` through `App.chainShutdown`.
- [x] **Specification by Example (XIV)**: Spec journeys 1–4 cover configuration, HTTP exchanges, and decoded claims; no wireframes apply.
- [x] **Coding Standards (XV)**: [contracts/observability.md](contracts/observability.md) follows LOGGING and OPENTELEMETRY: constant names, closed attribute sets, no principals or claims. Bundle downloads are traced where the broker owns the HTTP client: the `aib_bearer_token_file` plugin's `NewClient` wraps its transport with `telemetryhttp.NewTransport`, the repository's only outbound HTTP instrumentation path. Services that use OPA's inline credential forms use OPA's own client and stay uninstrumented; documentation states this.

Post-design re-check: no violation. Complexity Tracking lists the justified trade-offs.

## Spec Alignment

| Spec item | Design |
| --- | --- |
| FR-001, FR-002 | R10, [config contract](contracts/impersonation-opa-config.yaml) |
| FR-003, SR-002 | R2, R3, R7 |
| FR-004 | R2 (atomic swap; no decision reuse) |
| FR-005, FR-006, SR-003 | R8, [input contract § 1–2](contracts/policy-input-decision.md) |
| FR-007, SR-004 | R9, [decision contract § 3](contracts/policy-input-decision.md#3-decision-document) |
| FR-008 | [internal contracts § 3](contracts/internal-contracts.md) (pipeline order unchanged) |
| FR-009 | R10 (ordinary-exchange explicit `type: opa` rejected at startup; valid CEL unchanged) |
| FR-010 | Config contract `evaluation_timeout` |
| FR-011 | Documentation and example tasks (Constitution Check) |
| FR-012, SR-006 | R2, R3, R4 |
| FR-013, SR-005 | R5 (withdrawn), R13, ADR draft, CONST-AMEND |
| SR-001 | ADR decision 6 |

## Project Structure

### Documentation (this feature)

```text
specs/20261120-153502-opa-impersonation-authorization/
├── spec.md
├── plan.md                    # this file
├── research.md
├── data-model.md
├── quickstart.md
├── adr-20261121-061324-opa-impersonation-authorization.md   # Proposed ADR draft
├── contracts/
│   ├── impersonation-opa-config.yaml
│   ├── policy-input-decision.md
│   ├── openapi-token-opa.yaml
│   ├── observability.md
│   └── internal-contracts.md
├── checklists/requirements.md
└── tasks.md                   # task list
```

### Source Code (repository root)

Every path tasks.md touches, marked `(new)` or `(modify)`.

```text
# Ports and configuration
(modify) internal/ports/config.go                                  # ImpersonationAuthorizationConfig, OPA types
(modify) internal/ports/config_test.go                             # rule literal migration
(new)    internal/ports/impersonation_policy.go                    # port, revision, admission, errors
(new)    internal/ports/impersonation_policy_test.go               # sensitive input rendering safety
(modify) internal/config/loader.go                                 # decode hook rejecting unsupported rule opa keys
(modify) internal/config/loader_test.go
(modify) internal/config/validator.go                              # rule engine and engine-block validation
(modify) internal/config/validator_test.go
(modify) internal/config/impersonation_validator_test.go           # intent change: "non-cel authorization" case
# Shared file containment (Phase 0)
(new)    internal/fscontain/fscontain.go                           # OpenDir, Dir.ReadFile, Dir.Walk, Error
(new)    internal/fscontain/open_unix.go                           # openFlags O_RDONLY|O_NONBLOCK (moved)
(new)    internal/fscontain/open_other.go                          # openFlags (moved)
(new)    internal/fscontain/fscontain_test.go
(new)    internal/fscontain/open_unix_test.go                      # FIFO and escape cases (moved)
(modify) internal/adapters/jwks/trust_directory.go                 # uses fscontain; errors byte-for-byte unchanged
(delete) internal/adapters/jwks/trust_directory_unix.go            # moved to fscontain/open_unix.go
(delete) internal/adapters/jwks/trust_directory_other.go           # moved to fscontain/open_other.go
(modify) internal/adapters/jwks/trust_directory_unix_test.go       # FIFO case moves to fscontain
# Domain
(modify) internal/domain/tokenexchange/diagnostic.go               # opa_evaluation_failed, opa_decision_invalid
(modify) internal/domain/tokenexchange/service.go                  # ordinary-exchange type: opa startup rejection (T056)
(new)    internal/domain/impersonation/authorizer.go               # authorization seam, CEL and OPA implementations
(new)    internal/domain/impersonation/admission.go                # OPA candidate admission
(new)    internal/domain/impersonation/admission_test.go
(new)    internal/domain/impersonation/policy_input.go             # input builder
(new)    internal/domain/impersonation/policy_input_test.go
(new)    internal/domain/impersonation/opa_security_test.go
(new)    internal/domain/impersonation/opa_failure_test.go
(new)    internal/domain/impersonation/opa_mixed_rules_test.go
(modify) internal/domain/impersonation/cel.go                      # authorizationProgram as the CEL authorization
(modify) internal/domain/impersonation/rule.go, service.go, request.go, errors.go
(modify) internal/domain/impersonation/rule_test.go, service_test.go   # service_test.go "non-cel authorization": intent change
# Adapter
(new)    internal/adapters/policy/opa/source.go                    # shared engine: SDK runtime, trigger, readiness, stop
(new)    internal/adapters/policy/opa/bootstrap.go                 # closed bootstrap allowlist
(new)    internal/adapters/policy/opa/local.go                     # local bundle directory (R15)
(new)    internal/adapters/policy/opa/tokenplugin.go               # aib_bearer_token_file plugin (R16), traced transport
(new)    internal/adapters/policy/opa/revision.go                  # candidate, evaluation, decision parsing
(new)    internal/adapters/policy/opa/capabilities.go              # restricted built-ins, decision presence
(new)    internal/adapters/policy/opa/telemetry.go                 # span, metrics, bundle status listener, lifecycle logs
(new)    internal/adapters/policy/opa/{source,bootstrap,local,tokenplugin,revision,capabilities,telemetry}_test.go
(modify) internal/adapters/http/enduser/oauth2_token.go            # HTTP attributes, audit attributes
(modify) internal/adapters/http/enduser/oauth2_token_test.go
(modify) internal/app/builder.go                                   # factory wiring, NewService(..., nil, "") first, shutdown
(modify) internal/app/builder_test.go
(modify) internal/extproc/authorization/input_builder_test.go      # vectors only; no production coupling
# API, docs, examples, chart
(modify) api/enduser/openapi.yaml
(modify) docs/configuration.md, docs/api/enduser-api.md, docs/reference/token-exchange.md
(modify) docs/reference/telemetry-descriptions.yaml, docs/reference/telemetry.md   # just telemetry-doc
(new)    examples/config/impersonation-opa.yaml
(new)    examples/config/opa/impersonation/bootstrap.yaml, exceptional-modes.rego
(new)    examples/config/opa/impersonation/local/governance.rego, data.json, .manifest
(modify) examples/config/README.md
(modify) charts/agentic-identity-broker/values.schema.json, values.yaml (comment), README.md
# Architecture and governance
(modify) ARCHITECTURE.md                                           # glossary, § 3.1.y, § 10 ADR table
(modify) AGENTS.md                                                 # ADR index and Domain Glossary
(modify) internal/AGENTS.md, internal/ports/AGENTS.md, internal/adapters/AGENTS.md
(new)    adrs/20261121-061324-opa-impersonation-authorization.md   # moved from this feature directory
# Tests
(new)    tests/testdata/opa-envoy-input/basic_post.json, repeated_headers.json, query_string.json, filtered_headers.json
(new)    tests/e2e/opa_impersonation_authorization_test.go
(new)    tests/e2e/fixtures/opa/impersonation/governance.rego, data.json, data_revoked.json, exceptional.rego,
         undefined.rego, malformed.rego, slow.rego, http_send.rego, cel_equivalent.rego
(modify) tests/e2e/fixtures/impersonation_rules.go                 # paired CEL and OPA rule builders
(new)    tests/e2e/helpers/opa_bundle_server.go                    # in-process bundle server
(modify) tests/e2e/bootstrap/server_factory.go                     # OPA policy-source factory wiring
(modify) tests/e2e/bootstrap/telemetry.go                          # test-local span processor for Scenario 3.2
```

**Mechanical fixture migration (modify)**: the rule configuration type and `NewService` parameters
change, so these files migrate in the same commit as the type change (T015):
`tests/e2e/impersonation_test.go`, `tests/e2e/fixtures/impersonation_rules.go`,
`tests/e2e/agent_subject_impersonation_test.go`, `internal/ports/config_test.go`,
`internal/domain/impersonation/{service,service_category,service_security,subject_category,audience,agent_extraction_security}_test.go`,
`internal/config/impersonation_validator_test.go`, `internal/app/builder.go` (`NewService(..., nil, "")`),
`internal/app/builder_test.go`, `internal/app/builder_trust_anchor_test.go`,
`internal/adapters/http/enduser/oauth2_token_test.go`, and
`internal/adapters/http/enduser/oauth2_token_audit_test.go`, plus every other compile error from the
type and signature change that `go build ./... && go vet ./...` reports. Behavior stays unchanged
except two intent changes: the "non-cel authorization" cases in
`internal/config/impersonation_validator_test.go` and `internal/domain/impersonation/service_test.go`
(`type: "opa"` is no longer rejected as a type) assert the failures they now hit (a missing
`impersonation.opa` engine block), not a mechanical rename.

**Structure Decision**: Existing single Go module. The domain keeps the impersonation pipeline; one
new driven adapter owns the shared OPA engine; no new bounded context.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 1** | Setup: first task records supplied approval and prepares the ADR move; confirm existing OPA dependency | Included |
| **Phase 0** | Refactor: rule authorization behind the `authorizer` abstraction and single-revision read; `os.Root` containment extracted from `internal/adapters/jwks/trust_directory.go` into `internal/fscontain/`; no behavior change; first committed code change, separate from staged ADR/feature work | Included |
| **Phase 2** | Design preconditions: config types and validation, OpenAPI delta, E2E tests (red), diagnostics | **MANDATORY** |
| **Phase 2.7** | Entity boilerplate | Skipped: no entity |
| **Phase 2.5** | Foundation: port, adapter skeleton, bootstrap validation, capabilities | Included |
| **Phase 3+** | US1 → US2 → US3 → US4 | Included |
| **Phase N** | Constitution compliance, docs, Helm, ARCHITECTURE, security review | **MANDATORY** |

Execution order, per-task files, and checkpoints: [tasks.md § Phase Dependencies](tasks.md#phase-dependencies).

- [x] Phase 0 (refactoring): include. Today the domain reads `rule.authz` three times per evaluation; making it one revision read is a behavior-preserving change that existing tests cover, and it keeps the feature diff reviewable.
- [x] Phase 2.7 (entity boilerplate): skip. No new entity, handler, or repository.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/opa_impersonation_authorization_test.go`

**Framework**: Ginkgo/Gomega, following [tests/e2e/README.md](../../tests/e2e/README.md) and the
dual-server bootstrap used by `tests/e2e/impersonation_test.go`.

**Test Organization**: `Describe("OPA Impersonation Authorization")` with one `Context` per user story
and one `It` per scenario.

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1 Scenario 1.1 | `tests/e2e/opa_impersonation_authorization_test.go` | `It("issues the normal token when governance policy allows a delegated human")` |
| US1 Scenario 1.2 | same file | `It("denies with 403 access_denied when governance marks the agent unregistered")` |
| US1 Scenario 1.3 | same file | `It("keeps the delegation recovery response when the policy allows")` |
| US1 Scenario 1.4 | same file | `It("issues a signed self-agent token without human state")` |
| US2 Scenario 2.1 | same file | `It("exposes Envoy-compatible HTTP fields and CEL-equivalent context")` |
| US2 Scenario 2.2 | same file | `It("ignores identity-looking headers when the policy requires another client")` |
| US2 Scenario 2.3 | same file | `It("issues an unsigned human token with the subject's email when the policy allows")` |
| US2 Scenario 2.4 | same file | `It("issues an unsigned self-agent token for the exact target and rejects cross-agent subjects")` |
| US2 Scenario 2.5 | same file | `It("accepts audience-absent signed roles and rejects aud, bad signatures, and expiry")` |
| US3 Scenario 3.1 | same file | `It("denies the next request after a revoking revision activates")` |
| US3 Scenario 3.2 | same file | `It("fails closed without fallthrough on runtime failures and observes caller cancellation through correlated telemetry")` |
| US3 Scenario 3.3 | same file | `It("fails startup for a missing or invalid bootstrap or no ready policy within the bound")` |
| US3 Scenario 3.4 | same file | `It("keeps the last complete revision after a failed fetch or invalid update")` |
| US3 Scenario 3.5 | same file | `It("rejects revisions that use http.send without contacting the destination")` |
| US4 Scenario 4.1 | same file | `It("lets a later CEL rule authorize after an explicit OPA deny")` |
| US4 Scenario 4.2 | same file | `It("keeps CEL-only deployments unchanged without OPA configuration")` |
| US4 Scenario 4.3 | same file | `It("fails startup for unknown, conflicting, or missing engine configuration")` |

**Red Phase Requirements**: Tests compile against the new configuration types and assert real HTTP
status codes, OAuth error bodies, decoded token claims, startup errors, and captured log records. No
placeholder assertions, `Skip`, `XIt`, or `PIt`.

**Test Data Strategy**:
- Existing fixtures: test issuer and JWKS server, agents, user grants, impersonation rule builders in
  `tests/e2e/impersonation_test.go`.
- New fixtures: Rego policies and data for each journey in `tests/e2e/fixtures/opa/impersonation/`;
  a bundle helper that builds tarballs with `bundle.Write` and serves them with controllable ETag,
  failure, and latency, plus a recorder that proves the `http.send` target is never contacted. It is
  custom because OPA's `sdktest` server, used by ExtProc tests, cannot inject fetch failures, latency,
  bearer-token enforcement, or an outbound-request recorder.

**Test Execution Flow**: write the 17 tests (red) → implement per story → green → run existing
impersonation suites, behaviorally unchanged (their rule literals and `NewService` calls migrate
mechanically in the same commit as the type change).

**Bootstrap Strategy**: Production `app.Builder` through `tests/e2e/bootstrap/`; fresh server and
storage per test; OPA polling delays of 1 s; bootstrap, token, and local bundle files in `GinkgoT().TempDir()`
(absolute and never `/`), as existing file-backed E2E fixtures do.

**Helper Utilities**: Existing JWT signing and claim decoding helpers; a log-capture helper for
credential-free assertions (existing pattern in `impersonation_test.go`); `helpers.TrustDir` in
`tests/e2e/helpers/local_jwks.go` (`SwapKubernetesLayout`, `LinkRelativeEscape`, `LinkAbsolute`,
`WriteOutsideFile`) for token rotation and containment cases.

### Unit & Integration Tests

**Unit Tests**:
- `internal/domain/impersonation`: engine selection and validation; unchanged CEL binding errors; OPA
  admission (decision presence, all-or-nothing); single-revision use with a fake source whose revision swaps between steps;
  input builder field set, header filtering, the same values and types as the CEL activation; error
  mapping. OPA rules' CEL principal and email extraction use `defaultEvaluationTimeout` (100 ms,
  `cel.go`), because an OPA rule has no CEL timeout; `authorization.opa.evaluation_timeout` bounds
  only the OPA decision.
- `internal/adapters/policy/opa`: closed bootstrap allowlist (each rejected member class, no echo of
  values); capability rejection for each of the 9 removed built-ins and `io.jwt.decode_verify`
  allowed; decision presence by exact rule lookup; bundle download counter and gauge; decision parsing; timeout and cancellation; candidate rejection keeps the active revision;
  `-race` concurrent evaluation during swaps; telemetry content.
- `internal/config`: validator errors with indexed rule paths.
- `internal/adapters/http/enduser`: HTTP attributes and audit attributes.

**Cancellation acceptance**: Scenario 3.2 keeps one `It` with connected HTTP 500 variants and a
canceled-client variant. A test-local span processor waits for evaluation start; cancel the client,
wait for the correlated evaluation/request spans to end, and assert `canceled`, no issuance, and no
later-rule authorization. Domain tests cover internal terminal `server_error` mapping (A1).

**Initial completeness**: Test a usable first-bundle subset followed by a rejected complete
candidate; readiness must fail with no admitted subset (I1). Include this variant in Scenario 3.3.

**Shared input vectors**: One JSON file per case, `tests/testdata/opa-envoy-input/<case>.json`, shaped
`{"name", "request": {"method", "path", "raw_query", "headers": {name: value}}, "expected":
{"attributes.request.http", "parsed_path", "parsed_query"}, "expected_path": {"broker": ..., "extproc": ...},
"broker_excluded_headers": [...]}`, with single-valued headers to match ExtProc's builder.
`expected_path` is per builder because ExtProc keeps the query string in `path`
(`input_builder.go`, `headers[":path"]`) while the broker strips it. Cases: `basic_post.json`,
`repeated_headers.json`, `query_string.json`, `filtered_headers.json`. Broker-only `parsed_body`,
`version`, and `context` are tested only on the broker side (tasks.md T039). Broker and ExtProc tests consume these vectors for
common Envoy field semantics, preserving broker exclusions and the ADR's intentional parsing
divergence (F3). They must not import production packages across binaries.

**Sensitive input safety**: Port tests cover redacted `LogValue`, `String`, and `GoString` through
value/pointer formatting and both slog handlers; evaluation retains full data (C1).

**Container-free repository validation**: Use `ginkgo -v --procs=1 --skip='Agentgateway' ./tests/e2e/extproc/`
for direct ExtProc scenarios. The full `just test-e2e-extproc` recipe requires containers and is
deferred to downstream container validation, not run in the implementation sandbox (I4).
No tests or CI are changed to bypass container coverage.

**Integration Tests**: None. No database or container-backed behavior changes.

**Test Coverage Goals**: All new domain and adapter branches; 100% of acceptance scenarios in E2E;
existing impersonation unit and E2E suites pass with behavior unchanged after the mechanical fixture migration.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Superseding ADR for CEL-specific binding controls of accepted ADRs `031-unverified-subject-unsigned-jwt`, `034-audience-absent-impersonation-credentials`, `045-agent-subject-impersonation` and the OPA reservation in `009-cel-for-authorization-policies`, plus constitution amendment CONST-AMEND (Approval Items ADR-OPA, CONST-AMEND) | Q2 requires OPA for all existing credential modes; the accepted controls are written for a startup-compiled CEL expression, and the 2026-11-25 decision drops the static binding checks for OPA rules | Signed-only OPA was withdrawn by the maintainer; amending accepted ADRs in place is forbidden (Principle II); static binding analysis was rejected as shallow and not a security control |
| Broker-owned revision snapshots with a second copy of bundle data | Only way to reject a candidate after OPA activates it (prohibited built-in, incomplete bundles, undefined decision) and to keep one revision per request for every OPA rule with public OPA APIs | OPA's own compiler cannot enforce capabilities at activation; a process-global custom activator is shared mutable state |
| New shared package `internal/fscontain/` extracted from `internal/adapters/jwks/trust_directory.go` | The OPA adapter needs the same `os.Root` containment (regular file on the same descriptor, size bound, escape classification), and adapters must not import each other | A second copy of security-critical containment code would drift; the jwks tests guard the behavior-preserving extraction in Phase 0 |
| Restricting all nondeterministic built-ins except `time.now_ns` and `io.jwt.decode_verify` (Approval Item DEC-BUILTINS) | Fail closed for future network built-ins; `opa.runtime` exposes configuration | A network-only denylist fails open on OPA upgrades |

## Approval Items

Every row except CONST-AMEND is Proposed; no approval is recorded by this plan. CONST-AMEND's text
is applied to the constitution and takes effect for OPA rules once ADR-OPA is Accepted. Detailed
decisions and main alternatives are in the cited authoritative artifacts.

| ID | Type | Decision | Details |
| --- | --- | --- | --- |
| ADR-OPA | ADR | New ADR `20261121-061324-opa-impersonation-authorization`: OPA selectable per impersonation rule; managed bundles only; broker-owned admitted revisions; OPA rules keep every credential mode without static binding checks, partly superseding ADRs `009-cel-for-authorization-policies`, `031-unverified-subject-unsigned-jwt` (controls 3–4), `034-audience-absent-impersonation-credentials` (control 3), and `045-agent-subject-impersonation` (decision 2a) for OPA rules only; unsigned-subject exception scope unchanged. Alternatives: signed-only OPA (withdrawn by Q2); static binding analysis (withdrawn 2026-11-25); an unsigned exact-target subject staying human unless the decision opts in, or a decision echoing the approved `sub`/email (rejected 2026-11-26); required `https` or bundle signing (rejected 2026-11-26). Approval of this row includes the Principle I interpretation that bundle transport and signing are operator-trusted deployment configuration, not a broker security feature that must be on by default (spec 2026-11-24 rationale; ADR Consequences). | [ADR draft](adr-20261121-061324-opa-impersonation-authorization.md) |
| API-CFG | API | Proposed (revised 2026-11-24, option B): shared engine block `impersonation.opa` with `policy.config_file` or `policy.path` (exactly one) and `startup_timeout` (default `30s`, `1s`–`5m`), required when a rule selects OPA, and absent or `{}` otherwise (any populated key without an OPA rule fails); defaults applied at use; per-rule `authorization.type: opa` with `authorization.opa.package` (default `aib.broker.impersonation`), `.decision` (default `result`), and `.evaluation_timeout` (default `100ms`, `10ms`–`5s`); every other rule key fails startup, including reserved `policy_url`/`policy_path`; exactly one engine block per rule; all-or-nothing admission and one revision per request; matching Helm schema; OPA files by absolute path, each read through an `os.Root` over its own directory (no `security.trust_material_dir`); explicit ordinary-exchange `token_exchange.authorization.type: opa` fails startup, valid CEL unchanged. Alternatives: one runtime per rule (previous proposal), named policies. | `specs/20261120-153502-opa-impersonation-authorization/contracts/impersonation-opa-config.yaml:14-218` |
| API-BOOT | API | Bootstrap subset: only `services`, `bundles`, `keys`, `labels`, and `plugins.aib_bearer_token_file`, each limited to the closed member allowlist for OPA v1.21.4 (revised 2026-11-26): inline `bearer` token, inline OAuth2 client credentials, `s3_signing` environment/metadata/assume-role credentials, `gcp_metadata`, `azure_managed_identity`, and the token-file plugin; `bundles.<name>` `service`/`resource`/`signing`/`polling`/`size_limit_bytes`/`trigger`; `keys.<id>` `algorithm`/`key`/`scope`; every file-reading credential (`token_path`, `client_assertion_path`, web-identity, profile, SSO), `tls`, `client_tls`, `allow_insecure_tls`, and every other key or plugin (decision logs, status, discovery, persistence, …) and `persist: true` fail startup without echoing values; no https or signing requirement (operator choice, documented); if OPA v1.21.4 does not resolve `credentials.plugin` through `sdk.Options.Plugins`, inline credentials only; bootstrap is a contained regular file opened through `os.Root`, at most 1 MiB; all OPA-native local-file references are rejected; credentials inline or through the broker's contained token-file plugin, re-read before every bundle request (R16); local bundle directory mode read once through `os.Root` (R15). Alternative: unrestricted native file reads under a justified exception, full OPA configuration as in ExtProc (exports claims, runtime reconfiguration), or managed bundles with inline credentials only (previous proposal). | `specs/20261120-153502-opa-impersonation-authorization/contracts/impersonation-opa-config.yaml:121-195` |
| API-INPUT | API | Policy input: Envoy-compatible `attributes.request.http` (method, path, host and scheme from `server.enduser.public_url`, protocol, headers without `authorization`, `proxy-authorization`, `cookie`, pseudo-headers), `version`, `parsed_path`, `parsed_query`, Envoy-parsed form `parsed_body` with `client_assertion`/`actor_token`/`subject_token`/`client_secret`/`refresh_token`/`code`/`code_verifier` always omitted (revised 2026-11-25), `truncated_body: false`, constant `type: "impersonation"`, and CEL-equivalent `context` plus the broker-resolved `context.agent` object `{id, canonical_id?, external_id?}`, which reuses ExtProc's Grant Context agent shape (ADR `048-authzen-delegation-pip`) and omits unset IDs; no UUID fallback for governance lookups; ExtProc grant-context members (`principal`, `resource`, `grant`, `permission_sets`, `granted_permission_sets*`, `agent_session_id`, `mcp`) reserved and absent because OPA runs before delegation. Alternative: UUID-only governance maps, a flat `external_id` with `""` for absent (diverges from ExtProc), all headers, or a header allowlist. | `specs/20261120-153502-opa-impersonation-authorization/contracts/policy-input-decision.md:19-131` |
| API-DECISION | API | Decision `{"action":"allow"}` matches, `{"action":"deny"}` is a non-match; anything else and every evaluation failure maps internally to terminal `server_error` (domain text `authorization evaluation failed`; the token endpoint returns its fixed `internal server error during token exchange`, and an explicit deny with no later match returns `token exchange is not authorized`), with HTTP 500 for connected callers and no disconnected-client response guarantee; `POST /oauth2/token` description delta and 500 example; End-User API version `1.3.0` → `1.4.0`. A decision rule that exists but is undefined for a request is terminal (500); admission checks only that the rule exists (`GetRulesExact`); docs and examples require a `default` decision. Alternatives: undefined as deny (ExtProc behavior), or admission requiring a `default` rule (both rejected 2026-11-26). | `specs/20261120-153502-opa-impersonation-authorization/contracts/openapi-token-opa.yaml:1-85` |
| API-DIAG | API | New diagnostic details `client_authorization/opa_evaluation_failed` and `opa_decision_invalid`; span `tokenexchange.impersonation.opa.evaluate`; metrics `tokenexchange.impersonation.opa.revisions`, `.revision.age`, `.bundle.downloads`, and `.bundle.download.age` (the bundle metrics from an OPA bundle status listener, bounded `bundle` attribute); lifecycle events `oauth2_server.impersonation_policy.revision_admitted`/`revision_rejected`/`startup_failed`/`local_mode`/`bundle_download_failed`/`token_unavailable`; audit attributes `authorization_engine`, `policy_revision`, and `policy_admission_id` on the existing `impersonation.decision.recorded` event identifying the deciding immutable snapshot independently of empty/reused labels; request-correlated canceled evaluation telemetry without an HTTP response guarantee. Alternative: reuse `cel_evaluation_failed`. | `specs/20261120-153502-opa-impersonation-authorization/contracts/observability.md:1-119` |
| DEC-BUILTINS | Decision | Policy-time networking prohibition implemented as: every OPA built-in marked nondeterministic is unavailable except `time.now_ns` and `io.jwt.decode_verify` (revised 2026-11-26; removes `http.send`, `net.lookup_ip_addr`, `opa.runtime`, `rand.intn`, `uuid.rfc4122`, `io.jwt.encode_sign`, `io.jwt.encode_sign_raw`, `json.verify_schema`, `json.match_schema`); print disabled. Alternatives: deny only network built-ins; also remove `io.jwt.decode_verify`. | `specs/20261120-153502-opa-impersonation-authorization/research.md:155-177` |
| CONST-AMEND | Constitution | Applied (constitution v4.8.0, 2026-11-25, commit `fb84c31`; effective for OPA rules once ADR-OPA is Accepted): Amend Principle I's unsigned-subject sentence to: "It requires all ADR 031 compensating controls: explicit per-rule opt-in; signature-verified client assertion and actor; for rules authorized by a CEL predicate, a startup-verified authorization predicate reference to `subject_token` and an unverified email minted only when the predicate binds `subject_token.email`; fail-closed evaluation and extraction; and credential-free structured audit events. Rules authorized by an OPA policy under Accepted ADR `20261121-061324-opa-impersonation-authorization` have no static predicate-reference or email-binding check; the operator's policy is the only control on the caller-asserted subject and email." The exception's scope (subject role only, local-mode impersonation, explicit `verification: none`) is unchanged. Applied through `/speckit-constitution`. Alternative: keep Principle I unchanged and limit OPA rules to signed roles with an expected audience. | [ADR draft § Scope of the Unsigned-Subject Exception](adr-20261121-061324-opa-impersonation-authorization.md#scope-of-the-unsigned-subject-exception) |
