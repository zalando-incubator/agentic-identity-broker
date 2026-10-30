# Tasks: OPA-Based Authorization in ExtProc

**Input**: Design documents from `/specs/20260314-120100-extproc-opa-authorization/`
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓

**Tests**: Per Constitution Principle VIII (TDD), unit tests are written FIRST and verified to FAIL before implementation. E2E acceptance tests are written in Phase 2f (red phase) before any implementation begins.

**Organization**: Tasks grouped by user story to enable independent implementation and testing.

---

## Phase 1: Setup

**Purpose**: Add the new OPA SDK dependency and create the authorization package skeleton.

- [X] T001 Add OPA SDK dependency by running `go get github.com/open-policy-agent/opa/v1@latest` and verify go.mod/go.sum are updated
- [X] T002 Create the authorization package directory structure: `internal/extproc/authorization/` (empty — files added per user story)
- [X] T003 [P] Create Rego policy fixture directory `tests/e2e/extproc/fixtures/policies/` for test policy files

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Verify all design artifacts are complete and write E2E acceptance tests before implementation begins.

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete.

### Phase 2a: Domain Model & Glossary

- [X] T004 Update `ARCHITECTURE.md` Glossary with four new domain terms: OPAInput, OPADecision, Authorizer, ProtocolParser (as defined in specs/20260314-120100-extproc-opa-authorization/data-model.md)

**Checkpoint**: Domain terms documented in ARCHITECTURE.md

### Phase 2b: Configuration Design

- [X] T005 Verify `examples/config/extproc-opa-authorization.yaml` exists with both local-file and bundle-server examples matching `specs/20260314-120100-extproc-opa-authorization/contracts/configuration.md`
- [X] T006 [P] Update `examples/config/README.md` to add an entry referencing `extproc-opa-authorization.yaml`

**Checkpoint**: Configuration examples committed and README updated

### Phase 2c: API Design

- [X] T007 Confirm no REST/HTTP API changes (ExtProc is gRPC-only per ADR 20260223-120000; OPA input/output schema documented in `specs/20260314-120100-extproc-opa-authorization/contracts/opa-input-schema.md` — no OpenAPI update required)
- [X] T007a **BLOCKING T009/T010** Research and define the agentgateway ExtProc metadata key for protocol type: inspect agentgateway's ExtProc filter configuration (agentgateway config schema or source) to find the exact metadata namespace and key used to pass the protocol type (e.g., `agentgateway`/`protocol` or `filter`/`x-protocol`). Define the resolved key as a constant (e.g., `const agentgatewayProtocolMetadataKey = "..."`) in `internal/extproc/server/` before any E2E test files are written. Update the spec.md Assumption on line ~505 with the confirmed key name.

**Checkpoint**: Confirmed N/A, no OpenAPI changes needed; agentgateway metadata key documented

### Phase 2d: Database Design

- [X] T008 Confirm no database migrations needed (OPA authorization is entirely in-memory, ephemeral per-request; document this explicitly in PR description)

**Checkpoint**: Confirmed N/A, no migrations needed

### Phase 2e: Frontend/Design System Review

- [X] T008e [P] Confirm no frontend or design system changes (ExtProc is a gRPC-only binary; no UI components are added or modified in this feature — confirmed N/A per spec.md Out of Scope)

**Checkpoint**: Confirmed N/A, no frontend changes

### Phase 2f: E2E Acceptance Test Design

- [X] T009 Write in-process OPA E2E tests covering all US2–US5 acceptance scenarios in `tests/e2e/extproc/opa_authorization_test.go` (see plan.md scenario mapping for `It()` block descriptions)
- [X] T010 Write agentgateway integration E2E tests covering all US1 acceptance scenarios in `tests/e2e/extproc/opa_agentgateway_e2e_test.go` (Ordered Ginkgo suite following `agentgateway_e2e_test.go` pattern)
- [X] T011 [P] Create test Rego policy fixtures: `tests/e2e/extproc/fixtures/policies/allow_readonly.rego` (allows read-only tools, denies destructive), `deny_all.rego`, and `allow_all.rego`
- [X] T012 [P] Add `HaveImmediateResponseWithBody` custom Gomega matcher and extend `ProcessingRequestBuilder` with `WithRequestBody(body []byte)` in `tests/e2e/extproc/` test helpers
- [X] T012a [P] Verify new test files declare `package extproc_test` (matching the existing suite package) so they are automatically picked up by `tests/e2e/extproc/extproc_suite_test.go` — no separate suite runner file needed; update the suite file doc comment to reference OPA authorization tests
- [X] T013 Verify all 17 E2E scenarios fail (red phase): `cd tests/e2e/extproc && ginkgo -v --focus="OPA" ./...` — all `It()` blocks must FAIL before implementation (US4 Scenario 2 may be `Pending()` — acceptable)

**Checkpoint**: All E2E acceptance tests written and verified to FAIL

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Core types, config structs, and interface definitions that ALL user stories depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T014 Add `AuthorizationConfig` and `PolicyConfig` structs to `internal/extproc/config/config.go` and add `Authorization AuthorizationConfig` field to the top-level `Config` struct
- [X] T015 [P] Add authorization defaults to `internal/extproc/config/loader.go`: `authorization.enabled=false`, `policy.package=aib.extproc.authz`, `policy.decision=result`, `default_decision=deny`, `evaluation_timeout=100ms`, `max_body_size=1048576`
- [X] T016 [P] Write authorization validation unit tests (TDD — must FAIL first) in `internal/extproc/config/validate_test.go` covering: mutual exclusivity of path/config_file, enabled-but-no-source, path traversal rejection, invalid default_decision, non-positive timeout/body-size
- [X] T017 Add authorization validation rules to `internal/extproc/config/validate.go` following the 9 rules in `contracts/configuration.md` (mutual exclusivity, path existence, path traversal, default_decision values, positive timeout/body-size, non-empty package/decision when enabled)
- [X] T018 [P] Create `internal/extproc/authorization/input.go` with `OPAInput`, `MCPInput`, `RequestInput`, and `ContextInput` Go structs exactly matching the JSON schema in `data-model.md`
- [X] T019 [P] Create `internal/extproc/authorization/decision.go` with the `OPADecision` struct (`Action string`, `Reasons []string`) and `ParseDecision(result any) (*OPADecision, error)` function signature (implementation in Phase 3)
- [X] T020 [P] Define `Authorizer` interface in `internal/extproc/authorization/authorizer.go`: `Evaluate(ctx context.Context, input *OPAInput) (*OPADecision, error)` and `Stop(ctx context.Context)` (OPAAuthorizer impl in Phase 3)

**Checkpoint**: Config types compile, Authorizer interface defined, OPAInput/OPADecision types compile — user story implementation can begin

---

## Phase 3: User Story 1 — OPA Policy Evaluation for Tool Calls (Priority: P1) 🎯 MVP

**Goal**: The full OPA authorization pipeline works end-to-end: MCP tool call arrives → body buffered → OPA evaluates → allow proceeds to token exchange, deny returns 403.

**Independent Test**: Start ExtProc with `allow_readonly.rego`; send MCP `tools/call` for `list_repositories` (expect pass-through with exchanged token) and for `delete_repository` (expect 403 with reasons array). Run `ginkgo -v --focus="OPA Authorization via Agentgateway" tests/e2e/extproc/`.

### Tests for User Story 1 (TDD — write FIRST, verify FAIL)

- [X] T021 [P] [US1] Write unit tests for `OPAAuthorizer` in `internal/extproc/authorization/authorizer_test.go`: allow decision, deny decision with reasons, evaluation timeout (deny), undefined result (deny), OPA error (deny), graceful Stop
- [X] T022 [P] [US1] Write unit tests for `ParseDecision` in `internal/extproc/authorization/decision_test.go`: parse `{"action":"allow"}`, parse `{"action":"deny","reasons":["r1"]}`, parse `approval_required` (treated as deny), parse `ciba_required` (treated as deny), nil/empty result (deny)
- [X] T023 [P] [US1] Write unit tests for OPA-enabled server flow in `internal/extproc/server/server_test.go`: no Bearer token (passthrough), OPA disabled (direct token exchange), OPA enabled+allow (token exchange completes), OPA enabled+deny (403 ImmediateResponse with JSON body), mock Authorizer interface used throughout

### Implementation for User Story 1

- [X] T024 [US1] Implement `ParseDecision` in `internal/extproc/authorization/decision.go`: type-assert `sdk.DecisionResult.Result` to `map[string]any`, extract `action` and `reasons`, treat `approval_required`/`ciba_required` as deny
- [X] T025 [US1] Implement `OPAAuthorizer` struct and `NewOPAAuthorizer(cfg *config.AuthorizationConfig, logger *slog.Logger) (*OPAAuthorizer, error)` in `internal/extproc/authorization/authorizer.go` using `sdk.New()` — defer policy source initialization to T041 (US3) and T045 (US4)
- [X] T026 [US1] Add `authorizer Authorizer` field to `Server` struct and update `NewServer` signature to `NewServer(cfg, exchanger, authorizer, logger)` in `internal/extproc/server/server.go`
- [X] T027 [US1] Add `requestState` struct to `internal/extproc/server/server.go` tracking per-stream: `bearerToken string`, `resourceURI string`, `headers map[string]string`, `protocol string`
- [X] T028 [US1] Modify `processRequestHeaders` in `internal/extproc/server/server.go`: when OPA enabled and Bearer token present, extract and store state, return `HeadersResponse` with `ModeOverride.RequestBodyMode = BUFFERED`; for body-bearing requests, perform token exchange in the headers phase so the Authorization mutation is applied by the proxy
- [X] T029 [US1] Implement `processRequestBody` in `internal/extproc/server/server.go`: when OPA state present, call `authorizer.Evaluate()` with OPAInput, on allow echo the body unchanged (Authorization already mutated in the headers phase for body-bearing requests), and on deny return 403 `ImmediateResponse` with JSON `{"error":"access_denied","error_description":"...reasons..."}` body
- [X] T030 [US1] Add structured audit logging to `internal/extproc/authorization/authorizer.go`: log `tool_name`, `protocol`, `action`, `reasons`, `duration_ms` as structured fields per SR-004
- [X] T031 [US1] Wire `OPAAuthorizer` initialization and `defer authorizer.Stop(ctx)` in `cmd/extproc-token-exchange/root.go`; pass `nil` authorizer to `NewServer` when `Authorization.Enabled` is false

**Checkpoint**: US1 agentgateway E2E tests pass — `ginkgo -v --focus="OPA Authorization via Agentgateway" tests/e2e/extproc/`

---

## Phase 4: User Story 2 — Protocol-Aware Input Extraction (Priority: P2)

**Goal**: OPA policies can reference `input.mcp.tool_name`, `input.type`, and `input.mcp.method` directly. Input schema is correct for all MCP message types and the unknown-protocol fallback.

**Independent Test**: Invoke `BuildOPAInput("mcp", toolCallBody, headers)` and assert `type="mcp_tool_call"`, `mcp.tool_name="create_issue"`, `mcp.arguments.title="Bug"`. Run `ginkgo -v --focus="OPA Authorization" tests/e2e/extproc/opa_authorization_test.go`.

### Tests for User Story 2 (TDD — write FIRST, verify FAIL)

- [X] T032 [P] [US2] Write unit tests for MCP parser in `internal/extproc/authorization/parser_test.go`: `tools/call` extracts `tool_name`+`arguments`, non-tool-call method preserves `params`, `initialize` with params, malformed JSON returns error, empty body returns error, JSON array (batch) returns multiple messages per FR-023
- [X] T033 [P] [US2] Write unit tests for `BuildOPAInput` in `internal/extproc/authorization/input_builder_test.go`: `mcp_tool_call` type with correct mcp fields, `mcp_method` type for `initialize`, `unknown` type for unrecognized protocol, `Mcp-Session-Id` header populates `mcp.session_id`, envoy-compatible HTTP fields populated from headers

### Implementation for User Story 2

- [X] T034 [US2] Implement `ParseMCPMessage(body []byte) (*MCPMessage, error)` in `internal/extproc/authorization/parser.go` using `encoding/json` with mcp-go type definitions to deserialize JSON-RPC 2.0 envelope; extract `params.name` as `tool_name` and `params.arguments` for `tools/call`
- [X] T034a [US2] Implement JSON-RPC batch detection in `internal/extproc/authorization/parser.go` per FR-023: if the body is a JSON array, unmarshal as `[]json.RawMessage`, parse each element as a JSON-RPC message, and return a `ParseBatchResult` (slice of `*MCPMessage`). Integrate batch evaluation in `processRequestBody` in `internal/extproc/server/server.go`: evaluate each message independently; if any is denied, return 403 with combined reasons from all denying evaluations
- [X] T035 [US2] Implement `BuildOPAInput(protocol string, body []byte, headers map[string]string) (*OPAInput, error)` in `internal/extproc/authorization/input_builder.go`: dispatch on protocol, call `ParseMCPMessage` for `"mcp"`, set `type` discriminator, and populate `mcp.*` plus `context` on top of the envoy-compatible base document
- [X] T036 [US2] Handle `type="unknown"` in `internal/extproc/authorization/input_builder.go` for any unrecognized protocol value (including `"a2a"`): set `mcp=nil`, leaving the raw body available at `attributes.request.http.body`
- [X] T037 [US2] Extract `Mcp-Session-Id` header (case-insensitive) and populate `mcp.session_id` in `internal/extproc/authorization/input_builder.go` per FR-020
- [X] T038 [US2] Integrate `BuildOPAInput` call in `processRequestBody` in `internal/extproc/server/server.go` (replacing the placeholder from T029)
- [X] T039 [US2] Implement missing protocol metadata rejection in `internal/extproc/server/server.go`: when OPA enabled and `MetadataContext` lacks `agentgateway.protocol`, return 403 immediately and log warning per FR-003

**Checkpoint**: US2 in-process E2E scenarios pass in `tests/e2e/extproc/opa_authorization_test.go`

---

## Phase 5: User Story 3 — Local Rego File for Quick Setup (Priority: P3)

**Goal**: Operators can point ExtProc at a local `.rego` file; it compiles at startup, fails fast with clear error on invalid syntax or missing file.

**Independent Test**: Start ExtProc with `authorization.policy.path` set to a temp `.rego` file; verify startup succeeds and policy is applied. Set to a file with syntax errors; verify startup fails with clear message.

### Tests for User Story 3 (TDD — write FIRST, verify FAIL)

- [X] T040 [P] [US3] Extend unit tests in `internal/extproc/authorization/authorizer_test.go` for local file loading: valid `.rego` file succeeds, Rego syntax error causes `NewOPAAuthorizer` to return descriptive error, non-existent path causes error, path with `../` is rejected by validation

### Implementation for User Story 3

- [X] T041 [US3] Implement local Rego path mode in `NewOPAAuthorizer` in `internal/extproc/authorization/authorizer.go`: load `cfg.Policy.Path` directly and compile it with `rego.PreparedEvalQuery` at startup
- [X] T042 [US3] Implement startup failure propagation in `internal/extproc/authorization/authorizer.go`: `PrepareForEval` errors for invalid local Rego must surface as wrapped `NewOPAAuthorizer` errors with the file path and compilation message included
- [X] T043 [US3] Add path traversal validation (`../`) and path existence check to `internal/extproc/config/validate.go` authorization validation rules per SR-007

**Checkpoint**: US3 in-process E2E scenarios pass (local file load, syntax error fail, missing file fail)

---

## Phase 6: User Story 4 — OPA Configuration File for Bundle Pulling (Priority: P4)

**Goal**: Operators can provide an OPA config YAML file to enable bundle pulling, discovery, and other OPA-native policy management.

**Independent Test**: Start ExtProc with `authorization.policy.config_file` pointing to an OPA config with a `file://` bundle; verify OPA initializes and the policy is evaluated on requests.

### Tests for User Story 4 (TDD — write FIRST, verify FAIL)

- [X] T044 [P] [US4] Extend unit tests in `internal/extproc/authorization/authorizer_test.go` for OPA config file initialization: valid config file with local bundle succeeds, non-existent config file causes error, both `path` and `config_file` set is caught by config validation
- [X] T044a [P] [US4] Add E2E test `It("should follow OPA retry semantics when bundle server is unreachable")` in `tests/e2e/extproc/opa_authorization_test.go` for spec US4 Scenario 2: use an OPA config pointing to an unreachable server URL, verify `NewOPAAuthorizer` returns without error (OPA handles retries internally) and the server starts. Mark as `Pending()` with `// US4 Scenario 2 — requires live/mock bundle server; see plan.md` if full integration is not feasible in unit tests

### Implementation for User Story 4

- [X] T045 [US4] Implement OPA config file pass-through in `NewOPAAuthorizer` in `internal/extproc/authorization/authorizer.go` when `cfg.Policy.ConfigFile` is set: read the file, pass content via `sdk.Options.Config` reader to `sdk.New()`
- [X] T046 [US4] Add `config_file` existence validation to `internal/extproc/config/validate.go`: verify the file exists and is readable when `policy.config_file` is non-empty

**Checkpoint**: US4 scenarios pass — OPA config file initialization works with file-scheme bundle

---

## Phase 7: User Story 5 — Authorization is Optional (Priority: P5)

**Goal**: Existing token-exchange-only deployments are completely unaffected when `authorization.enabled` is false (or omitted). No body inspection, no overhead.

**Independent Test**: Start ExtProc with no `authorization` section; send a request with Bearer token; verify it proceeds to token exchange without body buffering mode being requested.

### Tests for User Story 5 (TDD — write FIRST, verify FAIL)

- [X] T047 [P] [US5] Extend unit tests in `internal/extproc/server/server_test.go` for the disabled path: `NewServer` with `nil` authorizer + `Authorization.Enabled=false` — RequestHeaders response must NOT set `ModeOverride`, token exchange must run in the headers phase as before

### Implementation for User Story 5

- [X] T048 [US5] Verify disabled-path guard in `processRequestHeaders` in `internal/extproc/server/server.go`: when `authorizer == nil` (or `cfg.Authorization.Enabled == false`), perform token exchange immediately in headers phase with no mode_override — zero behavioral change from pre-OPA behavior
- [X] T049 [US5] Verify `processRequestBody` in `internal/extproc/server/server.go` echos body unchanged when no OPA state is set (no requestState from headers phase) — protects against regression

**Checkpoint**: US5 E2E scenarios pass — OPA-disabled ExtProc is functionally identical to pre-feature behavior

---

## 🔒 Phase N: Constitution Compliance & Polish

**Purpose**: Verify all constitution requirements are met; create ADR; final quality pass.

### 🔒 Constitution Compliance Verification

#### Design Phase Verification

- [X] T050 Verify `ARCHITECTURE.md` Glossary contains all four new terms: OPAInput, OPADecision, Authorizer, ProtocolParser (Principle V)
- [X] T051 [P] Verify `examples/config/extproc-opa-authorization.yaml` exists and is correct (Principle VII)
- [X] T052 [P] Verify `examples/config/README.md` references `extproc-opa-authorization.yaml` (Principle VII)
- [X] T053 Confirm no OpenAPI changes needed — document in PR that ExtProc is gRPC-only (Principles IV, X — confirmed N/A)
- [X] T054 Confirm no database migrations — document in PR (Principle IX — confirmed N/A)
- [X] T055 Verify all 17 E2E acceptance tests in `tests/e2e/extproc/` have 1:1 mapping to spec scenarios from `specs/20260314-120100-extproc-opa-authorization/spec.md` (Principle XIII)
- [X] T056 Confirm E2E tests were verified to fail before implementation began (red phase documented) (Principle XIII)

#### Implementation Phase Verification

**Architecture & ADR** (Principle II):

- [X] T057 Create ADR `adrs/20260315-120000-opa-extproc-authorization.md` documenting the OPA SDK vs rego-package vs sidecar decision, fail-closed behavior, and the processing flow change (body buffering when OPA enabled)
- [X] T058 [P] Verify `ARCHITECTURE.md` is updated to reference the new `internal/extproc/authorization/` package and its role in the request pipeline

**Configuration** (Principle VII):

- [X] T059 [P] Verify `AuthorizationConfig` is loaded through the unified Viper configuration system in `internal/extproc/config/loader.go` (no ad-hoc loading)

**Security** (Principles I, III):

- [X] T060 Verify OPA authorization is disabled by default (`authorization.enabled: false`) and all enabled behavior is explicitly opt-in (SR-001)
- [X] T061 [P] Verify evaluation timeout is always enforced in `OPAAuthorizer.Evaluate()` and timeout causes deny (SR-003)
- [X] T062 [P] Verify body size handling in `processRequestBody` in `internal/extproc/server/server.go` (SR-006): bodies exceeding `max_body_size` MUST be rejected with 403 ImmediateResponse (truncation is explicitly prohibited — partial-body evaluation is a policy-evasion vector). Add unit test asserting 403 is returned when body size exceeds the configured limit.
- [X] T063 [P] Verify structured audit log entries in `internal/extproc/authorization/authorizer.go` include: `tool_name`, `protocol`, `action`, `reasons`, `duration_ms` (SR-004)

**Architecture Patterns** (Principle VI):

- [X] T064 [P] Verify `server.go` depends on the `Authorizer` interface (not `OPAAuthorizer` concrete type) — hexagonal pattern within ExtProc's scope

**Testing** (Principle VIII):

- [X] T065 Verify unit tests in `internal/extproc/authorization/*_test.go` were written FIRST and initially failed (red-green TDD)
- [X] T066 [P] Run full unit test suite: `just test` — all tests must pass with race detection enabled
- [X] T067 [P] Run code quality checks: `just check` — `fmt`, `vet`, `lint`, `test` must all pass

**E2E Acceptance Testing** (Principle XIII):

- [X] T068 Run full OPA E2E suite (green phase): `ginkgo -v --focus="OPA" tests/e2e/extproc/` — all 17 `It()` blocks must pass (US4 Scenario 2 may be `Pending()` with documented justification)
- [X] T069 [P] Verify each `It()` block in `opa_authorization_test.go` and `opa_agentgateway_e2e_test.go` has a comment referencing its spec scenario (e.g., `// US1 Scenario 2`)
- [X] T070 [P] Verify E2E tests use `Describe → Context → It` hierarchical structure per `tests/e2e/README.md`

### Additional Polish

- [X] T071 [P] Update `internal/extproc/AGENTS.md` with documentation of the new `authorization/` package, its interfaces, and the OPA processing flow
- [X] T072 [P] Verify performance: OPA evaluation adds <10ms p99 latency for the test Rego policies (SC-002) — benchmark with `go test -bench` or review evaluation duration logs from E2E runs

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Design Preconditions)**: Depends on Phase 1 — BLOCKS all implementation
  - 2a, 2b, 2c, 2d can run in parallel; 2f (E2E tests) can start alongside
  - **CRITICAL**: T013 (red phase verification) must complete before Phase 2.5 begins
- **Phase 2.5 (Foundation)**: Depends on ALL of Phase 2 — BLOCKS all user stories
  - T014 must complete before T015–T020 (config struct needed for defaults/validation)
  - T015, T016, T017, T018, T019, T020 can run in parallel after T014
- **User Stories (Phase 3–7)**: All depend on Phase 2 + Phase 2.5
  - US1 (Phase 3): Can start after Foundation complete
  - US2 (Phase 4): Can start after Foundation complete; T038–T039 depend on US1's server.go changes
  - US3 (Phase 5): Can start after Foundation complete; T041 extends US1's authorizer.go
  - US4 (Phase 6): Can start after Phase 5 (both extend authorizer.go)
  - US5 (Phase 7): Can run in parallel with US3/US4 (different concerns)
- **Phase N (Compliance)**: Depends on all desired user stories complete

### User Story Dependencies

- **US1 (P1)**: Independent after Foundation — implements full pipeline skeleton
- **US2 (P2)**: Depends on US1 server.go structure for `processRequestBody` integration
- **US3 (P3)**: Depends on US1's `OPAAuthorizer` skeleton in `authorizer.go`
- **US4 (P4)**: Depends on US3 (both extend `NewOPAAuthorizer`, sequential to avoid conflicts)
- **US5 (P5)**: Independent after Foundation — validates the nil/disabled path

### Parallel Opportunities

**Phase 2**: T004, T005, T006, T007, T008, T009–T013 can all run in parallel
**Phase 2.5**: T015–T020 can run in parallel after T014
**Phase 3 Tests**: T021, T022, T023 can run in parallel (different test files)
**Phase 4 Tests**: T032, T033 can run in parallel
**Phase N**: Most `[P]` verification tasks can run in parallel

---

## Parallel Example: Phase 3 (User Story 1)

```bash
# Run all US1 unit tests in parallel (different files, no conflicts):
Task: "Write OPAAuthorizer unit tests in internal/extproc/authorization/authorizer_test.go"  # T021
Task: "Write ParseDecision unit tests in internal/extproc/authorization/decision_test.go"    # T022
Task: "Write server OPA-enabled flow tests in internal/extproc/server/server_test.go"        # T023

# Then implement in parallel where possible:
Task: "Implement ParseDecision in decision.go"    # T024 — independent
Task: "Write server requestState struct"          # T027 — independent
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (add OPA dependency, create directory)
2. Complete Phase 2: Design Preconditions
   - 2a: Add ARCHITECTURE.md glossary terms (T004)
   - 2b: Verify/commit config example YAML (T005–T006)
   - 2c, 2d: Confirm N/A (T007–T008)
   - 2f: Write all E2E tests and verify red phase (T009–T013)
3. Complete Phase 2.5: Foundation (config types, input/decision types, Authorizer interface)
4. Complete Phase 3: US1 (full pipeline working with any inline test policy)
5. **STOP and VALIDATE**: agentgateway E2E tests for US1 pass
6. Deploy/demo with any Rego policy — core authorization is live

### Incremental Delivery

1. Foundation → US1 (MVP: works with tools/call and agentgateway)
2. US2 (input schema verified and correct for all MCP methods)
3. US3 (local Rego file — operators can configure without OPA infra)
4. US4 (OPA config file — production bundle server support)
5. US5 (backward compatibility verified — zero regression)

### Parallel Team / Agent Strategy

With multiple agents:
- **Agent A**: Phase 2 design tasks (T004–T008) in parallel
- **Agent B**: E2E test writing (T009–T013) in parallel with Agent A
- Once Phase 2 complete: **Agent A** takes Foundation (T014–T020), **Agent B** continues E2E polish
- Once Foundation complete: US1 through US5 can be worked by separate agents in priority order

---

## Notes

- `[P]` = task operates on a different file from other current tasks — safe to parallelize
- `[USN]` label maps task to acceptance scenario in `specs/20260314-120100-extproc-opa-authorization/spec.md`
- ADR 20260315-120000 (`adrs/20260315-120000-opa-extproc-authorization.md`) must be written in Phase N but can be drafted early
- The `authorization/authorizer.go` file is extended across US1 (interface + skeleton), US3 (local file), and US4 (config file) — these phases are intentionally sequential to avoid merge conflicts
- Body buffering via `mode_override` is only set when `OPA is enabled AND Bearer token is present` — zero overhead for non-authenticated requests even when OPA is configured
- `sdk.IsUndefinedErr(err)` must be used to distinguish "no rule matched" (default deny) from evaluation errors (also deny but different log message)

---

## Phase R: Reconcile E2E Tests with Eager-Exchange Architecture

**Context**: After implementing the eager-exchange-in-headers-phase approach, the E2E test helpers and one agentgateway E2E test were written against the old body-phase exchange model. These tasks reconcile the tests with the production behavior.

**Background**: Token exchange was moved to the headers phase for body-bearing requests. `requestBodyBufferingResponseWithAuth` now returns a `ProcessingResponse_RequestHeaders` carrying both the Authorization `SetHeaders` mutation and the `ModeOverride.RequestBodyMode = BUFFERED` instruction. The body phase runs OPA and echoes the body — no header mutations. `processHeadersOnlyOPA` (EOS=true path) is unchanged.

### Phase R.1: Permission Set Propagation (FR-024)

**Context**: The RFC 8693 token exchange response includes an optional `granted_permission_sets` field (map of permission set UUID → `[]string` of service UUIDs). This data must flow from the exchange response through the token cache and request state into the OPA input, alongside an explicit `context.granted_permission_sets_available` signal, so that policies can distinguish authoritative snapshots from broker omission or pre-exchange header-only evaluation.

- [X] T073a Add `GrantedPermissionSets map[string][]string \`json:"granted_permission_sets,omitempty"\`` to `tokenExchangeResponse` in `internal/extproc/server/exchanger.go` so the JSON decoder extracts it from the broker response
- [X] T073b Add `grantedPermissionSets map[string][]string` to `cachedToken` in `internal/extproc/server/exchanger.go` so cache hits return the same permission sets as the original live exchange
- [X] T073c Define `ExchangeResult` struct in `internal/extproc/server/server.go` (or `exchanger.go`): `type ExchangeResult struct { Token string; GrantedPermissionSets map[string][]string }`. Update the `Exchanger` interface method signature from `Exchange(ctx, subjectToken, resourceURI string) (string, error)` to `Exchange(ctx, subjectToken, resourceURI string) (ExchangeResult, error)`
- [X] T073d Update `TokenExchanger.Exchange()` implementation in `internal/extproc/server/exchanger.go`: populate `ExchangeResult.GrantedPermissionSets` from the parsed `tokenExchangeResponse`; store and retrieve `grantedPermissionSets` in `cachedToken` on cache miss/hit respectively
- [X] T073e Update all three `s.exchanger.Exchange()` call sites in `internal/extproc/server/server.go` to use `ExchangeResult`; extract `.Token` for the Authorization header; in `processRequestHeadersOPA` (the OPA eager-exchange path) store `result.GrantedPermissionSets` in `requestState`
- [X] T073f Add `grantedPermissionSets map[string][]string` field to `requestState` struct in `internal/extproc/server/server.go`; populate it from `ExchangeResult.GrantedPermissionSets` in the headers phase
- [X] T073g Extend `ContextInput` in `internal/extproc/authorization/input.go` with an explicit `GrantedPermissionSetsAvailable bool` field plus an `omitempty` `GrantedPermissionSets map[string][]string` field so OPA input can distinguish authoritative snapshots from unavailable context
- [X] T073h Update `BuildOPAInput` in `internal/extproc/authorization/input_builder.go` to accept `grantedPermissionSets map[string][]string`, set `context.granted_permission_sets_available` from authoritative-vs-unavailable input, and populate `context.granted_permission_sets` only when a snapshot is available; update the call site in `processRequestBody` in `internal/extproc/server/server.go` to pass `state.grantedPermissionSets`
- [X] T073i [P] Update unit tests in `internal/extproc/server/exchanger_test.go` and `exchanger_cache_test.go` for the new `ExchangeResult` return type and `grantedPermissionSets` caching; add a test case asserting cache hits return the same `GrantedPermissionSets` as the original exchange
- [X] T073j [P] Update unit tests in `internal/extproc/authorization/input_builder_test.go` asserting that non-empty snapshots populate `context.granted_permission_sets`, authoritative empty snapshots keep `context.granted_permission_sets_available=true`, and nil/header-only inputs omit the map while marking availability false
- [X] T073k [P] Update mock `Exchanger` implementations in `internal/extproc/server/server_test.go` for the `ExchangeResult` signature change

### Phase R.2: E2E Helper Reconciliation

- [X] T073 Update `ExtractMutatedAuthorizationHeader` in `tests/e2e/extproc/helpers/grpc_helpers.go`: the OPA path now puts the Authorization mutation in the headers-phase response (not the body-phase response); update the lookup to check the headers-phase response for OPA-enabled scenarios. The non-OPA path is unchanged.
- [X] T074 Update `SendHeadersAndBody` in `tests/e2e/extproc/helpers/grpc_helpers.go`: currently returns the body-phase response and only surfaces the headers-phase response when it is an `ImmediateResponse`. With the new architecture the headers-phase response for OPA+exchange carries an auth mutation (not an ImmediateResponse), so it must also be surfaced. Return both the headers-phase and body-phase responses so callers can inspect both; update `HaveReplacedAuthorizationHeader` to check the headers-phase response for the OPA path.
- [X] T075 Update the elicitation test in `tests/e2e/extproc/opa_agentgateway_e2e_test.go` (currently around line 321): `should return URLElicitationRequiredError from body phase with request id when OPA allows but exchange requires re-auth` — with eager exchange, re-auth happens in the headers phase before the body is read, so (a) the elicitation is in the headers-phase ExtProc response, and (b) the JSON-RPC id is `nil` (the body was not seen when exchange failed). Update the test description, assertions, and any `envelope.ID` checks accordingly.
- [X] T076 [P] Run full OPA E2E suite (green phase after reconciliation): `just extproc-test-e2e` — all It() blocks must pass. Confirm the agentgateway token-forwarding test `should allow read-only tool call and exchange token` (US1 Scenario 1) passes end-to-end with the exchanged token reaching the MCP server.
