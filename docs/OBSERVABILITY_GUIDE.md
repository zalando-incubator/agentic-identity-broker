# Observability Guide: Logs, Traces, and Metrics

**Status**: Proposed
**Applies to**: `internal/`, `cmd/agentic-identity-broker/`, `cmd/extproc-token-exchange/`
**Builds on**: [ADR 011](../adrs/011-opentelemetry-provider-pattern.md), [ADR 027](../adrs/027-extproc-telemetry-shared-dependency.md), spec `017-opentelemetry-support` (SR-001..SR-004), spec `033-request-security-context`, [token-exchange diagnostic attributes](reference/token-exchange.md#diagnostic-attributes), `specs/029-proactive-token-refresh/contracts/observability.md`

This guide gathers the telemetry rules that are spread across ADRs, specs, and `ARCHITECTURE.md`.
It also sets conventions where the code is inconsistent today. MUST/SHOULD follow RFC 2119.
If this guide conflicts with an Accepted ADR, the ADR wins.

---

## 1. What already exists

| Concern | Where | Notes |
|---|---|---|
| Provider lifecycle (traces, metrics, logs) | `internal/adapters/telemetry/provider.go` → `NewProvider` | Called from `internal/app/builder.go` and from `cmd/extproc-token-exchange/` (ADR 027). Disabled telemetry uses no-op globals. |
| Inbound HTTP spans and metrics | `otelchi` in `internal/adapters/http/server.go` | Span name `METHOD /route/{param}`. The token endpoint gets a credential-free request view. |
| Outbound HTTP spans | `internal/telemetryhttp/transport.go` | Records only method allowlist, status code, and `error.type`. URLs with a query string get no span. |
| Log → trace correlation | `internal/adapters/telemetry/context_handler.go` | Adds `trace_id`, `actor`, and `calling_peer` from the request security context, **only for `*Context` log calls**. |
| Log export to OTLP | `otelslog` bridge + `NewMultiHandler` | Enabled by `telemetry.logs.enabled`. With the bridge on, **every log record is exported telemetry**. |
| Request security context | `internal/domain/security` | `trace_id`, `actor`, `calling_peer`. `traceresponse` header matches the logged `trace_id`. |
| Bounded diagnostics | `OperationError` / `ErrorMetadata` (`internal/domain/oauth2session/errors.go`, `internal/extproc/server/exchange_errors.go`) | Operation, kind, dependency, status, allowlisted OAuth code. Never the error text. |
| Log-forging protection | `sanitizeLogValue` in `internal/adapters/http/middleware.go` | Escapes line breaks in method, path, client address, and panic values. |

---

## 2. Choosing a signal

| Question you want operators to answer | Signal |
|---|---|
| "How many / how fast / how saturated?" (alertable, aggregated) | **Metric** |
| "Where did time go in this request, and which dependency failed?" | **Span** (child of the request span) |
| "What happened, to whom, and why?" (audit, forensics, a security decision) | **Structured log event** |

A security decision (grant, deny, revoke, key rotation, credential use) MUST produce a log event.
It MAY also produce a metric. A span alone is not an audit record: spans are sampled.

---

## 3. Wiring rules

1. Do not create providers. Only `telemetry.NewProvider` creates them, and only the builder or `cmd/` calls it (ADR 011, ADR 027).
2. Get tracers and meters from the globals: `otel.Tracer(scope)` and `otel.Meter(scope)`. Do not inject a `TracerProvider` through constructors.
3. `internal/extproc/*` MUST NOT import `internal/ports` or `internal/adapters/telemetry`. Only the OTel API is allowed there.
4. Domain packages MAY import only the OTel **API** packages (`go.opentelemetry.io/otel`, `.../trace`, `.../metric`, `.../attribute`, `.../codes`). They MUST NOT import the SDK, exporters, or `internal/adapters/telemetry`. This records what `domain/approval` and `domain/oauth2session` already do; the "zero infrastructure imports" rule in `internal/AGENTS.md` should name this exception.
5. Tests MUST use `Builder.WithTracerProvider` or an in-memory recorder and shut it down in `AfterEach`. Do not rely on global state across tests.
6. Telemetry is fail-open: a failure to create an instrument or export data MUST NOT fail a request or startup. Log the failure once at WARN (see §5.5).

---

## 4. Traces

### 4.1 Instrumentation scope (tracer and meter name)

Use the bounded-context or adapter package name. Reuse an existing scope before you add one:

`storage`, `encryption`, `jwks`, `upstream`, `approval`, `oauth2session`, `tokenexchange`, `extproc`, `telemetryhttp`

The same scope name MUST be used for the tracer and the meter of a package.

### 4.2 Span names

- Format: `<scope>.<operation>[.<entity>]`, lower snake case per segment, for example `storage.get.agent`, `approval.list_pending`, `extproc.opa.evaluate`.
- Span names MUST be static string literals. Never put IDs, URLs, principals, or tool names in a span name. Put them in attributes only if §6 allows it.
- HTTP server spans come from `otelchi`. Do not create a second server span in a handler.

### 4.3 When to create a span

Create a child span for:

- each call that leaves the process (database, KMS/DynamoDB, upstream OAuth2, JWKS, OPA, broker-from-ExtProc);
- a domain operation that callers need to see separately in a trace (for example, the approval gate or token exchange).

Do not create spans for pure in-memory functions or for each iteration in a loop.
A background job that outlives the request MUST start a **new root span** with a `trace.Link` to the triggering span (precedent: `oauth2session.background_refresh`).

New Postgres repositories SHOULD add `storage.*` spans. Today only four repositories have them: agents, user grants, tool approvals, and approval sync state.

### 4.4 Span attributes

- Prefer OTel semantic-convention keys when one exists: `http.request.method`, `http.response.status_code`, `error.type`.
- Otherwise, use dotted lower snake case namespaced by scope: `token_exchange.outcome`, `approval.status`, `oauth2.refresh.outcome`.
- Values MUST be bounded enums or opaque IDs allowed by §6. Define enum values next to the code that produces them, and document them in the feature's reference doc.
- Use one key per concept. Do not use `error.kind` in one file and `error_kind` in another (see §8).

### 4.5 Errors and status

```go
span.SetStatus(codes.Error, "token exchange failed") // static description
span.SetAttributes(attribute.String("error.type", string(meta.Kind())))
```

- Status descriptions MUST be static strings.
- `span.RecordError(err)` MUST NOT be used. Raw errors can contain wrapped causes, provider bodies, URLs, or claims. Record the bounded `ErrorMetadata` fields instead.
- Caller cancellation is not a span error. Record it as an outcome such as `canceled`.

---

## 5. Logs

### 5.1 Always pass the context

Use `InfoContext`, `WarnContext`, `ErrorContext`, and `DebugContext` whenever a `context.Context` is in scope.
Without the context, the context handler cannot add `trace_id`, `actor`, and `calling_peer`, so the record cannot be correlated or attributed.
Logs without context are acceptable only at startup, at shutdown, and in callbacks that have no context.

### 5.2 Event names

The message is a stable event name, not a sentence:

```go
s.logger.WarnContext(ctx, "session.oauth2.refresh_failed",
    slog.String("event", "session.oauth2.refresh_failed"),
    slog.String("service_id", serviceID.String()),
    slog.String("session_id", sessionID.String()),
    slog.String("triggered_by", string(trigger)),
    slog.Any("oauth2_session", meta), // ErrorMetadata implements slog.LogValuer
)
```

- Format: `<bounded_context>.<entity>.<past_tense_or_condition>`, lower snake case per segment, for example `session.oauth2.token_refreshed` or `service.thirdparty.provider_created`.
- Set the `event` attribute to the same value, so the event survives handlers that rewrite the message.
- Once an event is released, its name and attributes are a compatibility surface. Operators build alerts on them. Changing or removing one is a breaking change, and the feature's observability contract MUST record it.

### 5.3 Attribute keys

- Lower snake case: `service_id`, `agent_id`, `session_id`, `approval_id`, `principal`, `operator_principal`, `triggered_by`, `outcome`, `duration_ms`.
- Errors: use the key `error_kind` with a bounded value, plus `operation` and `dependency` when known. When a raw error is logged at all (§6.3), its key is `error`, never `err`.
- Prefer typed `slog.Attr` constructors (`slog.String`, `slog.Int`) over alternating `key, value` pairs. `go vet` checks `slog` key-value pairs, but typed attributes make bad types visible in review.
- Use `principal` for an end user and `operator_principal` for an admin caller. Do not mix them.

### 5.4 Levels

| Level | Use for | Examples |
|---|---|---|
| ERROR | The broker failed to do its job, and an operator must act. | Repository unavailable, KMS failure, sweep aborted, panic recovered. |
| WARN | Degraded or rejected, and the caller or system can recover. | Upstream refresh rejected, pool saturated, CIMD fetch blocked, a configuration that is insecure for production. |
| INFO | Business or security events that complete successfully. | Token issued, grant created or revoked, key rotated, session established. |
| DEBUG | Developer diagnostics that are off in production. | Cache hits, branch decisions. The data rules in §6 still apply. |

A client mistake (bad request, invalid token) is WARN at most. Never log it at ERROR: that would let clients raise operator alerts.

### 5.5 Instrument-creation failures

Instrument constructors return an error. Do not discard it with `_`. Log it once at WARN and continue with the instrument, which the SDK returns as a no-op on error:

```go
counter, err := meter.Int64Counter("…")
if err != nil {
    logger.Warn("telemetry.instrument.create_failed", slog.String("instrument", "…"), slog.String("error", err.Error()))
}
```

---

## 6. What you may record

These rules apply to **all three signals**. Logs are exported through OTLP when the log bridge is on, so "it's only a log" is not an exception.

### 6.1 Never (MUST NOT appear in logs, span attributes, span names, events, or metric attributes)

| Category | Examples |
|---|---|
| Credentials and bearer material | Access, refresh, and ID tokens; subject tokens; client assertions; authorization codes; PKCE verifiers; `state` values; client secrets; API keys; cookies; `Authorization` headers |
| Key material | Plaintext DEKs, branch keys, private JWKs, KMS ciphertext blobs |
| JWT contents | Unvalidated claims, JOSE headers, whole decoded tokens |
| Request and response bodies | Inbound bodies, upstream provider bodies and headers |
| Query strings | Any full URL that may carry a query (`code=`, `state=`, `token=`) |
| Encrypted secrets | `Secret` values in either state. Use `Secret.Redacted()` only when the reason requires a hint |
| Recovery URIs | `error_uri` values. These go only in protocol responses |
| Process arguments | Command line and arguments, which are already stripped from resources |
| Panic values and stack traces on credential paths | Use a bounded summary (the token endpoint precedent) |

### 6.2 Allowed

| Data | Logs | Span attributes | Metric attributes |
|---|---|---|---|
| Opaque internal IDs (`agent_id`, `service_id`, `session_id`, `grant_id`, `approval_id`, `kid`, `branch_key_id`) | ✅ | ✅ | ❌ except bounded sets (see 6.4) |
| `trace_id`, `actor`, `calling_peer` (added by the context handler) | ✅ | n/a | ❌ |
| `principal` (user ID from `X-Remote-User`) | ✅ for audit events | ⚠️ only if the deployment's principal format is not PII (see §8) | ❌ |
| Bounded enums: outcome, stage, detail, `error_kind`, `operation`, `dependency`, `triggered_by` | ✅ | ✅ | ✅ |
| HTTP method (allowlisted), route pattern, status code | ✅ | ✅ | ✅ |
| Allowlisted OAuth error codes (`invalid_grant`, …) | ✅ | ✅ | ✅ |
| Durations and counts | ✅ | ✅ | as values, not as attributes |
| Tool name, MCP method, OPA result code, target server name | ✅ | ✅ | ❌ (unbounded) |
| Client IP | ✅ access log only | ❌ | ❌ |

### 6.3 Conditional (needs a stated reason in review)

- **Raw `error` values.** Logging `err` exports the whole wrapped chain, which can include provider bodies, URLs, or SQL fragments. New code SHOULD log the bounded `ErrorMetadata` group. If a raw error is unavoidable (for example, startup configuration errors), the reviewer must confirm that no wrapped cause in the chain can carry §6.1 data.
- **Client-supplied URLs** (`redirect_uri`, CIMD `client_id` URL, `client_uri`). Log only scheme, host, and path. Drop the query and fragment, and pass the value through `sanitizeLogValue`. Prefer `source_host` (the precedent in `cimd/keyset.go`).
- **Display names and user-entered free text.** Do not log them; they are unbounded and can contain PII. Use the ID.
- **Emails or names.** Log only presence, for example `has_email` (the precedent in `user_info_handler.go`).

### 6.4 Metric attribute cardinality

- Each attribute MUST come from a fixed, documented value set.
- `principal`, `session_id`, `agent_id`, `approval_id`, tool names, and URLs MUST NOT be metric attributes.
- `service_id` is allowed only on gauges where the number of services is operator-bounded (precedent: `oauth2_sessions`). A feature that adds it MUST say so in its observability contract.

### 6.5 Safety net (proposed follow-up)

Spec 017 chose "trust the developer" (code review only). To make the rules above cheaper to keep:

1. Implement `slog.LogValuer` on `model.Secret` and on the token value objects (`SubjectToken`, `ClientAssertion`) so that logging one by mistake prints `[REDACTED]`.
2. Add a unit test that logs each sensitive type and asserts that its plaintext does not appear in the output.
3. Add a `ReplaceAttr` in `cmd/*/root.go` that drops keys from a deny list (`token`, `access_token`, `refresh_token`, `client_secret`, `code`, `state`, `password`, `assertion`, `authorization`, `cookie`).

---

## 7. Metrics

- Create instruments **once**, in the constructor, never per request.
- Set a `WithDescription` and a UCUM `WithUnit` (`s`, `ms`, `{refresh}`, `{request}`).
- Counter, UpDownCounter, histogram, or observable gauge: pick by semantics. Use an observable gauge for values read from storage (precedent: `oauth2_sessions`, one query per export cycle).
- Use `context.WithoutCancel(ctx)` when you record after the request might already be canceled.
- Every new instrument goes in the feature's observability contract (`specs/NNN-*/contracts/observability.md`) with its kind, meter, attributes, and when it increments.
- **Naming (open decision, see §8):** new instruments SHOULD use OTel-style dotted names without a unit or `_total` suffix (`approval.gate.decisions`), and let the exporter or collector translate them for Prometheus. Do not rename released instruments.

---

## 8. Findings from the codebase sample (October 2026)

Sampled: all `otel.Tracer`/`otel.Meter` call sites, span names, span attributes, instrument definitions, and `slog` calls in `internal/` and `cmd/` (excluding tests).

| # | Finding | Evidence | Proposed action |
|---|---|---|---|
| F1 | Most log calls drop the request context, so most records have no `trace_id` or `actor`. | 398 calls without `Context` versus 112 with it. Concentrated in `adapters/http/handlers`, `adapters/http/oauth2_sessions`, `adapters/encryption/aws`. | Migrate to `*Context` (§5.1). Add a lint rule (`sloglint` with `context: scope`). |
| F2 | Raw errors are logged widely, and the log bridge exports them through OTLP. | About 160 `"error", err` / `"err", err` sites, for example `admin/agents_handler.go` (17) and `oauth2_sessions/handler.go` (16). | Replace with `ErrorMetadata` on paths that touch upstream providers or credentials first (§6.3). |
| F3 | Client-supplied URLs are logged unmodified, including query strings. | `domain/oauth2/cimd/service.go` logs `url` (it even warns that a query is present and logs it). `domain/oauth2/service.go` logs `redirect_uri` 9 times. | Log host and path only (§6.3). |
| F4 | Four event-naming styles. | Dotted (`session.oauth2.token_refreshed`), PascalCase (`TokenIssued`, `CredentialRotated`), snake (`cimd_document_invalid`), and free text (`"failed to list sessions"`). | New code uses §5.2. Rename existing events only alongside a contract note, because some are alert surfaces. |
| F5 | The same concept uses different attribute keys. | Spans: `error.kind` (`extproc/server/server.go`) versus `error_kind` (`extproc/server/exchanger.go`). Logs: `err` versus `error`. Access log writes the same value as both `remote_addr` and `client_ip`. | Standardize on `error.type` (spans), `error_kind` (logs), `error`. Deprecate one of the IP keys. |
| F6 | Two metric naming schemes. | Broker: `proactive_refresh_triggered_total`, `approvals_pending_total`. ExtProc: `extproc.token_exchange.requests`, `extproc.approval.gate.decisions`. | Decide once (recommendation in §7) and record it in an ADR. |
| F7 | `principal` is a span attribute. | `approval.principal` in `domain/approval/service.go`. Spec 017 SR-001 forbids PII such as email on spans, and `X-Remote-User` is often an email. | Remove it from spans or hash it. The audit log already carries `actor`. |
| F8 | Instrument-creation errors are handled inconsistently. | Discarded with `_` in `domain/oauth2session`, `domain/approval`, and the admin sweep handler; logged in `extproc`. | Follow §5.5. |
| F9 | Storage spans cover only 4 repositories. | `otel.Tracer("storage")` appears only in the agent, user grant, tool approval, and approval sync repositories. | Add spans when a repository is next modified (§4.3). |
| F10 | The domain layer imports the OTel API, but `internal/AGENTS.md` says "zero infrastructure imports". | `domain/approval`, `domain/oauth2session`. | Record the API-only exception (§3.4) in `internal/AGENTS.md`. |
| F11 | Panic recovery logs the panic value on non-token routes. | `RecoveryMiddleware` → `fmt.Sprint(err)`. | Use a bounded summary on every route, as the token endpoint already does. |

---

## 9. Review checklist

- [ ] Log calls use `*Context` and a stable dotted event name with a matching `event` attribute.
- [ ] No §6.1 data in any signal. Raw `err` and URLs are justified or replaced.
- [ ] Span names are static. Attributes are bounded or allowed IDs. No `RecordError`. Status descriptions are static.
- [ ] New metrics are created once, with a description, a unit, and bounded attributes, and are listed in the feature's observability contract.
- [ ] Background work starts a new root span linked to its trigger.
- [ ] Tests assert telemetry through an injected recorder, not through global state.
