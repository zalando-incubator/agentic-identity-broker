# Research: OpenTelemetry Support for ExtProc Token Exchange

**Branch**: `20260417-120000-extproc-otel` | **Date**: 2026-04-20

## Research Tasks & Findings

### R1: Can `internal/adapters/telemetry.NewProvider()` be reused as-is for ExtProc?

**Decision**: Yes — reuse without modification.

**Rationale**: `NewProvider()` accepts a `ports.TelemetryConfig` and `*slog.Logger`, creates OTel providers (TracerProvider, MeterProvider, LoggerProvider), registers propagators, and returns a shutdown function. It has no dependency on the identity broker's app layer. The ExtProc binary needs only to:
1. Map its own `TelemetryConfig` mirror type to `ports.TelemetryConfig`
2. Call `NewProvider()` with the mapped config
3. Store and invoke the returned shutdown function

**Alternatives considered**:
- Creating a separate `internal/extproc/telemetry` package: Rejected — would duplicate ~300 lines of OTel provider setup code for no benefit.
- Passing `ports.TelemetryConfig` directly into `internal/extproc/config`: Rejected — the spec clarifies that `internal/extproc/config` must not import `internal/ports` to maintain package independence.

### R2: How should trace context be extracted from inbound request headers in ExtProc?

**Decision**: Manual extraction inside `processRequestHeaders` from the forwarded `HttpHeaders` entry list in the `RequestHeaders` message, not via gRPC stream interceptor or gRPC metadata.

**Rationale**: Per spec clarification (2026-04-20): "Request-level — one span per `RequestHeaders` message, created manually inside `processRequestHeaders`, not via a stream interceptor." The ExtProc protocol multiplexes multiple request phases over a single gRPC stream; a stream-level interceptor would create one span per stream, losing per-request granularity.

**Implementation approach**:
1. In `processRequestHeaders`, iterate over the `HttpHeaders` entry list from the `RequestHeaders` message
2. Build an `http.Header`-style multi-value carrier (preserving repeated header keys) — not a plain `map[string]string`
3. Call `otel.GetTextMapPropagator().Extract(ctx, carrier)` to get a context with the parent span
4. Start a new span using `otel.Tracer("extproc").Start(ctx, "extproc.token_exchange")`
5. Set span attributes: `resource.uri`, `outcome`, `error.type` (on failure)
6. End the span when `processRequestHeaders` returns

**Alternatives considered**:
- gRPC `StreamServerInterceptor`: Rejected per spec — creates stream-level spans, not per-request spans.
- gRPC `UnaryServerInterceptor`: Not applicable — ExtProc uses streaming RPC, not unary.

### R3: How should the HTTP client be instrumented for outbound trace context injection?

**Decision**: Wrap the `*http.Client` transport with `otelhttp.NewTransport(base)`.

**Rationale**: The identity broker already uses this pattern (builder.go, around line 320). `otelhttp.NewTransport` creates a round-tripper that:
1. Extracts the active span from the request context
2. Injects trace context headers into the outbound HTTP request
3. Creates a client span for the HTTP call

**Critical requirement**: The `doExchange` method currently uses `context.WithTimeout(context.Background(), ...)` for its HTTP request context. This creates a fresh context with no trace information. The context passed to `doExchange` must carry the active span from `processRequestHeaders` for trace propagation to work. This means:
1. `processRequestHeaders` must accept/pass a context to the exchanger
2. `Exchange(ctx, subjectToken, resourceURI)` must accept a context parameter
3. `doExchange` must use this context instead of `context.Background()`

This is a **breaking change to the `Exchanger` interface** — all tests using the mock exchanger will need the context parameter added.

**Alternatives considered**:
- Manual header injection via `otel.GetTextMapPropagator().Inject()`: Rejected — `otelhttp.NewTransport` handles this automatically and also creates client spans.

### R4: What metrics should be recorded and with what instrument types?

**Decision**: Two instruments following OTel semantic conventions:
1. `extproc.token_exchange.requests` — Counter (Int64) with `outcome` attribute
2. `extproc.token_exchange.duration` — Histogram (Float64, seconds) with `outcome` attribute

**Outcome attribute values**: `success`, `exchange_failure`, `circuit_open`, `invalid_resource`, `assertion_expired`, `passthrough` (no bearer token — informational only).

**Rationale**: Counter + histogram is the standard request metrics pattern. Breakdown by outcome enables dashboards showing error rates and latency per failure mode. Go runtime metrics (goroutines, memory, GC) are automatically handled by `NewProvider()` which calls `runtimemetrics.Start()`.

**Alternatives considered**:
- Adding per-service-URI breakdown: Rejected for cardinality concerns — `resource.uri` is already a span attribute for trace-level debugging.

### R5: How should the telemetry config mapping avoid circular imports?

**Decision**: Mirror type in `internal/extproc/config`; mapper function in `cmd/extproc-token-exchange/root.go`.

**Rationale**: Per spec clarification (2026-04-20): "Mirror type defined in `internal/extproc/config`; mapper function lives in `cmd/extproc-token-exchange/root.go`, keeping the config package free of `internal/ports` imports." The `cmd/` package is allowed to import both `internal/extproc/config` and `internal/ports`, so the mapping function naturally lives there.

**Implementation**:
```go
// In cmd/extproc-token-exchange/root.go
func mapTelemetryConfig(src extprocconfig.TelemetryConfig) ports.TelemetryConfig {
    return ports.TelemetryConfig{
        Enabled:            src.Enabled,
        ServiceName:        src.ServiceName,
        ResourceAttributes: src.ResourceAttributes,
        Traces: ports.TracesConfig{...},
        Metrics: ports.MetricsConfig{...},
        Logs: ports.LogsConfig{...},
        Exporter: ports.OTLPExporterConfig{...},
    }
}
```

### R6: How should the `processRequestHeaders` signature change to support context passing?

**Decision**: `processRequestHeaders` gains a `context.Context` parameter. The `Process` method derives the base context from `stream.Context()` (preserving gRPC cancellation and deadline semantics) and passes it to `processRequestHeaders`, which then layers the extracted trace context on top.

**Rationale**: Currently `processRequestHeaders` takes only `*extprocv3.HttpHeaders` and has no way to carry a trace context. The context must flow from:
1. `Process()` → derive base context from `stream.Context()` so gRPC cancellation propagates
2. `processRequestHeaders(ctx, headers)` extracts trace context from the incoming `HttpHeaders` key-value list using `otel.GetTextMapPropagator().Extract(ctx, carrier)`, layering trace parentage onto the stream context
3. `processRequestHeaders` creates a span from the enriched context
4. Pass span context to `exchanger.Exchange(ctx, subjectToken, resourceURI)`

The `Exchanger` interface changes from `Exchange(subjectToken, resourceURI string)` to `Exchange(ctx context.Context, subjectToken, resourceURI string)`. This is the standard Go idiom for context propagation.

### R7: Default propagators list for ExtProc

**Decision**: `["tracecontext", "ottrace", "b3multi", "baggage"]` — ExtProc adds `tracecontext` to its own default propagator list. The identity broker's defaults (`["ottrace", "b3multi", "baggage"]`) remain unchanged.

**Rationale**: Per spec clarification (2026-04-20): "Yes — add `tracecontext` to defaults." The identity broker's current defaults are `["ottrace", "b3multi", "baggage"]`. ExtProc adds `tracecontext` (W3C Trace Context) to its own defaults because agentgateway sends W3C `traceparent` headers, and the primary use case (US1) depends on W3C trace context extraction. This divergence is documented in the spec's Configuration Requirements section and in the quickstart guide.

### R8: Telemetry shutdown ordering in graceful shutdown

**Decision**: Shutdown order: `GracefulStop()` → telemetry flush (5s deadline) → exit.

**Rationale**: Per FR-007: "After `GracefulStop()` completes. The telemetry provider shutdown MUST use a 5-second context deadline." The gRPC server must stop accepting requests first, allowing in-flight spans to complete, then telemetry providers are flushed.

**Implementation**:
```go
grpcSrv.GracefulStop()
flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
defer flushCancel()
if err := shutdownTelemetry(flushCtx); err != nil {
    logger.Warn("telemetry flush error", "error", err)
}
```
