# Data Model: OpenTelemetry Support for ExtProc Token Exchange

**Branch**: `20260417-120000-extproc-otel` | **Date**: 2026-04-20

## Overview

This feature introduces **no new domain entities**. It is a pure infrastructure/observability enhancement. The only data structures added are configuration types that mirror existing `ports.TelemetryConfig`.

## Configuration Types (internal/extproc/config)

### TelemetryConfig

Mirror of `ports.TelemetryConfig` — structurally identical, defined in `internal/extproc/config` to avoid importing `internal/ports`.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `bool` | `false` | Master toggle for all telemetry |
| `service_name` | `string` | `extproc-token-exchange` | Service name in telemetry data |
| `resource_attributes` | `map[string]string` | `{}` | Additional OTel resource attributes |
| `traces` | `TracesConfig` | see below | Distributed tracing config |
| `metrics` | `MetricsConfig` | see below | Metrics config |
| `logs` | `LogsConfig` | see below | OTLP log export config |
| `exporter` | `OTLPExporterConfig` | see below | Exporter connection config |

### TracesConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `bool` | `true` | Enable trace collection |
| `sampling_rate` | `float64` | `1.0` | Ratio-based sampling (0.0–1.0) |
| `propagators` | `[]string` | `["tracecontext","ottrace","b3multi","baggage"]` | Propagation formats |

### MetricsConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `bool` | `true` | Enable metrics export |
| `export_interval` | `time.Duration` | `30s` | Metrics export frequency |

### LogsConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `bool` | `true` | Enable OTLP log export |

### OTLPExporterConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `protocol` | `string` | `grpc` | Transport: `grpc`, `http`, `https` |
| `endpoint` | `string` | `""` | Collector endpoint (required when enabled) |
| `headers` | `map[string]string` | `{}` | Additional export headers |
| `timeout` | `time.Duration` | `10s` | Export timeout |
| `insecure` | `bool` | `false` | Disable TLS (gRPC only) |
| `compression` | `string` | `none` | Compression: `none`, `gzip` |

## OTel Instruments

### Span: `extproc.token_exchange`

Created per `RequestHeaders` message in `processRequestHeaders`.

| Attribute | Type | Description |
|-----------|------|-------------|
| `resource.uri` | string | Full resource URI (query string excluded) |
| `outcome` | string | One of: `success`, `exchange_failure`, `circuit_open`, `invalid_resource`, `assertion_expired`, `passthrough` |
| `error.type` | string | Error type classification (only on failure) |

### Counter: `extproc.token_exchange.requests`

| Attribute | Type | Values |
|-----------|------|--------|
| `outcome` | string | `success`, `exchange_failure`, `circuit_open`, `invalid_resource`, `assertion_expired`, `passthrough` |

### Histogram: `extproc.token_exchange.duration`

| Attribute | Type | Values | Unit |
|-----------|------|--------|------|
| `outcome` | string | Same as counter | seconds |

## Interface Changes

### Exchanger Interface (breaking change)

```go
// Before
type Exchanger interface {
    Exchange(subjectToken, resourceURI string) (string, error)
    Shutdown()
}

// After — context parameter added for trace propagation
type Exchanger interface {
    Exchange(ctx context.Context, subjectToken, resourceURI string) (string, error)
    Shutdown()
}
```

### processRequestHeaders Signature

```go
// Before
func (s *Server) processRequestHeaders(headers *extprocv3.HttpHeaders) *extprocv3.ProcessingResponse

// After — context parameter for span creation and propagation
func (s *Server) processRequestHeaders(ctx context.Context, headers *extprocv3.HttpHeaders) *extprocv3.ProcessingResponse
```

## Relationships

```
ExtProc Config (TelemetryConfig)
    │
    ├── mapped to ──→ ports.TelemetryConfig (via mapTelemetryConfig in cmd/)
    │                      │
    │                      └── passed to ──→ telemetry.NewProvider()
    │                                            │
    │                                            ├── TracerProvider (global)
    │                                            ├── MeterProvider (global)
    │                                            ├── LoggerProvider (global)
    │                                            └── shutdown func
    │
    └── processRequestHeaders
            │
            ├── Extract trace context from incoming HttpHeaders entry list
            │   (via http.Header multi-value carrier, preserving repeated keys)
            ├── Start span "extproc.token_exchange"
            ├── Record metrics (counter + histogram)
            └── Pass context to Exchanger.Exchange(ctx, ...)
                    │
                    └── otelhttp.NewTransport injects trace context
                        into outbound HTTP request to Identity Broker
```
