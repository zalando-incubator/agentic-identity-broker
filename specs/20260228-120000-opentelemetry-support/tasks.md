# Tasks: Configurable OpenTelemetry Support (017)

**Input**: Design documents from `/specs/20260228-120000-opentelemetry-support/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, quickstart.md ✅, contracts/ N/A (no new API endpoints)

**Tests**: Per Constitution Principle VIII (TDD), automated tests are MANDATORY. Test tasks are written FIRST within each user story and must fail (red phase) before implementation begins.

**Organization**: Tasks grouped by user story to enable independent implementation and testing. US3 appears before US1 in implementation order because US3 (provider initialization) is a hard dependency for US1 (HTTP instrumentation). Both are P1.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Install dependencies and create file structure before any implementation

- [x] T001 Add all OTel SDK and contrib dependencies to go.mod (otel v1.40.0, otel/sdk v1.40.0, otel/sdk/log v0.16.0, all OTLP exporters, contrib/instrumentation/runtime v0.65.0, contrib/bridges/otelslog v0.15.0, otelchi v0.12.2)
- [x] T002 [P] Create internal/adapters/telemetry/ package with empty stub files: provider.go, provider_noop.go, slog_handler.go (package declaration only)
- [x] T003 [P] Create adrs/20260301-120000-opentelemetry-provider-pattern.md documenting app-layer OTel provider pattern, otelchi library choice, and context-based span propagation approach

---

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites)

**Purpose**: All design documentation MUST be complete and verified before implementation begins
**⚠️ CRITICAL**: No code implementation proceeds until all sub-phases are confirmed complete

### Phase 2a: Domain Model & Glossary

**Constitution Reference**: Principles II (Architecture Documentation), V (Domain-Driven Design & Glossary Management)

- [x] T004 Update ARCHITECTURE.md Builder section to document OTel provider wiring pattern (NewProvider called in Build(), shutdown hooked into graceful shutdown, WithTracerProvider test override mirrors WithEncryption precedent)

**Checkpoint**: ARCHITECTURE.md reflects OTel provider wiring pattern

### Phase 2b: Configuration Design

**Constitution Reference**: Principle VII (Configuration-Driven Design)

- [x] T005 [P] Create examples/config/telemetry.yaml with complete production-ready YAML example covering enabled, service_name, resource_attributes, traces (sampling_rate, propagators), metrics (export_interval), exporter (protocol, endpoint, headers with ${ENV_VAR}, timeout, insecure)
- [x] T006 [P] Update examples/config/README.md to add telemetry.yaml reference under the Observability section (create section if absent)

**Checkpoint**: Configuration examples created and referenced

### Phase 2c: API Design

**Constitution Reference**: Principles IV (OpenAPI Transparency), X (API-First Development)

- [x] T007 Confirm no new HTTP API endpoints and no OpenAPI specification changes are required for this feature (purely internal instrumentation — document N/A confirmation inline as a comment in this file)
  <!-- N/A: OpenTelemetry is purely internal instrumentation. No new HTTP endpoints are exposed. /api/enduser/openapi.yaml and /api/admin/openapi.yaml are unchanged. -->

**Checkpoint**: API design N/A confirmed — no /api/*.yaml changes needed

### Phase 2d: Database Design

**Constitution Reference**: Principle IX (Persistence Pattern Consistency)

- [x] T008 Confirm no database schema changes and no new migration files are required (no new entities — document N/A confirmation inline)
  <!-- N/A: OpenTelemetry is purely infrastructure instrumentation. No new domain entities, no new tables, no new migrations. /migrations/ is unchanged. -->

**Checkpoint**: Database design N/A confirmed — no migrations added

### Phase 2e: Frontend / Design System Review

N/A — this feature is backend-only. No frontend components or design system changes required.

### Phase 2f: E2E Acceptance Test Design (Red Phase)

**Constitution Reference**: Principle XIII (End-to-End Acceptance Testing & Spec Traceability)

> **MANDATORY**: All E2E tests must be written BEFORE implementation and must FAIL before any implementation exists.

- [x] T009 [P] Create tests/e2e/bootstrap/telemetry.go with NewInMemoryTracerProvider() func returning (*sdktrace.TracerProvider, *tracetest.SpanRecorder) using sdktrace.WithSpanProcessor(recorder) and sdktrace.WithSampler(AlwaysSample())
- [x] T010 [P] Create tests/e2e/matchers/telemetry_matchers.go with three Gomega matchers: ContainSpanWithName(name string), ContainSpanWithAttribute(spanName, key, value string), HaveHTTPSpan(method, route string, statusCode int) operating on []sdktrace.ReadOnlySpan
- [x] T011 Write tests/e2e/telemetry_test.go with all 15 It() blocks mapped 1:1 to spec.md acceptance scenarios (US1-S1 through US1-S4, US2-S1 through US2-S3, US3-S1 through US3-S5, US4-S1 through US4-S3); use WithTracerProvider(tp) builder hook; add spec scenario comment references above each It() block
- [x] T012 Add minimal WithTracerProvider(tp *sdktrace.TracerProvider) *Builder stub method to internal/app/builder.go (stores tp in Builder struct field, no behaviour) to make tests/e2e/telemetry_test.go compile
- [x] T013 Run ginkgo -v ./tests/e2e/telemetry_test.go and confirm all 15 telemetry tests fail semantically (no spans emitted, no provider wired) — red phase confirmed before implementation proceeds

**Checkpoint**: All 15 E2E acceptance tests written, compile, and FAIL — red phase verified

---

## Phase 2.5: Foundational Infrastructure (Blocking All User Stories)

**Purpose**: Configuration schema, validation, and env var binding MUST be complete before any user story can be implemented

**⚠️ CRITICAL**: No user story work begins until this phase is complete

- [x] T014 Add TelemetryConfig, TracesConfig, MetricsConfig, OTLPExporterConfig Go structs with mapstructure tags, DefaultTelemetryConfig() constructor, and Telemetry TelemetryConfig field on the top-level Config struct in internal/ports/config.go
- [x] T015 Add telemetry setDefaults() calls (all 10 config keys with documented defaults) and BindEnv() for IDENTITY_BROKER_TELEMETRY_* env vars in internal/config/loader.go
- [x] T016 Write unit tests for all 7 validateTelemetryConfig() validation rules (skip-when-disabled, endpoint required, endpoint valid format, protocol valid, sampling rate range, export interval positive, exporter timeout positive) in internal/config/validator_test.go — TDD: write tests first, verify they fail
- [x] T017 Implement validateTelemetryConfig() function covering all 7 rules and integrate into the top-level Validate() call chain in internal/config/validator.go
- [x] T018 Add telemetry section to config.yaml with enabled: false (default) and full production example commented out

**Checkpoint**: Config schema complete, all 7 validation rules pass unit tests, env vars bind correctly

---

## Phase 3: User Story 3 — Configurable Telemetry Without Code Changes (Priority: P1)

**Goal**: OTel provider initializes correctly from TelemetryConfig; gRPC and HTTP exporters work; env vars override YAML; custom service name and resource attributes appear on all telemetry; disabled path has zero overhead.

**Implementation Note**: US3 is implemented before US1 (despite same P1 priority) because US3 provides the TracerProvider and MeterProvider that US1's HTTP instrumentation depends on.

**Independent Test**: Start app with different TelemetryConfig values (Enabled=false, gRPC endpoint, HTTP endpoint, custom service name) and verify correct provider initialization — unit tests in provider_test.go plus E2E US3-S1 through US3-S5.

### Tests for User Story 3 ⚠️

> **TDD**: Write tests FIRST — they MUST FAIL before provider implementation exists.

- [x] T019 [P] [US3] Write unit tests for NewProvider() covering: disabled fast-path returns noop, gRPC exporter path (mocked), HTTP exporter path (mocked), custom service name appears in resource, insecure=true emits warning log in internal/adapters/telemetry/provider_test.go

### Implementation for User Story 3

- [x] T020 [US3] Implement NewProvider(ctx, cfg TelemetryConfig, logger *slog.Logger) (shutdown func(context.Context) error, err error) skeleton with disabled fast-path (return noop when cfg.Enabled=false) in internal/adapters/telemetry/provider.go and implement NoopShutdown helper in internal/adapters/telemetry/provider_noop.go
- [x] T021 [US3] Implement OTel SDK resource.New() with semconv.ServiceName(cfg.ServiceName), additional cfg.ResourceAttributes key-value pairs, resource.WithTelemetrySDK(), resource.WithHost(), resource.WithProcess() in internal/adapters/telemetry/provider.go
- [x] T022 [US3] Implement OTLP gRPC exporter initialization for traces, metrics, and logs (otlptracegrpc, otlpmetricgrpc, otlploggrpc) with TLS by default (grpc.WithTransportCredentials when Insecure=false) and configurable endpoint/headers/timeout in internal/adapters/telemetry/provider.go
- [x] T023 [US3] Implement OTLP HTTP exporter initialization for traces, metrics, and logs (otlptracehttp, otlpmetrichttp, otlploghttp) with TLS by default and configurable endpoint/headers/timeout in internal/adapters/telemetry/provider.go
- [x] T024 [US3] Implement TracerProvider construction with ParentBased(TraceIDRatioBased(cfg.Traces.SamplingRate)) sampler, register configured propagators (tracecontext, baggage) via otel.SetTextMapPropagator, register globally via otel.SetTracerProvider in internal/adapters/telemetry/provider.go
- [x] T025 [US3] Implement MeterProvider construction with sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, WithInterval(cfg.Metrics.ExportInterval))) and register globally via otel.SetMeterProvider in internal/adapters/telemetry/provider.go
- [x] T026 [US3] Implement LoggerProvider construction with sdklog.NewBatchProcessor(logExporter) and register globally in internal/adapters/telemetry/provider.go
- [x] T027 [US3] Add insecure mode startup WARN log message when cfg.Exporter.Insecure=true, logged before returning the provider in internal/adapters/telemetry/provider.go
- [x] T028 [US3] Implement composite shutdown function that calls tp.Shutdown(ctx), mp.Shutdown(ctx), lp.Shutdown(ctx) in sequence in internal/adapters/telemetry/provider.go
- [x] T029 [US3] Wire NewProvider() call into internal/app/builder.go Build() method: call NewProvider(ctx, cfg.Telemetry, logger); if WithTracerProvider test override is set, register it as global provider instead; store shutdown func on the App struct

**Checkpoint**: US3 provider unit tests pass; app starts with gRPC config, HTTP config, and disabled config without errors

---

## Phase 4: User Story 1 — Enable Distributed Tracing for Request Debugging (Priority: P1)

**Goal**: Every inbound HTTP request on both servers emits a trace span with correct attributes; internal storage, encryption, JWKS, and upstream operations emit child spans; structured log entries include trace_id and span_id.

**Independent Test**: Use NewInMemoryTracerProvider() from bootstrap helper, inject via WithTracerProvider(), send a GET request to end-user server that triggers a storage query, verify SpanRecorder.Ended() contains the HTTP span (HaveHTTPSpan) and a storage child span (ContainSpanWithName("storage.get.agent")).

### Tests for User Story 1 ⚠️

> **TDD**: Write tests FIRST — they MUST FAIL before middleware and child span implementation exists.

- [x] T030 [P] [US1] Write integration test for postgres GetAgent method that sets up an in-memory SpanRecorder as global TracerProvider, calls GetAgent, and asserts ContainSpanWithName("storage.get.agent") with attribute db.system=postgresql in internal/adapters/storage/postgres/agent_repository_test.go

### Implementation for User Story 1

- [x] T031 [P] [US1] Register otelchi.Middleware("enduser", otelchi.WithTracerProvider(tp), otelchi.WithChiRoutes(r), otelchi.WithRequestMethodInSpanName(true)) conditionally when cfg.Telemetry.Enabled && cfg.Telemetry.Traces.Enabled in internal/adapters/http/routing/enduser.go
- [x] T032 [P] [US1] Register otelchi.Middleware("admin", otelchi.WithTracerProvider(tp), otelchi.WithChiRoutes(r), otelchi.WithRequestMethodInSpanName(true)) conditionally when cfg.Telemetry.Enabled && cfg.Telemetry.Traces.Enabled in internal/adapters/http/routing/admin.go
- [x] T033 [US1] Add child spans to key postgres storage methods: GetAgent ("storage.get.agent"), ListUserGrants ("storage.list.userGrants"), UpsertUserGrant ("storage.upsert.userGrant"), GetThirdPartyService ("storage.get.thirdPartyService") using otel.Tracer("storage").Start(ctx, spanName) with db.system=postgresql and db.operation attributes in internal/adapters/storage/postgres/*.go
- [x] T034 [P] [US1] Add child spans (encryption.encrypt, encryption.decrypt) to Encrypt() and Decrypt() methods using otel.Tracer("encryption").Start(ctx, spanName) with encryption.key_type=aws_kms attribute in internal/adapters/encryption/aws/*.go
- [x] T035 [P] [US1] Add child span (jwks.fetch) to Fetch() method using otel.Tracer("jwks").Start(ctx, "jwks.fetch") with url.full attribute in internal/adapters/jwks/adapter.go
- [x] T036 [P] [US1] Add child span (oauth2.token_exchange) to upstream token exchange HTTP calls using otel.Tracer("upstream").Start(ctx, "oauth2.token_exchange") with http.method and http.status_code attributes in internal/adapters/http/enduser/oauth2_token.go
- [x] T037 [US1] Implement NewMultiHandler(original slog.Handler, otelHandler slog.Handler) slog.Handler that fans records out to both handlers (tee pattern, ~20 lines) in internal/adapters/telemetry/slog_handler.go
- [x] T038 [US1] Wire slog multi-handler into logger creation in internal/app/builder.go: when cfg.Telemetry.Enabled=true, wrap base logger handler with NewMultiHandler(baseHandler, otelslog.NewHandler(cfg.Telemetry.ServiceName, otelslog.WithLoggerProvider(lp)))
- [x] T039 [US1] Update docs/configuration.md with new "Observability / OpenTelemetry" section covering all 13 config parameters, environment variable mapping, and quickstart example linking to examples/config/telemetry.yaml

**Checkpoint**: US1 E2E tests pass for contexts "when tracing is enabled" and "when telemetry is disabled"; storage integration test for span emission passes

---

## Phase 5: User Story 2 — Expose Runtime Metrics for Capacity Planning (Priority: P2)

**Goal**: When metrics are enabled, Go runtime metrics (goroutines, memory, GC) are exported automatically; HTTP request count, duration, and response size metrics are broken down by method/route/status (provided by otelchi via global MeterProvider from Phase 3).

**Independent Test**: Enable metrics with in-memory MeterProvider, generate requests, verify otelchi emits http.server.request.duration metric and runtime.Start() has been called (spot-check goroutine metric).

**Note**: HTTP metrics (request count, duration, response size) are emitted automatically by otelchi once a global MeterProvider is registered (done in Phase 3). This phase adds only the runtime.Start() call for Go runtime metrics.

### Implementation for User Story 2

- [x] T040 [US2] Start Go runtime metrics collection via runtime.Start(runtime.WithMinimumReadMemStatsInterval(15*time.Second)) in internal/adapters/telemetry/provider.go when cfg.Metrics.Enabled=true, after MeterProvider is registered globally
- [ ] T041 [P] [US2] Verify US2 E2E acceptance tests pass: run ginkgo -v ./tests/e2e/ -focus "when metrics are enabled" and confirm US2-S1 (HTTP metrics), US2-S2 (runtime metrics), US2-S3 (metrics disabled) all green — US2-S1 and US2-S2 currently skip (MeterProvider recorder not wired into E2E bootstrap)

**Checkpoint**: US2 E2E tests green; runtime.Start() called only when metrics enabled

---

## Phase 6: User Story 4 — Graceful Degradation When Collector is Unavailable (Priority: P2)

**Goal**: Application starts and serves requests normally with an unreachable OTLP endpoint; pending telemetry is flushed during shutdown; log warnings are emitted without flooding.

**Independent Test**: Configure unreachable OTLP endpoint (localhost:9999), start app, send requests, verify 200 responses with normal latency and no panics; verify startup log contains WARN about unreachable collector.

**Note**: The OTel SDK handles buffering, retry, and automatic reconnection internally. This phase wires the shutdown flushing into the existing graceful shutdown sequence.

### Implementation for User Story 4

- [x] T042 [US4] Add ShutdownTelemetry func(context.Context) error field to the App or builder result struct in internal/app/builder.go, assigned the composite shutdown function returned by NewProvider() — completed as part of T029
- [x] T043 [US4] Integrate ShutdownTelemetry(ctx) into the graceful shutdown sequence in the server adapter (internal/adapters/http/ or the errgroup shutdown path), called after HTTP servers have drained and before process exit — satisfies FR-009
- [x] T044 [P] [US4] Verify US4 E2E acceptance tests pass: run ginkgo -v ./tests/e2e/ -focus "when the OTLP collector is unreachable" and confirm US4-S1 (startup success) and US4-S2 (continued serving) both green

**Checkpoint**: US4 E2E tests green; app starts and serves normally with unreachable OTLP endpoint

---

## 🔒 Phase N: Constitution Compliance & Polish

**Purpose**: Verify all constitution principles have been followed and complete final quality checks
**🔒 CONSTITUTION REQUIREMENT**: This entire section MUST be completed before the feature is merged

### Design Phase Verification

**Constitution Reference**: PRECONDITIONS checklist — verify Phase 2 tasks completed correctly

- [x] T045 Verify ARCHITECTURE.md Builder section documents OTel provider wiring pattern (T004 — Principle II, V)
- [x] T046 Verify examples/config/telemetry.yaml exists and examples/config/README.md references it (T005, T006 — Principle VII)
- [x] T047 Verify no new HTTP API endpoints were added and /api/*.yaml files are unchanged (T007 — Principles IV, X)
- [x] T048 Verify no database migrations were added and /migrations/ is unchanged (T008 — Principle IX)
- [x] T049 Verify tests/e2e/telemetry_test.go contains exactly 15 It() blocks each mapped to one spec.md acceptance scenario (T011 — Principle XIII)
- [x] T050 Verify E2E tests were run and confirmed failing before any implementation began (T013 red phase — Principle XIII)

### Implementation Phase Verification

**Constitution Reference**: Implementation Phase checklist — verify all principles followed during implementation

**API & Documentation** (Principles IV, X):
- [x] T051 [P] Verify no API contract changes were made (no edits to /api/enduser/openapi.yaml or /api/admin/openapi.yaml)
- [x] T052 Verify docs/configuration.md Observability/OpenTelemetry section is present and covers all 13 config parameters (T039)

**Architecture & Documentation** (Principle II):
- [x] T053 Verify ARCHITECTURE.md documents OTel provider wiring in the Builder section and no new domain glossary terms were missed
- [x] T054 [P] Verify adrs/20260301-120000-opentelemetry-provider-pattern.md exists and documents: app-layer OTel pattern, otelchi choice, context-based propagation decision

**Configuration** (Principle VII):
- [x] T055 [P] Verify all telemetry configuration flows through internal/ports/config.go TelemetryConfig — no ad-hoc config loading in adapter files

**Database & Persistence** (Principle IX):
- [x] T056 [P] Verify no new migrations were added to /migrations/ (N/A for this feature — confirm no files changed)

**Security** (Principles I, III):
- [x] T057 Verify OTel exporter TLS is enabled by default (Insecure defaults to false in DefaultTelemetryConfig — Principle I)
- [x] T058 [P] Review all new span attribute additions in T033-T036 against SR-001 prohibited list (no tokens, encryption keys, PII, credentials, request body content) — code review gate
- [x] T059 [P] Verify no custom cryptography — only OTel Go SDK, otelslog bridge, otelchi used (Principle III)

**Architecture Patterns** (Principle VI):
- [x] T060 Verify no port interface for telemetry was added to internal/ports/ (OTel provider wired in app-layer only per spec clarification and ADR-20260301-120000)

**Testing** (Principle VIII — TDD):
- [x] T061 Verify provider unit tests in internal/adapters/telemetry/provider_test.go were written before implementation and initially failed (T019 red phase — Principle VIII)
- [x] T062 Verify validateTelemetryConfig() unit tests were written before implementation and initially failed (T016 red phase — Principle VIII)
- [x] T063 Verify storage integration test for span emission was written before child span implementation and initially failed (T030 red phase — Principle VIII)

**E2E Acceptance Testing** (Principle XIII):
- [x] T064 Verify all 15 It() blocks in tests/e2e/telemetry_test.go include comment references to spec.md scenario IDs (e.g., // US1-S1)
- [x] T065 Verify E2E tests use Ginkgo/Gomega hierarchical structure: Describe("OpenTelemetry Instrumentation") → Context("when tracing is enabled") → It("should emit...")
- [x] T066 Run full E2E test suite: ginkgo -v ./tests/e2e/ — all 15 telemetry acceptance tests MUST pass

### Polish

- [x] T067 Run `just check` (fmt → vet → lint) and `just verify`, and fix all issues — both must pass
- [x] T068 [P] Verify config.yaml telemetry section uses enabled: false default with full commented example to guide operators

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Design Preconditions)**: Depends on Phase 1 — BLOCKS all implementation
  - Sub-phases 2a, 2b, 2c, 2d, 2f can proceed in parallel
  - **CRITICAL**: Phase 2f E2E tests (T011-T013) must complete before Phase 2.5 begins
- **Phase 2.5 (Foundational)**: Depends on ALL of Phase 2 — BLOCKS all user stories
- **Phase 3 (US3)**: Depends on Phase 2 + Phase 2.5 — BLOCKS Phase 4 (US1) and Phase 5 (US2)
- **Phase 4 (US1)**: Depends on Phase 3 provider being wired into builder
- **Phase 5 (US2)**: Depends on Phase 3 MeterProvider registration (T025)
- **Phase 6 (US4)**: Depends on Phase 3 shutdown function (T028) and builder wiring (T029)
- **Phase N (Compliance)**: Depends on all user stories complete

### User Story Dependencies

```
Phase 2.5 (Foundational)
    └─> Phase 3 (US3 — provider setup)
            ├─> Phase 4 (US1 — HTTP middleware + child spans)
            ├─> Phase 5 (US2 — runtime metrics, HTTP metrics from otelchi)
            └─> Phase 6 (US4 — shutdown integration)
```

- **US3 before US1**: US1 otelchi middleware requires an initialized global TracerProvider (set in US3 T024)
- **US3 before US2**: US2 runtime metrics require an initialized global MeterProvider (set in US3 T025)
- **US4 after US3**: Shutdown function is created in US3 (T028); US4 wires it into graceful shutdown

### Within Each User Story

- Tests (T019, T030) MUST be written and verified failing before implementation
- Provider foundation (T020-T021) before exporter-specific code (T022-T023)
- Exporter code (T022-T023) can proceed in parallel
- TracerProvider before MeterProvider before LoggerProvider (shared resource)
- Provider wired in builder (T029) before middleware registration (T031-T032)
- Middleware registered before child spans added (T033-T036)

### Parallel Opportunities

Phase 1: T002 and T003 can run in parallel
Phase 2: Sub-phases 2b, 2c, 2d can run in parallel; T009 and T010 can run in parallel
Phase 3: T022 (gRPC exporter) and T023 (HTTP exporter) can run in parallel
Phase 4: T031 (integration test) can run in parallel with T032 (enduser middleware) and T033 (admin middleware); T034, T035, T036, T037 (child spans) can run in parallel
Phase 5: T041 (verify US2 tests) can run in parallel after T040

---

## Parallel Example: User Story 3 (Provider Setup)

```bash
# After T020 (disabled fast-path done), launch exporter implementations in parallel:
Task: "Implement OTLP gRPC exporter in internal/adapters/telemetry/provider.go (T022)"
Task: "Implement OTLP HTTP exporter in internal/adapters/telemetry/provider.go (T023)"
# Then: T024 (TracerProvider), T025 (MeterProvider), T026 (LoggerProvider) sequentially
```

## Parallel Example: User Story 1 (Instrumentation)

```bash
# After T031 (integration test) and T032 (enduser middleware) written:
Task: "Add child spans to encryption adapter (T034)"
Task: "Add child span to JWKS adapter (T035)"
Task: "Add child span to upstream adapter (T036)"
```

---

## Implementation Strategy

### MVP: Phase 1 + Phase 2 + Phase 2.5 + Phase 3 (US3) Only

1. Complete Phase 1: Install dependencies, create file stubs, write ADR
2. Complete Phase 2: Verify all design preconditions, write and verify E2E tests fail
3. Complete Phase 2.5: Config schema, validation, env var binding
4. Complete Phase 3 (US3): Full provider initialization for all protocols
5. **STOP and VALIDATE**: Run unit tests, verify app starts with telemetry config, verify E2E US3 scenarios pass
6. Deploy/demo: operators can configure telemetry endpoint and verify provider initializes — traces will not yet have HTTP spans or child spans, but the SDK is active

### Incremental Delivery

1. MVP (US3): Provider initialization + configuration — operators can connect a collector
2. Add US1: HTTP instrumentation + child spans + log correlation — full distributed tracing
3. Add US2: Runtime metrics activation — capacity planning dashboards available
4. Add US4: Shutdown flushing + verify graceful degradation — production-hardened
5. Each phase adds observable value without breaking previous phases

### Parallel Team Strategy

With 2+ developers or agents after Phase 2 + Phase 2.5 are complete:

- **Agent 1**: Phase 3 (US3) — provider initialization (blocking dependency for all others)
- After Phase 3: **Agent 1** takes Phase 4 (US1); **Agent 2** takes Phase 5 (US2) and Phase 6 (US4) in parallel

---

## Summary

| Phase | Tasks | User Story | Notes |
|---|---|---|---|
| Phase 1: Setup | T001-T003 | — | Install deps, create stubs, ADR |
| Phase 2a: Domain Model | T004 | — | ARCHITECTURE.md update |
| Phase 2b: Config Design | T005-T006 | — | examples/config/telemetry.yaml |
| Phase 2c: API Design | T007 | — | N/A confirmed |
| Phase 2d: DB Design | T008 | — | N/A confirmed |
| Phase 2f: E2E Tests | T009-T013 | — | 15 tests, red phase verified |
| Phase 2.5: Foundational | T014-T018 | — | Config schema + validation |
| Phase 3: US3 | T019-T029 | US3 (P1) | Provider init, gRPC/HTTP exporters |
| Phase 4: US1 | T030-T039 | US1 (P1) | otelchi, child spans, slog bridge |
| Phase 5: US2 | T040-T041 | US2 (P2) | runtime.Start(), HTTP metrics via otelchi |
| Phase 6: US4 | T042-T044 | US4 (P2) | Shutdown flushing |
| Phase N: Compliance | T045-T068 | — | Constitution verification |

**Total**: 68 tasks
**US3**: 11 implementation tasks (T019-T029)
**US1**: 11 implementation tasks (T030-T039; includes 1 integration test)
**US2**: 2 tasks (T040-T041)
**US4**: 3 tasks (T042-T044)
**Parallel opportunities**: 18 tasks marked [P]
**Suggested MVP scope**: Phase 1 + Phase 2 + Phase 2.5 + Phase 3 (US3) — 30 tasks

---

## Notes

- `[P]` tasks operate on different files with no dependencies on in-progress tasks
- `[US1]`/`[US2]`/`[US3]`/`[US4]` labels map tasks to spec.md user stories for traceability
- All child spans use `otel.Tracer(instrumentationName).Start(ctx, spanName)` — global tracer, no constructor injection, no port interface changes
- Span attribute guard (SR-001): always check against the prohibited list before adding attributes — no tokens, keys, PII, or body content
- `sdk/log` is beta (`v0.16.0`) — log pipeline failures are non-fatal; do not let LoggerProvider init failure block TracerProvider or MeterProvider startup
- The `WithTracerProvider(tp)` builder method (T012 stub, T029 full wiring) mirrors the existing `WithEncryption` precedent — only used in E2E tests with in-memory SpanRecorder
- otelchi automatically emits HTTP metrics when a global MeterProvider is registered — no separate HTTP metrics instrumentation code is required
- Commit after each checkpoint: after Phase 2.5, after Phase 3, after each user story
