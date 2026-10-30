# ADR 20260421-120000: ExtProc Telemetry Shared Dependency

**Status**: Accepted
**Date**: 2026-04-21
**Feature**: 20260417-120000-extproc-otel

---

## Context

The `cmd/extproc-token-exchange/` binary requires distributed tracing and observability via OpenTelemetry (OTel). The broker already has an established telemetry adapter in `internal/adapters/telemetry` with a `NewProvider()` function (from ADR 20260301-120000).

Currently, `cmd/extproc-token-exchange/` imports only packages under `internal/extproc/*`. To reuse the broker's telemetry infrastructure, the ExtProc binary must be permitted to import:

1. `internal/ports` — specifically the `TelemetryConfig` type
2. `internal/adapters/telemetry` — specifically the `NewProvider()` factory function

This represents a new cross-boundary dependency: the standalone ExtProc binary importing from the broker's inner layers.

---

## Decision

Allow `cmd/extproc-token-exchange/` to import `internal/ports` and `internal/adapters/telemetry` for OTel provider initialization. The mapping between ExtProc-specific config (`internal/extproc/config.TelemetryConfig`) and broker-shared types (`ports.TelemetryConfig`) occurs in the `cmd/` composition layer.

**Key constraints**:
- `internal/extproc/*` packages remain free of imports from `internal/ports` or `internal/adapters/telemetry`.
- The config domain type (`internal/extproc/config.TelemetryConfig`) is separate from the broker's shared type; a thin adapter function in `cmd/extproc-token-exchange/main.go` bridges the two.
- The `cmd/` layer is the outermost, appropriate place for such wiring and cross-boundary composition.

---

## Rationale

1. **DRY principle**: The OTel provider lifecycle (initialization, shutdown, span/meter/log exporters) is complex and well-tested in the broker. Reimplementing it in ExtProc would create maintenance burden and inconsistency.

2. **Shared observability**: Both the broker and ExtProc benefit from unified telemetry exporters (e.g., same Jaeger/OTEL Collector endpoint, same sampling policies). Reusing `NewProvider()` ensures consistency.

3. **Composition layer ownership**: The `cmd/` layer is the appropriate place for cross-module composition. It is already responsible for wiring domain, ports, and adapters. Importing `internal/ports` and `internal/adapters` from `cmd/` is consistent with existing architecture patterns (ADR 20251215-120200: Builder pattern in `cmd/agentic-identity-broker/root.go` imports `internal/app`).

4. **No domain contamination**: The ExtProc domain model remains independent. Config domain types (`internal/extproc/config`) are specific to ExtProc; the adapter in `cmd/` performs the mapping. This preserves the isolation constraint that `internal/extproc` packages do not depend on broker internals.

5. **Testability**: The `cmd/` wiring layer is thin and primarily testable at E2E and integration levels. Unit tests of ExtProc domain (token exchange, caching, gRPC processing) remain isolated and do not depend on telemetry.

---

## Consequences

**Positive**:
- ExtProc inherits stable, tested telemetry infrastructure without duplication.
- Unified telemetry configuration and shutdown across both binaries.
- Clear composition boundary: all cross-module wiring happens in `cmd/`.

**Negative**:
- `cmd/extproc-token-exchange/` adds a new dependency on broker internals. If ExtProc grows into a separate project, this coupling must be addressed (e.g., extract telemetry into a shared neutral module, or provide telemetry as a sidecar/library).
- Developers must understand that `cmd/` is a composition layer that may import from `internal/`, whereas pure `internal/extproc/*` packages may not.

---

## Alternatives Considered

1. **Duplicate telemetry in ExtProc**: Define a separate `internal/extproc/telemetry` package mirroring the broker's adapter. Rejected because it creates maintenance burden, inconsistency, and forgoes the benefit of shared exporters and sampling policies.

2. **Extract telemetry into `pkg/` or a separate module**: Move `internal/adapters/telemetry` to `pkg/telemetry` to signal that it is a reusable utility. Rejected because it requires significant refactoring and is premature if ExtProc remains a single-binary service. Can be revisited if ExtProc becomes a standalone project.

3. **Sidecar telemetry agent**: Deploy a separate OTel collector sidecar and have both binaries send telemetry via gRPC/HTTP. Rejected because it adds infrastructure complexity and does not solve the in-process initialization and shutdown coordination needed for the broker's graceful shutdown sequence (ADR 20260301-120000).

---

## Implementation Notes

**Config mapping function** (in `cmd/extproc-token-exchange/root.go`):

```go
// mapTelemetryConfig converts ExtProc's local TelemetryConfig to the ports.TelemetryConfig
// interface type for use with the telemetry adapter. This function lives in cmd/ to keep
// internal/extproc/config free of imports from internal/ports (architecture boundary).
func mapTelemetryConfig(c extprocconfig.TelemetryConfig) ports.TelemetryConfig {
    return ports.TelemetryConfig{
        Enabled:            c.Enabled,
        ServiceName:        c.ServiceName,
        ResourceAttributes: c.ResourceAttributes,
        Traces: ports.TracesConfig{
            Enabled:      c.Traces.Enabled,
            SamplingRate: c.Traces.SamplingRate,
            Propagators:  c.Traces.Propagators,
        },
        Metrics: ports.MetricsConfig{
            Enabled:        c.Metrics.Enabled,
            ExportInterval: c.Metrics.ExportInterval,
        },
        Logs: ports.LogsConfig{
            Enabled: c.Logs.Enabled,
        },
        Exporter: ports.OTLPExporterConfig{
            Protocol:    ports.OTLPProtocol(c.Exporter.Protocol),
            Endpoint:    c.Exporter.Endpoint,
            Headers:     c.Exporter.Headers,
            Timeout:     c.Exporter.Timeout,
            Insecure:    c.Exporter.Insecure,
            Compression: ports.OTLPCompression(c.Exporter.Compression),
        },
    }
}
```

**Provider initialization** (in `cmd/extproc-token-exchange/root.go`, after logger init):

```go
telemetryCfg := mapTelemetryConfig(cfg.Telemetry)
shutdown, err := telemetry.NewProvider(context.Background(), telemetryCfg, logger)
if err != nil {
    return fmt.Errorf("failed to initialize telemetry: %w", err)
}
telemetryShutdown = shutdown

// Register slog-to-OTel bridge when telemetry AND logs are both enabled
if cfg.Telemetry.Logs.Enabled {
    otelHandler := otelslog.NewHandler(cfg.Telemetry.ServiceName,
        otelslog.WithLoggerProvider(global.GetLoggerProvider()))
    logger = slog.New(telemetry.NewMultiHandler(logger.Handler(), otelHandler))
    slog.SetDefault(logger)
}
```

The ExtProc domain and gRPC server remain unaware of telemetry initialization; instrumentation uses the global OTel tracer pattern (ADR 20260301-120000) and is decoupled from provider lifecycle.
