# Feature Specification: Configurable OpenTelemetry Support

**Feature Branch**: `20260228-120000-opentelemetry-support`  
**Created**: 2026-02-28  
**Status**: Draft  
**Input**: User description: "Let's add configurable opentelemetry support"

## Clarifications

### Session 2026-03-01

- Q: How should internal span propagation (FR-012) be implemented architecturally? → A: Context-based propagation — instrument adapters by extracting the tracer from `context.Context` inside adapter methods (no port interface signature changes required if context is already threaded).
- Q: Which HTTP instrumentation library should be used to capture chi route patterns in trace spans? → A: `github.com/riandyrn/otelchi` — a chi-specific OTel middleware that correctly captures chi v5 route patterns (e.g., `/agents/{id}`) as span names.
- Q: Should OpenTelemetry provider setup be modelled as a hexagonal port or wired entirely in the app layer? → A: No port — wire OTel provider setup entirely in `builder.go` as an app-layer concern; no `TelemetryProvider` interface is defined in `internal/ports/`.
- Q: What enforcement mechanism should prevent PII/secrets from appearing in span attributes (SR-001)? → A: Trust-the-developer — no automated allowlist or span processor filter; rely on code review with explicitly documented prohibited attribute categories in the spec.
- Q: Should log correlation (trace ID injection into structured log entries) be in scope for this feature? → A: In scope — implement via a library (`go.opentelemetry.io/contrib/bridges/otelslog`) that bridges the existing `slog` logger to the OTel logs pipeline, automatically injecting `trace_id` and `span_id` into log records emitted within a traced request.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Enable Distributed Tracing for Request Debugging (Priority: P1)

As a platform operator running the Agentic Identity Broker in production, I need the system to emit distributed traces for every inbound request across both end-user and admin servers, so that I can diagnose latency issues, trace failures through the OAuth2 and token exchange flows, and correlate requests end-to-end.

**Why this priority**: Distributed tracing is the highest-value observability signal for a multi-component identity broker. Without it, debugging cross-service authorization flows (OAuth2 callbacks, token exchange, encryption operations) requires manual log correlation, which is error-prone and slow. This is the foundational capability that all other observability features build upon.

**Independent Test**: Can be fully tested by configuring an OTLP endpoint in the application configuration, sending a request to the end-user server, and verifying that a trace span appears at the configured collector with correct attributes (service name, request method, path, status code).

**Acceptance Scenarios**:

1. **Given** OpenTelemetry is enabled with a valid OTLP endpoint configured, **When** a request is made to the end-user server, **Then** a trace is exported to the configured endpoint containing at least the service name, request method, route, and response status code.
2. **Given** OpenTelemetry is enabled with a valid OTLP endpoint configured, **When** a request is made to the admin server, **Then** a trace is exported to the configured endpoint with distinct service identification for the admin server.
3. **Given** OpenTelemetry is enabled and a request triggers internal operations (storage queries, encryption calls, JWKS key fetches, upstream OAuth2 calls), **When** the request completes, **Then** the trace contains child spans for significant internal operations, allowing operators to identify slow subsystems.
4. **Given** OpenTelemetry is disabled (default), **When** a request is made to either server, **Then** no telemetry data is emitted and no performance overhead is incurred beyond a no-op check.

---

### User Story 2 - Expose Runtime Metrics for Capacity Planning (Priority: P2)

As a platform operator, I need the system to expose runtime metrics (request rates, error rates, latency distributions, active connections) so that I can set up dashboards and alerts for capacity planning and incident detection.

**Why this priority**: Metrics enable proactive monitoring and alerting. While slightly lower priority than tracing (which is needed for debugging), metrics are essential for production health monitoring and capacity planning. Metrics complement traces by providing aggregate views.

**Independent Test**: Can be fully tested by configuring a metrics exporter endpoint, generating a series of requests to both servers, and verifying that metric data (request count, latency histogram) appears at the configured collector or scrape endpoint.

**Acceptance Scenarios**:

1. **Given** OpenTelemetry metrics are enabled, **When** multiple requests are processed, **Then** the system exports request count, request duration, and response size metrics broken down by method, route, and status code.
2. **Given** OpenTelemetry metrics are enabled, **When** the system is running, **Then** runtime metrics (goroutine count, memory usage, GC statistics) are available for collection.
3. **Given** OpenTelemetry metrics are disabled but tracing is enabled, **When** the system processes requests, **Then** only trace data is exported and no metric data is emitted.

---

### User Story 3 - Configurable Telemetry Without Code Changes (Priority: P1)

As an operator deploying the identity broker across development, staging, and production environments, I need all OpenTelemetry settings to be configurable via YAML configuration files and environment variables, so that I can tune telemetry behavior per environment without rebuilding the application.

**Why this priority**: Configuration-driven design is a core principle of this application (Constitution Principle VII). Operators must be able to enable, disable, and tune telemetry per environment. Development environments may disable telemetry entirely, while production sends traces to a centralized collector.

**Independent Test**: Can be fully tested by starting the application with different telemetry configurations (disabled, OTLP gRPC, OTLP HTTP) and verifying correct behavior for each configuration without code changes.

**Acceptance Scenarios**:

1. **Given** no `telemetry` section exists in the configuration file, **When** the application starts, **Then** telemetry is disabled and the application starts normally with no telemetry overhead.
2. **Given** the telemetry configuration specifies an OTLP gRPC endpoint, **When** the application starts, **Then** traces and metrics are exported via gRPC to the specified endpoint.
3. **Given** the telemetry configuration specifies an OTLP HTTP endpoint, **When** the application starts, **Then** traces and metrics are exported via HTTP to the specified endpoint.
4. **Given** the telemetry configuration sets a custom service name and resource attributes, **When** telemetry data is exported, **Then** the custom service name and attributes appear on all exported telemetry.
5. **Given** the OTLP endpoint is provided via environment variable `IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT`, **When** the application starts, **Then** the environment variable value is used, consistent with the existing configuration precedence rules.

---

### User Story 4 - Graceful Degradation When Collector is Unavailable (Priority: P2)

As an operator, I need the identity broker to continue functioning normally if the telemetry collector becomes unreachable, so that a monitoring infrastructure failure does not become an application outage.

**Why this priority**: Production resilience requires that optional observability features never cause application failures. The identity broker must prioritize its core mission (identity brokering) over telemetry export.

**Independent Test**: Can be fully tested by configuring an unreachable OTLP endpoint, sending requests, and verifying the application responds normally without errors or excessive latency.

**Acceptance Scenarios**:

1. **Given** OpenTelemetry is enabled with an unreachable OTLP endpoint, **When** the application starts, **Then** the application starts successfully and logs a warning about the unreachable collector.
2. **Given** OpenTelemetry is enabled and the collector becomes unreachable during operation, **When** requests continue to arrive, **Then** the application continues serving requests with normal latency and logs periodic warnings about export failures without flooding the log.
3. **Given** OpenTelemetry is enabled and the previously unreachable collector becomes available, **When** new requests arrive, **Then** telemetry export resumes automatically without operator intervention.

---

### Edge Cases

- What happens when the configured OTLP endpoint URL is malformed? The application MUST reject the configuration at startup with a clear validation error.
- What happens when `telemetry.exporter.protocol` is set to an unrecognized value (neither `grpc` nor `http`)? The application MUST fail fast at startup with a clear validation error indicating the allowed values.
- What happens when trace sampling rate is set to 0? No traces should be collected or exported, but the instrumentation overhead should be minimal (no-op sampler).
- What happens during graceful shutdown while telemetry data is being exported? The system MUST flush pending telemetry data within the shutdown timeout before terminating.
- What happens when the telemetry configuration changes between restarts? The new configuration takes effect on next startup; there is no hot-reload for telemetry settings.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support enabling and disabling OpenTelemetry telemetry (traces and metrics) via configuration, with telemetry disabled by default. OpenTelemetry provider initialisation MUST be wired entirely in `internal/app/builder.go` as an app-layer concern; no port interface for the telemetry provider is defined in `internal/ports/`.
- **FR-002**: System MUST support exporting telemetry data via the OTLP protocol, supporting both gRPC and HTTP transports.
- **FR-003**: System MUST automatically instrument all inbound HTTP requests on both end-user and admin servers with trace spans containing service name, HTTP method, route pattern, response status code, and request duration. Instrumentation MUST use `github.com/riandyrn/otelchi` as a chi middleware registered in `internal/adapters/http/routing/` to ensure chi route patterns (e.g., `/agents/{id}`) are correctly captured as span names rather than raw request paths.
- **FR-004**: System MUST propagate trace context from inbound requests using W3C Trace Context headers, enabling end-to-end distributed tracing across upstream and downstream services.
- **FR-005**: System MUST support configurable trace sampling (always-on, always-off, and ratio-based sampling) to control telemetry volume in high-traffic environments.
- **FR-006**: System MUST expose standard HTTP server metrics (request count, request duration histogram, response size) broken down by method, route, and status code when metrics are enabled.
- **FR-007**: System MUST expose Go runtime metrics (goroutine count, memory allocation, GC statistics) when metrics are enabled.
- **FR-008**: System MUST allow operators to configure a custom service name and additional resource attributes (e.g., environment, version, deployment region) that are attached to all exported telemetry.
- **FR-009**: System MUST flush all pending telemetry data during graceful shutdown, respecting the existing shutdown timeout configuration.
- **FR-010**: System MUST continue operating normally if the telemetry collector is unreachable, logging warnings without impacting request-serving latency or availability.
- **FR-011**: System MUST validate all telemetry configuration at startup and fail with a clear error message if the configuration is invalid (e.g., malformed endpoint URL, invalid sampling rate).
- **FR-012**: System MUST create child spans for significant internal operations (storage queries, encryption operations, JWKS key fetches, upstream OAuth2 calls) to enable operators to pinpoint latency sources within a request. Child spans MUST be created using context-based propagation: adapters extract the active `trace.Tracer` from the incoming `context.Context` (via `otel.Tracer` or a tracer stored in context) without changes to port interface signatures. No constructor injection of tracers into adapters is required.
- **FR-013**: System MUST correlate structured log entries with active trace context (trace ID and span ID) by bridging the application's `slog` logger to the OTel logs pipeline via `go.opentelemetry.io/contrib/bridges/otelslog`. This enables operators to pivot from a log entry directly to its corresponding trace span. Log correlation MUST be active only when telemetry is enabled.

### Configuration Requirements

**Configuration Parameters**:

- **telemetry.enabled**: Boolean, master toggle for all telemetry. Default: `false`.
- **telemetry.service_name**: String, the service name reported in all telemetry. Default: `agentic-identity-broker`.
- **telemetry.resource_attributes**: Map of key-value pairs, additional resource attributes attached to all telemetry (e.g., environment, region). Default: empty.
- **telemetry.traces.enabled**: Boolean, enables distributed tracing. Default: `true` (when telemetry is enabled).
- **telemetry.traces.sampling_rate**: Float (0.0-1.0), ratio-based trace sampling rate. `1.0` = sample all, `0.0` = sample none. Default: `1.0`.
- **telemetry.traces.propagators**: List of propagation formats. Default: `["ottrace", "b3multi", "baggage"]`. Supported values: `ottrace` (OpenTracing interop via `ot-tracer-*` headers), `b3multi` (Zipkin B3 multiple headers), `b3` (B3 single header), `tracecontext` (W3C Trace Context), `baggage` (W3C Baggage). Propagators are registered unconditionally when telemetry is enabled, regardless of `traces.enabled`.
- **telemetry.metrics.enabled**: Boolean, enables metrics collection and export. Default: `true` (when telemetry is enabled).
- **telemetry.metrics.export_interval**: Duration, how frequently metrics are exported. Default: `30s`.
- **telemetry.logs.enabled**: Boolean, enables OTLP log export via the `slog` bridge. Default: `true` (when telemetry is enabled). Set to `false` if the collector does not support `opentelemetry.proto.collector.logs.v1.LogsService`.
- **telemetry.exporter.protocol**: String, transport protocol. One of: `grpc`, `http`, `https`. Default: `grpc`. The `https` protocol uses the HTTP OTLP exporter with TLS; bare `host:port` endpoints are auto-prefixed with `https://`.
- **telemetry.exporter.endpoint**: String, OTLP collector endpoint URL. Required when telemetry is enabled.
- **telemetry.exporter.headers**: Map of key-value pairs, additional headers sent with export requests (e.g., authentication tokens). Default: empty. Values support `${ENV_VAR}` substitution for secrets.
- **telemetry.exporter.timeout**: Duration, maximum time to wait for export to complete. Default: `10s`.
- **telemetry.exporter.compression**: String, payload compression. One of: `none`, `gzip`. Default: `none`. Applied to all signal pipelines (traces, metrics, logs).
- **telemetry.exporter.insecure**: Boolean, disables TLS verification for exporter connections. Default: `false`. Only for development/testing. Only meaningful for gRPC; for HTTP the URL scheme controls TLS.

**Example YAML Configuration**:
```yaml
# OpenTelemetry configuration
telemetry:
  enabled: true
  service_name: agentic-identity-broker
  resource_attributes:
    environment: production
    region: eu-west-1
    version: "1.0.0"

  traces:
    enabled: true
    sampling_rate: 0.1  # Sample 10% of traces in production
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
    compression: none  # or "gzip"
    insecure: false
```

**Configuration Location**: Will be added to `examples/config/telemetry.yaml` and referenced in `examples/config/README.md`.

### Security Requirements

- **SR-001**: Telemetry data MUST NOT contain sensitive information. Enforcement is by code review against documented prohibited categories. Prohibited span attribute categories include: authentication tokens and API keys, encryption keys and key material, raw user credentials (passwords, secrets), personally identifiable information (PII) such as names, emails, IP addresses beyond what OTel semantic conventions include by default, and request/response body content. Permitted attributes are limited to: standard OTel HTTP semantic conventions (`http.method`, `http.route`, `http.status_code`, `http.request_content_length`), standard OTel database conventions (`db.system`, `db.operation`), and custom non-sensitive operational attributes (e.g., agent ID where that is not PII).
- **SR-002**: Exporter authentication headers MUST support environment variable substitution (`${ENV_VAR}`) to avoid storing secrets in configuration files, consistent with existing secret handling patterns.
- **SR-003**: TLS MUST be enabled by default for exporter connections. Disabling TLS (insecure mode) MUST require explicit opt-in via configuration, and the system MUST log a warning when insecure mode is active.
- **SR-004**: Telemetry export failures MUST NOT leak internal system state or error details to external systems beyond what is necessary for debugging the export failure itself.

### Assumptions

- The OTLP protocol is the standard export mechanism. Vendor-specific exporters (Jaeger, Zipkin, Prometheus native) are out of scope for this feature; operators can use an OTLP-compatible collector to bridge to those systems.
- Telemetry configuration is loaded at startup and does not support hot-reloading, consistent with the existing configuration subsystem behavior.
- The OpenTelemetry SDK handles buffering and retry internally. The identity broker configures the SDK but does not implement custom retry logic.
- The default propagator set is OTTrace + B3 (multiple headers) + Baggage, ensuring interoperability between OpenTracing and OpenTelemetry instrumented components and Zipkin-based systems. W3C Trace Context (`tracecontext`) can be added to the list if needed.
- Log correlation (injecting `trace_id` and `span_id` into structured log entries) is **in scope** for this feature, implemented via `go.opentelemetry.io/contrib/bridges/otelslog` bridging the existing `slog` logger to the OTel logs pipeline.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can enable distributed tracing in a production environment within 5 minutes by adding the telemetry configuration section and restarting the application.
- **SC-002**: With telemetry enabled, the system adds less than 5% overhead to median request latency compared to running with telemetry disabled.
- **SC-003**: With telemetry disabled (default), the system incurs zero measurable latency overhead from telemetry instrumentation.
- **SC-004**: When the telemetry collector is unreachable, 100% of application requests continue to be served successfully with no degradation in response time.
- **SC-005**: Exported traces contain sufficient detail (HTTP method, route, status, internal operation spans) to diagnose the root cause of a request failure without consulting application logs.
- **SC-006**: All telemetry configuration is validated at startup, and 100% of configuration errors produce actionable error messages identifying the invalid field and expected format.
