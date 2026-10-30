---
description: "Task list for Request Security Context Propagation"
---

# Tasks: Request Security Context Propagation

**Input**: Design documents from `/specs/20260629-120000-request-security-context/`

**Prerequisites**: [plan.md](./plan.md) (required), [spec.md](./spec.md) (required), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/request-security-context.md](./contracts/request-security-context.md), [quickstart.md](./quickstart.md), [/.specify/memory/constitution.md](../../.specify/memory/constitution.md)

**Tests**: Per Constitution Principles VIII and XIII, automated tests are MANDATORY. Red-phase E2E acceptance tests are written in Phase 2f before implementation. Unit tests are written before or alongside each story’s implementation and must fail semantically before the corresponding code is completed.

**Organization**: Tasks are grouped by user story so each story remains independently implementable and testable. Shared refactoring, design, and foundational work appears in earlier phases.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Task can run in parallel with other tasks in the same phase because it touches different files and has no dependency on incomplete work.
- **[Story]**: User story label for story-specific work only (`[US1]`–`[US4]`).
- Every task includes exact repository file paths.

## Path Conventions

This feature stays inside the Go backend monorepo. Relevant paths are rooted at repository top level: `internal/domain/`, `internal/adapters/`, `internal/app/`, `internal/ports/`, `internal/config/`, `internal/extproc/`, `tests/e2e/`, `charts/`, `examples/`, `docs/`, `adrs/`, and `specs/20260629-120000-request-security-context/`.

---

## Phase 0: Pre-implementation Refactoring [OPTIONAL BUT INCLUDED]

**Purpose**: Relocate `otelchi` registration into the shared `http.NewHandler` seam before feature logic lands so the later security-context middleware can read the authoritative span trace ID without duplicating router-level instrumentation.

- [X] T001 Extend `internal/adapters/http/server.go` `ServerConfig` with the server name and telemetry fields needed to register `otelchi` centrally in `http.NewHandler`
- [X] T002 Register `otelchi` in `internal/adapters/http/server.go` `http.NewHandler` after `RecoveryMiddleware` and before `OptionalPrincipalMiddleware`, gated by the same telemetry condition used today in the routing layer
- [X] T003 [P] Remove the per-router `otelchi` registration from `internal/adapters/http/routing/admin.go`
- [X] T004 [P] Remove the per-router `otelchi` registration from `internal/adapters/http/routing/enduser.go`
- [X] T005 Pass the server name and telemetry config into `internal/adapters/http/server.go` from `internal/app/builder.go`
- [X] T006 Verify the refactor in `internal/adapters/http/server.go`, `internal/adapters/http/routing/admin.go`, `internal/adapters/http/routing/enduser.go`, and `internal/app/builder.go` with `just test`

**Checkpoint**: `otelchi` lives only in `http.NewHandler`, both routers stop registering it, and existing tests stay green with no behavior change.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the dependency baseline before implementation.

- [X] T007 Verify required libraries already exist in `go.mod` and `go.sum` (`riandyrn/otelchi`, `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/trace`, `go.opentelemetry.io/contrib/bridges/otelslog`, `log/slog`) so no new dependency bootstrap is needed

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete the design, documentation, and red-phase test work that blocks implementation.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [X] T008 Confirm `SecurityContext`, `Actor`, `CallingPeer`, `TraceID`, and `SecurityContextCaptured` invariants in `specs/20260629-120000-request-security-context/data-model.md`, including delegated token-exchange semantics and the `anonymous` fallback
- [X] T009 [P] Add `SecurityContext`, `Actor`, `CallingPeer`, `anonymous`, and `Trace ID` to the glossary in `ARCHITECTURE.md`
- [X] T010 [P] Update `ARCHITECTURE.md` with the request-security-context middleware ordering, the `traceresponse` response-header contract, the token-exchange effective perimeter, and the security/performance NFRs from `specs/20260629-120000-request-security-context/spec.md`
- [X] T011 Create `adrs/20260703-120100-request-security-context-propagation.md` capturing the `http.NewHandler` seam, `SecurityContext.TraceID` authority, deferred token-exchange finalization, `CallingPeer` semantics, and two-tier recovery behavior

**Checkpoint**: The domain model, glossary, and architecture decision record all describe `CallingPeer`, deferred delegated finalization, and trace-id authority consistently.

### Phase 2b: Configuration Design [MANDATORY]

- [X] T012 [P] Create `examples/config/request-context.yaml` documenting the `request_context` block with secure defaults for trusted-proxy handling and `trace.response_enabled`
- [X] T013 [P] Reference `examples/config/request-context.yaml` from `examples/config/README.md`
- [X] T014 [P] Add `request_context` settings to `charts/agentic-identity-broker/values.yaml`, `charts/agentic-identity-broker/values.schema.json`, `charts/agentic-identity-broker/templates/configmap.yaml`, and `charts/agentic-identity-broker/README.md`
- [X] T015 [P] Document the `request_context` configuration block and deployment behavior in `docs/configuration.md`

**Checkpoint**: Configuration is designed in repo docs, examples, and Helm artifacts before code wiring begins.

### Phase 2c: API Design [MANDATORY]

- [X] T016 Confirm no endpoint-body changes are required in `api/enduser/openapi.yaml` or `api/admin/openapi.yaml`, add a note documenting the additive global `traceresponse` response header to the `info.description` of both `api/enduser/openapi.yaml` and `api/admin/openapi.yaml`, document the additive global `traceresponse` header contract in `ARCHITECTURE.md` and `specs/20260629-120000-request-security-context/contracts/request-security-context.md`, and record the already-obtained API-006 stakeholder confirmation in `adrs/20260703-120100-request-security-context-propagation.md` and `specs/20260629-120000-request-security-context/plan.md`

**Checkpoint**: API scope is explicitly limited to the additive response header, with confirmation recorded in repository artifacts.

### Phase 2d: Database Design [MANDATORY]

- [X] T018 Confirm in `specs/20260629-120000-request-security-context/data-model.md` and `specs/20260629-120000-request-security-context/plan.md` that this feature adds no schema changes, no new persisted entities, and no files under `migrations/`

**Checkpoint**: Persistence scope is explicitly documented as unchanged.

### Phase 2e: Frontend / Design System Review [N/A]

- [X] T019 Confirm in `specs/20260629-120000-request-security-context/spec.md` and `specs/20260629-120000-request-security-context/plan.md` that no files under `web/` or `tests/e2e/frontend/` are in scope for this backend-only feature

**Checkpoint**: Frontend work is explicitly out of scope.

### Phase 2f: E2E Acceptance Test Design (Red Phase) [MANDATORY]

- [X] T020 [P] Add the buffered log sink, server bootstrap hooks, and repository-boundary observer seam needed for correlation assertions in `tests/e2e/bootstrap/logger.go` and `tests/e2e/bootstrap/test_server.go`
- [X] T021 [P] Add the trusted-proxy-enabled config fixture and delegated token-exchange fixture helpers in `tests/e2e/fixtures/config.go` and `tests/e2e/helpers/jwt_helpers.go`
- [X] T022 Write HTTP acceptance tests in `tests/e2e/request_security_context_test.go` using `Describe -> Context -> It` structure and spec-scenario comment references, covering US1 S1–S5, US2 S1–S5, US3 S1–S4, the spoofed-forwarding-header edge case, and the trusted-proxy edge case, with one `It()` per scenario and explicit assertions for `trace_id`, `actor`, `calling_peer`, `client_ip`, repository-boundary propagation, and `traceresponse`
- [X] T023 Write gRPC parity tests in `tests/e2e/extproc/request_trace_context_test.go` using `Describe -> Context -> It` structure and spec-scenario comment references, covering US4 S1–S3 across both the OPA-disabled and OPA-enabled ExtProc entry points, with one `It()` per scenario and assertions for propagated/generated `trace_id`, `actor`, and optional `calling_peer`
- [X] T024 Verify the red phase semantically in `tests/e2e/request_security_context_test.go` and `tests/e2e/extproc/request_trace_context_test.go`: tests compile, fail for missing behavior, use no `XIt`/`PIt`/`Skip()`, and include no placeholder always-fail assertions or “red phase” comments

**Checkpoint**: Acceptance tests exist for every scenario, including US1 S5 delegated `CallingPeer`, and fail for the right reasons before implementation.

---

## Phase 2.5: Foundational Infrastructure [BLOCKING]

**Purpose**: Build the shared security-context primitives and config plumbing all stories depend on.

- [X] T025 Write unit tests in `internal/domain/security/context_test.go` for `WithSecurityContext` / `FromContext` round-trip, immutable value semantics, absent-context behavior, empty `CallingPeer` handling, and delegated token-exchange capture-holder semantics where transport metadata is captured once and the final `SecurityContext` is resolved once for downstream readers
- [X] T026 Implement `internal/domain/security/context.go` with the immutable `SecurityContext` value object (`TraceID`, `Actor`, `CallingPeer`, `ClientIP`, `UserAgent`, `RequestMethod`, `RequestTarget`, `ReceivedAt`), the `anonymous` sentinel, context accessors, and request-scoped capture-holder support for deferred delegated finalization without changing the downstream read API
- [X] T029 [P] Write unit tests in `internal/adapters/telemetry/context_handler_test.go` proving the handler stamps `trace_id`, `actor`, and `calling_peer` from `security.FromContext(ctx)`, and falls back to span trace data only when no `SecurityContext` is present
- [X] T030 Implement the context-enriching slog handler in `internal/adapters/telemetry/context_handler.go`
- [X] T031 Add `RequestContextConfig` to `internal/ports/config.go` and register secure defaults plus tests in `internal/config/loader.go` and `internal/config/loader_test.go`
- [X] T032 Add request-context validation plus tests in `internal/config/validator.go` and `internal/config/validator_test.go`
- [X] T033 Extend `internal/adapters/http/server.go` and `internal/app/builder.go` to carry `RequestContextConfig` into the HTTP server stack and wrap the logger with `internal/adapters/telemetry/context_handler.go`

**Checkpoint**: The core `SecurityContext` type, deferred-finalization mechanism, logging enrichment, and config plumbing exist before any user story code lands.

---

## Phase 3: User Story 1 - Implicit security-context capture and propagation (Priority: P1) 🎯 MVP

**Goal**: Capture a complete request security context at the perimeter, propagate it implicitly through business and persistence layers, and preserve a distinct `calling_peer` for delegated token-exchange flows.

**Independent Test**: Issue an authenticated HTTP request and a delegated token-exchange request, then assert from `tests/e2e/request_security_context_test.go` that `trace_id`, `actor`, `calling_peer`, `client_ip`, and `traceresponse` are established at the perimeter and remain visible to downstream service and repository seams without endpoint-specific metadata plumbing.

### Tests for User Story 1 [MANDATORY]

- [X] T034 [P] [US1] Write middleware unit tests in `internal/adapters/http/middleware/security_context_test.go` for immediate finalization on ordinary requests, `anonymous` fallback, omission of `CallingPeer` when absent, query-string stripping, user-agent truncation, principal-length sanitization, W3C `traceresponse` formatting, `trace.response_enabled=false` header suppression without disabling capture/log enrichment, and non-rejection behavior
- [X] T035 [P] [US1] Write table-driven client-IP tests in `internal/adapters/http/middleware/clientip_test.go` covering direct `RemoteAddr`, trusted-proxy right-most forwarded entry, spoofed left-most entry ignored, and malformed forwarded-header handling
- [X] T036 [P] [US1] Extend `internal/domain/tokenexchange/service_test.go` to assert the validated `subject_token` principal becomes `actor`, distinct `client_assertion.sub` becomes `calling_peer`, and equal identities collapse to an empty `calling_peer`
- [X] T037 [P] [US1] Extend `internal/adapters/http/enduser/oauth2_token_test.go` to assert delegated token-exchange requests preserve the final `SecurityContext` (`trace_id`, `actor`, `calling_peer`) through handler logging and downstream service calls

### Implementation for User Story 1

- [X] T038 [P] [US1] Implement the trusted-proxy-aware client-IP resolver in `internal/adapters/http/middleware/clientip.go`
- [X] T039 [US1] Implement `SecurityContextMiddleware` in `internal/adapters/http/middleware/security_context.go` to capture transport metadata plus Trace ID into the request-scoped holder, finalize immediately when actor identity is already available, set the `traceresponse` header, suppress only the header when `trace.response_enabled=false`, and never reject the request
- [X] T040 [US1] Register `SecurityContextMiddleware` after `OptionalPrincipalMiddleware` and move `LoggingMiddleware` inside it in `internal/adapters/http/server.go` so the final stack is `Recovery -> TraceContextNormalization -> otelchi -> OptionalPrincipal -> SecurityContext -> Logging -> ContextRecovery -> routes`
- [X] T041 [US1] Convert `LoggingMiddleware` in `internal/adapters/http/middleware.go` to use context-aware logging and read the finalized `SecurityContext` from the request-scoped holder so delegated `calling_peer` values appear on access logs
- [X] T042 [US1] Add `ContextRecoveryMiddleware` in `internal/adapters/http/middleware.go` and wire it in `internal/adapters/http/server.go`, with coverage in `internal/adapters/http/middleware_test.go`, so route-handler panics log with the finalized `trace_id`, `actor`, and `calling_peer` and recovered `500` responses still carry `traceresponse` while the outer recovery layer remains the bare perimeter safety net
- [X] T043 [US1] Source the logged `remote_addr` from `SecurityContext.ClientIP` in `internal/adapters/http/middleware.go` and `internal/adapters/http/middleware/oauth2_audit.go`
- [X] T044 [US1] Convert the OAuth2 audit sink in `internal/adapters/http/middleware/oauth2_audit.go` to context-aware logging so `trace_id`, `actor`, and `calling_peer` flow into security-relevant audit records
- [X] T045 [P] [US1] Convert the token-issued audit logs in `internal/adapters/http/enduser/token_grant_strategy.go` to context-aware logging that reads the finalized request `SecurityContext`
- [X] T046 [P] [US1] Convert the consent grant-created audit log in `internal/adapters/http/handlers/consent/grants_handler.go` to context-aware logging that reads the finalized request `SecurityContext`
- [X] T047 [US1] Finalize delegated identity resolution in `internal/domain/tokenexchange/service.go` by deriving `actor` from the validated `subject_token` principal, deriving `calling_peer` from distinct `client_assertion.sub`, and building the final `SecurityContext` once from the captured request metadata
- [X] T048 [US1] Thread the finalized delegated `SecurityContext` through `internal/adapters/http/enduser/oauth2_token.go` so downstream domain/repository calls and handler-side logs observe the same `trace_id`, `actor`, and `calling_peer`

**Checkpoint**: HTTP requests automatically carry `SecurityContext`; delegated token-exchange requests preserve a distinct `calling_peer`; downstream services, repositories, access logs, and audit logs all observe the same finalized context.

---

## Phase 4: User Story 2 - End-to-end trace correlation (Priority: P1)

**Goal**: Reuse inbound trace IDs when present, generate them when absent, and keep one authoritative Trace ID across context, logs, and `traceresponse` for the lifetime of each request.

**Independent Test**: In `tests/e2e/request_security_context_test.go`, send one request with an inbound trace identifier and one without, then assert reused versus generated `trace_id`, matching `traceresponse`, and no cross-request leakage under concurrency.

### Tests for User Story 2 [MANDATORY]

- [X] T049 [P] [US2] Extend `internal/adapters/http/middleware/security_context_test.go` with cases for inbound span trace reuse, no-span fallback generation, and identical `<trace-id>` values across `SecurityContext.TraceID` and the `traceresponse` header

### Implementation for User Story 2

- [X] T050 [US2] Add the no-span `crypto/rand` fallback path in `internal/adapters/http/middleware/security_context.go` to mint the authoritative 32-hex `TraceID`, a 16-hex child-id, and `00` flags when tracing is disabled or absent
- [X] T051 [US2] Verify inbound trace propagation and generated-trace behavior in `tests/e2e/request_security_context_test.go` for the `traceparent` reuse and “generate when absent” scenarios, including the multiple/duplicate inbound trace-identifier edge case where the first valid identifier wins and invalid sets fall back to generation
- [X] T052 [US2] Verify in `tests/e2e/request_security_context_test.go` that every log line for a request carries one isolated `trace_id`, including the concurrent-request isolation scenario

**Checkpoint**: Trace IDs are propagated or generated exactly once per request and stay consistent across context, logs, and `traceresponse`.

---

## Phase 5: User Story 3 - Graceful degradation to anonymous actor (Priority: P2)

**Goal**: Missing or malformed identity/transport metadata never causes rejection by the capture mechanism; the request continues with `actor=anonymous`, safe defaults, and an always-present Trace ID while access control remains unchanged.

**Independent Test**: In `tests/e2e/request_security_context_test.go`, verify public unauthenticated requests succeed with `actor=anonymous`, protected routes still return `401`, and no secrets appear in context-derived logs.

### Tests for User Story 3 [MANDATORY]

- [X] T053 [P] [US3] Extend `internal/adapters/http/middleware/security_context_test.go` for no-principal `anonymous` fallback, malformed or oversized actor handling, safe defaults for missing metadata, and fail-open behavior

### Implementation for User Story 3

- [X] T054 [US3] Complete the fail-open defaults in `internal/adapters/http/middleware/security_context.go` for missing identity, missing client IP, empty user agent, malformed forwarded headers, and oversized actor values
- [X] T055 [US3] Verify protected-route rejection versus public-route degradation in `tests/e2e/request_security_context_test.go` so `RequirePrincipalMiddleware` behavior remains unchanged while degraded requests still receive `traceresponse`
- [X] T056 [US3] Verify in `tests/e2e/request_security_context_test.go` and `internal/adapters/http/middleware/security_context.go` that no credential material from headers, cookies, or query parameters leaks into `SecurityContext` fields or logs

**Checkpoint**: Degraded requests stay observable and safe, but never weaken existing authentication or authorization outcomes.

---

## Phase 6: User Story 4 - gRPC perimeter parity (Priority: P3)

**Goal**: ExtProc requests get the same trace-correlation and actor-binding behavior as HTTP requests, with parity in meaning for `trace_id`, `actor`, and optional `calling_peer` across both OPA-disabled and OPA-enabled gRPC entry points.

**Independent Test**: In `tests/e2e/extproc/request_trace_context_test.go`, send ExtProc requests with and without inbound trace headers through both gRPC execution paths and assert propagated/generated `trace_id`, consistent `actor`, and omitted `calling_peer` when no distinct peer is available.

### Tests for User Story 4 [MANDATORY]

- [X] T057 [P] [US4] Extend `internal/extproc/server/server_test.go` for propagated and generated `trace_id`, `actor=anonymous`, omitted `calling_peer` when unavailable, and both `processRequestHeaders` / `processRequestHeadersOPA` / `processHeadersOnlyOPA` paths

### Implementation for User Story 4

- [X] T058 [US4] Add a shared per-request logger helper in `internal/extproc/server/server.go` that reuses extracted trace context when available, generates a fallback trace ID when absent, and stamps `trace_id`, `actor`, and optional `calling_peer`
- [X] T059 [US4] Apply the per-request logger helper in `internal/extproc/server/server.go` to `processRequestHeaders`, `processRequestHeadersOPA`, and `processHeadersOnlyOPA` so both OPA-disabled and OPA-enabled paths satisfy the gRPC perimeter contract

**Checkpoint**: ExtProc logs have HTTP-equivalent trace/actor semantics across both dispatch paths, with `calling_peer` emitted only when truly available.

---

## 🔒 Phase N: Constitution Compliance & Polish [MANDATORY]

**Purpose**: Verify the finished feature satisfies constitution, architecture, and verification requirements before handoff.

### Design Phase Verification

- [X] T060 Verify `ARCHITECTURE.md` glossary entries for `SecurityContext`, `Actor`, `CallingPeer`, `anonymous`, and `Trace ID` match `specs/20260629-120000-request-security-context/data-model.md`, and that `ARCHITECTURE.md` plus `adrs/20260703-120100-request-security-context-propagation.md` document the `traceresponse` header, middleware ordering, token-exchange effective perimeter, `CallingPeer` semantics, and the SR-001–SR-006 / SC-008 non-functional requirements
- [X] T061 [P] Verify `examples/config/request-context.yaml`, `examples/config/README.md`, `charts/agentic-identity-broker/values.yaml`, `charts/agentic-identity-broker/values.schema.json`, `charts/agentic-identity-broker/templates/configmap.yaml`, and `charts/agentic-identity-broker/README.md` all reflect the shipped `request_context` parameters
- [X] T064 Verify no files under `migrations/` changed for this in-process value-object feature, consistent with `specs/20260629-120000-request-security-context/data-model.md`
- [X] T065 Verify `tests/e2e/request_security_context_test.go` and `tests/e2e/extproc/request_trace_context_test.go` map 1:1 to all acceptance scenarios, including US1 S5 and US4 S3, with no skipped or pending tests

### Implementation Phase Verification

- [X] T066 Verify configuration wiring stays inside `internal/ports/config.go`, `internal/config/loader.go`, `internal/config/validator.go`, `internal/adapters/http/server.go`, and `internal/app/builder.go` with no ad-hoc config loading elsewhere
- [X] T067 Verify hexagonal boundaries across `internal/domain/security/context.go`, `internal/domain/tokenexchange/service.go`, `internal/adapters/http/middleware/security_context.go`, and `internal/extproc/server/server.go`
- [X] T068 Verify focused authentication/authorization regression coverage by running the relevant scenarios in `tests/e2e/request_security_context_test.go` alongside existing auth-sensitive suites under `tests/e2e/`
- [X] T069 Verify gRPC parity by running `tests/e2e/extproc/request_trace_context_test.go` and `internal/extproc/server/server_test.go`
- [X] T070 Run `just check` after updating `ARCHITECTURE.md`, `adrs/20260703-120100-request-security-context-propagation.md`, `internal/domain/security/context.go`, `internal/adapters/http/middleware/security_context.go`, `internal/extproc/server/server.go`, and the new E2E suites

### Additional Polish

- [X] T071 Run the validation scenarios in `specs/20260629-120000-request-security-context/quickstart.md`, including repository-boundary propagation, the delegated token-exchange scenario, the ExtProc V10 parity scenario, and the `/health` `traceresponse` smoke check
- [X] T072 Add `BenchmarkSecurityContextMiddleware` to `internal/adapters/http/middleware/security_context_test.go` to validate the SC-008 performance and allocation budget

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 0**: Starts immediately. Must finish before any `http.NewHandler` security-context work begins.
- **Phase 1**: Independent verification only.
- **Phase 2**: Blocks all implementation. Sub-phases 2a–2f may run in parallel, but all must complete before Phase 2.5.
- **Phase 2.5**: Depends on Phase 0 and all of Phase 2. Blocks all user stories.
- **Phase 3 (US1)**: Depends on Phase 2.5. Establishes the HTTP capture-and-propagation spine and the delegated token-exchange seam.
- **Phase 4 (US2)**: Depends on Phase 3 because it extends the same `SecurityContextMiddleware` trace logic.
- **Phase 5 (US3)**: Depends on Phase 3 because it extends the same middleware with degradation behavior.
- **Phase 6 (US4)**: Depends on Phase 2.5 but is otherwise independent of the HTTP story line because ExtProc is a separate binary.
- **Phase N**: Depends on all implemented stories.

### User Story Dependency Graph

- **US1** → **US2** → **US3**
- **US4** can run in parallel after Phase 2.5

### Within Each User Story

- Tests before implementation.
- Capture primitives before middleware registration.
- Middleware before handler/audit conversions.
- Token-exchange identity finalization before delegated-flow verification.

### Parallel Opportunities

- Phase 0: T003 and T004.
- Phase 2: T009–T015, T020–T023 where file paths do not overlap.
- Phase 2.5: T029 and T031 can proceed in parallel after T025/T026 settle the core domain package shape.
- US1: T034–T037 are parallel test tasks; T045 and T046 are parallel after T044.
- US4: T057 is independent from the HTTP-story line and can proceed while US2/US3 are in progress.

---

## Parallel Example: User Story 1

```bash
Task: "Write middleware unit tests in internal/adapters/http/middleware/security_context_test.go"   # T034
Task: "Write client IP tests in internal/adapters/http/middleware/clientip_test.go"                # T035
Task: "Write delegated identity tests in internal/domain/tokenexchange/service_test.go"            # T036
Task: "Write oauth2 token handler tests in internal/adapters/http/enduser/oauth2_token_test.go"   # T037
```

## Parallel Example: User Story 2

```bash
Task: "Extend trace reuse/fallback tests in internal/adapters/http/middleware/security_context_test.go"  # T049
Task: "Verify propagated traceparent scenarios in tests/e2e/request_security_context_test.go"            # T051
Task: "Verify concurrent trace isolation in tests/e2e/request_security_context_test.go"                  # T052
```

## Parallel Example: User Story 3

```bash
Task: "Extend anonymous/degradation tests in internal/adapters/http/middleware/security_context_test.go"  # T053
Task: "Verify protected-route rejection in tests/e2e/request_security_context_test.go"                    # T055
Task: "Verify no-secrets leakage in tests/e2e/request_security_context_test.go"                           # T056
```

## Parallel Example: User Story 4

```bash
Task: "Extend ExtProc per-request logger tests in internal/extproc/server/server_test.go"                # T057
Task: "Implement per-request trace logger helper in internal/extproc/server/server.go"                    # T058
Task: "Verify gRPC parity scenarios in tests/e2e/extproc/request_trace_context_test.go"                   # T069
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 0 to centralize `otelchi` in `internal/adapters/http/server.go`.
2. Complete Phase 1 dependency verification.
3. Complete all Phase 2 design preconditions, especially red-phase E2E work in `tests/e2e/request_security_context_test.go` and `tests/e2e/extproc/request_trace_context_test.go`.
4. Complete Phase 2.5 foundational work in `internal/domain/security/context.go`, `internal/adapters/telemetry/context_handler.go`, and config plumbing.
5. Complete Phase 3 to ship automatic perimeter capture plus delegated `CallingPeer` propagation.
6. Validate the MVP with the US1-focused scenarios in `tests/e2e/request_security_context_test.go`.

### Incremental Delivery

1. **Foundation**: Phase 0 → Phase 2 → Phase 2.5.
2. **MVP**: Ship US1 with HTTP capture, propagation, and delegated token-exchange `calling_peer`.
3. **Correlation**: Add US2 trace reuse and isolation.
4. **Degradation**: Add US3 fail-open anonymous handling.
5. **Parity**: Add US4 ExtProc parity.
6. **Polish**: Finish Phase N verification and benchmark work.

### Suggested MVP Scope

- **MVP** = **User Story 1 only** after Phases 0, 1, 2, and 2.5 are complete.
- US1 is the smallest release that establishes automatic security-context capture, downstream propagation, `traceresponse`, and the delegated `CallingPeer` audit trail.

---

## Notes

- `[P]` means the task can run in parallel because it touches different files and has no unresolved dependency on another open task.
- `[US#]` labels appear only on story-specific tasks for traceability.
- `CallingPeer` is part of the authoritative feature scope and must be threaded through the domain value object, logging handler, delegated token-exchange seam, and US1 S5 E2E scenario.
- ExtProc parity is behavioral, not shared-package reuse; `internal/extproc/server/` must not import the broker core packages.
- Use the red-phase E2E files as the acceptance gate for the whole feature.
