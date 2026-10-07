# Feature Specification: OpenTelemetry Support for ExtProc Token Exchange

**Feature Branch**: `027-extproc-otel`  
**Created**: 2026-04-17  
**Status**: Draft  
**Input**: User description: "I want to be able to have opentelemetry traces end to end from agentgateway all the way to identity broker via the extproc extension. To do that enhance the extproc configuration and implementation to support the same configuration parameter and capabilities when it comes to opentelemetry as was implemented for the identity broker."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - End-to-End Distributed Tracing Through ExtProc (Priority: P1)

A platform operator wants to trace a request as it travels from agentgateway through the ExtProc Token Exchange service and on to the Identity Broker, so they can diagnose latency, identify failures in the token exchange flow, and correlate events across all three components in a single trace.

**Why this priority**: This is the core value of the feature. Without trace context propagation in ExtProc, the trace chain breaks at the gRPC boundary and operators see disconnected spans from agentgateway and the identity broker with no way to correlate them. This is the foundational capability.

**Independent Test**: Configure the ExtProc service with a valid OTLP endpoint, send a request through agentgateway with trace context headers, and verify that a trace span from ExtProc appears in the collector linked to the upstream agentgateway span via the same trace ID.

**Acceptance Scenarios**:

1. **Given** OpenTelemetry is enabled on ExtProc with a valid OTLP endpoint, **When** a request with W3C `traceparent` headers arrives from agentgateway, **Then** ExtProc exports a span that is a child of the incoming trace, allowing operators to view the complete request path in a single trace.
2. **Given** OpenTelemetry is enabled on ExtProc, **When** a request with a W3C `traceparent` header arrives and ExtProc forwards a token exchange request to a mock broker, **Then** the in-process `SpanRecorder` captures an `extproc.token_exchange` span that is a child of the incoming trace, and separately captures an HTTP client span (created by `otelhttp.NewTransport`) that is a child of `extproc.token_exchange`. The mock broker receives outbound headers containing a `traceparent` whose trace ID matches the incoming trace, proving two-hop propagation (ExtProc → broker). Full three-hop verification (agentgateway → ExtProc → broker) is an integration-environment concern outside the scope of automated E2E tests.
3. **Given** OpenTelemetry is enabled, **When** ExtProc performs a token exchange against the Identity Broker, **Then** the outbound HTTP request to the broker carries the active trace context, enabling end-to-end trace continuity all the way to the broker.
4. **Given** OpenTelemetry is enabled and a token exchange fails, **When** the failure span is exported, **Then** the span contains the error status and a non-sensitive description, allowing operators to identify the failure cause without consulting logs.
5. **Given** OpenTelemetry is disabled (default), **When** a request is processed, **Then** no telemetry data is emitted and no performance overhead is incurred beyond a no-op check.
6. **Given** `telemetry.enabled` is `true` but `telemetry.traces.enabled` is `false`, **When** a request with a W3C `traceparent` header arrives and ExtProc forwards a token exchange request to a mock broker, **Then** no ExtProc spans are recorded in the `SpanRecorder`, but the mock broker receives outbound headers containing a `traceparent` that preserves the incoming trace context — proving transparent context forwarding without local span creation.

---

### User Story 2 - Configurable Telemetry Without Code Changes (Priority: P1)

As an operator deploying ExtProc across development, staging, and production environments, I need all OpenTelemetry settings to be configurable via YAML configuration and environment variables using the same schema and parameter names as the identity broker (with TLS certificate paths delegated to the OTel SDK's standard `OTEL_EXPORTER_OTLP_*` environment variables — see Configuration Requirements), so that I can reuse operational knowledge and tooling across both services.

**Why this priority**: Operator experience parity is essential. The identity broker already has a well-designed, well-documented telemetry configuration schema. The ExtProc service adopts the same schema and parameter names so that operators do not have to learn a different configuration model. The `service_name` default, env var prefix (`EXTPROC_` vs `IDENTITY_BROKER_`), and default propagator list intentionally differ between the two services (see Configuration Requirements). Both services intentionally delegate TLS certificate management to the OTel SDK's standard `OTEL_EXPORTER_OTLP_*` environment variables rather than app-level config, keeping the telemetry config surface focused on observability semantics. The schema structure and parameter semantics are identical across all application-controlled telemetry settings.

**Independent Test**: Start ExtProc with the same `telemetry:` YAML block used for the identity broker (only changing the `service_name`), verify telemetry data appears at the configured collector, then restart with `telemetry.enabled: false` and verify no telemetry is emitted.

**Acceptance Scenarios**:

1. **Given** no `telemetry` section exists in the ExtProc configuration, **When** the service starts, **Then** telemetry is disabled and the service starts normally with no telemetry overhead.
2. **Given** the ExtProc configuration includes a `telemetry` block identical in schema to the identity broker's (with a distinct `service_name`), **When** the service starts, **Then** traces are exported to the configured OTLP endpoint under the configured service name.
3. **Given** the telemetry configuration specifies an OTLP gRPC endpoint, **When** the service starts, **Then** traces, metrics, and logs are exported via gRPC.
4. **Given** the telemetry configuration specifies `protocol: http`, **When** the service starts, **Then** traces, metrics, and logs are exported via HTTP/protobuf.
5. **Given** the telemetry configuration specifies `protocol: https`, **When** the service starts, **Then** traces, metrics, and logs are exported via HTTPS/protobuf with TLS verification using the system CA bundle (or the CA bundle specified via the OTel SDK's `OTEL_EXPORTER_OTLP_CERTIFICATE` environment variable). No app-level TLS cert path parameter is defined — TLS configuration follows the OTel SDK's standard environment variable conventions.
6. **Given** the OTLP endpoint is provided via the `EXTPROC_TELEMETRY_EXPORTER_ENDPOINT` environment variable, **When** the service starts, **Then** the environment variable value is used, consistent with the existing `EXTPROC_` env var prefix convention.
7. **Given** the telemetry configuration specifies an invalid OTLP endpoint URL or unsupported protocol, **When** the service starts, **Then** startup fails immediately with a clear validation error identifying the invalid field.

---

### User Story 3 - Runtime Metrics for Operational Visibility (Priority: P2)

As an operator, I need the ExtProc service to expose request throughput, latency distribution, and Go runtime metrics so that I can build dashboards, set capacity alerts, and detect performance regressions in the token exchange path.

**Why this priority**: Metrics complement traces by providing aggregate views over time. While traces are essential for debugging individual requests, metrics are needed for health monitoring and capacity planning. Lower priority than tracing because traces deliver the most immediate end-to-end value.

**Independent Test**: Enable metrics in ExtProc configuration, send a series of requests, and verify that a request counter and latency histogram appear at the configured OTLP metrics endpoint, broken down by outcome (success/failure).

**Acceptance Scenarios**:

1. **Given** metrics are enabled in the ExtProc configuration, **When** multiple token exchange requests are processed, **Then** the service exports a request counter and duration histogram broken down by outcome (success, exchange_failure, circuit_open, invalid_resource, assertion_expired, passthrough).
2. **Given** metrics are enabled, **When** the service is running, **Then** Go runtime metrics (goroutine count, memory, GC statistics) are available at the configured OTLP endpoint.
3. **Given** metrics are disabled but tracing is enabled, **When** requests are processed, **Then** only trace data is exported and no metric data is emitted.

---

### User Story 4 - Graceful Degradation When Collector is Unavailable (Priority: P2)

As an operator, I need the ExtProc service to continue processing token exchange requests normally if the telemetry collector becomes unreachable, so that a monitoring infrastructure failure does not interrupt agent request flows.

**Why this priority**: The ExtProc service is in the critical path of every agent request. Telemetry is observability infrastructure, not business logic. A collector outage must never cause token exchange failures.

**Independent Test**: Configure ExtProc with an unreachable OTLP endpoint, send token exchange requests, and verify all requests are handled successfully.

**Acceptance Scenarios**:

1. **Given** telemetry is enabled with an unreachable OTLP endpoint, **When** the service starts, **Then** the service starts successfully and continues processing requests. Export failures are handled by the OTel SDK without impacting request processing.
2. **Given** telemetry is enabled and the collector becomes unreachable during operation, **When** requests continue to arrive, **Then** token exchange continues normally with no degradation in availability. (Response-time impact from failed exports is expected to be negligible due to OTel SDK non-blocking design, but is not asserted in automated tests.)
3. **Given** telemetry is enabled and a previously unreachable collector becomes available again, **When** new requests arrive, **Then** telemetry export resumes automatically without operator intervention.

---

### User Story 5 - Structured Log Correlation with Trace Context (Priority: P3)

As an operator, I need structured log entries emitted by the ExtProc service to carry the active trace ID and span ID, so that I can pivot from a log line directly to its corresponding trace span in my observability platform.

**Why this priority**: Log-trace correlation is a quality-of-life improvement for debugging. It is not required for end-to-end tracing to work but significantly reduces the time to diagnose issues once tracing is established.

**Independent Test**: Enable telemetry including logs, send a request that triggers a log entry (e.g., a token exchange failure), and verify that the log record exported to the OTLP collector contains `trace_id` and `span_id` fields matching the active span.

**Acceptance Scenarios**:

1. **Given** telemetry is enabled with logs enabled, **When** the ExtProc service emits a structured log entry during a traced request, **Then** the exported log record contains `trace_id` and `span_id` fields matching the active trace span.
2. **Given** telemetry is enabled but `telemetry.logs.enabled` is set to `false`, **When** the service runs, **Then** no OTLP log records are exported (the mock OTLP collector's `LogsService.Export` RPC receives zero invocations).

---

### Edge Cases

- What happens when `telemetry.exporter.protocol` is set to an unrecognized value? The service MUST fail at startup with a clear validation error listing the allowed values (`grpc`, `http`, `https`).
- What happens when `telemetry.traces.sampling_rate` is `0.0`? No trace spans are collected or exported, but instrumentation overhead is minimal (no-op sampler path).
- What happens during graceful shutdown while telemetry data is being flushed? After `GracefulStop()` completes, the service MUST call the telemetry provider shutdown with a 5-second context deadline. Flush errors are logged but do not affect the exit code.
- What happens when a propagator name in `telemetry.traces.propagators` is unrecognized? The unrecognized name is logged as a warning and skipped; the remaining valid propagators are still registered.
- What happens when `telemetry.exporter.insecure` is `true` with protocol `grpc`? The service logs a security warning and proceeds — insecure mode is a deliberate developer opt-in, not an error.
- What happens when the incoming request carries no trace context headers? ExtProc starts a new root span and continues normally; trace propagation is only possible when the upstream sends context.

## Clarifications

### Session 2026-04-20

- Q: Should `tracecontext` (W3C) be added to the default propagators list, given FR-003 and US1 both reference W3C Trace Context? → A: Yes — add `tracecontext` to defaults: `["tracecontext", "ottrace", "b3multi", "baggage"]`
- Q: Should OTel spans be created at stream level (one span per gRPC stream via StreamServerInterceptor) or at request level (one span per RequestHeaders message)? → A: Request-level — one span per `RequestHeaders` message, created manually inside `processRequestHeaders`, not via a stream interceptor.
- Q: What is the maximum time to wait for telemetry flush during graceful shutdown (FR-007)? → A: 5 seconds — a fixed 5s context deadline for `telemetryShutdown(ctx)`, applied after `GracefulStop()` completes.
- Q: Where should the mapping from extproc's `TelemetryConfig` to `ports.TelemetryConfig` live? → A: Mirror type defined in `internal/extproc/config`; mapper function lives in `cmd/extproc-token-exchange/root.go`, keeping the config package free of `internal/ports` imports.
- Q: What span attributes should token-exchange spans carry? → A: Bounded `token_exchange.outcome`, failure stage/detail, recovery action/target, and exchange kind. Success logs/spans omit failure-only fields; successful metric observations use `none`. Requested resource URIs and credential-bearing text are never recorded.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The ExtProc service MUST support enabling and disabling OpenTelemetry telemetry (traces, metrics, logs) via a `telemetry` configuration block with the same schema and parameter names as `internal/ports/config.go` `TelemetryConfig`. Telemetry MUST be disabled by default. The ExtProc defaults for `service_name` and `propagators` differ from the identity broker's defaults (see Configuration Requirements).
- **FR-002**: The ExtProc service MUST export telemetry via the OTLP protocol, supporting gRPC, HTTP, and HTTPS transports, using the same transport options (TLS, compression, headers, timeout) as the identity broker.
- **FR-003**: The ExtProc service MUST extract inbound trace context from the incoming `HttpHeaders` entry list (the forwarded request headers inside the `RequestHeaders` message) using the configured propagators (W3C Trace Context, B3, OTTrace, Baggage) and create a child span **per `RequestHeaders` message** (i.e., per token exchange attempt) inside `processRequestHeaders` **when `telemetry.traces.enabled` is `true`**. When `telemetry.enabled` is `true` but `telemetry.traces.enabled` is `false`, propagators are still registered (FR-011) so that trace context is extracted and forwarded transparently — but no local spans are created or exported. `otelhttp.NewTransport` wraps the outbound HTTP client unconditionally; when traces are disabled, it operates as a no-op for tracing while still injecting propagation headers via the globally registered propagators. The carrier MUST be built as an `http.Header`-style multi-value structure (preserving repeated header keys) — not collapsed to a plain `map[string]string`. Extraction uses neither gRPC stream metadata nor a gRPC interceptor. This ensures each individual token exchange is independently traceable.
- **FR-004**: When performing a token exchange HTTP call to the Identity Broker, the ExtProc service MUST inject the active trace context into the outbound HTTP request headers, enabling end-to-end trace continuity.
- **FR-005**: The ExtProc service MUST support configurable trace sampling (always-on, always-off, ratio-based) via `telemetry.traces.sampling_rate`.
- **FR-006**: The ExtProc service MUST expose request-level metrics (request counter and duration histogram broken down by outcome: `success`, `exchange_failure`, `circuit_open`, `invalid_resource`, `assertion_expired`, `passthrough`) and Go runtime metrics when metrics are enabled. The `passthrough` outcome is recorded when a request bypasses token exchange (e.g., no matching resource URI).
- **FR-007**: The ExtProc service MUST flush all pending telemetry data during graceful shutdown, after `GracefulStop()` completes. The telemetry provider shutdown MUST use a 5-second context deadline. Flush errors are logged but do not affect the exit code.
- **FR-008**: The ExtProc service MUST continue operating normally if the telemetry collector is unreachable, without impacting token exchange availability. Export failure handling (including any warning logs) is delegated to the OTel SDK — the service itself does not define additional logging contracts for collector outages.
- **FR-009**: The ExtProc service MUST validate all telemetry configuration at startup and fail with a clear error message for invalid configuration (malformed endpoint URL, unsupported protocol, out-of-range sampling rate).
- **FR-010**: The ExtProc service MUST correlate structured log entries with active trace context (trace ID and span ID) via the slog-to-OTel bridge when both telemetry and logs are enabled.
- **FR-011**: The ExtProc service MUST register propagators unconditionally when telemetry is enabled, regardless of whether `telemetry.traces.enabled` is true, so that trace context is always extracted and forwarded transparently. When `traces.enabled` is false, extraction still populates the context for outbound propagation but no local spans are created or exported (see FR-003).
- **FR-012**: Telemetry initialisation for the ExtProc service MUST reuse the existing `internal/adapters/telemetry` package (`NewProvider`) rather than duplicating OTel provider setup code. The extproc config's telemetry block MUST be mapped to `ports.TelemetryConfig` at startup.

### Configuration Requirements

The `telemetry` block in the ExtProc configuration MUST use the same schema and parameter names as the identity broker's telemetry configuration. The differences are the default `service_name` value, the environment variable prefix (`EXTPROC_` instead of `IDENTITY_BROKER_`), and the default propagators list (ExtProc adds `tracecontext` because agentgateway sends W3C `traceparent` headers). TLS certificate configuration (CA bundles, client certificates) is intentionally not part of the app-level config schema — both services delegate this to the OTel SDK's standard environment variables (`OTEL_EXPORTER_OTLP_CERTIFICATE`, `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE`, `OTEL_EXPORTER_OTLP_CLIENT_KEY`), keeping the app config surface focused on telemetry semantics. When copying a `telemetry:` block from the identity broker to ExtProc, the `service_name` must be changed; all other parameters are compatible and the propagator list can be overridden explicitly if the defaults differ.

**Configuration Parameters**:

- **telemetry.enabled**: Boolean, master toggle for all telemetry. Default: `false`.
- **telemetry.service_name**: String, the service name reported in all telemetry. Default: `extproc-token-exchange`.
- **telemetry.resource_attributes**: Map of key-value pairs, additional resource attributes (e.g., environment, region). Default: empty.
- **telemetry.traces.enabled**: Boolean, enables distributed tracing. Default: `true` (when telemetry is enabled).
- **telemetry.traces.sampling_rate**: Float (0.0–1.0), ratio-based trace sampling rate. Default: `1.0`.
- **telemetry.traces.propagators**: List of propagation formats. Default: `["tracecontext", "ottrace", "b3multi", "baggage"]`. Supported values: `tracecontext`, `ottrace`, `b3multi`, `b3`, `baggage`.
- **telemetry.metrics.enabled**: Boolean, enables metrics collection and export. Default: `true` (when telemetry is enabled).
- **telemetry.metrics.export_interval**: Duration, how frequently metrics are exported. Default: `30s`.
- **telemetry.logs.enabled**: Boolean, enables OTLP log export via the slog bridge. Default: `true` (when telemetry is enabled).
- **telemetry.exporter.protocol**: String, transport protocol. One of: `grpc`, `http`, `https`. Default: `grpc`.
- **telemetry.exporter.endpoint**: String, OTLP collector endpoint. Required when telemetry is enabled.
- **telemetry.exporter.headers**: Map of key-value pairs, additional headers sent with export requests. Default: empty. Values support `${ENV_VAR}` substitution.
- **telemetry.exporter.timeout**: Duration, maximum time to wait for export. Default: `10s`.
- **telemetry.exporter.compression**: String, payload compression. One of: `none`, `gzip`. Default: `none`.
- **telemetry.exporter.insecure**: Boolean, disables TLS verification. Default: `false`. Development/testing only.

**TLS Certificate Configuration**: The `OTLPExporterConfig` intentionally mirrors the identity broker's config, which delegates TLS certificate management to the OTel SDK's standard environment variables rather than app-level config parameters. For HTTPS connections, the OTel SDK uses the system CA pool by default. To specify a custom CA bundle (e.g., for self-signed certs), set the `OTEL_EXPORTER_OTLP_CERTIFICATE` environment variable. Client certificate authentication can be configured via `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE` and `OTEL_EXPORTER_OTLP_CLIENT_KEY`. This approach avoids duplicating the OTel SDK's TLS configuration surface in the app config.

**Example YAML Configuration**:
```yaml
telemetry:
  enabled: true
  service_name: extproc-token-exchange
  resource_attributes:
    environment: production
    region: eu-west-1

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
    headers:
      Authorization: "Bearer ${OTEL_EXPORTER_AUTH_TOKEN}"
    timeout: 10s
    compression: none
    insecure: false
```

**Configuration Location**: Will be added to `examples/config/extproc-telemetry.yaml` and referenced in `examples/config/README.md`.

### Security Requirements

- **SR-001**: Telemetry data MUST NOT contain credentials, provider or broker descriptions, nested error messages, URLs or resource paths, headers, bodies, or process command arguments. Exchange spans, logs, and metrics use the bounded `token_exchange.*` classification contract in `ARCHITECTURE.md`. No exception event or span status can serialize an arbitrary error. ExtProc uses `unknown` exchange kind when the broker's selected exchange profile is not observable.
- **SR-002**: Exporter authentication headers MUST support environment variable substitution (`${ENV_VAR}`) to avoid storing secrets in configuration files.
- **SR-003**: TLS MUST be enabled by default for OTLP gRPC exporter connections. Disabling TLS (`insecure: true`) MUST log a warning at startup.
- **SR-004**: Telemetry export failures MUST NOT expose internal system state or error details beyond what is necessary to diagnose the export failure itself.

### Assumptions

- The ExtProc service reuses the existing `internal/adapters/telemetry.NewProvider` function. No separate telemetry adapter is created for ExtProc.
- The extproc config struct (`internal/extproc/config/config.go`) gains a `Telemetry` field of a new `TelemetryConfig` type that mirrors `ports.TelemetryConfig` structurally. A mapping function `mapTelemetryConfig` in `cmd/extproc-token-exchange/root.go` converts between the two at startup, keeping `internal/extproc/config` free of any `internal/ports` imports.
- Trace context propagation from agentgateway arrives as forwarded HTTP request headers inside the ExtProc `RequestHeaders` message. Extraction uses the `HttpHeaders` key-value pairs as the propagation carrier — not gRPC stream metadata and not a gRPC interceptor.
- No new domain objects are introduced. This is a pure infrastructure/observability enhancement.
- The telemetry configuration is loaded at startup and does not support hot-reloading.
- The `EXTPROC_` env var prefix is already established for this service. Telemetry env vars follow the pattern `EXTPROC_TELEMETRY_*`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can enable end-to-end tracing from agentgateway through ExtProc to the Identity Broker by adding a `telemetry` block to the ExtProc configuration and restarting the service, with no code changes required.
- **SC-002**: With telemetry enabled, trace spans from ExtProc appear in the same trace as upstream agentgateway spans, with no broken trace chains for requests that carry W3C or B3 trace context headers.
- **SC-003**: With telemetry disabled (default), the ExtProc service incurs zero measurable latency overhead from telemetry instrumentation.
- **SC-004**: When the telemetry collector is unreachable, 100% of token exchange requests continue to be processed successfully. Response-time impact is expected to be negligible due to OTel SDK non-blocking design but is not formally asserted.
- **SC-005**: All telemetry configuration errors produce actionable error messages identifying the invalid field and expected format, verified at startup before the gRPC server begins accepting connections.
- **SC-006**: The same `telemetry:` YAML block (with `service_name` changed and, optionally, the `propagators` list adjusted) can be copied from an identity broker configuration file into an ExtProc configuration file and produce equivalent telemetry behavior. The schemas are compatible; only the defaults for `service_name` and `propagators` differ.
