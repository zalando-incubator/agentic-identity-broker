# Quickstart: Configurable OpenTelemetry Support

**Branch**: `20260228-120000-opentelemetry-support`  
**Date**: 2026-03-01

This guide explains how to enable and configure OpenTelemetry distributed tracing and metrics on the Agentic Identity Broker.

---

## Prerequisites

- A running OpenTelemetry Collector (or compatible OTLP endpoint, e.g., Grafana Tempo, Jaeger with OTLP receiver, Honeycomb, Datadog)
- Access to the application's configuration file (default: `config.yaml`) or environment variables

---

## Default Behaviour

OpenTelemetry is **disabled by default**. When disabled:
- No OTel packages are initialized
- No network connections to a collector are made
- Zero performance overhead

---

## Step 1: Enable Telemetry

Add a `telemetry` section to your `config.yaml`:

```yaml
telemetry:
  enabled: true
  service_name: agentic-identity-broker   # appears in all exported telemetry
```

That's the minimum required configuration. All other settings have production-safe defaults.

---

## Step 2: Configure the OTLP Exporter

### Option A: gRPC (default, recommended for internal networks)

```yaml
telemetry:
  enabled: true
  service_name: agentic-identity-broker

  exporter:
    protocol: grpc
    endpoint: otel-collector.monitoring.svc:4317   # host:port — no scheme for gRPC
    timeout: 10s
    insecure: false   # TLS enabled by default
```

### Option B: HTTP

```yaml
telemetry:
  enabled: true
  service_name: agentic-identity-broker

  exporter:
    protocol: http
    endpoint: https://otel-collector.monitoring.svc:4318   # full URL for HTTP
    timeout: 10s
    insecure: false
```

### Exporter Authentication

Use `${ENV_VAR}` substitution to avoid storing credentials in config files:

```yaml
telemetry:
  exporter:
    headers:
      Authorization: "Bearer ${OTEL_EXPORTER_AUTH_TOKEN}"
      X-Honeycomb-Team: "${HONEYCOMB_API_KEY}"
```

Set the env var at runtime:
```bash
export OTEL_EXPORTER_AUTH_TOKEN="your-token-here"
```

### Development / Insecure Mode

**Only for development or testing. Never use in production.**

```yaml
telemetry:
  exporter:
    protocol: grpc
    endpoint: localhost:4317
    insecure: true   # disables TLS; triggers startup warning log
```

---

## Step 3: Configure Resource Attributes

Resource attributes appear on all exported telemetry and help identify the deployment:

```yaml
telemetry:
  enabled: true
  service_name: agentic-identity-broker
  resource_attributes:
    deployment.environment: production
    service.version: "1.2.0"
    service.namespace: identity
    cloud.region: eu-west-1
```

---

## Step 4: Configure Traces

```yaml
telemetry:
  traces:
    enabled: true       # default: true (when telemetry.enabled=true)
    sampling_rate: 0.1  # 10% of traces sampled in production; 1.0 = all
    propagators:
      - ottrace         # OpenTracing interop (ot-tracer-* headers)
      - b3multi         # Zipkin B3 multiple headers (X-B3-TraceId, X-B3-SpanId, …)
      - baggage         # W3C Baggage
      # Also available: tracecontext (W3C), b3 (single header)
```

**Propagator notes**:
- Propagators are registered unconditionally when telemetry is enabled, even if `traces.enabled` is false — this ensures inbound trace context is always forwarded.
- The default set (OTTrace + B3 multi + Baggage) ensures interoperability with OpenTracing, Zipkin, and Baggage-aware systems.

**Sampling guidance**:
- Development: `sampling_rate: 1.0` (sample everything)
- Staging: `sampling_rate: 0.5` (50%)
- Production (high traffic): `sampling_rate: 0.1` (10%)
- Debugging: temporarily set `sampling_rate: 1.0`

---

## Step 5: Configure Metrics

```yaml
telemetry:
  metrics:
    enabled: true       # default: true (when telemetry.enabled=true)
    export_interval: 30s
```

**Collected metrics**:
- `http.server.request.duration` — request latency histogram (per method, route, status code)
- `http.server.request.count` — request counter
- `http.server.response.size` — response size histogram
- `process.runtime.go.goroutines` — goroutine count
- `process.runtime.go.mem.heap_alloc` — heap allocations
- `process.runtime.go.gc.count` — GC runs

---

## Full Production Example

```yaml
telemetry:
  enabled: true
  service_name: agentic-identity-broker
  resource_attributes:
    deployment.environment: production
    service.version: "1.0.0"
    cloud.region: eu-west-1

  traces:
    enabled: true
    sampling_rate: 0.1
    propagators:
      - ottrace
      - b3multi
      - baggage

  metrics:
    enabled: true
    export_interval: 30s

  logs:
    enabled: true  # set false if collector lacks LogsService support

  exporter:
    protocol: grpc
    endpoint: otel-collector.monitoring.svc:4317
    headers:
      Authorization: "Bearer ${OTEL_EXPORTER_AUTH_TOKEN}"
    timeout: 10s
    compression: gzip  # recommended for production
    insecure: false
```

---

## Environment Variable Reference

All settings can be set via environment variables (useful for Kubernetes deployments):

```bash
IDENTITY_BROKER_TELEMETRY_ENABLED=true
IDENTITY_BROKER_TELEMETRY_SERVICE_NAME=agentic-identity-broker
IDENTITY_BROKER_TELEMETRY_TRACES_ENABLED=true
IDENTITY_BROKER_TELEMETRY_TRACES_SAMPLING_RATE=0.1
IDENTITY_BROKER_TELEMETRY_METRICS_ENABLED=true
IDENTITY_BROKER_TELEMETRY_METRICS_EXPORT_INTERVAL=30s
IDENTITY_BROKER_TELEMETRY_LOGS_ENABLED=true
IDENTITY_BROKER_TELEMETRY_EXPORTER_PROTOCOL=grpc
IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT=otel-collector:4317
IDENTITY_BROKER_TELEMETRY_EXPORTER_TIMEOUT=10s
IDENTITY_BROKER_TELEMETRY_EXPORTER_COMPRESSION=gzip
IDENTITY_BROKER_TELEMETRY_EXPORTER_INSECURE=false
```

---

## Traces Emitted

When tracing is enabled, the following spans are created per request:

| Span Name | Source | Key Attributes |
|---|---|---|
| `GET /api/consent/agents` | `otelchi` HTTP middleware | `http.method`, `http.route`, `http.status_code`, `http.server_name` |
| `GET /oauth2/authorize` | `otelchi` HTTP middleware | same as above |
| `storage.get.agent` | Storage adapter | `db.system=postgresql`, `db.operation=SELECT` |
| `storage.list.userGrants` | Storage adapter | `db.system=postgresql`, `db.operation=SELECT` |
| `encryption.encrypt` | AWS encryption adapter | `encryption.key_type=aws_kms` |
| `encryption.decrypt` | AWS encryption adapter | `encryption.key_type=aws_kms` |
| `jwks.fetch` | JWKS adapter | `url.full` |
| `oauth2.token_exchange` | Upstream HTTP adapter | `http.method`, `http.status_code` |

---

## Log Correlation

When telemetry is enabled, structured log entries automatically include `trace_id` and `span_id` fields that correlate with the active span context. This allows pivoting from a log entry to its corresponding trace span in your observability platform.

Example log entry (JSON format):
```json
{
  "time": "2026-03-01T12:00:00.123Z",
  "level": "INFO",
  "msg": "grant created",
  "agent_id": "abc-123",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7"
}
```

---

## Graceful Degradation

If the OTLP collector is unreachable:
- **Startup**: Application starts normally; logs a `WARN` about the unreachable collector
- **During operation**: Requests continue to be served normally; OTel SDK buffers export failures internally and retries
- **Recovery**: When the collector becomes available, export resumes automatically

The identity broker **never** fails requests due to telemetry export failures.

---

## Validation Errors

If the telemetry configuration is invalid, the application will refuse to start with a clear error:

```
FATAL: configuration validation failed: telemetry.exporter.endpoint: expected non-empty OTLP endpoint, got ""
FATAL: configuration validation failed: telemetry.exporter.protocol: expected one of: grpc, http, got "jaeger"
FATAL: configuration validation failed: telemetry.traces.sampling_rate: expected value between 0.0 and 1.0, got 1.5
```

---

## Disabling Individual Signals

Traces and metrics can be independently toggled:

```yaml
# Traces only (no metrics)
telemetry:
  enabled: true
  traces:
    enabled: true
  metrics:
    enabled: false

# Metrics only (no traces)
telemetry:
  enabled: true
  traces:
    enabled: false
  metrics:
    enabled: true
```
