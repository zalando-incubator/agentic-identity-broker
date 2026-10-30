# Data Model: Configurable OpenTelemetry Support

**Branch**: `20260228-120000-opentelemetry-support`  
**Phase**: 1 — Design & Contracts  
**Date**: 2026-03-01

---

## Overview

This feature introduces no new domain entities, aggregates, value objects, or database tables. The data model for this feature is entirely the **configuration schema extension** added to the existing configuration system (`internal/ports/config.go`).

## Configuration Schema

### Entity: `Config` (extended)

The top-level `Config` struct gains a `Telemetry TelemetryConfig` field.

```go
// internal/ports/config.go (extension to existing Config struct)
type Config struct {
    Log              LogConfig              `mapstructure:"log"`
    Server           ServerConfig           `mapstructure:"server"`
    Storage          StorageConfig          `mapstructure:"storage"`
    ThirdPartyOAuth2 ThirdPartyOAuth2Config `mapstructure:"third_party_oauth2"`
    OAuth2AuthServer OAuth2AuthServerConfig `mapstructure:"oauth2_authorization_server"`
    TokenExchange    TokenExchangeConfig    `mapstructure:"token_exchange"`
    Security         SecurityConfig         `mapstructure:"security"`
    Encryption       EncryptionConfig       `mapstructure:"encryption"`
    Telemetry        TelemetryConfig        `mapstructure:"telemetry"`   // NEW
}
```

---

### Entity: `TelemetryConfig`

**Purpose**: Master configuration for all OpenTelemetry observability features.

| Field | Type | mapstructure key | Default | Validation |
|---|---|---|---|---|
| `Enabled` | `bool` | `enabled` | `false` | — |
| `ServiceName` | `string` | `service_name` | `"agentic-identity-broker"` | Non-empty when Enabled=true |
| `ResourceAttributes` | `map[string]string` | `resource_attributes` | `{}` | — |
| `Traces` | `TracesConfig` | `traces` | see TracesConfig | — |
| `Metrics` | `MetricsConfig` | `metrics` | see MetricsConfig | — |
| `Exporter` | `OTLPExporterConfig` | `exporter` | see OTLPExporterConfig | — |

**Zero value / default**: `Enabled: false` — all other validation skipped when disabled.

---

### Entity: `TracesConfig`

**Purpose**: Configuration for distributed tracing.

| Field | Type | mapstructure key | Default | Validation |
|---|---|---|---|---|
| `Enabled` | `bool` | `enabled` | `true` (when parent Enabled=true) | — |
| `SamplingRate` | `float64` | `sampling_rate` | `1.0` | Range 0.0–1.0 (inclusive) |
| `Propagators` | `[]string` | `propagators` | `["ottrace", "b3multi", "baggage"]` | Each item must be one of: `"ottrace"`, `"b3multi"`, `"b3"`, `"tracecontext"`, `"baggage"` |

**Sampling semantics**:
- `1.0` → `AlwaysSample` (via `TraceIDRatioBased(1.0)` wrapped in `ParentBased`)
- `0.0` → effectively `NeverSample`
- `0.1` → 10% of root spans sampled; child spans honour parent decision

---

### Entity: `MetricsConfig`

**Purpose**: Configuration for metrics collection and export.

| Field | Type | mapstructure key | Default | Validation |
|---|---|---|---|---|
| `Enabled` | `bool` | `enabled` | `true` (when parent Enabled=true) | — |
| `ExportInterval` | `time.Duration` | `export_interval` | `30s` | Must be > 0 when Enabled=true |

**Collected metrics (when enabled)**:
- HTTP: request count, request duration histogram, response size — via `otelchi` (per route, method, status code)
- Runtime: goroutine count, heap allocations, GC statistics — via `go.opentelemetry.io/contrib/instrumentation/runtime`

---

### Entity: `LogsConfig`

**Purpose**: Configuration for OTLP log export. Not all collectors support `opentelemetry.proto.collector.logs.v1.LogsService`; disabling prevents connection errors.

| Field | Type | mapstructure key | Default | Validation |
|---|---|---|---|---|
| `Enabled` | `bool` | `enabled` | `true` (when parent Enabled=true) | — |

---

### Entity: `OTLPExporterConfig`

**Purpose**: OTLP exporter connection parameters.

| Field | Type | mapstructure key | Default | Validation |
|---|---|---|---|---|
| `Protocol` | `string` | `protocol` | `"grpc"` | Must be `"grpc"`, `"http"`, or `"https"` when Enabled=true |
| `Endpoint` | `string` | `endpoint` | `""` | **Required** when Enabled=true; must be a valid host:port or URL |
| `Headers` | `map[string]string` | `headers` | `{}` | Values support `${ENV_VAR}` substitution |
| `Timeout` | `time.Duration` | `timeout` | `10s` | Must be > 0 |
| `Compression` | `string` | `compression` | `"none"` | Must be `"none"` or `"gzip"` |
| `Insecure` | `bool` | `insecure` | `false` | — |

**Protocol behaviour**:
- `grpc` → gRPC OTLP exporter; endpoint format: `host:port`
- `http` → HTTP OTLP exporter; endpoint format: `http://host:port`
- `https` → HTTP OTLP exporter with TLS; bare `host:port` auto-prefixed with `https://`

**TLS behaviour**:
- `Insecure: false` (default) → TLS enabled for gRPC / HTTPS for HTTP
- `Insecure: true` → TLS disabled for gRPC; a `WARN` is emitted at startup. For HTTP, the URL scheme controls TLS; for `https` protocol, `insecure: true` is contradictory (warning logged).

**Header secret handling**:
- `${ENV_VAR}` references in header values are expanded during config loading by the existing `expandEnvVars()` mechanism in `internal/config/loader.go`
- Example: `Authorization: "Bearer ${OTEL_EXPORTER_AUTH_TOKEN}"`

---

## Validation Rules

Implemented in `internal/config/validator.go` as `validateTelemetryConfig(cfg *ports.TelemetryConfig)`:

| Rule | Condition | Error Field | Error Message Pattern |
|---|---|---|---|
| Skip all validation | `cfg.Enabled == false` | — | — |
| Endpoint required | `cfg.Exporter.Endpoint == ""` | `telemetry.exporter.endpoint` | `"non-empty OTLP endpoint"` |
| Endpoint valid format | not a valid host:port or URL | `telemetry.exporter.endpoint` | `"valid gRPC host:port or HTTP URL"` |
| Protocol valid | not `"grpc"`, `"http"`, or `"https"` | `telemetry.exporter.protocol` | `"one of: grpc, http, https"` |
| Compression valid | not `"none"` or `"gzip"` | `telemetry.exporter.compression` | `"one of: none, gzip"` |
| Sampling rate in range | `< 0.0` or `> 1.0` | `telemetry.traces.sampling_rate` | `"value between 0.0 and 1.0"` |
| Export interval positive | `cfg.Metrics.Enabled && cfg.Metrics.ExportInterval <= 0` | `telemetry.metrics.export_interval` | `"positive duration"` |
| Exporter timeout positive | `cfg.Exporter.Timeout <= 0` | `telemetry.exporter.timeout` | `"positive duration"` |

---

## Default Values

```go
// internal/ports/config.go
func DefaultTelemetryConfig() TelemetryConfig {
    return TelemetryConfig{
        Enabled:     false,
        ServiceName: "agentic-identity-broker",
        ResourceAttributes: map[string]string{},
        Traces: TracesConfig{
            Enabled:      true,
            SamplingRate: 1.0,
            Propagators:  []string{"ottrace", "b3multi", "baggage"},
        },
        Metrics: MetricsConfig{
            Enabled:        true,
            ExportInterval: 30 * time.Second,
        },
        Logs: LogsConfig{
            Enabled: true,
        },
        Exporter: OTLPExporterConfig{
            Protocol:    "grpc",
            Endpoint:    "",
            Headers:     map[string]string{},
            Timeout:     10 * time.Second,
            Compression: "none",
            Insecure:    false,
        },
    }
}
```

---

## Environment Variable Mapping

All telemetry config keys support the `IDENTITY_BROKER_*` env var naming convention:

| Config Path | Environment Variable |
|---|---|
| `telemetry.enabled` | `IDENTITY_BROKER_TELEMETRY_ENABLED` |
| `telemetry.service_name` | `IDENTITY_BROKER_TELEMETRY_SERVICE_NAME` |
| `telemetry.traces.enabled` | `IDENTITY_BROKER_TELEMETRY_TRACES_ENABLED` |
| `telemetry.traces.sampling_rate` | `IDENTITY_BROKER_TELEMETRY_TRACES_SAMPLING_RATE` |
| `telemetry.metrics.enabled` | `IDENTITY_BROKER_TELEMETRY_METRICS_ENABLED` |
| `telemetry.metrics.export_interval` | `IDENTITY_BROKER_TELEMETRY_METRICS_EXPORT_INTERVAL` |
| `telemetry.exporter.protocol` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_PROTOCOL` |
| `telemetry.exporter.endpoint` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT` |
| `telemetry.exporter.timeout` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_TIMEOUT` |
| `telemetry.exporter.insecure` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_INSECURE` |

---

## OTel Provider Runtime Lifecycle

The following runtime objects are created in `internal/app/builder.go` and managed by a shutdown function:

```
TelemetryConfig (from config) 
    └─> internal/adapters/telemetry/provider.go
            └─> resource.Resource (service.name + resource_attributes)
            └─> otlptracegrpc|otlptracehttp.Exporter → sdktrace.TracerProvider
            │       registered: otel.SetTracerProvider(tp)
            │       propagators: otel.SetTextMapPropagator(...)
            └─> otlpmetricgrpc|otlpmetrichttp.Exporter → metric.MeterProvider
            │       registered: otel.SetMeterProvider(mp)
            │       runtime metrics: runtime.Start(...)
            └─> otlploggrpc.Exporter → sdklog.LoggerProvider
            │       registered: global log provider (if OTel logs global registry used)
            └─> otelslog.Handler → slog.Logger (wrapped, replaces app logger)

Shutdown sequence (during graceful shutdown):
    1. HTTP servers drain (existing shutdown mechanism)
    2. tp.Shutdown(ctx)     ← flush pending spans
    3. mp.Shutdown(ctx)     ← flush pending metrics
    4. lp.Shutdown(ctx)     ← flush pending log records
```

---

## Security Model

Prohibited span attributes (enforced by code review per SR-001):
- Authentication tokens, API keys, JWE state values
- Encryption keys, branch key IDs, KMS key ARNs
- User credentials (passwords, secrets)
- PII: user email addresses, full names (agent IDs are permissible as they are non-personal)
- Request/response body content

Permitted span attributes:
- Standard OTel HTTP semantic conventions: `http.method`, `http.route`, `http.status_code`, `http.request_content_length`
- Standard OTel DB conventions: `db.system`, `db.operation`
- Custom operational attributes: `agent.id` (UUID), `service.id` (UUID), `oauth2.grant_type`
