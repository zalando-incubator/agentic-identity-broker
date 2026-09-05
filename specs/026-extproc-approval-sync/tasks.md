---
description: "Task list for ExtProc approval cache sync and OPA integration"
---

# Tasks: ExtProc Approval Cache Sync & OPA Integration

**Input**: Design documents from `/specs/026-extproc-approval-sync/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `quickstart.md`, and `contracts/`

**Tests**: Constitution Principles VIII and XIII require test-first delivery. Write each listed test task first, make it compile and fail semantically, then implement the corresponding production task. No skipped Ginkgo tests or placeholder assertions.

**Organization**: The broker response extensions are foundational because every approval-dependent ExtProc path relies on broker-authoritative identity and decision time. User-story phases then deliver and independently verify each observable increment.

## Phase 0: Pre-implementation Refactoring

**Purpose**: Isolate the two behavior-preserving signature changes required by the feature before approval logic is introduced.

- [X] T001 [P] Add regression coverage for parameterized URL elicitation responses, including preservation of the existing re-auth response, in `internal/extproc/server/server_test.go`
- [X] T002 Refactor URL elicitation construction to accept an explicit URL, message, and JSON-RPC id without changing the re-auth path in `internal/extproc/server/server.go`
- [X] T003 [P] Update `BuildOPAInput` call-site regression coverage to pass `authorization.ContextInput` while preserving existing permission-set input in `internal/extproc/authorization/input_builder_test.go`
- [X] T004 Replace the trailing permission-set parameter with `authorization.ContextInput` and migrate all call sites in `internal/extproc/authorization/input_builder.go`
- [X] T005 Verify behavior-preserving refactors remain green with `just extproc-test` using `internal/extproc/server/server_test.go` and `internal/extproc/authorization/input_builder_test.go`

**Checkpoint**: The elicitation and OPA-input signatures are ready for feature work with no approval behavior enabled.

---

## Phase 1: Setup

**Purpose**: Confirm the existing standalone ExtProc module and runners already provide every dependency required by the plan.

- [X] T006 Confirm no dependency installation is needed for OPA, MCP elicitation, `otelhttp`, `net/http`, and `internal/toolpattern` in `go.mod` and `justfile`

---

## Phase 2: Design Preconditions

**Purpose**: Complete constitution-required domain, configuration, API, database, and acceptance-test design before feature implementation.

### Phase 2a: Domain Model & Glossary

- [X] T007 Reconcile Approval Cache, Long-Poll Syncer, Approval Identity, and Approval Gate invariants with `specs/026-extproc-approval-sync/data-model.md`
- [X] T008 Add the ExtProc approval package, approval flow, security boundaries, and glossary terms to `ARCHITECTURE.md`

### Phase 2b: Configuration Design

- [X] T009 Validate all defaults, environment names, flag precedence, and V1–V7 startup rules against `specs/026-extproc-approval-sync/contracts/extproc-approval-config.yaml`
- [X] T010 Add the documented enabled approval-gating example to `examples/config/extproc-tool-approvals.yaml`
- [X] T011 Reference the approval-gating example and its intended use in `examples/config/README.md`

### Phase 2c: API Design

- [X] T012 Merge the confirmed optional `principal`, `agent_id`, and `approved_at` contract fields into `api/enduser/openapi.yaml`

### Phase 2d: Database Design

- [X] T014 Record and verify that this feature adds no persistent entity or migration because it only projects existing broker values in `specs/026-extproc-approval-sync/data-model.md`

### Phase 2e: Frontend/Design System Review

**Not applicable**: This feature changes no React surface or broker consent UI; do not add frontend tasks.

### Phase 2f: E2E Acceptance Test Design

- [X] T015 Create synchronized broker-shaped fake controls for auth headers, ETags, long-poll sequences, failures, and endpoint call counts in `tests/e2e/extproc/bootstrap/broker.go`
- [X] T016 Create the `approval_required` OPA fixture with permission-set allow, deny, and approval-required paths in `tests/e2e/extproc/fixtures/policies/approval_required.rego`
- [X] T017 Write semantically failing Ginkgo `It()` coverage for every user-story scenario, every documented edge case, and SC-001 timing in `tests/e2e/extproc/approval_sync_test.go`, `tests/e2e/token_exchange_test.go`, and `tests/e2e/approval_api_test.go`; extend `tests/e2e/helpers/approval_types.go` only as needed for those tests to compile

**Checkpoint**: Contracts, confirmed OpenAPI changes, design invariants, test strategy, configuration example, and no-migration decision are explicit.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Deliver the shared broker and ExtProc prerequisites that every approval-gated user story depends on.

- [X] T018 [P] Add table-driven broker token-exchange response tests for canonical principal and agent identity omission/serialization in `internal/domain/tokenexchange/response_test.go`
- [X] T019 Implement optional `Principal` and `AgentID` response fields and populate them from the verified principal plus resolved canonical agent ID in `internal/domain/tokenexchange/response.go` and `internal/domain/tokenexchange/service.go`
- [X] T020 [P] Add table-driven approval-sync summary tests for projecting `ApprovedAt` only on approved records in `internal/adapters/http/handlers/approval/sync_handler_test.go`
- [X] T021 Project optional `approved_at` from `ToolApproval.ApprovedAt` in `internal/adapters/http/handlers/approval/sync_handler.go`
- [X] T023 [P] Add table-driven V1–V7 validation, env/flag precedence, and disabled-by-default tests in `internal/extproc/config/validate_test.go` and `internal/extproc/config/loader_test.go`
- [X] T024 Add `ToolApprovalsConfig` and `SessionsConfig`, defaults, Viper/Cobra bindings, URL normalization, and fail-fast validation in `internal/extproc/config/config.go`, `internal/extproc/config/loader.go`, and `internal/extproc/config/validate.go`
- [X] T025 [P] Add unit coverage for token-exchange identity propagation and client-assertion access failure in `internal/extproc/server/exchanger_test.go`
- [X] T026 Extend exchange response parsing and `ExchangeResult` with broker-authoritative identity, and expose the existing client assertion through a narrow accessor in `internal/extproc/server/exchanger.go`
- [X] T027 [P] Add unit coverage for action constants, preserved `ciba_required` deny behavior, undefined-action warnings, and configured agent-session input in `internal/extproc/authorization/decision_test.go`, `internal/extproc/authorization/authorizer_test.go`, and `internal/extproc/authorization/input_builder_test.go`
- [X] T028 Preserve `approval_required`, add named action constants and `approval_context`, and emit configured `context.agent_session_id` without moving `mcp.session_id` in `internal/extproc/authorization/decision.go`, `internal/extproc/authorization/authorizer.go`, `internal/extproc/authorization/input.go`, and `internal/extproc/authorization/input_builder.go`
- [X] T029 [P] Add race-safe cache tests for scope filtering, vector parity, precedence, missing `approved_at`, reservation, and idle/session eviction in `internal/extproc/approval/cache_test.go`
- [X] T030 Implement the RWMutex-backed pair cache, `toolpattern` matching, decision-time ranking, one-time reservation, and eviction rules in `internal/extproc/approval/cache.go`
- [X] T031 [P] Add HTTP contract tests for sync, targeted reads, create, consume, credentials, query parameters, and status taxonomy in `internal/extproc/approval/client_test.go`
- [X] T032 Implement the stateless broker approval client with endpoint-specific credentials, timeouts, and full-snapshot decoding in `internal/extproc/approval/client.go`

**Checkpoint**: Broker fields, configuration, OPA input/action semantics, verified identity propagation, cache semantics, and broker HTTP protocol are available without enabling a request-path approval gate.

---

## Phase 3: User Story 5 — Broker Publishes Approval-Scoping Identity and Decision Time (Priority: P1)

**Goal**: Make broker-authoritative approval identity and decision time observable to ExtProc without token parsing, storage changes, or placeholders.

**Independent Test**: Complete a valid exchange and inspect `principal` plus canonical `agent_id`; approve a record and inspect `approved_at`; prove pending and denied summaries omit it; then prove a later decision wins an equal-specificity match.

- [X] T036 [US5] Verify broker field contracts with `just test-e2e-backend` using `tests/e2e/token_exchange_test.go` and `tests/e2e/approval_api_test.go`

**Checkpoint**: Every approval-gated ExtProc request can safely obtain its cache key and ranking time from broker-authenticated responses.

---

## Phase 4: User Story 1 — Agent Retries Tool Call After User Approves (Priority: P1) 🎯 MVP

**Goal**: Return a real approval URL for a standalone approval-required call, then forward retries only when a matching approval authorizes the concrete invocation.

**Independent Test**: Trigger an approval-required `tools/call`, receive `-32042` with the broker URL, approve the record, and retry successfully through both targeted-read and cached permanent/session/once paths.

- [X] T037 [P] Add gate unit tests for successful and failed authoritative refresh, dual-auth creation, no fabricated elicitation, missing or invalid subject-token denial, stale-cache idempotent creation, confirmed once consumption, and SC-001 processing-time measurement excluding a controllable broker-client duration in `internal/extproc/approval/gate_test.go`
- [X] T039 [US1] Implement the gate sequence cache match → targeted read → rematch → create/elicit only after a successful confirmed miss, with hard denial on refresh or consume failure in `internal/extproc/approval/gate.go`
- [X] T040 [US1] Inject the optional approval gate into the ExtProc server and handle standalone approval-required calls with real JSON-RPC ids in `internal/extproc/server/server.go`
- [X] T041 [US1] Verify retry, permanent/session/once scope, consume, and no-identity behavior with `just extproc-test` and `just test-e2e-extproc` using `internal/extproc/approval/gate_test.go` and `tests/e2e/extproc/approval_sync_test.go`

**Checkpoint**: A user-approved call can resume safely, with approval records kept outside OPA and every unavailable or malformed authorization dependency failing closed.

---

## Phase 5: User Story 2 — No Spurious Approval When the Policy Denies (Priority: P1)

**Goal**: Ensure only `approval_required` reaches Tier 2; `allow`, `deny`, `ciba_required`, undefined, and batch behavior remain correct and fail closed.

**Independent Test**: Exercise allowed, denied, undefined, CIBA-required, and batched approval-required calls and inspect the HTTP outcome plus zero approval creates except for standalone approval-required calls.

- [X] T042 [P] [US2] Add server regression tests for allow forwarding, deny suppression, CIBA denial, undefined-decision warning, and batch deny-wins reasons in `internal/extproc/server/server_test.go`
- [X] T044 [US2] Restrict gate entry to standalone `ActionApprovalRequired` while preserving fail-closed deny responses and batch retry guidance in `internal/extproc/server/server.go`
- [X] T045 [US2] Verify no denied path sends `POST /api/approvals` with `just extproc-test` and `just test-e2e-extproc` using `internal/extproc/server/server_test.go` and `tests/e2e/extproc/approval_sync_test.go`

**Checkpoint**: Policy denial can never produce an unusable approval prompt, and deferred CIBA remains a visible fail-closed deny.

---

## Phase 6: User Story 3 — Approval Cache Bootstraps on ExtProc Startup (Priority: P1)

**Goal**: Pre-warm the approval cache at startup without blocking service availability on a broker outage.

**Independent Test**: Start ExtProc with a pre-seeded permanent approval and verify its first call creates no approval; repeat with an unreachable broker and verify startup warning plus hard denial when the authoritative targeted read remains unavailable.

- [X] T046 [P] Add bootstrap tests for seeded permanent approvals, initial ETag reuse, non-blocking startup with a 5-second bootstrap deadline, broker-unavailable startup, and unknown-pair augmentation in `internal/extproc/approval/syncer_test.go`
- [X] T048 [US3] Build the cache, broker client, approval gate, non-blocking bootstrap request, and ordered shutdown lifecycle from loaded config in `cmd/extproc-token-exchange/root.go`
- [X] T049 [US3] Verify bootstrap readiness, the 5-second bootstrap deadline, and hard denial when the request-time authoritative read is unavailable with `just extproc-test` and `just test-e2e-extproc` against `cmd/extproc-token-exchange/root.go` and `tests/e2e/extproc/approval_sync_test.go`

**Checkpoint**: Startup opportunistically pre-warms approvals, but an unavailable broker cannot stop the process or bypass a later authorization check.

---

## Phase 7: User Story 4 — Long-Poll Goroutine Keeps Cache Current (Priority: P2)

**Goal**: Keep one cache per instance current through ETag long-polling, full-snapshot replacement, live-session parameters, and bounded retry.

**Independent Test**: Observe immediate re-poll after 304, approval propagation within two seconds, ETag replay after a disconnect, and replacement that removes a revoked record after an unknown ETag response.

- [X] T050 [P] [US4] Extend syncer state-machine tests for ETag/304, full-snapshot replacement, active session parameters, retry backoff, and cancellation in `internal/extproc/approval/syncer_test.go`
- [X] T052 [US4] Implement the single-goroutine ETag long-poll loop, 1s–60s retry backoff, atomic snapshot replacement, and session eviction ticker in `internal/extproc/approval/syncer.go`
- [X] T053 [US4] Start and stop exactly one syncer around the ExtProc service lifecycle in `cmd/extproc-token-exchange/root.go`
- [X] T054 [US4] Verify cache currency, reconnection, and race safety with `just extproc-test` and `just test-e2e-extproc` using `internal/extproc/approval/syncer_test.go` and `tests/e2e/extproc/approval_sync_test.go`

**Checkpoint**: Each instance keeps its own cache fresh without merging stale state or retaining session-scoped approvals past their active lifetime.

---

## Phase 8: Constitution Compliance & Polish

**Purpose**: Complete public documentation, validate configuration and architecture commitments, and run the feature gates.

- [X] T055 Update the ExtProc approval flow and operator failure behavior in `docs/guides/token-exchange-gateway.md`, document both configuration sections, defaults, and source precedence in `docs/configuration.md`, and document the additive approval API fields with response examples in `docs/api/approval-endpoints.md`
- [X] T056 Reconcile the final approval package, data flow, glossary, security boundary, and documented standalone configuration boundary with `ARCHITECTURE.md`
- [X] T057 Verify `charts/agentic-identity-broker/templates/deployment.yaml` deploys no ExtProc workload and `charts/agentic-identity-broker/templates/configmap.yaml` receives no ExtProc settings; retain Helm N/A under Constitution Principle VII and ADR 011, which require ExtProc configuration through its independently owned deployment artifact
- [X] T058 Run configuration startup success and V1–V7 failure smoke cases from `specs/026-extproc-approval-sync/quickstart.md` against `cmd/extproc-token-exchange/root.go`
- [X] T059 Run static formatting, vetting, and linting with `just check` for all changed files under `internal/extproc/`, `internal/domain/tokenexchange/`, and `internal/adapters/http/handlers/approval/`
- [X] T060 Run race-enabled ExtProc unit coverage with `just extproc-test` for `internal/extproc/approval/`, `internal/extproc/authorization/`, `internal/extproc/config/`, and `internal/extproc/server/`
- [X] T061 Run all feature acceptance suites with `just test-e2e-extproc` for `tests/e2e/extproc/approval_sync_test.go`, plus `just test-e2e-backend` for `tests/e2e/token_exchange_test.go` and `tests/e2e/approval_api_test.go`
- [X] T062 Run the full repository verification gate with `just verify` after all changes listed in `specs/026-extproc-approval-sync/tasks.md` are complete

---

## Dependencies & Execution Order

```text
Phase 0 (signature-only refactors) ─┐
Phase 1 (dependency confirmation) ─┼─> Phase 2 (design preconditions) ─> Phase 2.5 (shared foundation)
                                   │                                           │
                                   └───────────────────────────────────────────┼─> US5 (broker contract validation)
                                                                               ├─> US1 (approval retry MVP) ─> US2 (deny suppression)
                                                                               ├─> US3 (startup bootstrap)
                                                                               └─> US4 (long-poll currency) ─> Phase 8
```

### User Story Dependencies

- **US5 (P1)**: Its production changes are deliberately in Phase 2.5 because every safe ExtProc cache key and precedence decision depends on them; its phase proves the externally observable broker contract.
- **US1 (P1)**: Depends on Phase 2.5. It is the MVP and can use targeted reads independently of live long-poll delivery.
- **US2 (P1)**: Depends on Phase 2.5 and the US1 server gate entry point; it verifies that all non-approval actions bypass that gate.
- **US3 (P1)**: Depends on Phase 2.5; it wires a usable cache/gate lifecycle and bootstrap behavior.
- **US4 (P2)**: Depends on Phase 2.5 and the US3 lifecycle; it completes continuous cache currency.

### Parallel Opportunities

- Phase 0 tasks T001/T003 can proceed in parallel; each modifies an independent test/source pair.
- Phase 2 tasks T007, T009, T012, T014, T015, and T016 have independent design or fixture files; T017 follows T015 and T016 and makes every acceptance assertion semantically red before production work.
- In Phase 2.5, test-first pairs T018/T020/T023/T025/T027/T029/T031 target independent packages; implementation may proceed after its corresponding test task is semantically red.
- After T017 is semantically red, US5 verification and the unit-test/production pairs in the user-story phases may proceed according to the stated dependencies.

### Parallel Example: Phase 2

```text
Task: "Create broker-shaped fake controls in tests/e2e/extproc/bootstrap/broker.go"
Task: "Create the approval-required policy fixture in tests/e2e/extproc/fixtures/policies/approval_required.rego"
```

T017 follows both tasks and adds the complete red E2E suite before production implementation.

## Implementation Strategy

### MVP First

1. Complete Phases 0–2.5, including semantically failing test tasks before their paired production tasks.
2. Complete US5 contract validation; this verifies the foundation's two security-critical broker projections.
3. Complete US1 through T041 and validate the standalone approval/retry loop. This is the smallest end-to-end customer value.
4. Stop and demonstrate the US1 independent test before progressing.

### Incremental Delivery

1. Add US2 to prove no policy deny creates approval work.
2. Add US3 to pre-warm the cache safely at service startup.
3. Add US4 to keep every per-instance cache current after startup.
4. Complete Phase 8 only after all story checkpoints pass.

### Format Validation

Every executable task above uses `- [ ]`, a unique stable `T###` ID, an optional `[P]` only for independent work, a `[US#]` label only in user-story phases, and one or more exact file paths.
