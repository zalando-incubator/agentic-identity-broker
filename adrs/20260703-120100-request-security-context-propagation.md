# ADR 20260703-120100: Request Security Context Propagation

**Status**: Proposed
**Date**: 2026-07-03
**Feature**: 20260629-120000-request-security-context

---

## Context

The broker currently authenticates requests and emits traces/logs, but it does not yet carry one immutable,
request-scoped security context through every downstream layer. That leaves three gaps for forensic and support
workflows:

1. Request metadata such as actor identity, client IP, user agent, and Trace ID are not available uniformly
   through handlers, services, repositories, and audit logs.
2. Delegated RFC 8693 token-exchange requests have two relevant identities — the authenticated subject and the
   distinct calling peer that wielded the request — but the transport perimeter cannot know both until the
   validated `subject_token` and `client_assertion` have been processed.
3. The HTTP API surface needs a standard, additive way to return the request Trace ID to callers without
   introducing bespoke `X-` headers or changing any request or response body schemas.

This feature is intentionally backend-only. It adds no frontend work, no new persisted entities, no schema
changes, and no files under `migrations/`.

Per Constitution API-006, the additive response-header change required explicit stakeholder confirmation before
implementation. That confirmation has already been obtained for the W3C `traceresponse` header.

---

## Decision

### 1. Shared HTTP seam: `http.NewHandler`

All HTTP request-security-context capture is centralized in `internal/adapters/http/server.go` `NewHandler`, the
shared seam used by both HTTP servers. The perimeter stack is composed in this order (outermost → innermost):

1. `RecoveryMiddleware`
2. `TraceContextNormalizationMiddleware`
3. `otelchi` span middleware
4. `OptionalPrincipalMiddleware`
5. `SecurityContextMiddleware`
6. `LoggingMiddleware`
7. `ContextRecoveryMiddleware`
8. Route setup and handlers

This makes capture uniform for both the admin and end-user servers and avoids per-router or per-endpoint drift.

`TraceContextNormalizationMiddleware` is a narrow pre-OTel shim that exists only to resolve duplicate `traceparent` headers deterministically through the registered propagator. When multiple values are present, it keeps the first value whose extraction yields a valid span context and drops the rest; when all duplicates are invalid, it removes the header so `otelchi` creates a fresh trace instead of reusing malformed input.

### 2. `SecurityContext.TraceID` is the single authority

The final `SecurityContext` carries the authoritative request Trace ID.

- When a valid OTel span exists, `SecurityContext.TraceID` is the span trace ID.
- When tracing is disabled or no valid span exists, the middleware mints a `crypto/rand` fallback trace ID.
- Logs and the HTTP response header both read this carried value rather than inventing a second identifier.

This keeps context, logs, spans, and caller-visible trace correlation aligned.

### 3. Finalization seams and the deferred token-exchange perimeter

A request's `SecurityContext` is finalized exactly once, at one seam determined by the route. Ordinary HTTP
requests finalize directly in `SecurityContextMiddleware` (no capture holder). `POST /oauth2/token` is the only
route that defers finalization via a capture holder, resolved at one of its two effective-perimeter seams below.
The non-RFC 8693
handler seam and the last-resort perimeter net share a single helper — `FinalizeRequestSecurityContext` in
`internal/adapters/http/middleware` — which resolves `Actor` from the request principal (the `anonymous`
sentinel when absent), sets no `CallingPeer`, and is idempotent; a deferred request only ever finalizes once.
Keeping that logic in one exported helper prevents the perimeter and the handler copies from drifting.

1. **Ordinary HTTP requests** finalize in `SecurityContextMiddleware` immediately after principal extraction.
2. **`POST /oauth2/token`** is deferred by the middleware: it cannot read `grant_type` to distinguish delegated
   exchange from ordinary grants without consuming the one-shot request body, so it installs a capture holder
   and leaves finalization to the effective perimeter:
   - **Non-RFC 8693 grants** (`client_credentials`, `authorization_code`) finalize in `OAuth2TokenHandler` at
     the `/oauth2/token` seam, before dispatch to the grant handler. This must happen here rather than in the
     outer safety net (below) because the grant handler emits context-aware `TokenIssued` audit logs *during*
     handling; those lines must already carry `Actor` and `TraceID`.
   - **Delegated RFC 8693 token exchange** finalizes at the post-validation token-exchange seam
     (`tokenexchange.Exchange`), the request's effective perimeter for identity fields, and — being the only
     seam with a distinct authenticated peer — supplies the calling peer directly through the domain primitive
     `security.FinalizeCaptureHolder`. The final `SecurityContext` is instantiated exactly once there from
     captured transport metadata, the validated subject-token principal as `Actor`, and `client_assertion.sub`
     as `CallingPeer` when authenticated and distinct. Finalization precedes the privileged-client
     authorization decision, so a denied exchange whose tokens validated still carries `Actor`/`CallingPeer` on
     the failure audit path.

`CallingPeer` is omitted when unavailable or equal to `Actor`; the two identities are never collapsed.

As a last resort, `LoggingMiddleware` finalizes any still-open holder *after* the handler returns, so the
perimeter access log and any early-error `/oauth2/token` response still carry a finalized context. This net
cannot retroactively stamp audit lines a handler already emitted, which is why the handler-seam finalization
above is load-bearing rather than redundant.

### 4. Additive standard response header

The only HTTP contract change is one additive global response header on both HTTP perimeters:

- **Header**: `traceresponse`
- **Format**: `00-<trace-id>-<child-id>-<flags>`
- **Authority**: the `<trace-id>` field equals `SecurityContext.TraceID`
- **Config**: emitted by default and suppressed only when `request_context.trace.response_enabled: false`

No endpoint bodies, status codes, or request payload shapes change.

### 5. Two-tier recovery behavior

Two recovery layers are intentional:

- The outer `RecoveryMiddleware` is a last-resort perimeter net for panics that occur before the final security
  context exists.
- The inner `ContextRecoveryMiddleware` runs after context finalization so route-handler panics still log with
  `trace_id`, `actor`, and `calling_peer` when available.

### 6. Deployment-level configuration

The feature is always enabled. The only deployment controls are:

```yaml
request_context:
  trusted_proxy:
    enabled: false
    forwarded_header: X-Forwarded-For
  trace:
    response_enabled: true
```

- `trusted_proxy.enabled: false` is the secure default; forwarding headers are ignored and client IP comes from
  the direct connection.
- When trusted proxy support is enabled, the broker reads the configured header and treats the **right-most**
  entry as the trusted caller IP.
- `trace.response_enabled` suppresses only the response header. Capture and logging remain active.

---

## Consequences

**Positive**:
- One request-scoped value object becomes the shared forensic source for actor, optional calling peer, client IP,
  and Trace ID.
- Both HTTP perimeters gain consistent trace correlation without per-endpoint work.
- Delegated token exchange preserves the distinction between the subject on whose behalf the action occurs and the
  authenticated peer that wielded it.
- The caller-visible trace contract uses a W3C-standard response header rather than a bespoke `X-Trace-Id`.
- No persistence, migration, or frontend scope is introduced.

**Negative / trade-offs**:
- The stack now has two recovery layers, which must be kept in the documented order.
- Because the `/oauth2/token` body cannot be read in the middleware, that route defers finalization to two
  route-internal seams (the non-exchange grant handler and the delegated-exchange service) in addition to the
  outer middleware path. The three sites share one `FinalizeRequestSecurityContext` helper (the delegated seam
  additionally supplies the calling peer) so they cannot drift, but future token-grant work on this route must
  still preserve a finalization point ahead of any in-handler audit logging.
- Operators behind trusted proxies must opt in explicitly; misconfiguration will fall back to direct connection IPs.

---

## Alternatives Considered

1. **Capture in each router or endpoint**: Rejected because it duplicates middleware registration and breaks the
   guarantee that every request gets the same treatment automatically.
2. **Return `traceparent` or `X-Trace-Id` on responses**: Rejected because `traceparent` is request-oriented and
   `X-Trace-Id` is a bespoke header. `traceresponse` is the standards-aligned response contract.
3. **Finalize delegated identities in the outer middleware**: Rejected because the middleware does not own the
   validated `subject_token` and `client_assertion` claims. Doing so there would duplicate parsing and drift from
   the authoritative validation seam.
4. **Collapse `CallingPeer` into `Actor`**: Rejected because delegated requests need to preserve who the action is
   on behalf of versus which peer wielded it.

---

## Scope Notes

- HTTP and ExtProc gRPC surfaces must expose the same semantics for `trace_id`, `actor`, and optional
  `calling_peer`, but ExtProc achieves parity by behavior and logging contract rather than by importing the core
  broker package.
- The feature remains in-process and request-scoped. Database actor stamping, provenance columns, and any other
  persistence changes are out of scope for ADR 20260703-120100.
