# Implementation Plan: Configurable OpenTelemetry Support

**Branch**: `20260228-120000-opentelemetry-support` | **Date**: 2026-03-01 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `/specs/20260228-120000-opentelemetry-support/spec.md`

## Summary

Add configurable OpenTelemetry (OTel) distributed tracing, metrics, and log correlation to the Agentic Identity Broker. When enabled via YAML/env configuration, the system exports traces and metrics to an OTLP gRPC or HTTP endpoint, instruments all inbound HTTP requests on both servers using `otelchi`, creates child spans for significant internal operations (storage, encryption, upstream OAuth2), and correlates structured log entries with active trace context via the `otelslog` bridge. Telemetry is disabled by default; when disabled no OTel SDK code runs and the application incurs zero measurable overhead. All OTel provider setup is wired in `internal/app/builder.go`; no port interface is defined for telemetry.

## Technical Context

**Language/Version**: Go 1.25.6  
**Primary Dependencies**:
- `go.opentelemetry.io/otel v1.40.0` — Core OTel API + propagation
- `go.opentelemetry.io/otel/sdk v1.40.0` — TracerProvider, MeterProvider, resource
- `go.opentelemetry.io/otel/sdk/log v0.16.0` — LoggerProvider (beta)
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.40.0`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.40.0`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.40.0`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.40.0`
- `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.16.0`
- `go.opentelemetry.io/contrib/instrumentation/runtime v0.65.0` — Go runtime metrics
- `go.opentelemetry.io/contrib/bridges/otelslog v0.15.0` — slog bridge
- `github.com/riandyrn/otelchi v0.12.2` — chi v5 OTel HTTP middleware

**Storage**: N/A (no new persistence entities)  
**Testing**: Ginkgo v2/Gomega (E2E), Go stdlib testing (unit), `tracetest.SpanRecorder` (in-memory OTel)  
**Target Platform**: Linux server (Docker/Kubernetes)  
**Project Type**: Single Go binary (backend only)  
**Performance Goals**: ≤5% p50 latency overhead when enabled (SC-002); zero measurable overhead when disabled (SC-003)  
**Constraints**: No port interface for telemetry (spec clarification); context-based span propagation (no adapter constructor changes); TLS on by default (SR-003)  
**Scale/Scope**: Instrumentation of all HTTP routes on 2 servers; child spans in storage, encryption, JWKS, upstream OAuth2 adapters

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Configuration schema is the primary "model" — `TelemetryConfig` with sub-types for traces, metrics, exporter documented in `data-model.md`. No new domain aggregates (telemetry is purely infrastructure).
- [x] **Domain Concepts**: No new domain terms required. ARCHITECTURE.md updated to document OTel provider wiring pattern.
- [x] **Configuration Design**: All 13 config parameters identified with types, defaults, validation rules. See `data-model.md` and `quickstart.md`.
- [x] **Config Examples**: `examples/config/telemetry.yaml` will be added. `examples/config/README.md` updated.
- [x] **API Design First**: No new HTTP API endpoints. Feature is purely internal instrumentation. N/A.
- [x] **API Documentation**: No new API endpoints. N/A.
- [x] **API Changes**: No API contract changes. N/A.
- [x] **Database Design**: No new database schema. N/A.
- [x] **E2E Acceptance Tests**: E2E tests written BEFORE implementation for all 15 acceptance scenarios. See Testing Strategy.
- [x] **E2E Test Mapping**: Each acceptance scenario maps 1:1 to one `It()` block in `tests/e2e/telemetry_test.go`.
- [x] **E2E Red Phase**: E2E tests compile and fail BEFORE implementation because `WithTracerProvider()` builder method does not exist yet.

**Implementation Considerations**:

- [x] **Security-First**: TLS ON by default (`insecure: false`). Disabling TLS requires explicit opt-in + startup warning. SR-001 through SR-004 addressed.
- [x] **Architecture Docs**: `ARCHITECTURE.md` updated to document OTel provider wiring in the builder section.
- [x] **ADRs**: ADR `20260301-120000-opentelemetry-provider-pattern.md` created to record: app-layer provider pattern, `otelchi` library choice, context-based propagation approach.
- [x] **Library-First Security**: Using vetted OTel Go SDK. No custom crypto.
- [x] **Zalando Guidelines**: No new API endpoints, N/A.
- [x] **End-User Docs**: Configuration docs added to `docs/configuration.md` under "Observability / OpenTelemetry" section.
- [x] **Migration Testing**: No database migrations. N/A.
- [x] **Hexagonal Architecture**: OTel provider is app-layer only (confirmed by spec clarification). Adapters use global `otel.Tracer()` for child spans — no port changes required.
- [x] **Persistence Patterns**: No new persistence. N/A.

*Post-Phase 1 Re-Check: All preconditions confirmed met. No blocking violations.*

## Project Structure

### Documentation (this feature)

```text
specs/20260228-120000-opentelemetry-support/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)  ← COMPLETE
├── data-model.md        # Phase 1 output (/speckit.plan command)  ← COMPLETE
├── quickstart.md        # Phase 1 output (/speckit.plan command)  ← COMPLETE
├── contracts/           # N/A — no new HTTP API endpoints
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# New files
internal/adapters/telemetry/
├── provider.go                              # OTel provider setup (TracerProvider, MeterProvider, LoggerProvider)
├── provider_noop.go                         # Shutdown helper for no-op / disabled state
└── slog_handler.go                          # slog multi-handler: tee original handler + OTel bridge

adrs/
└── 20260301-120000-opentelemetry-provider-pattern.md    # New ADR: app-layer OTel, otelchi, context propagation

tests/e2e/
└── telemetry_test.go                        # E2E acceptance tests (all 15 scenarios)

tests/e2e/bootstrap/
└── telemetry.go                             # NewInMemoryTracerProvider() helper

tests/e2e/matchers/
└── telemetry_matchers.go                    # ContainSpanWithName, ContainSpanWithAttribute, HaveHTTPSpan

examples/config/
└── telemetry.yaml                           # Example YAML for full OTel configuration

# Modified files
internal/ports/config.go                     # + TelemetryConfig, TracesConfig, MetricsConfig, OTLPExporterConfig
internal/config/loader.go                    # + setDefaults() and BindEnv for all telemetry config keys
internal/config/validator.go                 # + validateTelemetryConfig()
internal/app/builder.go                      # + WithTracerProvider(); telemetry init in Build()
internal/adapters/http/routing/enduser.go    # + conditional otelchi middleware registration
internal/adapters/http/routing/admin.go      # + conditional otelchi middleware registration
internal/adapters/storage/postgres/*.go      # + child spans on key query methods (GetAgent, etc.)
internal/adapters/encryption/aws/*.go        # + child spans on Encrypt/Decrypt calls
internal/adapters/jwks/adapter.go            # + child span on JWKS Fetch()
internal/adapters/http/upstream/*.go         # + child span on upstream token exchange requests
docs/configuration.md                        # + "Observability / OpenTelemetry" section
ARCHITECTURE.md                              # + OTel provider wiring pattern in Builder section
config.yaml                                  # + telemetry section (disabled by default, commented example)
```

**Structure Decision**: Single Go backend project. New `internal/adapters/telemetry/` adapter package isolates OTel provider setup and is called from `builder.go`. No frontend or CDK/infra changes.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/telemetry_test.go`

**Framework**: Ginkgo/Gomega BDD framework following patterns in `tests/e2e/`

**Test Organization**:
- **Top-level Describe**: `"OpenTelemetry Instrumentation"`
- **Nested Context**: Grouped by user story (tracing, metrics, configuration, graceful degradation)
- **It blocks**: One per acceptance scenario from spec.md

**Scenario Mapping**:

| Spec Scenario | E2E Test Context | Test Description |
|---|---|---|
| US1-S1: End-user trace | `"when tracing is enabled"` | `It("should emit a trace span for end-user server requests")` |
| US1-S2: Admin trace | `"when tracing is enabled"` | `It("should emit a trace span for admin server requests with distinct server identification")` |
| US1-S3: Child spans for internal ops | `"when tracing is enabled"` | `It("should create child spans for storage operations within a request")` |
| US1-S4: No telemetry when disabled | `"when telemetry is disabled"` | `It("should not emit any trace spans")` |
| US2-S1: HTTP metrics | `"when metrics are enabled"` | `It("should export request count, duration, and response size metrics")` |
| US2-S2: Runtime metrics | `"when metrics are enabled"` | `It("should expose Go runtime metrics")` |
| US2-S3: Metrics disabled/traces on | `"when metrics are disabled and tracing is enabled"` | `It("should not export metric data")` |
| US3-S1: No config = disabled | `"when no telemetry config is present"` | `It("should start normally with telemetry disabled and no overhead")` |
| US3-S2: OTLP gRPC protocol | `"when OTLP gRPC protocol is configured"` | `It("should initialize a gRPC trace exporter successfully")` |
| US3-S3: OTLP HTTP protocol | `"when OTLP HTTP protocol is configured"` | `It("should initialize an HTTP trace exporter successfully")` |
| US3-S4: Custom service name + attrs | `"when custom resource attributes are configured"` | `It("should attach custom service name and attributes to all spans")` |
| US3-S5: Env var for endpoint | `"when OTLP endpoint is set via environment variable"` | `It("should use the environment variable value for the OTLP endpoint")` |
| US4-S1: Unreachable at startup | `"when the OTLP collector is unreachable"` | `It("should start successfully and log a warning about the unreachable collector")` |
| US4-S2: Unreachable during operation | `"when the OTLP collector is unreachable"` | `It("should continue serving requests normally with no latency impact")` |
| US4-S3: Auto-resume on reconnect | `"when the OTLP collector is unreachable"` | `It("should resume telemetry export automatically when the collector becomes available again")` |

*Populate line numbers during Phase 2f (E2E Acceptance Test Design) with actual line numbers*

**Test Data Strategy**:
- New bootstrap helper: `tests/e2e/bootstrap/telemetry.go` — `NewInMemoryTracerProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder)`
- New config fixture: `fixtures.TelemetryEnabledConfig(endpoint string)` or inline config mutation per `BeforeEach`
- New matchers: `ContainSpanWithName`, `ContainSpanWithAttribute`, `HaveHTTPSpan` in `tests/e2e/matchers/telemetry_matchers.go`
- Builder hook: `app.Builder.WithTracerProvider(*sdktrace.TracerProvider)` — mirrors `WithEncryption` precedent

**Test Execution Flow**:
1. **E2E Test Design**: Write `tests/e2e/telemetry_test.go` with all `It()` blocks — uses `WithTracerProvider` stub
2. **Verify Red Phase**: Add minimal `WithTracerProvider` stub to `Builder` to make compile; run `ginkgo -v ./tests/e2e/telemetry_test.go` — all tests FAIL semantically (no spans emitted)
3. **Implementation**: Config schema → validation → provider adapter → builder wiring → otelchi middleware → child spans
4. **Verify Green Phase**: Each implementation step turns tests green incrementally

**Bootstrap Strategy**:
- OTel E2E tests call `app.NewBuilder()` directly (bypass `ServerFactory`), following existing precedent for special-case tests
- Per-test isolation: fresh `SpanRecorder` + new `TracerProvider` + fresh in-memory storage + fresh `httptest.Server` in `BeforeEach`
- `AfterEach`: `tracerProvider.Shutdown(context.Background())` + `server.Close()`

**Helper Utilities**:
- Custom matchers needed: `ContainSpanWithName(name string)`, `ContainSpanWithAttribute(spanName, key, value string)`, `HaveHTTPSpan(method, route string, statusCode int)`
- HTTP helpers: existing `server.AuthenticatedGET`, `server.PublicGET` reused unchanged
- Mock services: none required (no external OTLP collector — in-memory `SpanRecorder` used)

### Unit & Integration Tests

**Unit Tests**:
- Location: `internal/config/validator_test.go`, `internal/adapters/telemetry/provider_test.go`
- Coverage: All validation paths (missing endpoint, invalid URL, invalid protocol, sampling rate range), provider factory branches (enabled/disabled, grpc/http)
- Strategy: TDD — write tests FIRST, verify FAIL, implement

**Integration Tests**:
- Location: `internal/adapters/storage/postgres/` (existing integration tests extended)
- Coverage: Spot-check that storage adapter child spans are created (e.g., `GetAgent` emits span with `db.system=postgresql`)
- Strategy: Use `tracetest.SpanRecorder` in integration test setup, assert spans after DB operations

**Test Coverage Goals**:
- Unit test coverage: 100% of validation paths, all provider factory branches
- Integration test coverage: one storage method, one encryption method verified with span emission
- E2E test coverage: 100% of acceptance scenarios from spec.md (mandatory per Principle XIII)

## Complexity Tracking

No Constitution Check violations. All preconditions met.

| Concern | Resolution |
|---|---|
| No port interface for telemetry (OTel provider wired in app-layer only) | Justified by explicit spec clarification. OTel provider has no domain logic to isolate via a hexagonal port. Consistent with Principle VI's "pragmatic deviations" allowance — telemetry is observable infrastructure, not domain behavior. ADR-20260301-120000 documents this decision. |
| `sdk/log` in beta (`v0.16.0`) | Official OTel Go recommendation and only supported path for `slog` log correlation. Log pipeline failure is non-fatal (SR-004 ensures no leakage). Beta risk is low given the stable `NewBatchProcessor`/`NewLoggerProvider` API surface. |
