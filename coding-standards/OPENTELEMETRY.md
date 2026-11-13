# OpenTelemetry Standard

This standard applies to all Go code in `cmd/` and `internal/`. Read it before you add or change a span, a span attribute, or a metric instrument.

Scope: OpenTelemetry traces and metrics. Log records, including log records exported through the OpenTelemetry log bridge, follow `coding-standards/LOGGING.md`.

Related binding sources: ADR 011 and ADR 027 (OpenTelemetry), ADR 033 (request security context),
spec `017-opentelemetry-support` (SR-001 to SR-004), `docs/reference/token-exchange.md#diagnostic-attributes`, and Constitution Principle I.

`docs/reference/telemetry.md` lists each span, metric and attribute that the code emits. `tools/telemetrydoc` generates it (section 9).

## 1. Provider Plumbing

| Rule | Reason |
|---|---|
| Create providers only with `telemetry.NewProvider`. Call it only from `internal/app/builder.go` and `cmd/extproc-token-exchange/`. | One place owns exporters, sampling, resource attributes, and shutdown order (ADR 011, ADR 027). |
| Get tracers and meters from the globals: `otel.Tracer(scope)` and `otel.Meter(scope)`. Do not inject a `TracerProvider` or `MeterProvider` through a constructor. | Constructors stay unchanged. Disabled telemetry is a no-op (ADR 011, Decision 3). |
| Domain packages may import only the OpenTelemetry API: `go.opentelemetry.io/otel`, `.../trace`, `.../metric`, `.../attribute`, `.../codes`, `.../propagation`. Do not import the SDK, an exporter, or `internal/adapters/telemetry`. | The API is a no-op interface. The SDK is infrastructure. |
| `internal/extproc/*` must not import `internal/ports` or `internal/adapters/telemetry`. | ADR 027. Only `cmd/extproc-token-exchange/` bridges the two. |
| Configure telemetry only through the binary's `telemetry.*` configuration: `ports.TelemetryConfig` in the broker, `internal/extproc/config.TelemetryConfig` in ExtProc. Only `cmd/extproc-token-exchange/` maps the ExtProc type to `ports.TelemetryConfig`. | Constitution Principle VII. ADR 027 keeps `internal/extproc/*` free of `internal/ports`. |
| Telemetry fails open. A failure to create an instrument or to export data must not fail a request or startup. | Observability must not change authentication or authorization results (ADR 033, SR-003). |
| Instrument outbound HTTP only with `telemetryhttp.NewTransport`. Do not use `otelhttp` on a client. | `telemetryhttp` records no URL, header, or body, and it skips URLs that have a query string. |
| Apply the principal policy to spans only in the span pipeline that `telemetry.NewProvider` builds. | Each binary that calls `NewProvider` gets the same policy without its own setup (section 4.3). |
| Instrument inbound HTTP only with the `otelchi` middleware in `internal/adapters/http/server.go`. Do not start a second server span in a handler. | One server span per request, named by route pattern. |

## 2. Instrumentation Scope

The scope is the name passed to `otel.Tracer` and `otel.Meter`.

- Use the bounded-context or adapter name. Reuse an existing scope before you add one.
- Use the same scope for the tracer and the meter of one package.

| Scope | Owner |
|---|---|
| `storage` | Storage adapters |
| `encryption` | Encryption adapters |
| `jwks` | JWKS adapter |
| `upstream` | Upstream OAuth2 calls |
| `telemetryhttp` | Outbound HTTP transport |
| `approval` | Approval domain and handlers |
| `oauth2session` | Third-party session domain and the sweep handler |
| `tokenexchange` | Broker token exchange |
| `extproc` | ExtProc |

## 3. Spans

### 3.1 When to Create a Span

Create a child span for:

- each call that leaves the process: database, KMS, DynamoDB, upstream OAuth2, JWKS, OPA, and broker calls from ExtProc;
- a domain operation that an operator must see separately in a trace, for example the approval gate or a token exchange.

Do not create a span for a pure in-memory function or for each iteration of a loop.

Each storage repository method creates a `storage.*` span.

### 3.2 Names

```text
<scope>.<operation>[.<entity>]

storage.get.agent
approval.list_pending
extproc.opa.evaluate
oauth2session.background_refresh
```

- Use lowercase `snake_case` segments separated by dots.
- Use a constant string. Do not put IDs, URLs, principals, tool names, or other values in a span name. Put values in attributes (section 4).
- A span name is a contract for trace queries. A rename is a breaking change. Record it in the changelog.

### 3.3 Kind

Set the span kind with `trace.WithSpanKind`.

| Kind | Use |
|---|---|
| `Server` | Inbound request handled by this process. `otelchi` sets it for HTTP. The ExtProc request span sets it explicitly. |
| `Client` | Outbound call: database, KMS, DynamoDB, HTTP, OPA bundle fetch |
| `Internal` | Domain operation that does not leave the process |

### 3.4 Status and Errors

```go
ctx, span := otel.Tracer("storage").Start(ctx, "storage.get.agent",
    trace.WithSpanKind(trace.SpanKindClient))
defer span.End()

agent, err := r.get(ctx, agentID)
if err != nil {
    kind := storage.ErrorKindUnknown
    var storageErr *storage.StorageError
    if errors.As(err, &storageErr) {
        kind = storageErr.Kind
    }
    if kind == storage.ErrorKindNotFound {
        span.SetAttributes(attribute.String("storage.outcome", string(kind)))
        return nil, err
    }
    span.SetAttributes(attribute.String("error.type", string(kind)))
    span.SetStatus(codes.Error, "agent lookup failed")
    return nil, err
}
```

- When the operation fails, set `codes.Error` and `error.type`.
- On a storage `Client` span, a result that the caller handles as normal flow is not a failure: no matching row, a conflict, or rejected input. Do not set `codes.Error` or `error.type`. Record it as `storage.outcome`. PostgreSQL repositories use `startStorageSpan`, which applies this rule.
  - The OpenTelemetry PostgreSQL conventions treat SQLSTATE class 02 or higher as an error by default. This rule deliberately narrows that default for conflicts and foreign-key violations, because the repository knows that its callers expect them.
  - Tail-sampling backends can keep every trace that has an error on a `Client` or `Server` span, so a false error also changes which traces are kept.
- Use a constant status description. Do not use `err.Error()`.
- Do not call `span.RecordError`. A wrapped cause can contain a URL with its query, a provider body, or a SQL fragment. Record typed metadata instead (section 4).
- Do not mark a client error (4xx) as an error on a `Server` span. `otelchi` derives the status from the response code. The ExtProc request span sets the error status only for the 5xx responses it sends.
- Record caller cancellation as an outcome, for example `canceled`. Do not set `codes.Error`.

### 3.5 Background Work

- Work that outlives the request starts a new root span with `trace.WithNewRoot()`.
- Link the new root span to the span that triggered it with `trace.WithLinks`.
- Precedent: `oauth2session.background_refresh` and the approval create path.

### 3.6 Propagation

- Use `otel.GetTextMapPropagator()` to extract and inject. Do not construct a propagator in a handler or adapter.
- The exception is the security-context capture code, which validates `traceparent` with `propagation.TraceContext{}` before the configured propagator runs (ADR 033).
- Do not copy a trace ID into a span attribute. The span already carries it.

## 4. Span Attributes

### 4.1 Keys

- Use the OpenTelemetry semantic-convention key when one exists.
- Otherwise, use `<namespace>.<name>`, with lowercase `snake_case` segments separated by dots, for example `token_exchange.failure_stage`, `approval.status`, `oauth2.refresh.outcome`.
- An attribute namespace names the domain concept the attribute describes. It is independent of the instrumentation scope (section 2): one scope can set attributes from several namespaces. Use a namespace from this table. Add a namespace to the table before you use it.

| Namespace | Describes | Reference |
|---|---|---|
| `token_exchange` | RFC 8693 token exchange in the broker and ExtProc | `docs/reference/token-exchange.md#diagnostic-attributes` |
| `approval` | Tool approvals | `specs/024-approval-api-ui/` |
| `authorization` | ExtProc OPA authorization | ADR 028 |
| `oauth2.refresh` | Third-party session refresh | `specs/029-proactive-token-refresh/contracts/observability.md` |
| `encryption` | Encryption operations | ADR 012 |
| `impersonation` | OAuth2 impersonation | ADR 032 |
| `storage` | Results of storage repository calls | `docs/configuration.md#what-is-instrumented`, `docs/reference/telemetry.md` |

- Use the canonical keys in this table. Do not make synonyms.

| Concept | Key | Value |
|---|---|---|
| HTTP method | `http.request.method` | Allowlisted method, `_OTHER` otherwise (`telemetryhttp.MethodName`) |
| HTTP status | `http.response.status_code` | Integer |
| Error classification | `error.type` | Enumerated value. Do not use `error.kind` or `error_kind` on spans. |
| Database | `db.system`, `db.operation` | `postgresql`, repository method name |
| Entity ID | `<namespace>.<entity>.id` | Typed ID string, for example `approval.id`, `token_exchange.agent.id` |
| Outcome | `<namespace>.outcome` | Enumerated value |
| Failure stage and detail | `<namespace>.failure_stage`, `<namespace>.failure_detail` | Enumerated values |
| Dependency | `<namespace>.dependency` | Enumerated value |
| Principal | `<namespace>.principal` | `id.Principal` through `SpanAttribute`. Refer to section 4.3. |
| Point in time | `<namespace>.<name>_at` | UTC RFC 3339 with nanoseconds |
| Duration | `<namespace>.duration_ms` | Integer milliseconds. Prefer the span duration. |

### 4.2 Values

- An attribute value is an enumerated value, a typed ID, a count, a duration, or a timestamp. Use typed string constants for enumerated values.
- Document each attribute family and its value set in the feature's reference document or observability contract.
- Set an attribute only when its value is known. Do not invent a placeholder, except `unknown` where the reference document defines it.

### 4.3 Principals

A principal is personal data (`coding-standards/LOGGING.md` section 5). Spec 017 SR-001, as amended, permits it on spans only under these rules. Span attributes go through the same central principal policy as log records, so a change to redaction or pseudonymisation reaches both signals in one change.

> **Until #356 lands:** `id.Principal.SpanAttribute` and the span principal policy do not exist yet. Do not add a new principal span attribute until they do. The existing `approval.principal` attributes change in #349.

1. Set a principal on a span only with `id.Principal.SpanAttribute(key)`. Do not use `attribute.String` with `p.String()` or `string(p)`.
2. Use a key that ends in `.principal`, for example `approval.principal`. Keys that end in `.principal` are reserved for principals.
3. The span pipeline that `telemetry.NewProvider` builds renders the value of each reserved key with the principal policy before export. It uses the same policy function as the log handler. It is the only component that renders a principal on a span.
4. ExtProc must not import `internal/domain/id`. ExtProc receives the principal from the broker's token exchange response as the raw identifier, because it uses the value as the approval sync key. ExtProc sets it only under a reserved key. ExtProc calls `telemetry.NewProvider` (ADR 027), so the same policy applies without separate setup.
5. Set a principal only on spans of operations that act for one owner, for example the approval and token exchange spans. Do not set it on storage, encryption, or HTTP client spans. The parent span carries it.
6. Do not put a principal in a span name, a span event, or a status description. The policy does not see them.
7. Do not use a principal as a metric attribute (section 5.3).

```go
// Correct
span.SetAttributes(approval.Principal.SpanAttribute("approval.principal"))

// Incorrect: a plain string under a non-reserved key bypasses the policy
span.SetAttributes(attribute.String("approval.owner", string(approval.Principal)))
```

## 5. Metrics

### 5.1 Instruments

| Rule | Reason |
|---|---|
| Create each instrument one time, in the constructor. Do not create an instrument per request. | Instrument creation allocates and registers state. |
| Handle the error from an instrument constructor. Log it one time at `Warn` and continue with the returned instrument. Do not discard it with `_`. | A missing instrument must be visible. The returned instrument is a safe no-op. |
| Set `metric.WithDescription` and `metric.WithUnit` on each instrument. Use UCUM units: `s`, `ms`, `By`, `{request}`, `{session}`. | Units drive exporter naming and dashboards. |
| Choose the instrument by semantics: `Counter` for a monotonic count, `UpDownCounter` for a value that goes up and down, `Histogram` for a distribution, an observable gauge for a value read from storage. | The instrument kind decides the aggregation. |
| Record with `context.WithoutCancel(ctx)` when the request can already be canceled. | A canceled context must not drop the measurement. |
| An observable gauge that queries storage runs one query per export cycle. If the query fails, export no points for that cycle and log at `Warn`. | Precedent: `oauth2_sessions`. |

### 5.2 Names

```text
<scope>.<noun>[.<qualifier>]

extproc.token_exchange.requests
extproc.token_exchange.duration
oauth2session.proactive_refresh.triggered
approval.pending
```

- Use lowercase `snake_case` segments separated by dots. Start with the scope.
- Do not add a unit suffix (`_seconds`) or a `_total` suffix. The unit goes in `WithUnit`. The Prometheus exporter or collector adds suffixes.
- A metric name is a contract for dashboards and alerts. A rename is a breaking change. Record it in the changelog, and emit the old and the new name for one release.

### 5.3 Attributes

- Use lowercase `snake_case` keys without a namespace, for example `outcome`, `triggered_by`, `dry_run`.
- Exception: when a reference document defines an attribute family for spans and metrics together, the metric uses the same namespaced key, for example `token_exchange.outcome` (`docs/reference/token-exchange.md#diagnostic-attributes`).
- Each attribute has a closed, documented value set.
- Do not use these as metric attributes: principal, `session_id`, `grant_id`, `approval_id`, tool name, URL, `client_id`, or any value supplied by a caller.
- `service_id` and `agent_id` are permitted only when the observability contract states that operators bound the set. Precedent: the `oauth2_sessions` gauge.

### 5.4 Contract

Each new instrument goes in the feature's observability contract (`specs/NNN-*/contracts/observability.md`). Include the name, kind, unit, scope, attributes with their value sets, and when it changes. Precedent: `specs/029-proactive-token-refresh/contracts/observability.md`.

## 6. Permitted Data

Spans are exported to systems outside the broker's trust boundary and are kept by tracing backends. Metric attributes are indexed and kept longer than logs. The rules in `coding-standards/LOGGING.md` section 6 apply, with these additions.

### 6.1 Never Record

These data classes are prohibited in span names, span attributes, span events, span status descriptions, metric names, and metric attributes:

- each class in `coding-standards/LOGGING.md` section 6.1;
- a principal, except as a span attribute under a reserved key through the principal policy (section 4.3);
- an error value or error string (section 3.4);
- a full URL, a URL path that contains IDs or caller input, or a requested resource URI. Use the registered ID, for example `token_exchange.service.id`;
- a request or response header value, except the allowlisted method and the status code;
- process command lines and arguments. `telemetry.NewProvider` removes them from resource attributes.

### 6.2 Where Data Is Permitted

| Data | Span attribute | Metric attribute |
|---|---|---|
| Enumerated outcome, stage, detail, error type, dependency, trigger | Yes | Yes |
| Allowlisted HTTP method, route pattern, status code | Yes | Yes |
| Allowlisted OAuth error code (`IsSafeOAuthErrorCode`) | Yes | Yes |
| Typed entity ID, `kid` | Yes | No. `service_id` and `agent_id` only per section 5.3. |
| `client_id` | Yes | No |
| Tool name, MCP method, target server name bounded by configuration | Yes | No |
| Grant timestamps | Yes | No |
| Counts and durations | Yes | As measurement values only |
| Principal | Only per section 4.3 | No |
| Client IP, User-Agent | No | No |

### 6.3 Resource Attributes

- Operators set resource attributes through `telemetry.resource_attributes`. Do not add resource attributes in code, except in `telemetry.NewProvider`.
- Do not add a hostname, IP, or credential beyond what `telemetry.NewProvider` detects.

## 7. Tests

- Inject an in-memory span recorder with `Builder.WithTracerProvider`, or set a test provider and shut it down in `AfterEach`. Do not depend on global state from another test.
- Use an `sdkmetric.ManualReader` to assert metric values and attributes.
- Assert span names, span kinds, status codes, and key attributes. Do not assert status descriptions.
- For each path that handles a credential, assert that no span attribute, event, or status contains the credential under test. Examples: `internal/extproc/server/exchange_safety_test.go`, `internal/adapters/telemetry/resource_security_test.go`.
- For each span that sets a principal, assert that the attribute value is the policy output.
- For each new metric, assert the attribute set, so a new attribute cannot raise cardinality without review.

## 8. Review Checklist

- [ ] No new provider, exporter, or SDK import outside `internal/adapters/telemetry` and `cmd/`.
- [ ] The tracer and the meter use the package's scope (section 2).
- [ ] Span names are constant and follow `<scope>.<operation>[.<entity>]`. The span kind is set.
- [ ] Failed operations set `codes.Error` with a constant description and `error.type`. `RecordError` is not called.
- [ ] Background work starts a new root span linked to its trigger.
- [ ] Span attribute keys are canonical and use a listed namespace (section 4.1). Values are bounded.
- [ ] Instruments are created one time, with a description and a unit, and the constructor error is handled.
- [ ] Metric names follow section 5.2. Metric attributes have closed value sets and no IDs other than those permitted by section 5.3.
- [ ] No section 6.1 data is in a span or a metric.
- [ ] Principals are set on spans only through `id.Principal.SpanAttribute` under a `.principal` key, and only where section 4.3 permits. No principal is a metric attribute.
- [ ] The observability contract lists each new instrument.
- [ ] A test asserts that credentials on this path are not in span attributes, and asserts each new metric's attribute set.
- [ ] `docs/reference/telemetry.md` is regenerated with `just telemetry-doc` (section 9).

## 9. Telemetry Reference

`docs/reference/telemetry.md` is the reference for operators and agents that query traces and metrics. `tools/telemetrydoc` reads the code with `go/packages` and generates the page. It does not run the code.

| Rule | Reason |
|---|---|
| After you add, rename, or remove a span, a metric, or an attribute, run `just telemetry-doc` and commit the page. `TestReferenceIsCurrent` fails while the page is out of date. | The page stays the same as the code in each pull request. |
| Do not edit the page by hand. Change the code or `tools/telemetrydoc`. | The next generation overwrites hand edits. |
| Pass a span name, a metric name, and an attribute key as a constant, or as a constant argument to a function that passes it on, as with `id.Principal.SpanAttribute(key)`. | Sections 3.2 and 4.1 already require constant names. The generator cannot list a name it cannot resolve. |
| Set each attribute, span status, span name, and measurement where the generator can follow it to the span or the instrument. It follows variables, struct fields, closures, the parameters of functions that the module only calls directly, and the span in a context back to its `Start` call. It fails at each call that it cannot follow. | A call that the generator cannot follow would be missing from the page. |
| Give an enumerated value a typed string constant (section 4.2). | The page lists the constants of the type as the value set. |

The page does not cover the `otelchi` server spans and metrics, metrics that the SDK, the Go runtime, or a library creates, span events, resource attributes, or log records. The generator fails when a span event has attributes.
