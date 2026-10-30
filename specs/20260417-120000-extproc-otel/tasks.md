# Tasks: OpenTelemetry Support for ExtProc Token Exchange

**Input**: Design documents from `/specs/20260417-120000-extproc-otel/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, quickstart.md

**Tests**: Per Constitution Principle VIII (Test-Driven Development & Automated Testing), automated tests are MANDATORY for all features. Test tasks are included in each user story below and MUST be written before or alongside implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (ADR & Dependency Decision)

**Purpose**: Create the mandatory ADR for ExtProc → broker telemetry dependency crossing (plan.md Phase 1, constitution check PENDING item)

- [x] T001 Create ADR documenting `cmd/extproc-token-exchange/` importing `internal/ports` and `internal/adapters/telemetry` in `adrs/20260421-120000-extproc-telemetry-shared-dependency.md`
- [x] T002 [P] Update `ARCHITECTURE.md` to note ExtProc telemetry capability and the shared telemetry dependency

**Checkpoint**: ADR accepted, architecture documentation updated — implementation may proceed

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: Domain model, configuration, and E2E test design MUST all be complete before implementation

**⚠️ CRITICAL**: No code implementation can begin until this entire phase is complete

### Phase 2a: Domain Model & Glossary [MANDATORY]

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [x] T003 Confirm no new domain entities — pure infrastructure/observability enhancement (data-model.md confirms this)
- [x] T004 [P] Verify ARCHITECTURE.md Glossary does not need new domain terms (telemetry is infrastructure, not domain)

**Checkpoint**: Domain model confirmed — no changes needed

### Phase 2b: Configuration Design [MANDATORY]

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [x] T005 Create example YAML showing all telemetry config options with defaults in `examples/config/extproc-telemetry.yaml`
- [x] T006 [P] Update `examples/config/README.md` to reference new `extproc-telemetry.yaml` configuration example
- [x] T007 [P] Update `examples/config/extproc-token-exchange.yaml` to add a reference/pointer to the telemetry example
- [x] T007a [P] Update `docs/configuration.md` with ExtProc telemetry configuration section (Constitution Principle VII)

**Checkpoint**: Configuration requirements designed with YAML examples

### Phase 2c: API Design [MANDATORY]

**Constitution Reference**: Principles IV (API Documentation & OpenAPI Transparency), X (API-First Development)

- [x] T008 Confirm no API changes needed — no new HTTP/gRPC endpoints introduced (contracts/README.md confirms this)

**Checkpoint**: API design confirmed — no changes needed

### Phase 2d: Database Design [MANDATORY]

**Constitution Reference**: Principle IX (Persistence Pattern Consistency & Database Migration Management)

- [x] T009 Confirm no database changes needed — no new domain entities or persistence requirements

**Checkpoint**: Database design confirmed — no changes needed

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

**Constitution Reference**: Principle XI (Design System Compliance & Consistency)

- [x] T010 Confirm no frontend changes needed — pure backend infrastructure enhancement

**Checkpoint**: Frontend review confirmed — no changes needed

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)

- [x] T011 Write E2E acceptance tests in `tests/e2e/extproc/telemetry_test.go` for all 21 acceptance scenarios from spec.md, organized as Describe (feature) → Context (user story) → It (scenario)
- [x] T012 [P] Extend existing E2E bootstrap in `tests/e2e/extproc/bootstrap/bootstrap.go` with `tracetest.SpanRecorder` injection, `sdkmetric.ManualReader`, `sdklog.InMemoryExporter`, mock OTLP collectors (gRPC, HTTP, HTTPS), mock `Exchanger` hook, and unreachable-endpoint helpers
- [x] T013 Verify E2E tests FAIL semantically (red phase): detailed expectations present and failing, NOT placeholder always-fail assertions; no `XIt`/`PIt`/`Skip()` pending markers; no "red phase" comments in test files

**Checkpoint**: E2E acceptance tests written and verified to fail semantically before implementation

---

## Phase 2.5: Foundational Infrastructure (ExtProc Config & Mapping)

**Purpose**: ExtProc TelemetryConfig type, config loading, validation, and mapping function — MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T014 [P] Define `TelemetryConfig`, `TracesConfig`, `MetricsConfig`, `LogsConfig`, `OTLPExporterConfig` mirror types with defaults in `internal/extproc/config/config.go`
- [x] T015 [P] Add Viper bindings for `telemetry.*` keys and `EXTPROC_TELEMETRY_*` env var mappings in `internal/extproc/config/loader.go`
- [x] T016 [P] Extend `expandEnvVars()` in `internal/extproc/config/loader.go` to process `${ENV_VAR}` substitution in telemetry string fields (especially `telemetry.exporter.headers` map values per SR-002)
- [x] T017 [P] Add telemetry validation rules in `internal/extproc/config/validate.go`: endpoint required when enabled, protocol in allowed set (`grpc`/`http`/`https`), sampling rate 0.0–1.0, unrecognized propagators warned
- [x] T018 Add `mapTelemetryConfig(extprocconfig.TelemetryConfig) ports.TelemetryConfig` mapping function in `cmd/extproc-token-exchange/root.go`
- [x] T019 [P] Add unit tests for TelemetryConfig loading, defaults, env var override, and validation in `internal/extproc/config/loader_test.go`
- [x] T020 [P] Add unit tests for `mapTelemetryConfig` field-for-field parity with `ports.TelemetryConfig` in `cmd/extproc-token-exchange/root_test.go`

**Checkpoint**: Foundation ready — TelemetryConfig defined, loaded, validated, mapped. User story implementation can now begin.

---

## Phase 3: User Story 1 — End-to-End Distributed Tracing Through ExtProc (Priority: P1) 🎯 MVP

**Goal**: Enable end-to-end distributed tracing from agentgateway through ExtProc to Identity Broker, with per-request spans in `processRequestHeaders` and outbound trace injection via `otelhttp.NewTransport`.

**Independent Test**: Configure ExtProc with a valid OTLP endpoint, send a request through agentgateway with trace context headers, and verify that a trace span from ExtProc appears in the collector linked to the upstream agentgateway span via the same trace ID.

### Tests for User Story 1 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T021 [P] [US1] Write unit tests for span creation in `processRequestHeaders` in `internal/extproc/server/server_test.go` — verify `extproc.token_exchange` span with `resource.uri` and `outcome` attributes
- [x] T022 [P] [US1] Write unit tests for `otelhttp.NewTransport` wrapping and trace context injection in `internal/extproc/server/exchanger_test.go` — verify outbound `traceparent` header
- [x] T023 [P] [US1] Write unit tests for propagation-only round-tripper (traces.enabled=false) in `internal/extproc/server/exchanger_test.go` — verify context forwarding without span creation
- [x] T024 [US1] Verify US1 unit tests compile and FAIL semantically before implementation

### Implementation for User Story 1

- [x] T025 [US1] Add `context.Context` parameter to `Exchanger` interface `Exchange` method in `internal/extproc/server/server.go` — breaking change, update all callers and mock
- [x] T026 [US1] Add telemetry initialization in `cmd/extproc-token-exchange/root.go`: call `telemetry.NewProvider()` with mapped config after logger init, store shutdown function, add slog-to-OTel bridge via `otelslog.NewHandler`
- [x] T027 [US1] Implement per-request span creation in `processRequestHeaders` in `internal/extproc/server/server.go`: derive base context from `stream.Context()`, extract trace context from `HttpHeaders` entry list using `http.Header`-style multi-value carrier, start `extproc.token_exchange` span, set `resource.uri`/`outcome`/`error.type` attributes
- [x] T028 [US1] Wrap HTTP client transport with `otelhttp.NewTransport(base)` in `internal/extproc/server/exchanger.go` when `traces.enabled=true`; implement propagation-only round-tripper when `traces.enabled=false` (FR-003/FR-011)
- [x] T029 [US1] Update `doExchange` in `internal/extproc/server/exchanger.go` to use the passed `context.Context` instead of `context.Background()` for HTTP request context
- [x] T030 [US1] Update all existing exchanger tests and mock exchangers to include `context.Context` parameter in `internal/extproc/server/exchanger_test.go`, `internal/extproc/server/server_test.go`, and `tests/e2e/extproc/`

**Checkpoint**: US1 complete — end-to-end distributed tracing works. Spans created per request in processRequestHeaders, trace context injected on outbound HTTP calls. E2E scenarios US1-S1 through US1-S6 should pass.

---

## Phase 4: User Story 2 — Configurable Telemetry Without Code Changes (Priority: P1)

**Goal**: All OpenTelemetry settings configurable via YAML and environment variables using the same schema as the identity broker, with proper startup validation.

**Independent Test**: Start ExtProc with the same `telemetry:` YAML block used for the identity broker (only changing `service_name`), verify telemetry data appears, then restart with `telemetry.enabled: false` and verify no telemetry.

### Tests for User Story 2 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T031 [P] [US2] Write unit tests for schema parity between `extprocconfig.TelemetryConfig` and `ports.TelemetryConfig` in `cmd/extproc-token-exchange/root_test.go`
- [x] T032 [P] [US2] Write unit tests for startup validation error messages (invalid endpoint, unsupported protocol, out-of-range sampling rate) in `internal/extproc/config/loader_test.go`
- [x] T033 [US2] Verify US2 unit tests compile and FAIL semantically before implementation

### Implementation for User Story 2

- [x] T034 [US2] Add startup validation gate in `cmd/extproc-token-exchange/root.go`: validate telemetry config before gRPC server starts, fail immediately with clear error identifying invalid field (FR-009)
- [x] T035 [US2] Verify `EXTPROC_TELEMETRY_*` env var support works via Viper `AutomaticEnv()` + `EXTPROC_` prefix convention in `cmd/extproc-token-exchange/root.go`
- [x] T036 [US2] Add `insecure: true` security warning log at startup in `cmd/extproc-token-exchange/root.go` (SR-003)

**Checkpoint**: US2 complete — telemetry fully configurable via YAML and env vars with identical schema to identity broker. E2E scenarios US2-S1 through US2-S7 should pass.

---

## Phase 5: User Story 3 — Runtime Metrics for Operational Visibility (Priority: P2)

**Goal**: Expose request throughput counter, latency histogram, and Go runtime metrics when metrics are enabled.

**Independent Test**: Enable metrics, send requests, verify counter and histogram appear at OTLP endpoint broken down by outcome.

### Tests for User Story 3 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T037 [P] [US3] Write unit tests for `extproc.token_exchange.requests` counter and `extproc.token_exchange.duration` histogram recording with outcome breakdown in `internal/extproc/server/server_test.go`
- [x] T038 [US3] Verify US3 unit tests compile and FAIL semantically before implementation

### Implementation for User Story 3

- [x] T039 [US3] Create OTel instruments (Int64Counter for `extproc.token_exchange.requests`, Float64Histogram for `extproc.token_exchange.duration`) in `internal/extproc/server/server.go`
- [x] T040 [US3] Record counter increment and histogram observation with `outcome` attribute in `processRequestHeaders` in `internal/extproc/server/server.go` — outcomes: `success`, `exchange_failure`, `circuit_open`, `invalid_resource`, `assertion_expired`, `passthrough`
- [x] T041 [US3] Verify Go runtime metrics are automatically exported by `NewProvider()` when metrics enabled — no additional code needed (confirm via E2E test)

**Checkpoint**: US3 complete — metrics exported with outcome breakdown. E2E scenarios US3-S1 through US3-S3 should pass.

---

## Phase 6: User Story 4 — Graceful Degradation When Collector is Unavailable (Priority: P2)

**Goal**: Token exchange continues normally when telemetry collector is unreachable — observability failures never impact business availability.

**Independent Test**: Configure ExtProc with unreachable OTLP endpoint, send requests, verify all succeed.

### Tests for User Story 4 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T042 [US4] Verify E2E tests for US4-S1/S2/S3 in `tests/e2e/extproc/telemetry_test.go` are present and failing (these are primarily OTel SDK behavior verification — minimal implementation code expected)

### Implementation for User Story 4

- [x] T043 [US4] Verify OTel SDK non-blocking export behavior — no additional implementation code needed; E2E tests with unreachable endpoint and toggleable collector confirm behavior
- [x] T044 [US4] Implement graceful shutdown telemetry flush in `cmd/extproc-token-exchange/root.go`: after `GracefulStop()`, call telemetry shutdown with 5-second context deadline (FR-007), log flush errors without affecting exit code

**Checkpoint**: US4 complete — token exchange unaffected by collector outages. E2E scenarios US4-S1 through US4-S3 should pass.

---

## Phase 7: User Story 5 — Structured Log Correlation with Trace Context (Priority: P3)

**Goal**: Structured log entries carry active trace ID and span ID for log-to-trace correlation.

**Independent Test**: Enable telemetry with logs, trigger a log entry during a traced request, verify exported log record contains `trace_id` and `span_id` matching the active span.

### Tests for User Story 5 [MANDATORY - Principle VIII] ⚠️

> **Constitution Requirement (Principle VIII)**: Tests MUST be written FIRST using TDD. Ensure they FAIL before implementation begins.

- [x] T045 [P] [US5] Write unit test verifying slog handler includes trace_id/span_id in log records in `cmd/extproc-token-exchange/root_test.go`
- [x] T046 [US5] Verify US5 unit tests compile and FAIL semantically before implementation

### Implementation for User Story 5

- [x] T047 [US5] Wire slog-to-OTel bridge in `cmd/extproc-token-exchange/root.go`: when telemetry and logs both enabled, wrap logger with `otelslog.NewHandler` + `telemetry.NewMultiHandler` following the identity broker pattern in `builder.go` (FR-010)

**Checkpoint**: US5 complete — log entries carry trace_id/span_id. E2E scenarios US5-S1 and US5-S2 should pass.

---

## 🔒 Phase N: Constitution Compliance & Polish

**Purpose**: Verify constitution requirements and final polish

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [x] T048 Verify configuration design YAML examples exist in `examples/config/extproc-telemetry.yaml` (Principle VII)
- [x] T049 Verify configuration examples referenced in `examples/config/README.md` (Principle VII)
- [x] T050 Verify E2E acceptance tests written in `tests/e2e/extproc/telemetry_test.go` for all 21 spec scenarios (Principle XIII)
- [x] T051 Verify E2E tests verified to FAIL before implementation (red phase): detailed expectations written and failing; no placeholder always-fail assertions; no `XIt`/`PIt`/`Skip()` markers; no "red phase" comments (Principle XIII)

#### Implementation Phase Verification [MANDATORY]

**Architecture & Documentation** (Principle II):
- [x] T055 Verify `ARCHITECTURE.md` was updated with ExtProc telemetry capability description (done in T002)
- [x] T056 [P] Verify ADR `adrs/20260421-120000-extproc-telemetry-shared-dependency.md` is accepted and referenced

**Configuration** (Principle VII):
- [x] T057 [P] Verify telemetry config uses `internal/extproc/config` port (not custom loading)
- [x] T057b [P] Verify `docs/configuration.md` updated with ExtProc telemetry configuration section (Principle VII)

**Security** (Principles I, III):
- [x] T058 Verify TLS enabled by default for OTLP connections (SR-003), `insecure: true` logs warning
- [x] T059 [P] Verify no sensitive data in spans — token values never appear as span attributes (SR-001)
- [x] T060 [P] Verify no custom cryptography — only official OTel Go SDK used (Principle III)

**Architecture Patterns** (Principle VI):
- [x] T061 Verify hexagonal architecture maintained — `internal/extproc/config` free of `internal/ports` imports, mapping function only in `cmd/` layer

**Testing** (Principle VIII - Unit & Integration Tests):
- [x] T062 Verify unit tests written FIRST and failed before implementation (red-green TDD)
- [x] T063 Verify tests drive design — implementation emerges from test requirements

**E2E Acceptance Testing** (Principle XIII):
- [x] T064 Verify each `It()` block maps to exactly ONE acceptance scenario from spec.md
- [x] T065 Verify E2E tests turned GREEN as implementation satisfied acceptance criteria
- [x] T066 Verify E2E tests use Ginkgo/Gomega framework following `tests/e2e/README.md` patterns
- [x] T067 Run full E2E test suite: `ginkgo -v ./tests/e2e/extproc/` (all tests must pass)

### Additional Polish

- [x] T068 Run `just check` (fmt → vet → lint) and `just verify` — all must pass
- [x] T069 Run quickstart.md validation — verify end-to-end tracing guide works as documented
- [x] T070 Code cleanup and remove any TODO/FIXME markers introduced during development

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Design Preconditions (Phase 2)**: Depends on Phase 1 completion (ADR must be accepted) — BLOCKS all implementation
  - **CRITICAL**: Configuration requirements must be designed with YAML examples
  - **CRITICAL**: E2E acceptance tests must be written with detailed expectations and verified to FAIL semantically (red phase)
  - Phase 2a, 2b, 2c, 2d, 2e can proceed in parallel; Phase 2f depends on all of them
- **Foundational Infrastructure (Phase 2.5)**: Depends on ALL Phase 2 completion — BLOCKS all user stories
- **User Stories (Phases 3–7)**: All depend on Phase 2 + Phase 2.5 completion
  - US1 and US2 are both P1 but US2 depends on US1 (telemetry init + spans must exist before config parity testing)
  - US3 depends on US1 (metrics recorded in processRequestHeaders alongside spans)
  - US4 depends on US1 + US3 (tests verify request processing continues with unreachable endpoint)
  - US5 depends on US1 (log correlation requires active trace context from span creation)
- **Polish (Phase N)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2.5 — foundational tracing capability
- **User Story 2 (P1)**: Can start after US1 — config parity relies on working telemetry
- **User Story 3 (P2)**: Can start after US1 — metrics augment span creation in processRequestHeaders
- **User Story 4 (P2)**: Can start after US1 + US3 — degradation tests verify both traces and metrics continue
- **User Story 5 (P3)**: Can start after US1 — log correlation requires active spans

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Interface changes before implementation
- Core implementation before integration
- Story complete before moving to next priority

### Parallel Opportunities

- Phase 2 tasks T003–T010 can all run in parallel (different concerns, no file conflicts)
- Phase 2.5 tasks T014, T015, T016, T017 can run in parallel (different files in `internal/extproc/config/`)
- Within US1: T021, T022, T023 (test writing) can run in parallel (different test files)
- Within US3: T037 can start while US2 is still being implemented (test writing is independent)
- Phase N verification tasks marked [P] can run in parallel

---

## Parallel Example: User Story 1

```bash
# Launch all US1 tests together (parallel — different files):
Task: T021 "Unit tests for span creation in internal/extproc/server/server_test.go"
Task: T022 "Unit tests for otelhttp transport in internal/extproc/server/exchanger_test.go"
Task: T023 "Unit tests for propagation-only round-tripper in internal/extproc/server/exchanger_test.go"

# After tests fail (red), implement sequentially (shared files):
Task: T025 "Add context.Context to Exchanger interface"
Task: T027 "Span creation in processRequestHeaders"
Task: T028 "otelhttp.NewTransport wrapping"
Task: T029 "Update doExchange to use passed context"
Task: T030 "Update existing tests for context parameter"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (ADR creation)
2. Complete Phase 2: Design Preconditions (config examples + E2E test stubs)
3. Complete Phase 2.5: Foundational Infrastructure (TelemetryConfig type + loading + validation + mapping)
4. Complete Phase 3: User Story 1 (distributed tracing)
5. **STOP and VALIDATE**: Run E2E suite for US1 scenarios, verify spans in SpanRecorder
6. Deploy/demo if ready — end-to-end tracing is the core value

### Incremental Delivery

1. Setup → Design Preconditions → Foundational Infrastructure → Foundation ready
2. Add US1 (tracing) → Test independently → **MVP!**
3. Add US2 (config parity) → Test independently → Deploy/Demo
4. Add US3 (metrics) → Test independently → Deploy/Demo
5. Add US4 (degradation) → Test independently → Deploy/Demo
6. Add US5 (log correlation) → Test independently → Deploy/Demo
7. Each story adds observability value without breaking previous stories

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- No new domain entities, APIs, database migrations, or frontend changes — pure infrastructure/observability
- Total: 69 tasks across 9 phases (consolidated from 72 by removing duplicate Phase N verification tasks, added T007a and T057b)
