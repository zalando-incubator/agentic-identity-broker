# Research: 017 — Configurable OpenTelemetry Support

**Branch**: `20260228-120000-opentelemetry-support`  
**Date**: 2026-03-01  
**Status**: Complete — all NEEDS CLARIFICATION resolved

---

## 1. OTel Go SDK — Packages & Versions

### Decision
Use the OTel Go SDK v1.40.0 monorepo (released Feb 2026) plus contrib packages for runtime metrics and otelslog bridge.

### Packages Required (go.mod additions)

```
# Core SDK
go.opentelemetry.io/otel                                              v1.40.0
go.opentelemetry.io/otel/trace                                        v1.40.0
go.opentelemetry.io/otel/metric                                       v1.40.0
go.opentelemetry.io/otel/propagation                                  v1.40.0   # W3C TraceContext + Baggage built-in
go.opentelemetry.io/otel/sdk                                          v1.40.0   # includes sdk/trace, sdk/metric
go.opentelemetry.io/otel/sdk/log                                      v0.16.0   # beta, separate module

# OTLP exporters (traces)
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc      v1.40.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp      v1.40.0

# OTLP exporters (metrics)
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc    v1.40.0
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp    v1.40.0

# OTLP exporters (logs)
go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc          v0.16.0
go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp          v0.16.0

# Contrib
go.opentelemetry.io/contrib/instrumentation/runtime                   v0.65.0   # Go runtime metrics
go.opentelemetry.io/contrib/bridges/otelslog                          v0.15.0   # slog bridge

# Chi OTel middleware
github.com/riandyrn/otelchi                                           v0.12.2

# Testing
# go.opentelemetry.io/otel/sdk/trace/tracetest — sub-package of otel/sdk, no separate require
```

### Rationale
- v1.40.0 is the latest stable GA release as of Feb 2026 and is API-stable.
- Contrib packages follow their own semver but align with the main SDK release cadence.
- `sdk/log` is in beta (`v0.16.0`) but is the official OTel Go logs pipeline and is the only supported integration point for `otelslog` bridge.

### Semconv Sub-Package
The semconv directory to import is `go.opentelemetry.io/otel/semconv/v1.39.0` (sub-directory inside the `otel/sdk` module, not a separate `go get`).

---

## 2. otelchi Middleware for Chi v5

### Decision
Use `github.com/riandyrn/otelchi v0.12.2` to instrument chi v5 HTTP routes.

### API Surface
```go
otelchi.Middleware(serverName string, opts ...Option) func(next http.Handler) http.Handler
```

**Key options**:
- `WithTracerProvider(tp)` — bind a specific `TracerProvider` to this middleware instance; enables different `service.name` per server
- `WithChiRoutes(routes chi.Routes)` — pass the chi router for early route-pattern resolution (recommended)
- `WithRequestMethodInSpanName(true)` — span names become `GET /api/consent/agents` instead of `/api/consent/agents`
- `WithFilter(fn)` — skip tracing for specific paths (e.g., health checks)
- `WithPublicEndpoint()` — treat incoming span context as a link (not parent), for public-facing servers

**Captured attributes**:
| OTel Key | Value |
|---|---|
| `http.method` | Request method |
| `http.route` | Chi route pattern (e.g., `/agents/{id}`) |
| `http.status_code` | Response status |
| `http.server_name` | The `serverName` argument |
| `http.scheme` | `http` / `https` |
| `http.host` | Host header |

### Per-Server Differentiation
If separate `service.name` per server is required, create two `TracerProvider` instances with different resources and pass each via `WithTracerProvider(...)`. A simpler alternative (sufficient for most use cases) is to use a single shared provider and differentiate via the `http.server_name` attribute (the `serverName` argument to `Middleware()`).

**Chosen approach**: Single shared `TracerProvider` with a single `service.name` (from `telemetry.service_name` config), differentiating servers via `http.server_name` attribute set to `"enduser"` and `"admin"` respectively.

**Rationale**: The spec (FR-008) states a single configurable service name; separate providers add initialization complexity without proportional benefit for a single-binary deployment.

---

## 3. Configuration Extension Pattern

### Decision
Add `TelemetryConfig` struct to `internal/ports/config.go`, following the exact pattern of existing sections (ServerConfig, StorageConfig, EncryptionConfig).

### Key Findings (from codebase analysis)
1. **`${ENV_VAR}` substitution is already supported** — implemented in `internal/config/loader.go` `expandEnvVars()`. Viper flattens `map[string]string` into dotted keys, so `${VAR}` in map values (e.g., `exporter.headers.Authorization`) is expanded correctly at config load time.
2. **Validation is hand-written** — the `validate:"..."` struct tags are documentation only; all validation logic is in `internal/config/validator.go` using `formatValidationError()`.
3. **Defaults via `setDefaults()`** — called in the loader's `GetConfig()` phase; use `l.v.SetDefault()` and `l.v.BindEnv()` for each field.
4. **Optional sections** — validate only if the section is non-zero (see `validateOAuth2AuthServerConfig` pattern: check `cfg.Enabled` first).

### TelemetryConfig Schema

```go
// TelemetryConfig in internal/ports/config.go
type TelemetryConfig struct {
    Enabled            bool              `mapstructure:"enabled"`
    ServiceName        string            `mapstructure:"service_name"`
    ResourceAttributes map[string]string `mapstructure:"resource_attributes"`
    Traces             TracesConfig      `mapstructure:"traces"`
    Metrics            MetricsConfig     `mapstructure:"metrics"`
    Exporter           OTLPExporterConfig `mapstructure:"exporter"`
}

type TracesConfig struct {
    Enabled      bool     `mapstructure:"enabled"`
    SamplingRate float64  `mapstructure:"sampling_rate"`
    Propagators  []string `mapstructure:"propagators"`
}

type MetricsConfig struct {
    Enabled        bool          `mapstructure:"metrics_enabled"`
    ExportInterval time.Duration `mapstructure:"export_interval"`
}

type OTLPExporterConfig struct {
    Protocol string            `mapstructure:"protocol"`  // "grpc" | "http"
    Endpoint string            `mapstructure:"endpoint"`
    Headers  map[string]string `mapstructure:"headers"`
    Timeout  time.Duration     `mapstructure:"timeout"`
    Insecure bool              `mapstructure:"insecure"`
}
```

### Environment Variable Naming Convention (matches existing pattern `IDENTITY_BROKER_*`)
```
IDENTITY_BROKER_TELEMETRY_ENABLED
IDENTITY_BROKER_TELEMETRY_SERVICE_NAME
IDENTITY_BROKER_TELEMETRY_OTLP_ENDPOINT
IDENTITY_BROKER_TELEMETRY_OTLP_PROTOCOL
IDENTITY_BROKER_TELEMETRY_OTLP_INSECURE
IDENTITY_BROKER_TELEMETRY_TRACES_ENABLED
IDENTITY_BROKER_TELEMETRY_TRACES_SAMPLING_RATE
IDENTITY_BROKER_TELEMETRY_METRICS_ENABLED
IDENTITY_BROKER_TELEMETRY_METRICS_EXPORT_INTERVAL
```

---

## 4. TracerProvider Lifecycle

### Decision
Single `*sdktrace.TracerProvider` created in `app.Builder.Build()`, registered globally via `otel.SetTracerProvider(tp)`. Shutdown is triggered by the existing graceful shutdown mechanism in the server adapter (errgroup).

### Sampler Pattern
```go
// ParentBased wrapper respects upstream sampling decisions (W3C propagation).
// Root spans use TraceIDRatioBased(cfg.SamplingRate).
sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Traces.SamplingRate))
```
- `SamplingRate = 1.0` → `AlwaysSample` (via ratio sampler at 100%)
- `SamplingRate = 0.0` → `NeverSample` effectively (ratio sampler at 0%)

### Resource Setup
```go
res, _ := resource.New(ctx,
    resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
    resource.WithAttributes(extraAttributes...),    // from cfg.ResourceAttributes map
    resource.WithTelemetrySDK(),
    resource.WithHost(),
    resource.WithProcess(),
)
```

### Shutdown
The `app.App` struct will hold a `ShutdownTelemetry func(context.Context) error` field. The server adapter calls this during graceful shutdown, after HTTP servers drain, before process exit. This satisfies FR-009.

---

## 5. MeterProvider Setup

### Decision
Single `*metric.MeterProvider` created alongside the `TracerProvider` in `Build()`. Go runtime metrics started via `go.opentelemetry.io/contrib/instrumentation/runtime`.

### Runtime Metrics
```go
_ = runtime.Start(runtime.WithMinimumReadMemStatsInterval(15 * time.Second))
```
Provides: goroutine count, heap allocations, GC pause, CPU usage.

---

## 6. Log Correlation (otelslog Bridge)

### Decision
When telemetry is enabled, the existing `*slog.Logger` is wrapped with an `otelslog` bridge handler. The bridge is a `slog.Handler` implementation that forwards log records — with injected `trace_id` and `span_id` from active context spans — to the OTel Logs pipeline.

### Pattern
```go
otelHandler := otelslog.NewHandler(cfg.ServiceName,
    otelslog.WithLoggerProvider(loggerProvider),
)
// Fan-out: keep existing JSON/text handler AND add OTel bridge
multiHandler := io.MultiWriter-style via slog.Handler composition (slogmulti or tee-handler)
// Or: replace the base logger handler entirely with the bridge
logger = slog.New(otelHandler)
```

**Chosen approach**: Create a tee handler that writes to both the original handler (stderr JSON/text) and the OTel bridge. This preserves existing log output while adding telemetry correlation. If a tee library is not available, use a simple `MultiHandler` struct (straightforward 20-line implementation, not custom crypto — this is allowable per Principle III).

---

## 7. No-Operation Path (Telemetry Disabled)

### Decision
When `telemetry.enabled = false` (default), **no OTel packages are called**. The global OTel API installs a no-op `TracerProvider` by default, so any `otel.Tracer()` calls from instrumented adapters are zero-cost no-ops. The `otelchi` middleware still registers but the global no-op provider means zero overhead per the OTel spec (no-op spans are allocated but immediately discarded).

For absolutely zero overhead when disabled, the `otelchi` middleware registration is conditional on telemetry being enabled (check `cfg.Telemetry.Enabled` in routing functions).

---

## 8. E2E Testing Strategy

### Problem
E2E tests cannot connect to a real OTLP collector. Testing that spans are emitted requires an in-memory collector.

### Decision
Use `tracetest.SpanRecorder` from `go.opentelemetry.io/otel/sdk/trace/tracetest` (sub-package of `otel/sdk`). Add `WithTracerProvider(*sdktrace.TracerProvider)` to `app.Builder` (mirrors `WithEncryption`). E2E tests create an in-memory `TracerProvider` backed by a `SpanRecorder`, inject it into the builder, and assert on `recorder.Ended()` after making requests.

### Builder Extension
```go
// In app/builder.go
type Builder struct {
    // ... existing fields ...
    tracerProvider *sdktrace.TracerProvider  // optional test override
}

func (b *Builder) WithTracerProvider(tp *sdktrace.TracerProvider) *Builder {
    b.tracerProvider = tp
    return b
}
```

### Bootstrap Helper
```go
// In tests/e2e/bootstrap/telemetry.go
func NewInMemoryTracerProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
    recorder := tracetest.NewSpanRecorder()
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSpanProcessor(recorder),
        sdktrace.WithSampler(sdktrace.AlwaysSample()),
    )
    return tp, recorder
}
```

### Span Matchers
New file `tests/e2e/matchers/telemetry_matchers.go` with:
- `ContainSpanWithName(name string)` — asserts a span with given name exists in `[]sdktrace.ReadOnlySpan`
- `ContainSpanWithAttribute(spanName, key, value string)` — asserts a named span has a specific attribute value
- `HaveHTTPSpan(method, route string, statusCode int)` — convenience matcher for HTTP instrumentation spans

---

## 9. Internal Span Creation (FR-012)

### Decision
Adapter methods create child spans using `otel.Tracer(instrumentationName).Start(ctx, spanName)` — the global tracer is used, so no constructor injection is needed. Context must flow through adapter method signatures (already the case as most methods accept `context.Context`).

### Instrumentation Targets
| Adapter | Span Name Pattern | Key Attributes |
|---|---|---|
| `storage/postgres` queries | `storage.{operation}.{entity}` (e.g., `storage.get.agent`) | `db.system=postgresql`, `db.operation` |
| `encryption/aws` operations | `encryption.{operation}` (e.g., `encryption.encrypt`) | `encryption.key_type=aws_kms` |
| `adapters/jwks` fetch | `jwks.fetch` | `url.full` (endpoint URL) |
| `adapters/http/upstream` | `oauth2.token_exchange` | `http.method`, `http.status_code` |

---

## 10. Resolved Clarifications from Spec

| Q | Resolved Decision |
|---|---|
| Internal span propagation (FR-012) | Context-based — adapters call `otel.Tracer().Start(ctx, name)`, no port changes |
| HTTP instrumentation library | `github.com/riandyrn/otelchi v0.12.2` |
| OTel provider as port? | No port — wired entirely in `builder.go` |
| PII enforcement (SR-001) | Trust-the-developer + code review; no automated filter |
| Log correlation | `go.opentelemetry.io/contrib/bridges/otelslog`; active only when telemetry enabled |

---

## 11. Alternatives Considered

| Decision | Alternative | Rejected Because |
|---|---|---|
| `otelchi` for chi middleware | Roll custom chi middleware | `otelchi` correctly captures route patterns; custom middleware would require re-implementing chi route context extraction |
| Single shared `TracerProvider` | Per-server providers | Spec only requires single configurable service name; per-server providers add complexity without spec requirement |
| `tracetest.SpanRecorder` for E2E | Real OTLP collector via testcontainers | Testcontainers adds CI overhead and flakiness; in-memory is faster and deterministic |
| Manual `MultiHandler` for log fan-out | `github.com/samber/slog-multi` | Prefer minimal new dependencies; a 20-line tee handler is trivial and avoids adding a new indirect library |
| Conditional middleware registration | Always register otelchi (even when disabled) | Conditional gives zero-overhead path with no-op provider check eliminated (spec SC-003: zero measurable overhead when disabled) |
