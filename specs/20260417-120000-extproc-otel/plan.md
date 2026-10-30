# Implementation Plan: OpenTelemetry Support for ExtProc Token Exchange

**Branch**: `20260417-120000-extproc-otel` | **Date**: 2026-04-20 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/20260417-120000-extproc-otel/spec.md`

## Summary

Add OpenTelemetry instrumentation (traces, metrics, logs) to the ExtProc Token Exchange service, reusing the existing `internal/adapters/telemetry.NewProvider()`. The ExtProc service adopts the same `telemetry:` configuration schema as the Identity Broker (with `EXTPROC_` env var prefix and `extproc-token-exchange` default service name). This enables end-to-end distributed tracing from agentgateway → ExtProc → Identity Broker, with per-request spans in `processRequestHeaders`, trace context injection on outbound HTTP calls via `otelhttp.NewTransport`, request-level metrics (counter + histogram by outcome), and slog-to-OTel log correlation. No new domain objects, APIs, database migrations, or frontend changes are introduced — this is a pure infrastructure/observability enhancement.

**Architecture note**: The plan requires `cmd/extproc-token-exchange/root.go` to import `internal/ports` (for `TelemetryConfig` mapping) and `internal/adapters/telemetry` (for `NewProvider`). Currently the ExtProc binary imports only `internal/extproc/*`. This is an acceptable dependency direction — `cmd/` is the outermost composition layer and may import any internal package. The `internal/extproc/config` package remains free of `internal/ports` imports; only the `cmd/` mapper function bridges the two. If this shared dependency becomes problematic, a future ADR can extract the telemetry package into a neutral shared module.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: OpenTelemetry Go SDK (`go.opentelemetry.io/otel`), `otelhttp`, `otelslog`, existing `internal/adapters/telemetry` package, Cobra/Viper, `envoyproxy/go-control-plane`
**Storage**: N/A — no database changes
**Testing**: Go `testing` + `testify` for unit tests; Ginkgo/Gomega for E2E (extending existing `tests/e2e/extproc/` suite); `tracetest.SpanRecorder` for in-memory span verification
**Target Platform**: Linux container (Docker, Kubernetes)
**Project Type**: Backend Go service (ExtProc gRPC service)
**Performance Goals**: Zero measurable latency overhead when telemetry disabled (SC-003); no degradation when collector unreachable (SC-004)
**Constraints**: Telemetry must never block token exchange; flush within 5s on shutdown (FR-007)
**Scale/Scope**: ~8 files modified, ~4 files created; no new domain objects

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: No new domain entities — pure infrastructure/observability enhancement (spec Assumptions)
- [x] **Domain Concepts**: No new domain terms — telemetry is infrastructure, not domain
- [x] **Entity IDs**: N/A — no new domain entities
- [x] **Configuration Design**: Telemetry config schema mirrors `ports.TelemetryConfig` exactly (spec §Configuration Requirements). YAML examples provided in spec.
- [x] **Config Examples**: Will add `examples/config/extproc-telemetry.yaml` and update `examples/config/README.md`
- [x] **Helm Chart**: N/A — no Helm chart exists for ExtProc. The identity broker Helm chart is unaffected.
- [x] **API Design First**: N/A — no API changes. This feature adds no HTTP/gRPC endpoints.
- [x] **API Documentation**: N/A — no API changes
- [x] **API Changes**: N/A — no API changes
- [x] **Database Design**: N/A — no database changes
- [x] **E2E Acceptance Tests**: E2E tests will extend the existing `tests/e2e/extproc/` suite (which has its own bootstrap at `tests/e2e/extproc/bootstrap/`) as `tests/e2e/extproc/telemetry_test.go`. Tests use in-memory span recorder (`tracetest.SpanRecorder`) for span verification.
- [x] **E2E Test Mapping**: All acceptance scenarios map 1:1 to `It()` blocks in `tests/e2e/extproc/telemetry_test.go`. Config lifecycle scenarios (US2-S1, US2-S6, US2-S7) are E2E tests that exercise the real startup/loader path from within the Ginkgo suite (not cmd/ integration tests), satisfying the project's mandated 1:1 acceptance-test structure.
- [x] **E2E Red Phase**: E2E tests will contain detailed assertions (span names, parent-child relationships, attributes, metric values) and fail semantically before implementation
- [x] **Frontend Playwright E2E**: N/A — no frontend changes
- [x] **Frontend Screenshots**: N/A — no frontend changes

**Implementation Considerations**:

- [x] **Security-First**: TLS enabled by default (SR-003). `insecure: true` logs warning. No sensitive data in spans (SR-001). Token values never appear as attributes.
- [x] **Architecture Docs**: Minor ARCHITECTURE.md update to note ExtProc telemetry capability
- [ ] **ADRs**: **PENDING** — A lightweight ADR is required before implementation begins to document the decision for `cmd/extproc-token-exchange/` to import `internal/ports` and `internal/adapters/telemetry`, crossing the current ExtProc package boundary. This ADR must be created in Phase 1 (which is now mandatory). Implementation MUST NOT proceed until the ADR is accepted.
- [x] **Library-First Security**: Uses official OTel Go SDK (`go.opentelemetry.io/otel`) — no custom crypto
- [x] **Zalando Guidelines**: N/A — no API changes
- [x] **End-User Docs**: Will update `docs/configuration.md` with ExtProc telemetry configuration section (Principle VII)
- [x] **Migration Testing**: N/A — no database changes
- [x] **Hexagonal Architecture**: ExtProc is a separate binary. The `cmd/extproc-token-exchange/` layer (outermost composition) imports `internal/ports` (for config type mapping) and `internal/adapters/telemetry` (for `NewProvider`). The `internal/extproc/` packages remain free of these imports. This follows standard Go `cmd/` composition patterns — the entrypoint wires dependencies from any internal package. **Note**: This crosses the current ExtProc boundary (which today imports only `internal/extproc/*`). A mandatory ADR is required before implementation (see ADRs checkbox above — marked PENDING).
- [x] **Persistence Patterns**: N/A — no persistence changes

*All BLOCKING checks pass except ADRs (PENDING — must be created in Phase 1 before implementation).*

## Project Structure

### Documentation (this feature)

```text
specs/20260417-120000-extproc-otel/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output (minimal — no new domain entities)
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (empty — no API changes)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
# ExtProc config — new TelemetryConfig mirror type
internal/extproc/config/
├── config.go                  # Add TelemetryConfig struct + defaults
├── loader.go                  # Add telemetry flag registration + loading
├── validate.go                # Add telemetry-specific validation rules
└── loader_test.go             # Add telemetry config tests

# ExtProc server — instrumentation
internal/extproc/server/
├── server.go                  # Add OTel span creation in processRequestHeaders
├── server_test.go             # Add span verification tests
├── exchanger.go               # Wrap HTTP client with otelhttp.NewTransport
└── exchanger_test.go          # Add trace propagation tests

# ExtProc entrypoint — telemetry lifecycle
cmd/extproc-token-exchange/
├── root.go                    # Add telemetry init, config mapping, shutdown, slog bridge
└── root_test.go               # (new) Telemetry lifecycle tests

# Existing telemetry adapter — reused as-is
internal/adapters/telemetry/
├── provider.go                # No changes — reused via NewProvider()
└── provider_noop.go           # No changes

# Configuration examples
examples/config/
├── extproc-telemetry.yaml     # (new) ExtProc telemetry config example
├── extproc-token-exchange.yaml # Update with telemetry section reference
└── README.md                  # Update with extproc-telemetry.yaml entry

# E2E tests — extend existing ExtProc E2E suite
tests/e2e/extproc/
├── telemetry_test.go            # (new) OTel telemetry E2E tests
└── bootstrap/                   # Extend existing bootstrap with tracer provider injection
```

**Structure Decision**: Backend-only changes in the existing ExtProc service binary. No new packages created — telemetry config is added to the existing `internal/extproc/config` package, and instrumentation is added directly to the existing server and exchanger. E2E tests extend the existing `tests/e2e/extproc/` suite (which has its own bootstrap, fixtures, and helpers) rather than creating a new top-level E2E file. The `internal/adapters/telemetry` adapter is reused without modification.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | N/A — no pre-implementation refactoring needed | Skip |
| **Phase 1** | Setup — Create ADR for ExtProc → broker telemetry dependency | **MANDATORY** |
| **Phase 2** | Design Preconditions (config, E2E test stubs) | **MANDATORY** |
| **Phase 2.7** | N/A — no new entities | Skip |
| **Phase 2.5** | ExtProc TelemetryConfig type + config loading + validation | **Yes** |
| **Phase 3** | US1 — Distributed tracing (spans + propagation) | **Yes** |
| **Phase 4** | US2 — Config parity (mapping, validation, startup) | **Yes** |
| **Phase 5** | US3 — Request-level metrics (counter + histogram) | **Yes** |
| **Phase 6** | US4 — Graceful degradation + US5 — Log correlation | **Yes** |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

**Phase decisions**:
- [x] Phase 0 (refactoring): Skip — no structural changes needed before feature work
- [x] Phase 2.7 (entity boilerplate): Skip — no new domain entities

### Phase 2: Design Preconditions

1. **2a: ExtProc TelemetryConfig type** — Define mirror `TelemetryConfig` struct in `internal/extproc/config/config.go` matching `ports.TelemetryConfig` field-for-field. Add defaults in `applyDefaults()`.
2. **2b: Config loading** — Add Viper bindings for `telemetry.*` keys in `loader.go`. Add `EXTPROC_TELEMETRY_*` env var mappings. Register CLI flags.
3. **2b-env: Env var expansion for telemetry fields** — Extend the existing `expandEnvVars()` function in `loader.go` to process `${ENV_VAR}` substitution in telemetry string fields, especially `telemetry.exporter.headers` map values (SR-002) and `telemetry.exporter.endpoint`. Add unit tests verifying expansion in header values.
3. **2c: Config validation** — Add validation rules in `validate.go`: endpoint required when enabled, protocol in allowed set, sampling rate 0.0–1.0, unrecognized propagators warned.
4. **2d: Config mapping function** — Add `mapTelemetryConfig(extprocconfig.TelemetryConfig) ports.TelemetryConfig` in `cmd/extproc-token-exchange/root.go`, keeping `internal/extproc/config` free of `internal/ports` imports.
5. **2e: Example config** — Create `examples/config/extproc-telemetry.yaml`, update README.md.
6. **2f: E2E acceptance tests** — Write E2E tests in `tests/e2e/extproc/telemetry_test.go` for all spec scenarios, extending the existing ExtProc E2E suite. Tests compile and fail semantically (red phase).

### Phase 3: Distributed Tracing (US1 — P1)

1. **3a: Telemetry initialization in root.go** — Call `telemetry.NewProvider()` with mapped config after logger init, before token exchanger creation. Store shutdown function.
2. **3b: Per-request span creation** — In `processRequestHeaders`, derive the base context from `stream.Context()`, then extract trace context from the incoming `HttpHeaders` entry list using `otel.GetTextMapPropagator().Extract()` with an `http.Header`-style multi-value carrier (preserving repeated keys). Create a span, set attributes: `resource.uri`, `outcome`, `error.type` (on failure). Never include token values (SR-001).
3. **3c: Outbound trace injection** — When `telemetry.traces.enabled` is `true`: wrap the HTTP client with `otelhttp.NewTransport(base)`, which creates HTTP client spans and injects trace context automatically. When `telemetry.enabled` is `true` but `telemetry.traces.enabled` is `false`: use manual propagation instead — implement a propagation-only `http.RoundTripper` in `internal/extproc/server/exchanger.go` (as a small unexported type or function) that calls `otel.GetTextMapPropagator().Inject(ctx, carrier)` on outbound request headers without creating spans. This preserves transparent context forwarding (FR-011) without violating the "no local spans" contract (FR-003).
4. **3d: Propagator registration** — Propagators are registered unconditionally by `NewProvider()` when telemetry is enabled (FR-011) — this is already handled by the adapter.

### Phase 4: Configuration Parity (US2 — P1)

1. **4a: Startup validation** — Validate telemetry config before gRPC server starts. Fail immediately with clear error message for invalid config (FR-009).
2. **4b: Env var support** — `EXTPROC_TELEMETRY_*` env vars supported via existing Viper `AutomaticEnv()` + `EXTPROC_` prefix convention.
3. **4c: Schema parity verification** — Unit tests verifying that ExtProc's `TelemetryConfig` struct has field-for-field parity with `ports.TelemetryConfig`.

### Phase 5: Metrics (US3 — P2)

1. **5a: Request counter** — Add `extproc.token_exchange.requests` counter broken down by `outcome` attribute (success, exchange_failure, circuit_open, invalid_resource, assertion_expired, passthrough). The `passthrough` outcome is recorded when a request bypasses token exchange (e.g., no matching resource URI).
2. **5b: Duration histogram** — Add `extproc.token_exchange.duration` histogram broken down by `outcome`.
3. **5c: Runtime metrics** — Already handled by `NewProvider()` which starts runtime metrics when metrics are enabled.

### Phase 6: Graceful Degradation + Log Correlation (US4 P2 + US5 P3)

1. **6a: Shutdown telemetry flush** — After `GracefulStop()`, call shutdown function with 5-second context deadline (FR-007). Log flush errors but don't affect exit code.
2. **6b: Graceful degradation** — Inherent in OTel SDK design: failed exports don't block request processing. No startup connectivity probe is needed. No service-owned warning-logging contract is defined for collector outages — export failure handling (including any warning logs) is delegated entirely to the OTel SDK. E2E tests verify only that token exchange continues successfully with an unreachable endpoint — they do not assert specific log messages or response-time thresholds.
3. **6c: Slog-to-OTel bridge** — When telemetry and logs are both enabled, wrap the logger with `otelslog.NewHandler` + `telemetry.NewMultiHandler` following the identity broker pattern in `builder.go` (FR-010).

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/extproc/telemetry_test.go`

**Framework**: Ginkgo/Gomega BDD framework, extending the existing ExtProc E2E suite in `tests/e2e/extproc/`

**Test Organization**:
- **Top-level Describe**: "ExtProc OpenTelemetry Instrumentation"
- **Nested Describes**: Per user story (US1: Tracing, US2: Configuration, US3: Metrics, US4: Degradation, US5: Logs)
- **It blocks**: One per acceptance scenario from spec.md

**Note on E2E approach**: The existing `tests/e2e/extproc/` suite has its own bootstrap infrastructure (`tests/e2e/extproc/bootstrap/`) that starts an in-process ExtProc gRPC server with mock HTTP servers. Telemetry E2E tests will extend this bootstrap with `tracetest.SpanRecorder` injection.

For **unit-level span and metric verification** (US1-S4, US1-S5, US1-S6, US2-S1, US2-S6, US2-S7, US3, US4): Use the existing ExtProc bootstrap with an in-memory `SpanRecorder`, `sdkmetric.ManualReader`, and a mock `Exchanger` for controlled outcomes.

For **protocol-specific export verification** (US2-S3, US2-S4, US2-S5): Use mock OTLP collectors — a mock gRPC server implementing the OTLP `TraceService`/`MetricsService`/`LogsService` RPCs for gRPC (US2-S3), a mock `httptest.Server` for HTTP (US2-S4), and a mock `httptest.NewTLSServer` for HTTPS (US2-S5). Each verifies the real exporter path, protocol-specific header injection, and TLS handshake for HTTPS. These verify the real exporter path, not just in-memory readers.

For **log export verification** (US5-S1, US5-S2): US5-S1 uses a test double `sdklog.Exporter` or the SDK's `logtest` helpers to verify trace_id/span_id fields on log records. US5-S2 uses a mock OTLP gRPC collector and asserts zero `LogsService.Export` RPC invocations when `logs.enabled=false`.

For **outbound trace propagation** (US1-S1 to US1-S3): Use a real `TokenExchanger` against a mock broker HTTP server (`httptest.Server`). The mock broker captures inbound request headers so the test can verify that `traceparent` was injected by `otelhttp.NewTransport`. For US1-S2, the test asserts the span hierarchy: `extproc.token_exchange` is a child of the incoming trace, `otelhttp.NewTransport` creates an HTTP client span that is a child of `extproc.token_exchange`, and the outbound `traceparent`'s trace ID matches the incoming trace. The test verifies parent-child linkage via span parent IDs in the `SpanRecorder`, not by asserting against the outbound `traceparent` parent ID directly (since `otelhttp` injects its own client span context). Full three-hop verification (agentgateway → ExtProc → broker with parent-child relationships across all three) is an integration-environment concern, not an automated E2E test target.

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1-S1 | `tests/e2e/extproc/telemetry_test.go` | `It("creates a child span linked to incoming trace context")` |
| US1-S2 | `tests/e2e/extproc/telemetry_test.go` | `It("produces span hierarchy: extproc.token_exchange → HTTP client span, with matching trace ID to broker")` |
| US1-S3 | `tests/e2e/extproc/telemetry_test.go` | `It("injects trace context into outbound HTTP token exchange requests")` |
| US1-S4 | `tests/e2e/extproc/telemetry_test.go` | `It("records error status and description on failed token exchange spans")` |
| US1-S5 | `tests/e2e/extproc/telemetry_test.go` | `It("emits no telemetry when disabled")` |
| US1-S6 | `tests/e2e/extproc/telemetry_test.go` | `It("forwards trace context without creating spans when traces.enabled=false")` |
| US2-S1 | `tests/e2e/extproc/telemetry_test.go` | `It("starts normally with no telemetry section in config")` — exercises real startup path |
| US2-S2 | `tests/e2e/extproc/telemetry_test.go` | `It("exports traces under configured service name")` |
| US2-S3 | `tests/e2e/extproc/telemetry_test.go` | `It("exports via gRPC when protocol is grpc")` |
| US2-S4 | `tests/e2e/extproc/telemetry_test.go` | `It("exports via HTTP when protocol is http")` |
| US2-S5 | `tests/e2e/extproc/telemetry_test.go` | `It("exports via HTTPS when protocol is https")` — uses `httptest.NewTLSServer` mock collector |
| US2-S6 | `tests/e2e/extproc/telemetry_test.go` | `It("uses EXTPROC_TELEMETRY_ env vars")` — exercises Viper env var loading |
| US2-S7 | `tests/e2e/extproc/telemetry_test.go` | `It("fails startup with validation error for invalid config")` — exercises startup validation |
| US3-S1 | `tests/e2e/extproc/telemetry_test.go` | `It("exports request counter and duration histogram by outcome")` |
| US3-S2 | `tests/e2e/extproc/telemetry_test.go` | `It("exports Go runtime metrics when metrics enabled")` |
| US3-S3 | `tests/e2e/extproc/telemetry_test.go` | `It("emits only traces when metrics disabled")` |
| US4-S1 | `tests/e2e/extproc/telemetry_test.go` | `It("starts successfully with unreachable OTLP endpoint")` |
| US4-S2 | `tests/e2e/extproc/telemetry_test.go` | `It("continues processing when collector becomes unreachable")` |
| US4-S3 | `tests/e2e/extproc/telemetry_test.go` | `It("resumes telemetry export when collector recovers")` |
| US5-S1 | `tests/e2e/extproc/telemetry_test.go` | `It("includes trace_id and span_id in exported log records")` |
| US5-S2 | `tests/e2e/extproc/telemetry_test.go` | `It("does not export OTLP logs when logs disabled")` — asserts zero LogsService.Export RPC invocations |

**Red Phase Requirements**:
- E2E tests MUST compile and contain detailed, realistic expectations
- Assertions target actual span attributes, trace IDs, metric values
- No placeholder always-fail assertions
- No `XIt`, `PIt`, `Skip()` — all tests must run and fail
- No red-phase annotation comments

**Test Data Strategy**:
- Mock `Exchanger` interface for controlled success/failure outcomes (unit-level scenarios)
- Real `TokenExchanger` + mock broker `httptest.Server` for outbound propagation scenarios
- In-memory `tracetest.SpanRecorder` for span verification
- ExtProc config fixtures with telemetry enabled/disabled variants

**Bootstrap Strategy**:
- Tests extend the existing `tests/e2e/extproc/bootstrap/` infrastructure
- Bootstrap enhanced with:
  - **`tracetest.SpanRecorder`** injection via `sdktrace.NewTracerProvider` — for span verification (US1, US2 trace scenarios)
  - **`sdkmetric.ManualReader`** — for metric verification (US3 scenarios). Tests call `Collect()` to read in-memory metric data without an OTLP collector.
  - **`sdklog.InMemoryExporter`** — for log correlation verification (US5-S1). Used to assert trace_id/span_id on exported log records.
  - **Mock OTLP gRPC collector** — for protocol-specific verification (US2-S3, US5-S2). A lightweight gRPC server implementing `TraceService.Export`, `MetricsService.Export`, and `LogsService.Export` RPCs. Records received payloads and per-RPC invocation counts. For US2-S3, asserts traces/metrics exported. For US5-S2, asserts zero `LogsService.Export` invocations when `logs.enabled=false` (not by connection count, but by per-service RPC invocation tracking).
  - **Mock OTLP HTTP collector** — for protocol-specific verification (US2-S4). An `httptest.Server` accepting OTLP HTTP/protobuf POST requests at `/v1/traces`, `/v1/metrics`, `/v1/logs`. Records received payloads for export format verification.
  - **Mock OTLP HTTPS collector** — for HTTPS verification (US2-S5). An `httptest.NewTLSServer` with self-signed cert. Tests set the `OTEL_EXPORTER_OTLP_CERTIFICATE` environment variable (via `t.Setenv()`) to point to the self-signed CA cert, verifying HTTPS export via the OTel SDK's standard TLS configuration (no app-level TLS config parameters exist).
  - **Mock `Exchanger`** injection hook — for controlled success/failure outcomes (US1-S4, US1-S5, US3)
  - **Config-loader bootstrap path** — for config lifecycle scenarios (US2-S1, US2-S6, US2-S7), tests exercise the real Viper-based config loading and startup validation path. The bootstrap creates a temporary config file (or sets `EXTPROC_TELEMETRY_*` env vars via `t.Setenv()`), then invokes the actual startup/config-loading flow to verify behavior. This keeps these acceptance scenarios in the E2E suite while still testing the real loader path.
  - **Propagation-only round-tripper** — for `traces.enabled=false` scenarios, the bootstrap configures the propagation-only transport (manual header injection without `otelhttp.NewTransport`) to verify transparent context forwarding without span creation.
  - **Unreachable endpoint** — tests bind a TCP listener and immediately close it to get a guaranteed-unreachable port for US4 scenarios; no mock OTLP collector needed since the assertion is "requests still succeed"
  - **Collector recovery** (US4-S3) — uses a toggleable `httptest.Server` OTLP endpoint that can be paused/resumed mid-test
- Fresh server per test via BeforeEach/AfterEach
- Mock broker `httptest.Server` captures request headers for propagation assertions

### Frontend Playwright E2E Tests

N/A — no frontend changes.

### Unit & Integration Tests

**Unit Tests**:
- `internal/extproc/config/loader_test.go`: TelemetryConfig loading, defaults, env var override, validation
- `internal/extproc/server/server_test.go`: Span creation in processRequestHeaders, attribute verification
- `internal/extproc/server/exchanger_test.go`: otelhttp transport wrapping, trace context injection
- `cmd/extproc-token-exchange/root_test.go`: mapTelemetryConfig correctness, field-for-field parity

**Integration Tests**:
- N/A — all acceptance scenarios are covered in the E2E suite. Config lifecycle scenarios (US2-S1, US2-S6, US2-S7) exercise the real startup/loader path from within the Ginkgo E2E framework by bootstrapping the ExtProc server with programmatic config and env var injection.

**Test Coverage Goals**:
- Unit test coverage: All new code paths (config loading, validation, span creation, metrics recording)
- E2E test coverage: All 21 acceptance scenarios in `tests/e2e/extproc/telemetry_test.go` (1:1 mapping)
- Config validation edge cases: all edge cases from spec (invalid protocol, out-of-range sampling, unrecognized propagators)

## Complexity Tracking

> No Constitution Check violations to justify.
