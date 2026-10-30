# ADR 20260301-120000: OpenTelemetry Provider Pattern

**Status**: Accepted
**Date**: 2026-03-01
**Feature**: 20260228-120000-opentelemetry-support

## Context

The Agentic Identity Broker requires distributed tracing, metrics, and log correlation via OpenTelemetry (OTel). Three key design decisions must be made:

1. Where in the architecture should the OTel SDK providers be initialized and owned?
2. Which library should be used to instrument the chi v5 HTTP servers?
3. How should child spans be propagated through internal operations without changing adapter constructors?

## Decision 1: App-Layer OTel Provider Pattern (No Port Interface)

The OTel `TracerProvider`, `MeterProvider`, and `LoggerProvider` are initialized in `internal/app/builder.go` via `NewProvider()`, which is called during `Build()`. No port interface (`internal/ports/telemetry.go`) is defined for telemetry.

**Rationale**:

- Telemetry is observable infrastructure, not domain behavior. It has no domain logic to isolate behind a hexagonal port.
- The OTel SDK's global provider pattern (`otel.SetTracerProvider`, `otel.SetMeterProvider`) provides a well-established mechanism for decoupling instrumentation from provider lifecycle, without requiring a custom port.
- Adapters that emit child spans use `otel.Tracer(name).Start(ctx, spanName)` with the global tracer — no constructor changes required. This satisfies the constraint that no adapter constructors change (spec clarification).
- A `WithTracerProvider(*sdktrace.TracerProvider)` builder method allows test overrides with in-memory span recorders, mirroring the existing `WithEncryption` precedent.
- Defining a telemetry port interface would add abstraction without benefit: OTel's own API already provides the necessary interface boundary between instrumentation and backend.

**Graceful shutdown sequence**:
1. HTTP servers drain in-flight requests
2. `tp.Shutdown(ctx)` — flush pending trace spans
3. `mp.Shutdown(ctx)` — flush pending metric data points
4. `lp.Shutdown(ctx)` — flush pending log records

The composite shutdown function is stored on the `App` struct as `ShutdownTelemetry func(context.Context) error` and integrated into the errgroup shutdown path.

## Decision 2: otelchi for chi v5 HTTP Middleware

We use `github.com/riandyrn/otelchi v0.12.2` as the OpenTelemetry middleware for chi v5 HTTP servers.

**Rationale**:

- `otelchi` is purpose-built for chi v5, supporting `WithChiRoutes(r)` for parametrized route names (e.g., `/api/agents/{id}` instead of `/api/agents/123`).
- `WithRequestMethodInSpanName(true)` produces span names in the `METHOD /path` format expected by the E2E matchers.
- `WithTracerProvider(tp)` option allows injecting a test provider for span assertions without global state.
- Alternative (`go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`) does not support chi's parametrized route names natively.

**Registration**:

```go
// Registered conditionally when cfg.Telemetry.Enabled && cfg.Telemetry.Traces.Enabled.
// Propagators are registered unconditionally (propagation ≠ tracing).
r.Use(otelchi.Middleware("enduser",
    otelchi.WithTracerProvider(tp),
    otelchi.WithChiRoutes(r),
    otelchi.WithRequestMethodInSpanName(true),
    otelchi.WithPropagators(otel.GetTextMapPropagator()),
))
```

## Decision 3: Context-Based Span Propagation

Adapters emit child spans using the OTel global tracer via `otel.Tracer(instrumentationName).Start(ctx, spanName)`. No adapter constructor changes are required.

**Rationale**:

- The global tracer pattern is idiomatic OTel Go and is the recommended approach when constructor injection would create pervasive interface changes.
- Context propagation ensures child spans are automatically parented to the active span from the HTTP middleware, creating correct trace hierarchies without explicit parent linking.
- When telemetry is disabled, the global tracer is the no-op tracer (set by `otel.SetTracerProvider(noop.NewTracerProvider())`), so child span code is a no-op with negligible overhead.
- This approach avoids the anti-pattern of passing `*sdktrace.TracerProvider` through domain service constructors, which would violate the hexagonal architecture boundary.

**Instrumentation scope**:
- `otel.Tracer("storage")` — storage adapter operations (GetAgent, ListUserGrants, etc.)
- `otel.Tracer("encryption")` — Encrypt/Decrypt calls in the AWS adapter
- `otel.Tracer("jwks")` — JWKS fetch operations
- `otel.Tracer("upstream")` — Upstream OAuth2 token exchange HTTP calls

## Consequences

**Positive**:
- No adapter constructor changes required (spec constraint met)
- Clean test override path via `WithTracerProvider` (mirrors `WithEncryption`)
- Zero overhead when disabled (no-op global provider)
- otelchi automatically provides HTTP metrics (request count, duration, size) when a global MeterProvider is registered

**Negative**:
- Global state via OTel's `otel.SetTracerProvider` must be managed carefully in tests to avoid cross-test contamination. Each test must call `tp.Shutdown()` in `AfterEach` and use `WithTracerProvider` injection rather than relying on global state.
- `sdk/log v0.16.0` is a beta API — log pipeline failures must be non-fatal and must not block `TracerProvider` or `MeterProvider` initialization.

## Alternatives Considered

1. **Port interface for telemetry** (`internal/ports/telemetry.go`): Rejected because OTel's own API already provides the abstraction boundary. A custom port would add complexity without benefit.
2. **Constructor injection of TracerProvider**: Rejected because it would require changing every adapter constructor and would violate the hexagonal architecture constraint (spec clarification).
3. **`go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`**: Rejected because it does not support chi parametrized route names out of the box.
