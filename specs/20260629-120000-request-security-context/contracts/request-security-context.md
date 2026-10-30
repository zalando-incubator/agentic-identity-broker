# Contract: Request Security Context Propagation

**Feature**: 20260629-120000-request-security-context | **Date**: 2026-06-29 | **Spec**: [../spec.md](../spec.md)

This feature adds **no new HTTP endpoints**. Its external surface is: (1) an additive global response header,
(2) a configuration contract, (3) an internal middleware-ordering contract, and (4) the gRPC perimeter log
field contract. No `api/{enduser,admin}/openapi.yaml` endpoint definitions change; the global response header
will be documented in `ARCHITECTURE.md`. Per Constitution Principles IV/X (API-006), this additive header
**requires explicit stakeholder confirmation before implementation** — this has been obtained for the W3C `traceresponse` header. It is opt-out via `request_context.trace.response_enabled`.

---

## C1. Response header contract (HTTP — both perimeters)

| Property | Value |
|---|---|
| Header name | `traceresponse` (W3C Trace Context Level 2 response header); emitted unless disabled via `request_context.trace.response_enabled: false` |
| Applies to | **Every** response on the admin (:8000) and end-user (:14000) servers, including errors, `/health`, and unauthenticated responses |
| Value | W3C form `00-<trace-id>-<child-id>-<flags>`: `<trace-id>` = `SecurityContext.TraceID` (32-char lower-case hex, the carried authority); `<child-id>` = the perimeter span's 16-char hex SpanID, or a `crypto/rand` 16-char hex when no span exists; `<flags>` = the span's sampled flag (`01`/`00`), or `00` in the tracing-off fallback |
| Idempotency | Exactly one header per response; its `<trace-id>` field equals the `trace_id` in that request's log lines (and the span, when tracing is enabled) because all read the carried `SecurityContext.TraceID` |
| Absence | Never absent — present even for `anonymous`/degraded requests (**FR-007 / SC-001**) |

**Consumer guarantee**: An operator or external caller can take the `<trace-id>` field of the `traceresponse`
value from any response and retrieve every log entry and span for that request (**SC-002**).

**Header standard**: `traceresponse` is the W3C Trace Context **Level 2** response header for returning trace
context to a caller (a Candidate Recommendation Draft; the wire format is stable). Using it — rather than the
request-only `traceparent` (which means "the parent to continue downstream", not "the id of the request you
just made") or a bespoke `X-`-prefixed name — makes the response standards-compliant and removes the RFC 6648 /
Zalando "Do Not Use X- Headers" concern entirely. Rationale in ADR 20260703-120100 and research D10.

---

## C2. Configuration contract

Added under a new top-level `request_context` block in the application config (`ports.Config.RequestContext`).

```yaml
request_context:
  trusted_proxy:
    enabled: false              # bool. Default false. When false, forwarding headers are NOT trusted.
    forwarded_header: "X-Forwarded-For"   # string. Header read for client IP when enabled.
  trace:
    response_enabled: true                # bool. Default true. Set false to suppress the traceresponse header.
```

| Key | Type | Default | Validation |
|---|---|---|---|
| `request_context.trusted_proxy.enabled` | bool | `false` | — |
| `request_context.trusted_proxy.forwarded_header` | string | `"X-Forwarded-For"` | non-empty when `enabled: true` |
| `request_context.trace.response_enabled` | bool | `true` | — |

**Behavioral contract**:
- Capture itself is **always on** (no enable flag) — secure by default (**FR-011 / SR-001**).
- Capture and log enrichment are unaffected by `trace.response_enabled`; the flag only governs whether the
  `traceresponse` header is written on the response (default on) — the always-present trace correlation in logs
  is independent of it.
- When `trusted_proxy.enabled: false`, the client IP is `r.RemoteAddr` and client-supplied forwarding headers
  are ignored (no spoofing — **FR-010 / SR-006**).
- When `trusted_proxy.enabled: true`, the client IP is the **right-most** entry of `forwarded_header` (the hop
  the trusted proxy appended), generalizing to stripping a configured trusted-hop count/CIDR from the right.
  The left-most entries are attacker-controllable and MUST NOT be taken (**FR-010 / SR-006**).

**Deployment obligation** (Constitution Principle VII): mirrored into
`charts/agentic-identity-broker/values.yaml` + ConfigMap template + chart README, and
`examples/config/request-context.yaml` (referenced from `examples/config/README.md`).

---

## C3. Middleware ordering contract (internal, both HTTP servers)

`http.NewHandler` MUST compose the perimeter stack in this exact order (outermost → innermost):

```text
1. RecoveryMiddleware              (bare outer net; perimeter-middleware panics cannot carry trace_id/actor)
2. TraceContextNormalizationMiddleware  (when duplicate `traceparent` values are supplied, keeps the first value the configured propagator accepts and removes the rest before tracing)
3. otelchi span middleware         (gated on Telemetry.Enabled — the exact condition the routing functions use today; behavior-neutral relocation)
4. OptionalPrincipalMiddleware
5. SecurityContextMiddleware       (captures trace + transport metadata for every request, writes the traceresponse header unless disabled, and finalizes SecurityContext immediately only when actor identity is already available)
6. LoggingMiddleware               (emits via *Context; reads the final SecurityContext from the request-scoped capture holder so delegated identities are visible on access logs)
7. ContextRecoveryMiddleware       (inner context-aware recovery; route-handler panics carry trace_id + actor + calling_peer when available; returns 500)
8. <route setup: CORS, RequirePrincipal on protected subrouters, handlers>
```

**Invariants**:
- `SecurityContextMiddleware` MUST run after `otelchi` (so the span trace ID is available) and after `OptionalPrincipalMiddleware` (so ordinary pre-auth routes can finalize immediately). When no valid span exists it mints the `crypto/rand` fallback so `SecurityContext.TraceID` is always populated.
- `POST /oauth2/token` defers finalization: `SecurityContextMiddleware` installs a capture holder but does not finalize it (it cannot read `grant_type` without consuming the one-shot body). The route finalizes at exactly one of two effective-perimeter seams, and a last-resort net covers the rest: (a) **non-RFC 8693 grants** finalize in `oauth2_token.go` at the `/oauth2/token` seam *before* dispatching to the grant handler, so the handler's context-aware `TokenIssued` audit logs (`token_grant_strategy.go:205,241`, see C4) carry `trace_id`/`actor`; (b) **delegated token exchange** finalizes in `domain/tokenexchange/service.go` from the captured trace/transport metadata plus the validated subject-token principal as `Actor` and `client_assertion.sub` as `CallingPeer` when distinct, before the privileged-client authorization decision so denied-but-validated exchanges still carry identity; (c) `LoggingMiddleware` finalizes any holder still open after the handler returns (perimeter access log + early-error paths only — it cannot stamp already-emitted in-handler logs). Seams (a) and the middleware share one `FinalizeRequestSecurityContext` helper. Finalization happens exactly once; no code may overwrite the final `SecurityContext` after that point.
- Downstream code and the log handler MUST observe only the finalized `SecurityContext`, never a partially populated value object.
- Two-tier recovery is required: the outer `RecoveryMiddleware` runs before the final security context exists, so panics in `TraceContextNormalizationMiddleware` / `otelchi` / `OptionalPrincipal` / `SecurityContext` cannot carry `trace_id`/`actor`; the inner `ContextRecoveryMiddleware` (after `LoggingMiddleware`) ensures route-handler panics do (**FR-006**, ADR 20260703-120100).
- `otelchi` registration MUST be removed from `SetupAdminRoutes`/`SetupEnduserRoutes` (relocated to `NewHandler`); the routing functions MUST NOT re-register it.
- `SecurityContextMiddleware` MUST NOT reject any request (fail-open capture — **FR-008**).
- Access-control middlewares (`RequirePrincipalMiddleware`) retain their existing reject behavior unchanged (**FR-009 / SR-003**).
## C4. Log field contract (both perimeters)

Every structured log entry emitted while handling a request MUST include:

| Field | Source | Notes |
|---|---|---|
| `trace_id` | `SecurityContext.TraceID` (carried authority) | present on the perimeter access log and all context-aware log sites, in **both** tracing-on and tracing-off modes (**FR-006**) |
| `actor` | `SecurityContext.Actor` | `anonymous` when unauthenticated; present on security-relevant audit logs (**SR-004**) |
| `calling_peer` | `SecurityContext.CallingPeer` | present only when the request carries a separately authenticated peer distinct from `actor`; omitted otherwise (**FR-006 / FR-015**) |

Log entries MUST NOT contain credential material derived from the request (**FR-012 / SR-005**).

**Caller-IP authority**: `SecurityContext.ClientIP` is the authoritative caller IP for logs. The existing access-log/audit `remote_addr` field (`middleware.go:49`, `oauth2_audit.go:55`, currently `r.RemoteAddr`) MUST be sourced from `SecurityContext.ClientIP` so one request never shows two conflicting caller IPs under a trusted proxy (**FR-010**).

**Audit sinks**: the security-relevant audit sites (FR-006) MUST use context-aware logging so their lines carry `trace_id` / `actor` / `calling_peer` when present (**FR-006 / SR-004**): the OAuth2 **authorization** sink (`oauth2_audit.go:105`, currently `context.Background()`), the **token issuance** `TokenIssued` events (`token_grant_strategy.go:205,241`), and the **consent** `grant created` event (`grants_handler.go:297`).
## C5. gRPC (ExtProc) perimeter contract

| Property | Value |
|---|---|
| Capture points | **Every** inbound-request entry point: `Server.processRequestHeaders` (`server.go:296`, OPA disabled) AND `Server.processRequestHeadersOPA` (`server.go:402`) + `Server.processHeadersOnlyOPA` (OPA enabled, ADR 20260315-120000) |
| Trace ID | when tracing is enabled, reused from inbound `traceparent`/b3 (propagated via `headerCarrier`, `server.go:878`, using `s.extractTraceContext`); otherwise (no inbound identifier, or tracing disabled) generated |
| Per-request logger | `s.logger.With("trace_id", <id>, "actor", <actor>)` used for that request's log lines, adding `calling_peer` only when available |
| Actor | authenticated identity when the gRPC request already carries one in trusted metadata/auth context; otherwise `anonymous` |
| Calling peer | authenticated calling peer when already available and distinct from `actor`; omitted otherwise |
| Current parity note | the current direct token-exchange path sees only an opaque bearer token plus request metadata, so today ExtProc normally logs `actor=anonymous` with no `calling_peer`; future tool-authz style requests may populate both fields without changing the contract |
| Constraint | No import of `internal/domain|ports|adapters` (ADR 20260223-120000); parity achieved by behavior, not shared code |
## Contract test mapping (red-phase E2E)

| Contract | Verified by (spec scenario → E2E) |
|---|---|
| C1 response header present + matches log trace_id | US1 S1, US2 S3/S4 |
| C1 header present on unauthenticated/degraded responses | US3 S1, US3 S4 |
| C2 proxy-trust off ignores forged `X-Forwarded-For` | Edge case (spoofed forwarding header) |
| C2 proxy-trust on derives IP from forwarded header | US1 S1 (with trusted_proxy enabled fixture) |
| C3 final context construction: actor + calling_peer + trace_id available downstream | US1 S2, US1 S3, US1 S5 |
| C3 fail-open: missing metadata does not reject | US3 S2 |
| C4 every log line carries trace_id; delegated flows add calling_peer; concurrent isolation holds | US1 S5, US2 S3, US2 S5 |
| C5 gRPC logs carry propagated/generated trace_id | US4 S1, US4 S2 |
| C5 gRPC logs carry actor and optional calling_peer consistently with HTTP semantics | US4 S3 |
| C4 remote_addr sourced from SecurityContext.ClientIP under trusted proxy | US1 S1 (trusted_proxy fixture) |
| C5 gRPC OPA-mode path logs propagated/generated trace_id | US4 S1, US4 S2 (authorizer-enabled fixture) |
