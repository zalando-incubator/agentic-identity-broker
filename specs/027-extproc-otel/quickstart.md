# Quickstart: OpenTelemetry Support for ExtProc Token Exchange

**Branch**: `027-extproc-otel` | **Date**: 2026-04-20

## What This Feature Does

Adds OpenTelemetry instrumentation (distributed tracing, metrics, structured log correlation) to the ExtProc Token Exchange gRPC service, enabling end-to-end observability from agentgateway through ExtProc to the Identity Broker.

## Enable Telemetry

Add the `telemetry` block to your ExtProc YAML configuration:

```yaml
# config.extproc.yaml
telemetry:
  enabled: true
  service_name: extproc-token-exchange
  traces:
    enabled: true
    sampling_rate: 1.0
    propagators:
      - tracecontext
      - ottrace
      - b3multi
      - baggage
  metrics:
    enabled: true
    export_interval: 30s
  logs:
    enabled: true
  exporter:
    protocol: grpc
    endpoint: otel-collector.monitoring.svc:4317
    insecure: false
```

Or via environment variables:

```bash
export EXTPROC_TELEMETRY_ENABLED=true
export EXTPROC_TELEMETRY_EXPORTER_ENDPOINT=otel-collector:4317
export EXTPROC_TELEMETRY_EXPORTER_PROTOCOL=grpc
```

## Verify End-to-End Tracing

1. Configure both ExtProc and the Identity Broker with the same OTLP collector endpoint. **Important**: Ensure the Identity Broker's `telemetry.traces.propagators` list includes `tracecontext` (W3C Trace Context) to preserve trace context across the ExtProc-to-broker hop. The default for identity broker includes this, but verify your config has `tracecontext` in the propagators list if you have customized it.
2. Send a request through agentgateway with a W3C `traceparent` header
3. Open your trace UI (Jaeger, Grafana Tempo, etc.)
4. Search for the trace ID — you should see four connected spans across three services:
   - **agentgateway** (root span)
   - **extproc-token-exchange** → `extproc.token_exchange` (child of agentgateway)
   - **extproc-token-exchange** → HTTP client span (child of `extproc.token_exchange`, created by `otelhttp.NewTransport`)
   - **agentic-identity-broker** (child of the HTTP client span)

ExtProc extracts HTTP propagation fields first, then applies valid gRPC metadata trace context in preference to HTTP trace context. With the `baggage` propagator enabled, HTTP baggage is preserved when the gRPC metadata carries no baggage.

## Key Implementation Files

| File | What Changed |
|------|-------------|
| `internal/extproc/config/config.go` | New `TelemetryConfig` mirror type + defaults |
| `internal/extproc/config/loader.go` | Viper bindings for `telemetry.*` keys |
| `internal/extproc/config/validate.go` | Telemetry validation rules |
| `cmd/extproc-token-exchange/root.go` | Telemetry init, config mapping, shutdown, slog bridge |
| `internal/extproc/server/server.go` | Per-request span creation in `processRequestHeaders` |
| `internal/extproc/server/exchanger.go` | HTTP client wrapped with `otelhttp.NewTransport` |
| `examples/config/extproc-telemetry.yaml` | Configuration example |

## Configuration Parity with Identity Broker

The ExtProc `telemetry:` block uses the **same schema** as the Identity Broker's telemetry configuration. TLS certificate configuration (CA bundles, client certificates) is not part of this schema — both services delegate TLS cert paths to the OTel SDK's standard environment variables (`OTEL_EXPORTER_OTLP_CERTIFICATE`, `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE`, `OTEL_EXPORTER_OTLP_CLIENT_KEY`). The known app-level config differences are:

| Parameter | Identity Broker | ExtProc |
|-----------|----------------|---------|
| `service_name` default | `agentic-identity-broker` | `extproc-token-exchange` |
| Env var prefix | `IDENTITY_BROKER_TELEMETRY_*` | `EXTPROC_TELEMETRY_*` |
| Default propagators | `ottrace, b3multi, baggage` | `tracecontext, ottrace, b3multi, baggage` |

You can copy the Identity Broker's `telemetry:` block and change the `service_name`. **Important**: If the copied block includes an explicit `propagators` list that omits `tracecontext`, you must add it — ExtProc requires `tracecontext` for W3C `traceparent` propagation from agentgateway. If the `propagators` key is omitted entirely, ExtProc's defaults (`tracecontext, ottrace, b3multi, baggage`) apply automatically.

## Graceful Shutdown

On SIGTERM/SIGINT:
1. gRPC `GracefulStop()` completes in-flight streams
2. Telemetry providers flush pending data (5-second deadline)
3. Flush errors are logged but don't affect exit code
4. Service exits cleanly

## Metrics Emitted

| Metric | Type | Attributes |
|--------|------|------------|
| `extproc.token_exchange.requests` | Counter | `outcome` |
| `extproc.token_exchange.duration` | Histogram (seconds) | `outcome` |

Outcome values: `success`, `exchange_failure`, `circuit_open`, `invalid_resource`, `assertion_expired`, `passthrough`.

Go runtime metrics (goroutines, memory, GC) are automatically exported when metrics are enabled.

## Troubleshooting

**No spans appearing?**
- Check `telemetry.enabled: true` and `telemetry.traces.enabled: true`
- Verify the OTLP endpoint is reachable from the ExtProc container
- Check `telemetry.traces.sampling_rate` is > 0.0

**Broken trace chain?**
- Ensure agentgateway sends W3C `traceparent` headers
- Verify `tracecontext` is in the `propagators` list
- Check that the Identity Broker also has `tracecontext` in its propagators

**Startup failure?**
- Look for validation errors: invalid endpoint URL, unsupported protocol, out-of-range sampling rate
- All telemetry config errors produce actionable messages identifying the invalid field
