# Phase 0 Research: Request Security Context Propagation

**Feature**: 20260629-120000-request-security-context | **Date**: 2026-06-29 | **Spec**: [spec.md](./spec.md)

This document records the design decisions that resolve the open questions for implementing system-wide
security-context capture and propagation. Each decision is grounded in the existing codebase (file + symbol
references) so implementation follows established patterns rather than introducing parallel mechanisms.

---

## D1. Single perimeter seam for HTTP capture

**Decision**: Centralize the entire perimeter middleware stack in `http.NewHandler`
(`internal/adapters/http/server.go:117`), which is the single shared entry point used by **both** the admin
(:8000) and end-user (:14000) servers (and by tests). Introduce one `SecurityContextMiddleware` and register
it there, so capture is uniform and not per-endpoint (satisfies **FR-005**).

**Rationale**:
- `NewServer`/`NewHandler` already own the system-wide stack (`RecoveryMiddleware` → `LoggingMiddleware` →
  `OptionalPrincipalMiddleware`) and are the only place both servers converge. Adding capture here guarantees
  every request on every interface is covered with zero per-route opt-in.
- `OptionalPrincipalMiddleware` (`internal/adapters/http/middleware/principal_middleware.go:191`) already runs
  globally and resolves the principal into context via `principal.WithPrincipal`
  (`internal/domain/principal/context.go:31`). The actor for the security context is read from that same
  context value — reusing the existing identity mechanism (**FR-015**), not a new one.

**Alternatives considered**:
- *Register per-router inside `SetupAdminRoutes`/`SetupEnduserRoutes`*: rejected — duplicates registration in
  two places, risks drift, and the base `Recovery`/`Logging` middlewares in `NewHandler` would remain outer to
  it and miss the captured context.
- *Extend `OptionalPrincipalMiddleware` to also capture network metadata*: rejected — conflates authentication
  (identity resolution) with forensic context capture; violates single-responsibility and complicates the
  optional/require principal variants.

---

## D2. Trace ID authority = `SecurityContext.TraceID` (span trace ID, else `crypto/rand` fallback)

**Decision**: The Trace ID is carried on `SecurityContext.TraceID`, which is the **single authority** for the
value written to logs, spans, and the `traceresponse` response header. The security-context middleware runs
**after** the `otelchi` span middleware and sets `SecurityContext.TraceID` from the **OpenTelemetry span trace
ID** (`trace.SpanContextFromContext(ctx).TraceID()`) when a valid span exists — the normal case, which inherits
an inbound `traceparent`. When distributed tracing is disabled (no span in context), and only then, the
middleware generates a single W3C-compatible 16-byte hex trace ID (via `crypto/rand`) and stores it on
`SecurityContext.TraceID`. Because the log handler reads the carried `SecurityContext.TraceID` (research D3) —
not the span directly — the header, logs, and context agree in **both** modes, including the tracing-off
fallback.

**Rationale**:
- `otelchi` (`internal/adapters/http/routing/{admin,enduser}.go:46,73`) already derives a per-request trace ID
  — inheriting an inbound `traceparent` when present (propagators configured in
  `internal/adapters/telemetry/provider.go:333`, W3C `tracecontext` + b3 + ot) or minting one via the SDK
  otherwise. Reusing it means the response header `traceresponse`, the `SecurityContext`, spans, and
  `otelslog`-bridged logs all agree (satisfies **SC-002** end-to-end correlation and **SC-007** propagation).
- Minting an independent ID in the base stack would diverge from the span/log `trace_id` whenever there is no
  inbound `traceparent`, breaking correlation. This was explicitly flagged and is avoided.

**Consequence — middleware ordering** (outermost → innermost), owned by `NewHandler`:
1. `RecoveryMiddleware` — bare last-resort net catching panics in the outer perimeter middlewares
   (`TraceContextNormalizationMiddleware`/`otelchi`/`OptionalPrincipal`/`SecurityContext`) that run before the
   security context exists; such panics cannot carry `trace_id`/`actor` (inherent, documented in ADR 20260703-120100).
2. `TraceContextNormalizationMiddleware` — when duplicate inbound `traceparent` values are supplied, keeps the
   first value the configured propagator accepts and removes the rest before tracing, so `otelchi` reuses one
   deterministic inbound trace ID instead of falling back to a fresh trace.
3. `otelchi` span middleware — establishes the authoritative trace ID (moved here from the routing functions;
   gated on `Telemetry.Enabled` — the **exact** condition `SetupAdminRoutes`/`SetupEnduserRoutes` use today, so
   the relocation is behavior-neutral; the `crypto/rand` fallback covers the no-span case).
4. `OptionalPrincipalMiddleware` — resolves the principal into context.
5. `SecurityContextMiddleware` — reads the span trace ID (or mints the fallback) + principal (or `anonymous`) +
   client IP + user agent + method/target, builds the immutable `SecurityContext`, injects it into the request
   context, sets the `traceresponse` response header (value format in D10).
6. `LoggingMiddleware` — now inner to the span + context, so its access-log line carries `trace_id` + `actor`.
7. `ContextRecoveryMiddleware` — inner context-aware recovery so route-handler panics are logged with
   `trace_id`/`actor` (FR-006) and return 500.

`otelchi` registration is therefore **removed** from `SetupAdminRoutes`/`SetupEnduserRoutes` and relocated into
`NewHandler`, which must learn the server name ("admin"/"enduser") and telemetry config via an extended
`http.ServerConfig`.

**Alternatives considered**:
- *Read the inbound `traceparent` directly in the base stack and generate otherwise*: rejected — re-implements
  what `otelchi` already does and risks producing a different ID than the span, the exact divergence we must
  avoid.
- *Always start an explicit span in the security middleware*: rejected — duplicates `otelchi`'s span lifecycle
  and complicates sampling/parenting.

---

## D3. Logs carry `trace_id` + `actor` + optional `calling_peer` via a context-enriching slog handler

**Decision**: Add a context-enriching `slog.Handler` in `internal/adapters/telemetry` (e.g. `context_handler.go`) whose `Handle(ctx, record)` resolves `trace_id` from `security.FromContext(ctx).TraceID` **first** — the carried authority (D2), present for every perimeter request in both tracing-on and tracing-off modes — and falls back to `trace.SpanContextFromContext(ctx)` only when no `SecurityContext` is present (background / non-perimeter code). It also appends `actor` and, when non-empty, `calling_peer` from the same value object. Wrap the base console handler with it in `app.Builder.Build()` (`internal/app/builder.go:324-327`, alongside the existing `otelslog` + `NewMultiHandler` composition). Update the perimeter middlewares (`LoggingMiddleware`, `RecoveryMiddleware`/`ContextRecoveryMiddleware` in `internal/adapters/http/middleware.go`) and token-exchange logging sites (`internal/adapters/http/enduser/oauth2_token.go`) to emit through the `*Context` slog methods so the request context reaches the handler intact.

**Rationale**:
- The existing `multiHandler` (`internal/adapters/telemetry/slog_handler.go:17`) only fans out; nothing injects `trace_id` into the plain text/JSON console handler today, and `LoggingMiddleware` currently uses `logger.Info` without context (`internal/adapters/http/middleware.go:44`), so the access log lacks request-scoped identity fields. A context-enriching handler is the standard, low-churn way to stamp every context-aware log line without editing each call site (satisfies **FR-006**).
- The new `calling_peer` field matters specifically on delegated flows where the actor and the peer that wields the action diverge. Putting the field on the shared handler keeps those logs consistent anywhere the request context is already available.

**Scope note**: This feature guarantees the **perimeter access/audit logs** and all already-context-aware log sites carry `trace_id` + `actor`, plus `calling_peer` when present. The **security-relevant audit sites** named by FR-006 are converted to context-aware logging so their lines carry the same fields (FR-006 / SR-004): the OAuth2 **authorization** sink (`internal/adapters/http/middleware/oauth2_audit.go:105`, currently `logger.Log(context.Background(), …)`), the **token issuance** `TokenIssued` events (`internal/adapters/http/enduser/token_grant_strategy.go:205,241`), and the **consent** `grant created` event (`internal/adapters/http/handlers/consent/grants_handler.go:297`). A blanket migration of every remaining legacy `logger.Info(...)` call is out of scope.

**Alternatives considered**:
- *Rely solely on `otelslog`*: rejected — `otelslog` stamps trace context into exported **OTLP logs** only and only for context-aware calls; the human-readable console handler still needs enrichment, and OTLP log export is disabled by default (`LogsConfig.Enabled=false`).
## D4. Client IP resolution with trusted-proxy awareness

**Decision**: Resolve the client IP from `r.RemoteAddr` by default. Only when
`request_context.trusted_proxy.enabled` is `true` does the middleware read the configured forwarded header
(default `X-Forwarded-For`) to determine the client IP, taking the **right-most** entry — the address the
trusted proxy appended — not the left-most. When proxy trust is disabled, client-supplied forwarding headers
are ignored entirely.

**Right-most, not left-most**: with `client → proxy → server`, the proxy appends the real client to whatever
the client already sent, producing `X-Forwarded-For: <client-supplied…>, <real-client>`. The left-most entries
are attacker-controllable; only the right-most entry is the hop the trusted proxy added. The middleware
therefore takes the right-most entry of the configured header. For multi-proxy deployments this generalizes to
stripping a configured number of trusted hops (or trusted-proxy CIDRs) from the right and taking the first
address that is not a known proxy. Taking the left-most entry would re-introduce the exact spoof this guards
against (**SR-006/FR-010**).

**Rationale**:
- Secure-by-default per **Constitution Principle I** and **SR-006/FR-010**: an attacker must not be able to
  spoof the recorded client IP by sending a forged `X-Forwarded-For`. The existing code reads `r.RemoteAddr`
  directly today (`middleware.go:49`, `oauth2_audit.go:55`); this decision makes forwarded-header trust an
  explicit, opt-in deployment choice and pins the parse direction to the right.

**Alternatives considered**:
- *Always trust `X-Forwarded-For`*: rejected — IP spoofing vulnerability.
- *Auto-detect proxies*: rejected — unreliable; explicit configuration is auditable.

---

## D5. Domain placement and identity-source precedence

**Decision**: Introduce `internal/domain/security` containing the immutable `SecurityContext` value object plus context accessors `WithSecurityContext(ctx, sc)` / `FromContext(ctx) (SecurityContext, bool)`, mirroring the existing `internal/domain/principal` package pattern (unexported context-key type, `With…`/`FromContext` API). `SecurityContext` carries `actor` (always populated, default `anonymous`) and `calling_peer` (optional, recorded only when distinct). The final `SecurityContext` is constructed exactly once per request:
1. on standard HTTP requests, in the generic perimeter middleware, using transport metadata + `principal.FromContext(ctx)` when present;
2. on token-exchange requests, at the post-validation token-exchange seam — the request's effective perimeter for identity fields — using the transport metadata/trace captured earlier plus the validated subject-token principal and `client_assertion.sub` when distinct;
3. otherwise with `anonymous` and no `calling_peer`.

For token exchange, the broker already validates `subject_token` and `client_assertion` in `internal/domain/tokenexchange/service.go:174-208`. Reuse that flow: the extracted subject-token principal remains the `actor`, and `client_assertion.sub` becomes `calling_peer` when distinct, before any downstream service/repository work or request-scoped logging that should observe the final `SecurityContext`. This is **not** a mutation of an earlier `SecurityContext`; the earlier middleware step only captures the transport metadata and trace inputs in a request-scoped holder so the final value object can be built once and then observed consistently by downstream code and outer loggers.

**Rationale**:
- The domain layer imports no infrastructure (Constitution Principle VI); a value-object package with context helpers matches `principal`'s proven shape (`internal/domain/principal/context.go`). `SecurityContext` is a free name (no collision; `CELRequestContext` in `ports`/`tokenexchange` is an unrelated token-exchange DTO).
- It is a **value object**, not an entity with a UUID primary key, so **ADR 20260313-120000 (typed entity IDs)** does not apply — no `XxxID` is added to `internal/domain/id/`. The Trace ID is a correlation string, not a domain entity.
- Reusing the validated token-exchange identities avoids reparsing raw JWTs or HTTP form bodies in the global middleware just to recover a delegated actor/calling-peer pair. That keeps identity derivation aligned with the code path that already owns signature verification, CEL extraction, and authorization policy while preserving FR-013 immutability.
- **`actor` remains distinct from `principal`**: `principal` is the access-control identity resolved on ordinary HTTP requests, whereas `actor` is the always-populated forensic label carried by the security context. `calling_peer` adds the second half of delegated flows without overloading either term.

**Alternatives considered**:
- *Extend the `principal` package*: rejected — `principal` models one access-control identity only; bundling network metadata plus delegated peer identity there overloads a focused package.
- *Parse `subject_token` / `client_assertion` in generic middleware*: rejected — duplicates existing token-exchange validation, consumes request bodies too early, and invites drift from the authoritative JWT/CEL path.
- *Collapse `calling_peer` into `actor`*: rejected — loses the distinction between “who the action is on behalf of” and “which peer wielded it,” the exact ambiguity the clarification resolved.
## D6. Configuration shape and wiring

**Decision**: Add a top-level `RequestContext RequestContextConfig` field to `ports.Config`
(`internal/ports/config.go:37`), since trusted-proxy posture and the trace response-header emission are
deployment-wide (apply to both servers). Plumb it (with the server name + telemetry config) into an extended
`http.ServerConfig` from `app.Builder.Build()`. Register defaults and validation in `internal/config/`
(`loader.go` defaults, `validator.go`). Capture is always enabled; only trusted-proxy behavior is configurable
(**FR-011/SR-001**).

```yaml
request_context:
  trusted_proxy:
    enabled: false            # secure default — do not trust forwarding headers
    forwarded_header: "X-Forwarded-For"
  trace:
    response_enabled: true
```

**Helm + examples obligation** (Constitution Principle VII): add the parameters to
`charts/agentic-identity-broker/values.yaml` and its ConfigMap template + README, and add
`examples/config/request-context.yaml` referenced from `examples/config/README.md`.

**Alternatives considered**:
- *Per-`ServerInstanceConfig` block*: rejected — trusted-proxy posture is environment-wide; duplicating per
  server invites inconsistency.
- *Under `SecurityConfig`*: viable, but `SecurityConfig` currently holds only dev-only "skip" toggles; a
  dedicated top-level block reads clearer and matches the spec's example.

---

## D7. gRPC (ExtProc) perimeter parity

**Decision**: In `internal/extproc/server`, derive the Trace ID for each request from the already-extracted OTel span context (the service already adapts ExtProc headers to a propagation carrier — `headerCarrier`, `server.go:878` — and supports `traceparent`/b3/baggage via `s.extractTraceContext`). Create a per-request child logger that always includes `trace_id` and `actor`, and appends `calling_peer` only when a distinct authenticated peer is already available in trusted request metadata or auth context. Reuse an inbound trace identifier when present and tracing is enabled (`s.extractTraceContext` short-circuits when telemetry is off, `server.go:129-131`) and generate one otherwise — consistent with the HTTP perimeter. Apply this at **every** inbound-request entry point, not just the direct token-exchange path: `Server.processRequestHeaders` (`server.go:296`, OPA disabled) **and** the OPA-mode entry points `Server.processRequestHeadersOPA` (`server.go:402`) and `Server.processHeadersOnlyOPA`.

**Rationale**:
- ExtProc is a standalone binary that must **not** import `internal/domain|ports|adapters` (ADR 20260223-120000), but it already shares a telemetry dependency (**ADR 20260421-120000**) and already propagates trace context. Parity is achieved by behavior, not shared code (matches the spec assumption). The `*slog.Logger` lives on the `Server` struct (`server.go:42`).
- The current direct token-exchange flow only sees an opaque bearer token plus request metadata (`internal/extproc/server/server.go:339-380`, `405-458`). It does **not** currently expose a validated subject-token principal or client-assertion subject at the ExtProc perimeter, so actor must honestly default to `anonymous` and `calling_peer` must remain omitted unless a future authz path supplies those identities explicitly.
- Optional `calling_peer` semantics let the gRPC surface stay correct today while remaining forward-compatible for tool-authz style requests that may eventually carry a distinct peer identity.

**Alternatives considered**:
- *Share the core `security` package with ExtProc*: rejected — violates ADR 20260223-120000 standalone-binary rule.
- *Parse opaque bearer or subject tokens in ExtProc just for logging*: rejected — adds token-handling risk, diverges from the broker's authoritative validation path, and could fabricate identities the service has not actually authenticated.
## D8. Graceful degradation and access-control invariance

**Decision**: The `SecurityContextMiddleware` never rejects a request. Missing identity → actor `"anonymous"`;
missing/malformed network metadata → safe defaults (empty/"unknown"); a trace ID is always present
(**FR-008**). Authentication/authorization is untouched: `RequirePrincipalMiddleware` still rejects
unauthenticated requests on protected routes (**FR-009/SR-003**). Downstream consumers of
`security.FromContext` must tolerate absence (background/non-perimeter code) by treating it as
`anonymous`/system.

**Rationale**: The capture mechanism is a fail-open *observability* control layered beneath the unchanged
fail-closed *access* controls — the only safe way to apply it universally without creating a new failure mode.

---

## D9. Architecture documentation & ADR

**Decision**: Record the cross-cutting decision in a new **ADR `adrs/20260703-120100-request-security-context-propagation.md`** (Status: Proposed within this PR) and add the new domain terms (`SecurityContext`, `Actor`, `CallingPeer`, `anonymous`, `Trace ID`) to the **ARCHITECTURE.md** glossary. Document the additive global `traceresponse` response header and the new middleware ordering in `ARCHITECTURE.md`.

**Rationale**: Constitution Principles II (ADRs binding) and V (glossary). The ADR is a proposal in this PR and does not self-justify; it documents the seam, ordering, trace-id authority rule (`SecurityContext.TraceID`), and delegated identity semantics for future contributors.

---
## D10. Response header = W3C `traceresponse` (not `traceparent`, not `X-Trace-Id`)

**Decision**: Return the request's trace context to the caller via the **W3C Trace Context Level 2
`traceresponse`** response header, value `00-<trace-id>-<child-id>-<flags>`. `<trace-id>` is the carried
`SecurityContext.TraceID`; `<child-id>` is the perimeter span's `SpanID`
(`trace.SpanContextFromContext(ctx).SpanID()`), or a `crypto/rand` 16-char hex (8-byte) value when no span exists; `<flags>`
is the span's `TraceFlags` (`01` sampled, else `00`), or `00` in the tracing-off fallback. Emitted on every
response unless `request_context.trace.response_enabled` is `false`. OTel Go does not emit `traceresponse`
automatically, so the middleware formats the fields itself (the same seam that already reads the span context).

**Rationale**:
- `traceparent` is a **request** header (Trace Context Level 1, W3C REC): it means "the parent to continue the
  trace **downstream**", not "the id of the request you just handled". Putting it on a response is a semantic
  misuse. `traceresponse` (Level 2) is the standard's purpose-built **response** header for handing trace
  context back to a caller — exactly this feature's goal (**FR-007 / SC-002**).
- Adopting the standard **removes** the RFC 6648 / Zalando "Do Not Use X- Headers" concern the bespoke
  `X-Trace-Id` carried: the header is now standards-compliant, not a documented deviation.
- Config becomes a boolean `response_enabled` (default `true`): a standardized header name must not be
  renameable, but suppressing the header for privacy-conscious deployments stays available (D6).

**Consequence**: the value object is unchanged — `SpanID`/`TraceFlags` are read at the same span-context point
the middleware already uses for the trace ID and are not needed downstream, so they are not stored on
`SecurityContext`. The carried `<trace-id>` field still equals the `trace_id` in logs, preserving SC-002.

**Alternatives considered**:
- *Response `traceparent`*: rejected — request-only semantics; misleads standard tracing tooling.
- *Keep bespoke `X-Trace-Id`, or emit it alongside `traceresponse`*: rejected — two headers for one fact; the
  `traceresponse` `<trace-id>` field is the same 32-hex value, just offset within the envelope.
- *Wait for Level 2 to reach REC*: rejected — the wire format is stable; CR-draft is acceptable here.

---

## Resolved unknowns summary

| Question | Resolution |
|---|---|
| Where to capture (HTTP) | `http.NewHandler` shared seam; one middleware (D1) |
| Trace ID authority | `SecurityContext.TraceID` = span trace ID, else `crypto/rand` fallback; logs read the carried value (D2/D3) |
| Middleware ordering | Recovery → TraceContextNormalization → otelchi → OptionalPrincipal → SecurityContext → Logging → ContextRecovery (D2) |
| Logs carry trace_id/actor | Context-enriching slog handler + `*Context` perimeter logging (D3) |
| Client IP / proxy trust | `RemoteAddr` default; opt-in forwarded header (D4) |
| Domain placement | `internal/domain/security` value object; no typed ID (D5) |
| Config shape | Top-level `request_context`; Helm + examples updated (D6) |
| gRPC parity | ExtProc per-request child logger at every entry point incl. the OPA path (D7) |
| Degradation / auth invariance | Fail-open capture beneath unchanged fail-closed auth (D8) |
| Docs | ADR 20260703-120100 + ARCHITECTURE.md glossary/header note (D9) |
| Response header | W3C `traceresponse` (`00-<trace-id>-<child-id>-<flags>`); opt-out via `response_enabled` (D10) |
